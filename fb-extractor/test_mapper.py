"""
Unit tests for fb-extractor mapper.

Run with:  python3 -m unittest -v

These tests use realistic yt-dlp ``info`` samples for Facebook content so the
mapper can be validated without any network access or the yt_dlp dependency.
"""

import unittest

from mapper import build_streams, map_fb_response


# ============================================
# SAMPLE yt-dlp FORMATS (shaped like real Facebook output)
# ============================================

# Facebook DASH: separated video-only renditions + a single audio-only track,
# plus two progressive (sd/hd) muxed formats.
FB_DASH_FORMATS = [
    {
        "format_id": "dash_sd_src_no_ratelimit",
        "url": "https://video.fb/dash_sd_video.mp4",
        "ext": "mp4",
        "vcodec": "h264",
        "acodec": "none",
        "width": 640,
        "height": 360,
        "fps": 30,
        "tbr": 512.0,
        "vbr": 480.0,
        "filesize": 1_500_000,
        "format_note": "sd",
    },
    {
        "format_id": "dash_hd_src_no_ratelimit",
        "url": "https://video.fb/dash_hd_video.mp4",
        "ext": "mp4",
        "vcodec": "h264",
        "acodec": "none",
        "width": 1280,
        "height": 720,
        "fps": 30,
        "tbr": 1500.0,
        "vbr": 1400.0,
        "filesize_approx": 4_200_000,
        "format_note": "hd",
    },
    {
        "format_id": "dash_audio",
        "url": "https://video.fb/dash_audio.m4a",
        "ext": "m4a",
        "vcodec": "none",
        "acodec": "aac",
        "abr": 128.0,
        "tbr": 128.0,
        "filesize": 320_000,
        "format_note": "audio",
    },
    {
        "format_id": "sd",
        "url": "https://video.fb/progressive_sd.mp4",
        "ext": "mp4",
        "vcodec": "h264",
        "acodec": "aac",
        "width": 640,
        "height": 360,
        "tbr": 640.0,
    },
    {
        "format_id": "hd",
        "url": "https://video.fb/progressive_hd.mp4",
        "ext": "mp4",
        "vcodec": "h264",
        "acodec": "aac",
        "width": 1280,
        "height": 720,
        "tbr": 1640.0,
    },
    # Storyboard-like entry that must be skipped.
    {
        "format_id": "sb0",
        "url": "https://video.fb/storyboard.jpg",
        "ext": "mhtml",
        "vcodec": "none",
        "acodec": "none",
    },
]


def _video_info():
    return {
        "id": "1234567890",
        "title": "A funny clip",
        "description": "great stuff #fun #cats @page",
        "webpage_url": "https://www.facebook.com/watch/?v=1234567890",
        "uploader": "Some Page",
        "uploader_id": "page-1",
        "uploader_url": "https://www.facebook.com/somepage",
        "duration": 42.0,
        "view_count": 1000,
        "like_count": 50,
        "ext": "mp4",
        "thumbnail": "https://video.fb/thumb.jpg",
        "formats": FB_DASH_FORMATS,
        "subtitles": {"en": [{"url": "x"}]},
        "automatic_captions": {"vi": [{"url": "y"}]},
    }


