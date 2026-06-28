"""
Mapper module to convert yt-dlp info into the fb-extractor API response.

The response is intentionally aligned with yt-extractor so that clients can reuse
the same "format picker" logic across platforms. Two views of the same data are
returned:

- ``videoStreams`` / ``audioStreams``: YouTube-style structured format lists.
- ``media``: the legacy progressive/DASH URL shape consumed by fb-downloader.

yt-dlp format types (same rules as yt-extractor):
- AUDIO_ONLY:   vcodec == 'none' and acodec != 'none'
- VIDEO_ONLY:   vcodec != 'none' and acodec == 'none'
- PROGRESSIVE:  vcodec != 'none' and acodec != 'none'  (video + audio in one file)
- STORYBOARD:   vcodec == 'none' and acodec == 'none'  (skipped)

This module is pure: it only depends on plain dicts, never on yt_dlp itself, so it
can be unit-tested without network access or the yt_dlp dependency installed.
"""

from __future__ import annotations

import re
from typing import Any, Optional

FORMAT_NAMES = {
    "mp4": "MPEG-4",
    "webm": "WebM",
    "m4a": "M4A",
    "mp3": "MP3",
    "opus": "Opus",
    "aac": "AAC",
}

NONE_VALUES = (None, "none", "")


# ============================================
# SMALL HELPERS
# ============================================

def _is_present(codec: Optional[str]) -> bool:
    """Return True when a codec value represents a real (non-empty) codec."""
    return codec not in NONE_VALUES


def _track_present(codec: Optional[str], ext: Optional[str]) -> bool:
    """
    Decide whether a video/audio track is present for a format.

    Facebook progressive formats often arrive with ``vcodec``/``acodec`` set to
    ``None`` and only expose ``video_ext``/``audio_ext`` (e.g. ``"mp4"`` / ``"none"``).
    Prefer the codec when known, otherwise fall back to the ext hint.
    """
    if codec not in (None, ""):
        return codec != "none"
    if ext not in (None, ""):
        return ext != "none"
    return False


def _has_video(fmt: dict) -> bool:
    """True when a format carries a video track."""
    return _track_present(fmt.get("vcodec"), fmt.get("video_ext"))


def _has_audio(fmt: dict) -> bool:
    """True when a format carries an audio track."""
    return _track_present(fmt.get("acodec"), fmt.get("audio_ext"))


def _get_format_name(ext: Optional[str]) -> Optional[str]:
    """Convert a file extension into a human-friendly format name."""
    if not ext:
        return None
    return FORMAT_NAMES.get(ext, ext.upper())


def _get_filesize(fmt: dict) -> Optional[int]:
    """Get filesize, preferring the exact value over the approximate one."""
    return fmt.get("filesize") or fmt.get("filesize_approx")


def _build_codec_string(vcodec: Optional[str], acodec: Optional[str]) -> Optional[str]:
    """Build a combined codec string from the video and audio codecs."""
    has_video = _is_present(vcodec)
    has_audio = _is_present(acodec)
    if has_video and has_audio:
        return f"{vcodec}, {acodec}"
    if has_video:
        return vcodec
    if has_audio:
        return acodec
    return None


def _classify_format(fmt: dict) -> str:
    """Classify a yt-dlp format into video / audio / storyboard."""
    if _has_video(fmt):
        return "video"
    if _has_audio(fmt):
        return "audio"
    return "storyboard"


# ============================================
# STREAM MAPPERS
# ============================================

def _map_video_stream(fmt: dict) -> dict:
    """Map a yt-dlp video (or progressive) format to an API video stream."""
    vcodec = fmt.get("vcodec")
    acodec = fmt.get("acodec")
    ext = fmt.get("ext") or "mp4"
    height = fmt.get("height")

    quality = fmt.get("format_note")
    if not quality and height:
        quality = f"{height}p"
    if not quality:
        # Facebook progressive formats expose neither; fall back to format_id (sd/hd).
        quality = fmt.get("format_id") or "source"

    return {
        "url": fmt.get("url"),
        "quality": quality,
        "format": _get_format_name(ext),
        "mimeType": f"video/{ext}",
        "bitrate": fmt.get("vbr") or fmt.get("tbr"),
        "fileSize": _get_filesize(fmt),
        "codec": _build_codec_string(vcodec, acodec),
        "width": fmt.get("width"),
        "height": height,
        "fps": fmt.get("fps"),
        "videoOnly": not _has_audio(fmt),
    }


def _map_audio_stream(fmt: dict) -> dict:
    """Map a yt-dlp audio-only format to an API audio stream."""
    ext = fmt.get("ext") or "m4a"
    abr = fmt.get("abr")

    quality = fmt.get("format_note")
    if not quality and abr:
        quality = f"{int(abr)}kbps"

    return {
        "url": fmt.get("url"),
        "quality": quality,
        "format": _get_format_name(ext),
        "mimeType": f"audio/{ext}",
        "bitrate": abr or fmt.get("tbr"),
        "fileSize": _get_filesize(fmt),
        "codec": fmt.get("acodec"),
    }


def _raw_video_sort_key(fmt: dict) -> tuple:
    """Sort raw video formats best-first (height, bitrate, yt-dlp quality)."""
    return (
        fmt.get("height") or 0,
        fmt.get("vbr") or fmt.get("tbr") or 0,
        fmt.get("quality") or 0,
    )


def _raw_audio_sort_key(fmt: dict) -> tuple:
    """Sort raw audio formats best-first (bitrate, yt-dlp quality)."""
    return (
        fmt.get("abr") or fmt.get("tbr") or 0,
        fmt.get("quality") or 0,
    )


