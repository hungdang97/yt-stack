# YouTube Downloader API

Base URL: `https://{server}.ytconvert.org`

All requests require header: `X-Hub-Token: <token>`

---

## 1. Create Download Job

```
POST /api/download
```

**Request body:**

```json
{
  "url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
  "os": "windows",
  "output": {
    "type": "video",
    "format": "mp4",
    "quality": "1080p"
  },
  "audio": {
    "trackId": "en.vss_abc123",
    "bitrate": "192k"
  },
  "trim": {
    "start": 10,
    "end": 60,
    "accurate": false
  },
  "filenameStyle": "basic",
  "enableMetadata": false,
  "ctier": 1,
  "premium": false
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | YouTube URL |
| `os` | string | No | `windows` \| `macos` \| `linux` \| `android` \| `ios`. Default: `windows` |
| `output.type` | string | Yes | `video` \| `audio` |
| `output.format` | string | Yes | See formats below |
| `output.quality` | string | No | Video only. `2160p` \| `1440p` \| `1080p` \| `720p` \| `480p` \| `360p` \| `144p` |
| `audio.trackId` | string | No | Audio track ID (for multi-language videos) |
| `audio.bitrate` | string | No | `64k` \| `128k` \| `192k` \| `320k`. Default: `192k` |
| `trim.start` | number | No | Start time in seconds |
| `trim.end` | number | No | End time in seconds |
| `trim.accurate` | bool | No | `true` = frame-accurate (re-encodes), `false` = fast (keyframe-based) |
| `filenameStyle` | string | No | `classic` \| `basic` \| `pretty` \| `nerdy`. Default: `classic` |
| `enableMetadata` | bool | No | Embed title/artist/thumbnail into file |
| `ctier` | int | No | Customer tier (1 = Premium) |
| `premium` | bool | No | Use premium cookies |

**Supported formats:**

| Type | Formats |
|------|---------|
| Video | `mp4` `webm` `mkv` `avi` `flv` `mov` |
| Audio | `mp3` `m4a` `wav` `opus` `flac` `ogg` `aac` `alac` |

> **Trim not supported** for: `avi`, `flv`, `mov`, `aac`, `alac` (silently ignored)

**Response (200):**

```json
{
  "statusUrl": "https://.../api/status/{id}?token=xxx&expires=xxx",
  "title": "Video Title",
  "duration": 213.5,
  "qualityChanged": false,
  "selectedQuality": "1080p",
  "needsReencode": false,
  "availableAudioLanguages": ["en", "vi"],
  "audioLanguageChanged": false
}
```

| Field | Type | Description |
|-------|------|-------------|
| `statusUrl` | string | Signed URL to poll job status |
| `title` | string | Video title |
| `duration` | number | Duration in seconds |
| `qualityChanged` | bool | `true` if requested quality was unavailable |
| `qualityChangeReason` | string | Why quality changed (if applicable) |
| `selectedQuality` | string | Actual quality selected |
| `needsReencode` | bool | `true` if video codec needs re-encoding |
| `availableAudioLanguages` | string[] | Available audio languages |
| `audioLanguageChanged` | bool | `true` if requested audio track was unavailable |

---

## 2. Poll Job Status

```
GET /api/status/{id}?token=xxx&expires=xxx
```

Use `statusUrl` from step 1 directly.

**Response (200):**

```json
{
  "status": "completed",
  "progress": 100,
  "title": "Video Title",
  "duration": 213.5,
  "downloadUrl": "https://.../stream/{id}?token=xxx&expires=xxx"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `pending` \| `completed` \| `error` |
| `progress` | int | 0-100 |
| `title` | string | Video title |
| `duration` | number | Duration in seconds |
| `downloadUrl` | string | Available when `status=completed`. Open this URL to download the file. |
| `jobError` | string | Error message when `status=error` |

**Frontend flow:**
1. Call `POST /api/download` → get `statusUrl`
2. Poll `statusUrl` every 2-3 seconds
3. When `status=completed` → redirect user to `downloadUrl`
4. When `status=error` → show `jobError` to user

---

## 3. Download File

```
GET {downloadUrl}
```

Use `downloadUrl` from status response directly. Returns the file as binary stream with proper headers:

```
Content-Type: video/mp4 (or appropriate MIME type)
Content-Disposition: attachment; filename="Title (1080p).mp4"
```

> No auth header needed — token is embedded in the URL.

---

## 4. Delete Job

```
DELETE /api/jobs/{id}
```

**Response (200):**

```json
{ "deleted": true }
```

---

## Error Format

All errors return:

```json
{
  "error": "error_code",
  "message": "Human-readable message"
}
```

| HTTP | When |
|------|------|
| 400 | Invalid params, validation error |
| 401 | Missing token |
| 403 | Invalid/expired token |
| 404 | Job or stream not found |
| 500 | Server error |

---

## Filename Styles

| Style | Video | Audio |
|-------|-------|-------|
| `classic` | `youtube_dQw4w9WgXcQ_1080p.mp4` | `youtube_dQw4w9WgXcQ_audio.mp3` |
| `basic` | `Title - Author (1080p).mp4` | `Title - Author.mp3` |
| `pretty` | `Title - Author (1080p, youtube).mp4` | `Title - Author (youtube).mp3` |
| `nerdy` | `Title - Author (1080p, youtube, dQw4w9WgXcQ).mp4` | `Title - Author (youtube, dQw4w9WgXcQ).mp3` |
