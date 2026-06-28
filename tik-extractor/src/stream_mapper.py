"""
Normalized stream mapper for TikTok detail responses.

The TikTok extractor is the vendored DouK-Downloader project, whose ``/tiktok/detail``
endpoint returns a single simplified ``data`` dict (not a yt-dlp ``formats`` array).
Unlike YouTube, TikTok exposes at most one downloadable video (the no-watermark
render) plus one music/audio track, so the structured list is intentionally short.

This module adds a cross-platform ``videoStreams`` / ``audioStreams`` view that
matches the shape returned by yt-extractor, fb-extractor and x-extractor, so a
single client-side "format picker" can handle every platform uniformly.

It is pure: it depends only on plain dicts (no DouK / network imports), so it can be
unit-tested in isolation and never diverges from upstream extraction logic.
"""

from __future__ import annotations

from typing import Optional
from urllib.parse import urlparse

FORMAT_NAMES = {
    "mp4": "MPEG-4",
    "webm": "WebM",
    "m4a": "M4A",
    "mp3": "MP3",
    "aac": "AAC",
}

AUDIO_MIME = {
    "mp3": "audio/mpeg",
    "m4a": "audio/mp4",
    "aac": "audio/aac",
}


# ============================================
# HELPERS
# ============================================

def _format_name(ext: str) -> str:
    """Convert a file extension into a human-friendly format name."""
    return FORMAT_NAMES.get(ext, ext.upper())


def _infer_ext(url: Optional[str], default: str) -> str:
    """Best-effort file-extension detection from a URL path."""
    if not url:
        return default
    path = urlparse(url).path.lower()
    if "." in path:
        ext = path.rsplit(".", 1)[-1]
        if ext in FORMAT_NAMES:
            return ext
    return default


def _video_download_url(data: dict) -> Optional[str]:
    """
    Return the no-watermark video URL when the post is a video.

    DouK sets ``downloads`` to a single URL string for videos and to a list of
    image URLs for photo/slideshow posts. Only the former yields a video stream.
    """
    downloads = data.get("downloads")
    if isinstance(downloads, str) and downloads:
        return downloads
    return None


# ============================================
# STREAM BUILDERS
# ============================================

def build_video_streams(data: dict) -> list[dict]:
    """Build the (at most one) video stream for a TikTok video post."""
    url = _video_download_url(data)
    if not url:
        return []

    height = data.get("height") or None
    width = data.get("width") or None
    ext = _infer_ext(url, "mp4")

    return [{
        "url": url,
        "quality": f"{height}p" if height else "source",
        "format": _format_name(ext),
        "mimeType": f"video/{ext}",
        "bitrate": None,          # DouK does not expose per-format bitrate
        "fileSize": None,
        "codec": None,
        "width": width,
        "height": height,
        "fps": None,
        # TikTok's no-watermark render is muxed (carries audio).
        "videoOnly": False,
    }]


def build_audio_streams(data: dict) -> list[dict]:
    """Build the (at most one) audio stream from the post's music track."""
    music_url = data.get("music_url")
    if not isinstance(music_url, str) or not music_url:
        return []

    ext = _infer_ext(music_url, "mp3")
    title = data.get("music_title") or None

    return [{
        "url": music_url,
        "quality": title or "music",
        "format": _format_name(ext),
        "mimeType": AUDIO_MIME.get(ext, f"audio/{ext}"),
        "bitrate": None,
        "fileSize": None,
        "codec": None,
    }]


# ============================================
# ENRICHMENT ENTRY POINT
# ============================================

def enrich_with_streams(data: dict) -> dict:
    """
    Add ``videoStreams`` / ``audioStreams`` to a DouK detail ``data`` dict in place.

    Returns the same dict for convenience. Non-dict input (e.g. raw ``source``
    responses or ``None``) is returned unchanged.
    """
    if not isinstance(data, dict):
        return data

    data["videoStreams"] = build_video_streams(data)
    data["audioStreams"] = build_audio_streams(data)
    return data
