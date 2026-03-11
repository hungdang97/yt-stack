from __future__ import annotations
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from typing import Optional
import instaloader
import re
import json
import urllib.parse

app = FastAPI(
    title="Instagram Extractor",
    version="2.0.0",
    description="Extract media links from Instagram posts, reels, IGTV and profiles. Powered by instaloader.",
)

# Enable CORS for client UI integration
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # TODO: Restrict to specific domains in production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

_loader = instaloader.Instaloader(
    download_pictures=False,
    download_videos=False,
    download_video_thumbnails=False,
    download_comments=False,
    download_geotags=False,
    save_metadata=False,
    compress_json=False,
)


def _ensure_login(cookie: Optional[str] = None):
    """
    Login with Instagram cookies.

    Args:
        cookie: Can be either:
            - Simple sessionid string (backward compatible)
            - JSON object with multiple cookies: {"sessionid": "...", "csrftoken": "...", "ds_user_id": "...", "mid": "...", "ig_did": "..."}
    """
    if not cookie or _loader.context.is_logged_in:
        return

    # Try to parse as JSON first (full cookies object)
    try:
        cookies_dict = json.loads(urllib.parse.unquote(cookie))
        _loader.context.update_cookies(cookies_dict)
    except (json.JSONDecodeError, ValueError):
        # Fallback: treat as simple sessionid string
        _loader.context.update_cookies({"sessionid": cookie})

    try:
        _loader.context.test_login()
    except Exception:
        pass  # Ignore login test failures, continue anyway


def parse_shortcode(url: str) -> str:
    patterns = [
        r"instagram\.com/p/([A-Za-z0-9_-]+)",
        r"instagram\.com/reel/([A-Za-z0-9_-]+)",
        r"instagram\.com/tv/([A-Za-z0-9_-]+)",
    ]
    for p in patterns:
        m = re.search(p, url)
        if m:
            return m.group(1)
    return url.strip("/")


@app.get("/health")
def health():
    return {"status": "ok", "service": "insta-extractor", "version": "1.0.0"}


@app.get("/extract", summary="Extract media from Instagram post")
def extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None):
    shortcode = parse_shortcode(url)

    if proxy:
        _loader.context._session.proxies = {"https": proxy, "http": proxy}

    _ensure_login(cookie)

    try:
        post = instaloader.Post.from_shortcode(_loader.context, shortcode)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Failed to fetch post: {e}")

    media = []
    if post.typename == "GraphSidecar":
        for node in post.get_sidecar_nodes():
            media.append({
                "is_video": node.is_video,
                "video_url": node.video_url,
                "display_url": node.display_url,
            })
    elif post.is_video:
        media.append({
            "is_video": True,
            "video_url": post.video_url,
            "display_url": post.url,
        })
    else:
        media.append({
            "is_video": False,
            "video_url": None,
            "display_url": post.url,
        })

    location = None
    if post.location:
        loc = post.location
        location = {
            "id": getattr(loc, "id", None),
            "name": getattr(loc, "name", None),
            "slug": getattr(loc, "slug", None),
            "lat": getattr(loc, "lat", None),
            "lng": getattr(loc, "lng", None),
        }

    return {
        "shortcode": shortcode,
        "media_id": post.mediaid,
        "typename": post.typename,
        "caption": post.caption,
        "caption_hashtags": post.caption_hashtags,
        "caption_mentions": post.caption_mentions,
        "tagged_users": post.tagged_users,
        "likes": post.likes,
        "comments": post.comments,
        "date_utc": post.date_utc.isoformat(),
        "date_local": post.date_local.isoformat(),
        "is_video": post.is_video,
        "is_pinned": post.is_pinned,
        "is_sponsored": post.is_sponsored,
        "video_duration": post.video_duration if post.is_video else None,
        "video_view_count": post.video_view_count if post.is_video else None,
        "video_play_count": post.video_play_count if post.is_video else None,
        "title": post.title,
        "accessibility_caption": post.accessibility_caption,
        "owner_username": post.owner_username,
        "owner_id": post.owner_id,
        "location": location,
        "media_count": post.mediacount,
        "media": media,
    }


@app.get("/profile/{username}", summary="Get profile info with pagination")
def profile(
    username: str,
    proxy: Optional[str] = None,
    cookie: Optional[str] = None,
    offset: int = 0,
    limit: int = 12,
):
    """
    Get Instagram profile information with paginated posts.

    Args:
        username: Instagram username
        proxy: Optional proxy server (http://host:port)
        cookie: Optional session cookie for authentication
        offset: Number of posts to skip (default: 0)
        limit: Maximum number of posts to return (default: 12, max: 100)

    Returns:
        Profile info + paginated posts with has_more flag
    """
    if proxy:
        _loader.context._session.proxies = {"https": proxy, "http": proxy}

    _ensure_login(cookie)

    # Limit validation (prevent abuse)
    if limit > 100:
        limit = 100
    if limit < 1:
        limit = 1
    if offset < 0:
        offset = 0

    try:
        prof = instaloader.Profile.from_username(_loader.context, username)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Failed to fetch profile: {e}")

    posts = []
    current_index = 0
    has_more = False
    error_fetching_posts = False

    try:
        for post in prof.get_posts():
            # Skip posts before offset
            if current_index < offset:
                current_index += 1
                continue

            # Stop if we've collected enough posts
            if len(posts) >= limit:
                has_more = True  # There are more posts available
                break

            try:
                posts.append({
                    "shortcode": post.shortcode,
                    "url": f"https://www.instagram.com/p/{post.shortcode}/",
                    "is_video": post.is_video,
                    "typename": post.typename,
                    "likes": post.likes,
                    "comments": post.comments,
                    "date_utc": post.date_utc.isoformat(),
                    "caption": (post.caption or "")[:100],
                    "thumbnail": post.url,  # Thumbnail/display image URL
                    "video_url": post.video_url if post.is_video else None,
                })
                current_index += 1
            except Exception as e:
                # Skip individual post errors, continue with others
                current_index += 1
                continue
    except Exception as e:
        # Error iterating posts (e.g., rate limit, network issue)
        error_fetching_posts = True

    return {
        "username": prof.username,
        "full_name": prof.full_name,
        "bio": prof.biography,
        "followers": prof.followers,
        "following": prof.followees,
        "posts_count": prof.mediacount,
        "is_private": prof.is_private,
        "is_verified": prof.is_verified,
        "is_business_account": prof.is_business_account,
        "profile_pic": prof.profile_pic_url,
        "posts": posts,
        "pagination": {
            "offset": offset,
            "limit": limit,
            "returned": len(posts),
            "has_more": has_more,
            "next_offset": offset + len(posts) if has_more else None,
            "error": error_fetching_posts,
        },
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