class BuildStreamsTest(unittest.TestCase):
    def test_splits_video_and_audio(self):
        video, audio = build_streams(FB_DASH_FORMATS)
        # 2 DASH video-only + 2 progressive = 4 video streams; storyboard skipped.
        self.assertEqual(len(video), 4)
        self.assertEqual(len(audio), 1)

    def test_video_sorted_best_first(self):
        video, _ = build_streams(FB_DASH_FORMATS)
        heights = [v["height"] for v in video]
        self.assertEqual(heights, sorted(heights, reverse=True))
        self.assertEqual(video[0]["height"], 720)

    def test_progressive_flagged_not_video_only(self):
        video, _ = build_streams(FB_DASH_FORMATS)
        progressive = [v for v in video if not v["videoOnly"]]
        video_only = [v for v in video if v["videoOnly"]]
        self.assertEqual(len(progressive), 2)
        self.assertEqual(len(video_only), 2)

    def test_video_stream_fields(self):
        video, _ = build_streams(FB_DASH_FORMATS)
        hd = next(v for v in video if v["videoOnly"] and v["height"] == 720)
        self.assertEqual(hd["url"], "https://video.fb/dash_hd_video.mp4")
        self.assertEqual(hd["quality"], "hd")
        self.assertEqual(hd["format"], "MPEG-4")
        self.assertEqual(hd["mimeType"], "video/mp4")
        self.assertEqual(hd["bitrate"], 1400.0)        # prefers vbr over tbr
        self.assertEqual(hd["fileSize"], 4_200_000)    # filesize_approx fallback
        self.assertEqual(hd["codec"], "h264")          # audio is 'none'
        self.assertEqual(hd["fps"], 30)

    def test_progressive_codec_string_combines(self):
        video, _ = build_streams(FB_DASH_FORMATS)
        prog = next(v for v in video if not v["videoOnly"] and v["height"] == 720)
        self.assertEqual(prog["codec"], "h264, aac")
        self.assertEqual(prog["quality"], "720p")  # no format_note -> height fallback

    def test_progressive_quality_falls_back_to_height(self):
        video, _ = build_streams(FB_DASH_FORMATS)
        prog = next(v for v in video if not v["videoOnly"] and v["height"] == 360)
        self.assertEqual(prog["quality"], "360p")

    def test_audio_stream_fields(self):
        _, audio = build_streams(FB_DASH_FORMATS)
        a = audio[0]
        self.assertEqual(a["url"], "https://video.fb/dash_audio.m4a")
        self.assertEqual(a["format"], "M4A")
        self.assertEqual(a["mimeType"], "audio/m4a")
        self.assertEqual(a["bitrate"], 128.0)
        self.assertEqual(a["codec"], "aac")
        self.assertEqual(a["fileSize"], 320_000)

    def test_skips_formats_without_url(self):
        formats = [{"vcodec": "h264", "acodec": "none", "height": 480}]  # no url
        video, audio = build_streams(formats)
        self.assertEqual(video, [])
        self.assertEqual(audio, [])

    def test_empty_formats(self):
        video, audio = build_streams([])
        self.assertEqual(video, [])
        self.assertEqual(audio, [])


class MapFbResponseTest(unittest.TestCase):
    def test_adds_stream_lists(self):
        result = map_fb_response(_video_info())
        self.assertEqual(len(result["videoStreams"]), 4)
        self.assertEqual(len(result["audioStreams"]), 1)

    def test_preserves_legacy_media_shape(self):
        result = map_fb_response(_video_info())
        self.assertEqual(result["media_count"], 1)
        item = result["media"][0]
        self.assertTrue(item["is_video"])
        # best video: both HD are 720p, so the higher-tbr progressive (1640) wins
        # over the DASH video-only (1500) — preserving legacy fb-downloader behavior.
        self.assertEqual(item["video_url"], "https://video.fb/progressive_hd.mp4")
        # best progressive (height tie 720 -> highest tbr)
        self.assertEqual(item["video_progressive_url"], "https://video.fb/progressive_hd.mp4")
        self.assertEqual(item["audio_url"], "https://video.fb/dash_audio.m4a")
        self.assertEqual(item["display_url"], "https://video.fb/thumb.jpg")

    def test_metadata_fields(self):
        result = map_fb_response(_video_info())
        self.assertEqual(result["id"], "1234567890")
        self.assertEqual(result["typename"], "Video")
        self.assertEqual(result["title"], "A funny clip")
        self.assertEqual(result["hashtags"], ["fun", "cats"])
        self.assertEqual(result["mentions"], ["page"])
        self.assertEqual(result["owner_username"], "Some Page")
        self.assertTrue(result["is_video"])
        self.assertEqual(result["video_duration"], 42.0)
        self.assertEqual(result["subtitles"], ["en"])
        self.assertEqual(result["automatic_captions"], ["vi"])
        self.assertEqual(result["extractor"], "yt-dlp")

    def test_reel_typename(self):
        info = _video_info()
        info["webpage_url"] = "https://www.facebook.com/reel/9988"
        result = map_fb_response(info)
        self.assertEqual(result["typename"], "Reel")

    def test_image_post_has_no_streams(self):
        info = {
            "id": "img1",
            "title": "A photo",
            "webpage_url": "https://www.facebook.com/photo/?fbid=img1",
            "ext": "jpg",
            "url": "https://photo.fb/image.jpg",
            "thumbnail": "https://photo.fb/thumb.jpg",
        }
        result = map_fb_response(info)
        self.assertEqual(result["videoStreams"], [])
        self.assertEqual(result["audioStreams"], [])
        self.assertFalse(result["is_video"])
        self.assertEqual(result["media_count"], 1)
        self.assertFalse(result["media"][0]["is_video"])
        self.assertEqual(result["media"][0]["display_url"], "https://photo.fb/image.jpg")
        self.assertIsNone(result["video_view_count"])

    def test_carousel_uses_primary_video_entry(self):
        info = {
            "id": "album1",
            "title": "Album",
            "webpage_url": "https://www.facebook.com/media/set/?set=album1",
            "entries": [
                {"id": "p1", "ext": "jpg", "url": "https://photo.fb/1.jpg"},
                {"id": "v1", "ext": "mp4", "thumbnail": "https://video.fb/t.jpg",
                 "formats": FB_DASH_FORMATS},
            ],
        }
        result = map_fb_response(info)
        # Streams come from the video entry, not the leading image.
        self.assertEqual(len(result["videoStreams"]), 4)
        self.assertEqual(len(result["audioStreams"]), 1)
        self.assertEqual(result["media_count"], 2)
        self.assertTrue(result["is_video"])

    def test_single_url_video_without_formats(self):
        info = {
            "id": "v9",
            "title": "Direct",
            "ext": "mp4",
            "url": "https://video.fb/direct.mp4",
            "webpage_url": "https://www.facebook.com/watch/?v=v9",
        }
        result = map_fb_response(info)
        self.assertEqual(result["videoStreams"], [])
        item = result["media"][0]
        self.assertTrue(item["is_video"])
        self.assertEqual(item["video_url"], "https://video.fb/direct.mp4")


