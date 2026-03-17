from __future__ import annotations
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from typing import Optional
import yt_dlp
import re
import urllib.parse
import logging
import tempfile
import os

logger = logging.getLogger("uni-extractor")

app = FastAPI(
    title="Universal Extractor",
    version="2.0.0",
    description="Extract media links from any supported site. Powered by yt-dlp.",
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


# ============================================
# YT-DLP EXTRACTION
# ============================================

def _build_ydl_opts(proxy: Optional[str] = None, cookie: Optional[str] = None, url: Optional[str] = None) -> tuple[dict, Optional[str]]:
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
    if cookie and url:
        # Extract domain from URL for cookie
        try:
            parsed = urllib.parse.urlparse(url)
            domain = parsed.hostname or ""
        except Exception:
            domain = ""

        if domain:
            cookie_file = tempfile.NamedTemporaryFile(
                mode="w", suffix=".txt", delete=False, prefix="uni_cookie_"
            )
            cookie_file.write("# Netscape HTTP Cookie File\n")
            # Write raw cookie string as a single cookie line
            for part in cookie.split(";"):
                part = part.strip()
                if "=" in part:
                    name, value = part.split("=", 1)
                    cookie_file.write(f".{domain}\tTRUE\t/\tTRUE\t0\t{name.strip()}\t{value.strip()}\n")
            cookie_file.close()
            opts["cookiefile"] = cookie_file.name

    return opts, cookie_file.name if cookie_file else None


def _extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None) -> dict:
    opts, cookie_path = _build_ydl_opts(proxy, cookie, url)

    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            info = ydl.extract_info(url, download=False)
    finally:
        if cookie_path and os.path.exists(cookie_path):
            os.unlink(cookie_path)

    if not info:
        raise Exception("yt-dlp returned empty result")

    # Detect live streams
    if _is_live_stream(info):
        raise Exception("Live streams are not supported. Please provide a VOD/clip URL instead.")

    return _map_response(info)


def _is_live_stream(info: dict) -> bool:
    """Detect live streams via yt-dlp fields and heuristics."""
    # Explicit flags
    if info.get("is_live") is True:
        return True
    if info.get("live_status") in ("is_live", "is_upcoming"):
        return True

    # Heuristic: no duration + all formats are m3u8 = likely live
    if info.get("duration") is None:
        formats = info.get("formats", [])
        if formats and all(
            (f.get("protocol") or "").startswith("m3u8") for f in formats
        ):
            return True

    return False


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
                def _is_direct(f):
                    """Returns True if the format is a direct HTTP(S) download (not HLS/DASH manifest)."""
                    proto = (f.get("protocol") or "").lower()
                    url_val = (f.get("url") or "").lower()
                    return proto not in ("m3u8", "m3u8_native", "dash") and ".m3u8" not in url_val and ".mpd" not in url_val

                def _quality_key(f):
                    return (f.get("height") or 0, f.get("quality") or 0, f.get("tbr") or 0)

                # Best progressive: has both video+audio
                progressive = [f for f in formats
                               if f.get("acodec") != "none" and f.get("vcodec") != "none"]
                if progressive:
                    # At same quality, prefer direct over m3u8 (faster download)
                    best_prog = max(progressive, key=lambda f: (*_quality_key(f), _is_direct(f)))
                    video_progressive_url = best_prog.get("url")

                # Best video (any, including video-only DASH)
                video_formats = [f for f in formats if f.get("vcodec") != "none"]
                if video_formats:
                    best_video = max(video_formats, key=lambda f: (*_quality_key(f), _is_direct(f)))
                    video_url = best_video.get("url")

                # Best audio-only
                audio_formats = [f for f in formats if f.get("acodec") != "none" and f.get("vcodec") == "none"]
                if audio_formats:
                    best_audio = max(audio_formats, key=lambda f: (f.get("abr") or f.get("tbr") or 0, _is_direct(f)))
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
        "typename": info.get("extractor_key", "Unknown"),
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
    return {"status": "ok", "service": "uni-extractor", "version": "2.0.0"}


@app.get("/extract", summary="Extract media from any supported URL")
def extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None):
    if not url:
        raise HTTPException(status_code=400, detail="URL is required")

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
    uvicorn.run(app, host="0.0.0.0", port=8004)