def build_streams(formats: list[dict]) -> tuple[list[dict], list[dict]]:
    """
    Split a list of yt-dlp formats into sorted video and audio streams.

    Progressive formats (video + audio) are listed under ``videoStreams`` with
    ``videoOnly == False``. Storyboards are skipped. Both lists are sorted so the
    best quality stream is first, ranking on the raw yt-dlp fields (which include
    ``quality`` even when ``height`` is missing, as with Facebook sd/hd).
    """
    raw_videos: list[dict] = []
    raw_audios: list[dict] = []

    for fmt in formats:
        if not fmt.get("url"):
            continue
        kind = _classify_format(fmt)
        if kind == "video":
            raw_videos.append(fmt)
        elif kind == "audio":
            raw_audios.append(fmt)
        # storyboards are intentionally skipped

    raw_videos.sort(key=_raw_video_sort_key, reverse=True)
    raw_audios.sort(key=_raw_audio_sort_key, reverse=True)

    video_streams = [_map_video_stream(f) for f in raw_videos]
    audio_streams = [_map_audio_stream(f) for f in raw_audios]

    return video_streams, audio_streams


# ============================================
# LEGACY MEDIA SHAPE (consumed by fb-downloader)
# ============================================

def _best_progressive_url(formats: list[dict]) -> Optional[str]:
    """Pick the best progressive (video + audio) URL.

    Uses the original ``!= "none"`` predicate so a null codec counts as present,
    preserving the proven behaviour fb-downloader depends on.
    """
    progressive = [
        f for f in formats
        if f.get("acodec") != "none" and f.get("vcodec") != "none"
    ]
    if not progressive:
        return None
    best = max(progressive, key=lambda f: (
        f.get("height") or 0, f.get("quality") or 0, f.get("tbr") or 0,
    ))
    return best.get("url")


def _best_video_url(formats: list[dict]) -> Optional[str]:
    """Pick the best video URL (including video-only DASH)."""
    videos = [f for f in formats if f.get("vcodec") != "none"]
    if not videos:
        return None
    best = max(videos, key=lambda f: (
        f.get("height") or 0, f.get("quality") or 0, f.get("tbr") or 0,
    ))
    return best.get("url")


def _best_audio_url(formats: list[dict]) -> Optional[str]:
    """Pick the best audio-only (DASH) URL."""
    audios = [
        f for f in formats
        if f.get("acodec") != "none" and f.get("vcodec") == "none"
    ]
    if not audios:
        return None
    best = max(audios, key=lambda f: f.get("abr") or f.get("tbr") or 0)
    return best.get("url")


def _display_url(entry: dict) -> Optional[str]:
    """Resolve the best display/thumbnail URL for an entry."""
    thumbnail = entry.get("thumbnail")
    if thumbnail:
        return thumbnail
    thumbnails = entry.get("thumbnails") or []
    if thumbnails:
        return thumbnails[-1].get("url")
    return None


def _build_media_item(entry: dict) -> dict:
    """Build a single legacy media item from a yt-dlp entry."""
    formats = entry.get("formats") or []
    is_video = entry.get("ext") in ("mp4", "webm") or bool(formats)

    if is_video:
        if formats:
            return {
                "is_video": True,
                "video_url": _best_video_url(formats),
                "video_progressive_url": _best_progressive_url(formats),
                "audio_url": _best_audio_url(formats),
                "display_url": _display_url(entry),
            }
        return {
            "is_video": True,
            "video_url": entry.get("url"),
            "video_progressive_url": None,
            "audio_url": None,
            "display_url": _display_url(entry),
        }

    return {
        "is_video": False,
        "video_url": None,
        "video_progressive_url": None,
        "audio_url": None,
        "display_url": entry.get("url") or entry.get("thumbnail"),
    }


def _build_media(info: dict) -> list[dict]:
    """Build the legacy media[] list, handling carousels and fallbacks."""
    entries = info.get("entries") or [info]
    media = [_build_media_item(entry) for entry in entries]

    # Fallback when nothing was parsed but a direct URL exists.
    if not media and info.get("url"):
        is_video = info.get("ext") in ("mp4", "webm")
        media.append({
            "is_video": is_video,
            "video_url": info.get("url") if is_video else None,
            "video_progressive_url": None,
            "audio_url": None,
            "display_url": info.get("thumbnail") or info.get("url"),
        })

    return media


def _primary_entry(info: dict) -> dict:
    """
    Return the entry that represents the primary video.

    For a carousel, that is the first entry exposing formats; otherwise the info
    object itself is used.
    """
    entries = info.get("entries")
    if not entries:
        return info
    for entry in entries:
        if entry.get("formats") or entry.get("ext") in ("mp4", "webm"):
            return entry
    return entries[0]


# ============================================
# TOP-LEVEL MAPPER
# ============================================

def map_fb_response(info: dict) -> dict:
    """
    Map a yt-dlp info dict to the fb-extractor API response.

    Adds YouTube-style ``videoStreams`` / ``audioStreams`` while preserving the
    legacy ``media`` shape used by fb-downloader.
    """
    media = _build_media(info)

    primary = _primary_entry(info)
    video_streams, audio_streams = build_streams(primary.get("formats") or [])

    is_video = any(item["is_video"] for item in media)
    description = info.get("description") or ""
    webpage_url = info.get("webpage_url") or ""

    return {
        "id": str(info.get("id") or ""),
        "typename": "Reel" if "/reel/" in webpage_url else "Video",
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
        "videoStreams": video_streams,
        "audioStreams": audio_streams,
        "media_count": len(media),
        "media": media,
        "subtitles": list((info.get("subtitles") or {}).keys()),
        "automatic_captions": list((info.get("automatic_captions") or {}).keys()),
        "extractor": "yt-dlp",
    }