# Regression: real Facebook progressive formats arrive with vcodec/acodec == null
# and only expose video_ext/audio_ext + format_id (sd/hd) + a numeric quality.
# (Captured from a live facebook.com/watch extraction via yt-dlp.)
FB_REAL_PROGRESSIVE_FORMATS = [
    {
        "format_id": "sd",
        "url": "https://video.fb/real_sd.mp4",
        "ext": "mp4",
        "vcodec": None,
        "acodec": None,
        "video_ext": "mp4",
        "audio_ext": "none",
        "quality": -3,
    },
    {
        "format_id": "hd",
        "url": "https://video.fb/real_hd.mp4",
        "ext": "mp4",
        "vcodec": None,
        "acodec": None,
        "video_ext": "mp4",
        "audio_ext": "none",
        "quality": -2,
    },
]


class RealFacebookFormatTest(unittest.TestCase):
    """Guards against the null-codec regression found via live extraction."""

    def _info(self):
        return {
            "id": "real1",
            "title": "How to Share",
            "webpage_url": "https://www.facebook.com/watch/?v=real1",
            "ext": "mp4",
            "thumbnail": "https://video.fb/thumb.jpg",
            "formats": FB_REAL_PROGRESSIVE_FORMATS,
        }

    def test_null_codecs_still_produce_video_streams(self):
        video, audio = build_streams(FB_REAL_PROGRESSIVE_FORMATS)
        self.assertEqual(len(video), 2)
        self.assertEqual(audio, [])

    def test_best_quality_first_via_numeric_quality(self):
        video, _ = build_streams(FB_REAL_PROGRESSIVE_FORMATS)
        # hd (quality -2) must rank above sd (quality -3) despite null heights.
        self.assertEqual(video[0]["quality"], "hd")
        self.assertEqual(video[0]["url"], "https://video.fb/real_hd.mp4")

    def test_quality_falls_back_to_format_id(self):
        video, _ = build_streams(FB_REAL_PROGRESSIVE_FORMATS)
        self.assertEqual({v["quality"] for v in video}, {"sd", "hd"})

    def test_legacy_media_video_url_not_empty(self):
        result = map_fb_response(self._info())
        item = result["media"][0]
        self.assertTrue(item["is_video"])
        # The downloader requires a usable video URL — this was the live regression.
        self.assertTrue(item["video_url"])
        self.assertEqual(result["videoStreams"][0]["url"], "https://video.fb/real_hd.mp4")


if __name__ == "__main__":
    unittest.main(verbosity=2)
