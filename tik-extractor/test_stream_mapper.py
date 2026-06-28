"""
Unit tests for the TikTok stream mapper (src/stream_mapper.py).

Run from the tik-extractor root with:  python3 -m unittest -v

These tests use DouK-shaped ``data`` dicts so the mapper can be validated without
network access or the DouK / yt-dlp dependencies.
"""

import unittest

from src.stream_mapper import (
    build_audio_streams,
    build_video_streams,
    enrich_with_streams,
)


def _video_data():
    return {
        "id": "7123",
        "type": "video",
        "desc": "a clip",
        "downloads": "https://v.tiktokcdn.com/video_nowm.mp4?sig=abc",
        "music_url": "https://sf.tiktokcdn.com/song.mp3?sig=xyz",
        "music_title": "Original Sound",
        "height": 1024,
        "width": 576,
        "duration": "00:00:15",
    }


def _photo_data():
    return {
        "id": "7999",
        "type": "图集",
        "desc": "a slideshow",
        "downloads": [
            "https://p.tiktokcdn.com/img1.jpg",
            "https://p.tiktokcdn.com/img2.jpg",
        ],
        "music_url": "https://sf.tiktokcdn.com/song2.mp3",
        "height": 0,
        "width": 0,
    }


class BuildVideoStreamsTest(unittest.TestCase):
    def test_video_post_has_single_stream(self):
        streams = build_video_streams(_video_data())
        self.assertEqual(len(streams), 1)

    def test_video_stream_fields(self):
        stream = build_video_streams(_video_data())[0]
        self.assertEqual(stream["url"], "https://v.tiktokcdn.com/video_nowm.mp4?sig=abc")
        self.assertEqual(stream["quality"], "1024p")
        self.assertEqual(stream["format"], "MPEG-4")
        self.assertEqual(stream["mimeType"], "video/mp4")
        self.assertEqual(stream["width"], 576)
        self.assertEqual(stream["height"], 1024)
        self.assertFalse(stream["videoOnly"])

    def test_quality_falls_back_when_no_height(self):
        data = _video_data()
        data["height"] = 0
        stream = build_video_streams(data)[0]
        self.assertEqual(stream["quality"], "source")

    def test_photo_post_has_no_video_stream(self):
        self.assertEqual(build_video_streams(_photo_data()), [])

    def test_missing_downloads(self):
        self.assertEqual(build_video_streams({"music_url": "x"}), [])


class BuildAudioStreamsTest(unittest.TestCase):
    def test_music_yields_audio_stream(self):
        streams = build_audio_streams(_video_data())
        self.assertEqual(len(streams), 1)
        a = streams[0]
        self.assertEqual(a["url"], "https://sf.tiktokcdn.com/song.mp3?sig=xyz")
        self.assertEqual(a["format"], "MP3")
        self.assertEqual(a["mimeType"], "audio/mpeg")
        self.assertEqual(a["quality"], "Original Sound")

    def test_photo_post_still_has_music(self):
        streams = build_audio_streams(_photo_data())
        self.assertEqual(len(streams), 1)
        self.assertEqual(streams[0]["quality"], "music")  # no music_title

    def test_no_music_url(self):
        self.assertEqual(build_audio_streams({"downloads": "x.mp4"}), [])


class EnrichWithStreamsTest(unittest.TestCase):
    def test_adds_both_lists_to_video(self):
        data = enrich_with_streams(_video_data())
        self.assertEqual(len(data["videoStreams"]), 1)
        self.assertEqual(len(data["audioStreams"]), 1)

    def test_photo_has_audio_but_no_video(self):
        data = enrich_with_streams(_photo_data())
        self.assertEqual(data["videoStreams"], [])
        self.assertEqual(len(data["audioStreams"]), 1)

    def test_mutates_in_place_and_returns_same_object(self):
        data = _video_data()
        result = enrich_with_streams(data)
        self.assertIs(result, data)
        self.assertIn("videoStreams", data)

    def test_non_dict_input_returned_unchanged(self):
        self.assertIsNone(enrich_with_streams(None))
        self.assertEqual(enrich_with_streams([1, 2]), [1, 2])

    def test_preserves_existing_fields(self):
        data = enrich_with_streams(_video_data())
        # Original DouK fields remain intact for backward compatibility.
        self.assertEqual(data["id"], "7123")
        self.assertEqual(data["downloads"], "https://v.tiktokcdn.com/video_nowm.mp4?sig=abc")
        self.assertEqual(data["music_url"], "https://sf.tiktokcdn.com/song.mp3?sig=xyz")


if __name__ == "__main__":
    unittest.main(verbosity=2)
