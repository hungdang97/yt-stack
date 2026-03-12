from __future__ import annotations
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from typing import Optional
import instaloader
import yt_dlp
import re
import json
import urllib.parse
import logging
import tempfile
import os

logger = logging.getLogger("insta-extractor")

app = FastAPI(
    title="Instagram Extractor",
    version="2.0.0",
    description="Extract media links from Instagram posts, reels, IGTV and profiles. yt-dlp primary, instaloader fallback.",
)

# Enable CORS for client UI integration
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
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


# ============================================
# SHARED HELPERS
# ============================================

def _ensure_login(cookie: Optional[str] = None):
    if not cookie or _loader.context.is_logged_in:
        return
    try:
        cookies_dict = json.loads(urllib.parse.unquote(cookie))
        _loader.context.update_cookies(cookies_dict)
    except (json.JSONDecodeError, ValueError):
        _loader.context.update_cookies({"sessionid": cookie})
    try:
        _loader.context.test_login()
    except Exception:
        pass


def parse_shortcode(url: str) -> str:
    patterns = [
        r"instagram\.com/p/([A-Za-z0-9_-]+)",
        r"instagram\.com/reels?/([A-Za-z0-9_-]+)",
        r"instagram\.com/tv/([A-Za-z0-9_-]+)",
    ]
    for p in patterns:
        m = re.search(p, url)
        if m:
            return m.group(1)
    return url.strip("/")


def _safe_int(value) -> int:
    """Convert value to int, return 0 if not possible."""
    if value is None:
        return 0
    try:
        return int(value)
    except (ValueError, TypeError):
        return 0


def _build_instagram_url(shortcode: str) -> str:
    return f"https://www.instagram.com/p/{shortcode}/"


# ============================================
# YT-DLP EXTRACTION (PRIMARY)
# ============================================

def _build_ydl_opts(proxy: Optional[str] = None, cookie: Optional[str] = None) -> tuple[dict, Optional[str]]:
    """Build yt-dlp options. Returns (opts, temp_cookie_file_path)."""
    opts = {
        "quiet": True,
        "no_warnings": True,
        "skip_download": True,
        "extract_flat": False,
        "no_check_formats": True,
        "dump_single_json": True,
    }

    if proxy:
        opts["proxy"] = proxy

    cookie_file = None
    if cookie:
        try:
            cookies_dict = json.loads(urllib.parse.unquote(cookie))
        except (json.JSONDecodeError, ValueError):
            cookies_dict = {"sessionid": cookie}

        # Write Netscape cookie file for yt-dlp
        cookie_file = tempfile.NamedTemporaryFile(
            mode="w", suffix=".txt", delete=False, prefix="insta_cookie_"
        )
        cookie_file.write("# Netscape HTTP Cookie File\n")
        for name, value in cookies_dict.items():
            cookie_file.write(f".instagram.com\tTRUE\t/\tTRUE\t0\t{name}\t{value}\n")
        cookie_file.close()
        opts["cookiefile"] = cookie_file.name

    return opts, cookie_file.name if cookie_file else None


def _extract_with_ytdlp(shortcode: str, proxy: Optional[str] = None, cookie: Optional[str] = None) -> dict:
    """Extract Instagram post using yt-dlp. Returns response in standard format."""
    url = _build_instagram_url(shortcode)
    opts, cookie_path = _build_ydl_opts(proxy, cookie)

    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            info = ydl.extract_info(url, download=False)
    finally:
        if cookie_path and os.path.exists(cookie_path):
            os.unlink(cookie_path)

    if not info:
        raise Exception("yt-dlp returned empty result")

    return _map_ytdlp_response(info, shortcode)


