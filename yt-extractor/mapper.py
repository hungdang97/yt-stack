"""
Mapper module to convert yt-dlp info to API response format.

yt-dlp format types:
- AUDIO_ONLY: vcodec='none', acodec != 'none' (e.g., m4a, webm audio)
- VIDEO_ONLY: vcodec != 'none', acodec='none' (e.g., mp4/webm video without audio)
- VIDEO+AUDIO: vcodec != 'none', acodec != 'none' (e.g., format_id=18, 360p mp4)
- STORYBOARD: vcodec='none', acodec='none' (skip these)
"""

from urllib.parse import urlparse, parse_qs


def parse_audio_lang_from_url(url):
    """
    Extract language code and audio content type from URL xtags parameter.

    URL contains xtags like: acont=dubbed:lang=vi or acont=original:lang=en

    Returns:
        tuple: (lang_code, audio_content_type) e.g. ('vi', 'dubbed') or ('en', 'original')
               Returns (None, None) if not found
    """
    if not url:
        return None, None

    try:
        parsed = urlparse(url)
        params = parse_qs(parsed.query)
        xtags = params.get('xtags', [''])[0]

        lang_code = None
        acont_type = None

        for part in xtags.split(':'):
            if part.startswith('lang='):
                lang_code = part.split('=', 1)[1]
            elif part.startswith('acont='):
                acont_type = part.split('=', 1)[1]

        return lang_code, acont_type
    except Exception:
        return None, None


FORMAT_NAMES = {
    'mp4': 'MPEG-4',
    'webm': 'WebM',
    'm4a': 'M4A',
    'opus': 'Opus',
}


def get_format_name(ext):
    """Convert extension to format name."""
    return FORMAT_NAMES.get(ext, ext.upper() if ext else None)


def get_filesize(fmt):
    """Get filesize from format, preferring exact size over approximate."""
    return fmt.get('filesize') or fmt.get('filesize_approx')


def get_best_thumbnail(thumbnails):
    """Get best quality thumbnail URL from thumbnails list."""
    if not thumbnails:
        return None

    # Prefer maxresdefault or hqdefault
    for thumb in thumbnails:
        thumb_id = thumb.get('id', '')
        if 'maxres' in thumb_id or 'hq' in thumb_id:
            return thumb.get('url')

    # Fallback to last thumbnail (usually highest quality)
    return thumbnails[-1].get('url')


def build_codec_string(vcodec, acodec):
    """Build codec string from video and audio codecs."""
    if vcodec and vcodec != 'none' and acodec and acodec != 'none':
        return f"{vcodec}, {acodec}"
    if vcodec and vcodec != 'none':
        return vcodec
    return None


def map_video_stream(fmt):
    """Map yt-dlp format to API video stream format."""
    vcodec = fmt.get('vcodec')
    acodec = fmt.get('acodec')
    ext = fmt.get('ext', 'mp4')

    return {
        'url': fmt.get('url'),
        'quality': fmt.get('format_note') or f"{fmt.get('height', 0)}p",
        'format': get_format_name(ext),
        'mimeType': f"video/{ext}",
        'bitrate': fmt.get('vbr') or fmt.get('tbr'),
        'fileSize': get_filesize(fmt),
        'codec': build_codec_string(vcodec, acodec),
        'width': fmt.get('width'),
        'height': fmt.get('height'),
        'fps': fmt.get('fps'),
        'videoOnly': acodec == 'none' or acodec is None,
    }


def map_audio_stream(fmt):
    """Map yt-dlp format to API audio stream format."""
    ext = fmt.get('ext', 'm4a')
    url = fmt.get('url')
    
    # ✅ Use yt-dlp's official 'language' field as primary source
    lang_code = fmt.get('language') or fmt.get('audio_lang')
    
    # Parse format_note to extract original/dubbed info
    format_note = fmt.get('format_note', '')
    is_original = 'original' in format_note.lower()
    
    # Fallback: parse from URL if language field is not available
    if not lang_code:
        url_lang, url_acont = parse_audio_lang_from_url(url)
        lang_code = url_lang
        acont_type = url_acont
    else:
        # If language field exists, determine content type from format_note
        acont_type = 'original' if is_original else None
        
        # Secondary fallback: check URL for content type if not in format_note
        if not acont_type:
            _, url_acont = parse_audio_lang_from_url(url)
            acont_type = url_acont

    return {
        'url': url,
        'quality': format_note or f"{int(fmt.get('abr') or 0)}kbps",
        'format': get_format_name(ext),
        'mimeType': f"audio/{ext}",
        'bitrate': fmt.get('abr') or fmt.get('tbr'),
        'fileSize': get_filesize(fmt),
        'codec': fmt.get('acodec'),
        'audioTrackId': lang_code,
        'audioTrackType': acont_type,
        'isOriginal': is_original or acont_type == 'original',
    }


def map_yt_dlp_to_api(info):
    """
    Map yt-dlp info to API response format.

    Args:
        info: yt-dlp extract_info result

    Returns:
        dict with videoStreams, audioStreams, and metadata
    """
    video_streams = []
    audio_streams = []

    for fmt in info.get('formats', []):
        vcodec = fmt.get('vcodec', 'none')
        acodec = fmt.get('acodec', 'none')

        # Skip storyboards (no video or audio codec)
        if vcodec == 'none' and acodec == 'none':
            continue

        if vcodec != 'none':
            video_streams.append(map_video_stream(fmt))
        elif acodec != 'none':
            audio_streams.append(map_audio_stream(fmt))

    # Extract available audio languages (unique language codes)
    available_languages = []
    seen_languages = set()
    for stream in audio_streams:
        lang = stream.get('audioTrackId')
        if lang and lang not in seen_languages:
            available_languages.append(lang)
            seen_languages.add(lang)

    categories = info.get('categories')

    return {
        'id': info.get('id'),
        'title': info.get('title'),
        'uploaderName': info.get('uploader') or info.get('channel'),
        'uploaderUrl': info.get('uploader_url') or info.get('channel_url'),
        'thumbnailUrl': get_best_thumbnail(info.get('thumbnails', [])),
        'duration': info.get('duration'),
        'videoStreams': video_streams,
        'audioStreams': audio_streams,
        'availableAudioLanguages': available_languages,
        'subtitles': [],
        'category': categories[0] if categories else None,
        'tags': info.get('tags', []),
    }
