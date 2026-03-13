from __future__ import annotations
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from typing import Optional
import yt_dlp
from yt_dlp.networking.impersonate import ImpersonateTarget
import re
import json
import urllib.parse
import logging
import tempfile
import os

logger = logging.getLogger("fb-extractor")

app = FastAPI(
    title="Facebook Extractor",
    version="1.0.0",
    description="Extract media links from Facebook videos, reels, stories. Powered by yt-dlp.",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ============================================
# HELPERS
# ============================================

def _safe_int(value) -> int:
    if value is None:
        return 0
    try:
        return int(value)
    except (ValueError, TypeError):
        return 0


def _is_facebook_url(url: str) -> bool:
    return bool(re.search(
        r"(?:facebook\.com|fb\.watch|fb\.com|facebookwkhpilnemxj7asaniu7vnjjbiltxjqhye3mhbshg7kx5tfyd\.onion)",
        url, re.IGNORECASE,
    ))


# ============================================
# YT-DLP EXTRACTION
# ============================================

def _build_ydl_opts(proxy: Optional[str] = None, cookie: Optional[str] = None) -> tuple[dict, Optional[str]]:
    opts = {
        "quiet": True,
        "no_warnings": True,
        "skip_download": True,
        "extract_flat": False,
        "no_check_formats": True,
        "dump_single_json": True,
        "impersonate": ImpersonateTarget.from_str("chrome"),
    }

    if proxy:
        opts["proxy"] = proxy

    cookie_file = None
    if cookie:
        try:
            cookies_dict = json.loads(urllib.parse.unquote(cookie))
        except (json.JSONDecodeError, ValueError):
            cookies_dict = {"c_user": cookie} if cookie.isdigit() else {"xs": cookie}

        cookie_file = tempfile.NamedTemporaryFile(
            mode="w", suffix=".txt", delete=False, prefix="fb_cookie_"
        )
        cookie_file.write("# Netscape HTTP Cookie File\n")
        for name, value in cookies_dict.items():
            cookie_file.write(f".facebook.com\tTRUE\t/\tTRUE\t0\t{name}\t{value}\n")
        cookie_file.close()
        opts["cookiefile"] = cookie_file.name

    return opts, cookie_file.name if cookie_file else None


def _extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None) -> dict:
    opts, cookie_path = _build_ydl_opts(proxy, cookie)

    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            info = ydl.extract_info(url, download=False)
    finally:
        if cookie_path and os.path.exists(cookie_path):
            os.unlink(cookie_path)

    if not info:
        raise Exception("yt-dlp returned empty result")

    return _map_response(info)


def _map_response(info: dict) -> dict:
    media = []
    entries = info.get("entries", [info])

    for entry in entries:
        is_video = entry.get("ext") in ("mp4", "webm") or bool(entry.get("formats"))
        video_url = None
        video_progressive_url = None
        audio_url = None
        display_url = None

        if is_video:
            formats = entry.get("formats", [])
            if formats:
                # Best progressive: has both video+audio
                # Facebook labels these as "sd"/"hd" with quality ranking but no height
                progressive = [f for f in formats
                               if f.get("acodec") != "none" and f.get("vcodec") != "none"]
                if progressive:
                    best_prog = max(progressive, key=lambda f: (
                        f.get("height") or 0, f.get("quality") or 0, f.get("tbr") or 0,
                    ))
                    video_progressive_url = best_prog.get("url")

                # Best video (any, including video-only DASH)
                video_formats = [f for f in formats if f.get("vcodec") != "none"]
                if video_formats:
                    best_video = max(video_formats, key=lambda f: (
                        f.get("height") or 0, f.get("quality") or 0, f.get("tbr") or 0,
                    ))
                    video_url = best_video.get("url")

                # Best audio-only (DASH)
                audio_formats = [f for f in formats if f.get("acodec") != "none" and f.get("vcodec") == "none"]
                if audio_formats:
                    best_audio = max(audio_formats, key=lambda f: f.get("abr") or f.get("tbr") or 0)
                    audio_url = best_audio.get("url")
            else:
                video_url = entry.get("url")

            display_url = entry.get("thumbnail") or (entry.get("thumbnails") or [{}])[-1].get("url")
        else:
            display_url = entry.get("url") or entry.get("thumbnail")

        media.append({
            "is_video": is_video,
            "video_url": video_url,
            "video_progressive_url": video_progressive_url,
            "audio_url": audio_url,
            "display_url": display_url,
        })

    # Fallback if no entries parsed
    if not media and info.get("url"):
        is_video = info.get("ext") in ("mp4", "webm")
        media.append({
            "is_video": is_video,
            "video_url": info.get("url") if is_video else None,
            "video_progressive_url": None,
            "audio_url": None,
            "display_url": info.get("thumbnail") or info.get("url"),
        })

    is_video = any(m["is_video"] for m in media)
    description = info.get("description") or ""

    return {
        "id": str(info.get("id") or ""),
        "typename": "Reel" if "/reel/" in (info.get("webpage_url") or "") else "Video",
        "title": info.get("title") or "",
        "description": description,
        "caption": description,
        "hashtags": re.findall(r"#(\w+)", description),
        "mentions": re.findall(r"@(\w+)", description),
        "owner_username": info.get("uploader") or "",
        "owner_id": info.get("uploader_id") or info.get("channel_id") or "",
        "owner_url": info.get("uploader_url") or "",
        "likes": info.get("like_count"),
        "comments": info.get("comment_count"),
        "shares": info.get("repost_count"),
        "date_utc": info.get("upload_date"),
        "timestamp": info.get("timestamp"),
        "is_video": is_video,
        "video_duration": info.get("duration"),
        "video_view_count": info.get("view_count") if is_video else None,
        "media_count": len(media),
        "media": media,
        "subtitles": list((info.get("subtitles") or {}).keys()),
        "automatic_captions": list((info.get("automatic_captions") or {}).keys()),
        "extractor": "yt-dlp",
    }


# ============================================
# ROUTES
# ============================================

@app.get("/health")
def health():
    return {"status": "ok", "service": "fb-extractor", "version": "1.0.0"}


@app.get("/extract", summary="Extract media from Facebook video/reel")
def extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None):
    if not _is_facebook_url(url):
        raise HTTPException(status_code=400, detail="Not a valid Facebook URL")

    try:
        logger.info(f"[Extract] yt-dlp attempt for {url}")
        result = _extract(url, proxy=proxy, cookie=cookie)
        logger.info(f"[Extract] yt-dlp success for {url}")
        return result
    except Exception as e:
        logger.error(f"[Extract] yt-dlp failed for {url}: {e}")
        raise HTTPException(status_code=400, detail=f"Extraction failed: {e}")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8002)