def _map_ytdlp_response(info: dict, shortcode: str) -> dict:
    """Map yt-dlp info dict to the standard API response format."""
    media = []
    entries = info.get("entries", [info])

    for entry in entries:
        is_video = entry.get("ext") in ("mp4", "webm") or bool(entry.get("url", ""))
        video_url = None
        display_url = None

        if is_video:
            # Get best video URL
            formats = entry.get("formats", [])
            if formats:
                # Pick best quality video
                best = max(formats, key=lambda f: (f.get("height") or 0, f.get("tbr") or 0))
                video_url = best.get("url")
            else:
                video_url = entry.get("url")

            # Thumbnail as display_url
            display_url = entry.get("thumbnail") or entry.get("thumbnails", [{}])[-1].get("url")
        else:
            # Image post
            display_url = entry.get("url") or entry.get("thumbnail")

        media.append({
            "is_video": is_video,
            "video_url": video_url,
            "display_url": display_url,
        })

    # If no entries found but info itself has data
    if not media and info.get("url"):
        is_video = info.get("ext") in ("mp4", "webm")
        media.append({
            "is_video": is_video,
            "video_url": info.get("url") if is_video else None,
            "display_url": info.get("thumbnail") or info.get("url"),
        })

    is_video = any(m["is_video"] for m in media)
    typename = "GraphSidecar" if len(media) > 1 else ("GraphVideo" if is_video else "GraphImage")

    caption = info.get("description") or ""
    caption_hashtags = re.findall(r"#(\w+)", caption)
    caption_mentions = re.findall(r"@(\w+)", caption)

    return {
        "shortcode": shortcode,
        "media_id": _safe_int(info.get("id")),
        "typename": typename,
        "caption": caption,
        "caption_hashtags": caption_hashtags,
        "caption_mentions": caption_mentions,
        "tagged_users": [],
        "likes": info.get("like_count"),
        "comments": info.get("comment_count"),
        "date_utc": info.get("upload_date"),
        "date_local": info.get("upload_date"),
        "is_video": is_video,
        "is_pinned": False,
        "is_sponsored": False,
        "video_duration": info.get("duration"),
        "video_view_count": info.get("view_count") if is_video else None,
        "video_play_count": None,
        "title": info.get("title") or "",
        "accessibility_caption": None,
        "owner_username": info.get("uploader") or info.get("uploader_id") or "",
        "owner_id": info.get("uploader_id") or info.get("channel_id"),
        "location": None,
        "media_count": len(media),
        "media": media,
        "extractor": "yt-dlp",
    }


# ============================================
# INSTALOADER EXTRACTION (FALLBACK)
# ============================================

def _extract_with_instaloader(shortcode: str, proxy: Optional[str] = None, cookie: Optional[str] = None) -> dict:
    """Extract Instagram post using instaloader (fallback)."""
    if proxy:
        _loader.context._session.proxies = {"https": proxy, "http": proxy}

    _ensure_login(cookie)

    post = instaloader.Post.from_shortcode(_loader.context, shortcode)

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
        "extractor": "instaloader",
    }


# ============================================
# ROUTES
# ============================================

@app.get("/health")
def health():
    return {"status": "ok", "service": "insta-extractor", "version": "2.0.0"}


@app.get("/extract", summary="Extract media from Instagram post")
def extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None):
    shortcode = parse_shortcode(url)

    # Attempt 1: yt-dlp (fast, no cookie needed)
    try:
        logger.info(f"[Extract] yt-dlp attempt for {shortcode}")
        result = _extract_with_ytdlp(shortcode, proxy=proxy, cookie=cookie)
        logger.info(f"[Extract] yt-dlp success for {shortcode}")
        return result
    except Exception as e:
        logger.warning(f"[Extract] yt-dlp failed for {shortcode}: {e}")

    # Attempt 2: instaloader fallback
    try:
        logger.info(f"[Extract] instaloader fallback for {shortcode}")
        result = _extract_with_instaloader(shortcode, proxy=proxy, cookie=cookie)
        logger.info(f"[Extract] instaloader success for {shortcode}")
        return result
    except Exception as e:
        logger.error(f"[Extract] instaloader also failed for {shortcode}: {e}")
        raise HTTPException(status_code=400, detail=f"All extractors failed for {shortcode}: {e}")


@app.get("/profile/{username}", summary="Get profile info with pagination")
def profile(
    username: str,
    proxy: Optional[str] = None,
    cookie: Optional[str] = None,
    offset: int = 0,
    limit: int = 12,
):
    if proxy:
        _loader.context._session.proxies = {"https": proxy, "http": proxy}

    _ensure_login(cookie)

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
            if current_index < offset:
                current_index += 1
                continue

            if len(posts) >= limit:
                has_more = True
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
                    "thumbnail": post.url,
                    "video_url": post.video_url if post.is_video else None,
                })
                current_index += 1
            except Exception:
                current_index += 1
                continue
    except Exception:
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
