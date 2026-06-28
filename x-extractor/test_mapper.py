"""
Unit tests for x-extractor mapper.

Run with:  python3 -m unittest -v

These tests use realistic yt-dlp ``info`` samples for X/Twitter content so the
mapper can be validated without any network access or the yt_dlp dependency.
"""

import unittest

from mapper import build_streams, map_x_response


# ============================================
# SAMPLE yt-dlp FORMATS (shaped like real X/Twitter output)
# ============================================

# X/Twitter typically serves HLS renditions at several resolutions plus a
# combined http-mp4 progressive variant. Codecs are usually muxed (video+audio).
X_FORMATS = [
    {
        "format_id": "hls-256",
        "url": "https://video.twimg.com/hls_320x180.m3u8",
        "ext": "mp4",
        "vcodec": "avc1.42001e",
        "acodec": "mp4a.40.2",
        "width": 320,
        "height": 180,
        "tbr": 256.0,
        "format_note": "180p",
    },
    {
        "format_id": "hls-832",
        "url": "https://video.twimg.com/hls_640x360.m3u8",
        "ext": "mp4",
        "vcodec": "avc1.4d001f",
        "acodec": "mp4a.40.2",
        "width": 640,
        "height": 360,
        "tbr": 832.0,
        "format_note": "360p",
    },
    {
        "format_id": "hls-2176",
        "url": "https://video.twimg.com/hls_1280x720.m3u8",
        "ext": "mp4",
        "vcodec": "avc1.640020",
        "acodec": "mp4a.40.2",
        "width": 1280,
        "height": 720,
        "tbr": 2176.0,
        "format_note": "720p",
    },
    {
        "format_id": "http-832",
        "url": "https://video.twimg.com/vid/640x360.mp4",
        "ext": "mp4",
        "vcodec": "avc1.4d001f",
        "acodec": "mp4a.40.2",
        "width": 640,
        "height": 360,
        "tbr": 832.0,
        "filesize": 2_100_000,
    },
    # Storyboard-like entry that must be skipped.
    {
        "format_id": "sb0",
        "url": "https://video.twimg.com/sb.jpg",
        "ext": "mhtml",
        "vcodec": "none",
        "acodec": "none",
    },
]


def _tweet_info():
    return {
        "id": "1789",
        "title": "Big news",
        "description": "Launch day! #ship #build @teammate",
        "webpage_url": "https://x.com/acme/status/1789",
        "uploader": "Acme",
        "uploader_id": "acme",
        "uploader_url": "https://x.com/acme",
        "duration": 30.0,
        "view_count": 9999,
        "ext": "mp4",
        "thumbnail": "https://pbs.twimg.com/thumb.jpg",
        "formats": X_FORMATS,
        "subtitles": {},
        "automatic_captions": {},
    }


class BuildStreamsTest(unittest.TestCase):
    def test_all_progressive_no_audio_only(self):
        video, audio = build_streams(X_FORMATS)
        # 4 muxed video formats; storyboard skipped; no audio-only streams.
        self.assertEqual(len(video), 4)
        self.assertEqual(audio, [])

    def test_every_video_is_muxed(self):
        video, _ = build_streams(X_FORMATS)
        self.assertTrue(all(v["videoOnly"] is False for v in video))

    def test_video_sorted_best_first(self):
        video, _ = build_streams(X_FORMATS)
        heights = [v["height"] for v in video]
        self.assertEqual(heights, sorted(heights, reverse=True))
        self.assertEqual(video[0]["height"], 720)

    def test_video_stream_fields(self):
        video, _ = build_streams(X_FORMATS)
        top = video[0]
        self.assertEqual(top["url"], "https://video.twimg.com/hls_1280x720.m3u8")
        self.assertEqual(top["quality"], "720p")
        self.assertEqual(top["format"], "MPEG-4")
        self.assertEqual(top["mimeType"], "video/mp4")
        self.assertEqual(top["bitrate"], 2176.0)  # tbr (no vbr)
        self.assertEqual(top["codec"], "avc1.640020, mp4a.40.2")
        self.assertEqual(top["width"], 1280)

    def test_filesize_carried(self):
        video, _ = build_streams(X_FORMATS)
        http = next(v for v in video if v["url"].endswith("640x360.mp4"))
        self.assertEqual(http["fileSize"], 2_100_000)

    def test_empty_formats(self):
        video, audio = build_streams([])
        self.assertEqual(video, [])
        self.assertEqual(audio, [])


class MapXResponseTest(unittest.TestCase):
    def test_adds_stream_lists(self):
        result = map_x_response(_tweet_info())
        self.assertEqual(len(result["videoStreams"]), 4)
        self.assertEqual(result["audioStreams"], [])

    def test_typename_always_tweet(self):
        result = map_x_response(_tweet_info())
        self.assertEqual(result["typename"], "Tweet")

    def test_preserves_legacy_media_shape(self):
        result = map_x_response(_tweet_info())
        self.assertEqual(result["media_count"], 1)
        item = result["media"][0]
        self.assertTrue(item["is_video"])
        # best video: 720p has the highest height -> wins
        self.assertEqual(item["video_url"], "https://video.twimg.com/hls_1280x720.m3u8")
        # best progressive (muxed) also resolves to 720p
        self.assertEqual(item["video_progressive_url"], "https://video.twimg.com/hls_1280x720.m3u8")
        # no audio-only stream on X
        self.assertIsNone(item["audio_url"])
        self.assertEqual(item["display_url"], "https://pbs.twimg.com/thumb.jpg")

    def test_metadata_fields(self):
        result = map_x_response(_tweet_info())
        self.assertEqual(result["id"], "1789")
        self.assertEqual(result["title"], "Big news")
        self.assertEqual(result["hashtags"], ["ship", "build"])
        self.assertEqual(result["mentions"], ["teammate"])
        self.assertEqual(result["owner_username"], "Acme")
        self.assertTrue(result["is_video"])
        self.assertEqual(result["video_duration"], 30.0)
        self.assertEqual(result["video_view_count"], 9999)
        self.assertEqual(result["extractor"], "yt-dlp")

    def test_image_only_tweet_has_no_streams(self):
        info = {
            "id": "img9",
            "title": "A photo tweet",
            "webpage_url": "https://x.com/acme/status/img9",
            "ext": "jpg",
            "url": "https://pbs.twimg.com/media/photo.jpg",
            "thumbnail": "https://pbs.twimg.com/media/thumb.jpg",
        }
        result = map_x_response(info)
        self.assertEqual(result["videoStreams"], [])
        self.assertEqual(result["audioStreams"], [])
        self.assertFalse(result["is_video"])
        self.assertFalse(result["media"][0]["is_video"])
        self.assertEqual(result["media"][0]["display_url"], "https://pbs.twimg.com/media/photo.jpg")
        self.assertIsNone(result["video_view_count"])

    def test_multi_media_tweet_uses_primary_video_entry(self):
        info = {
            "id": "thread1",
            "title": "Mixed media",
            "webpage_url": "https://x.com/acme/status/thread1",
            "entries": [
                {"id": "p1", "ext": "jpg", "url": "https://pbs.twimg.com/1.jpg"},
                {"id": "v1", "ext": "mp4", "thumbnail": "https://pbs.twimg.com/t.jpg",
                 "formats": X_FORMATS},
            ],
        }
        result = map_x_response(info)
        self.assertEqual(len(result["videoStreams"]), 4)
        self.assertEqual(result["media_count"], 2)
        self.assertTrue(result["is_video"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
