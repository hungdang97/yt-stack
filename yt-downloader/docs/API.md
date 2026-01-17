# YT Downloader API

Base URL: `https://api.ytconvert.org`

---

## Response Format

### Success (HTTP 2xx)

```json
{
  "id": "abc123",
  "status": "completed",
  "progress": 100
}
```

### Error (HTTP 4xx/5xx)

```json
{
  "error": {
    "code": "JOB_NOT_FOUND",
    "message": "Job not found"
  }
}
```

---

## Error Codes

| Code | HTTP | Description |
|------|------|-------------|
| `INVALID_REQUEST` | 400 | Invalid request body |
| `VALIDATION_ERROR` | 400 | Validation failed |
| `INVALID_URL` | 400 | Invalid YouTube URL |
| `INVALID_JOB_ID` | 400 | Invalid job ID format |
| `JOB_NOT_READY` | 400 | Job not ready yet |
| `UNAUTHORIZED` | 401 | Missing token/expires |
| `FORBIDDEN` | 403 | Invalid or expired token |
| `JOB_NOT_FOUND` | 404 | Job not found |
| `VIDEO_NOT_FOUND` | 404 | No video stream available |
| `AUDIO_NOT_FOUND` | 404 | No audio stream available |
| `FILE_NOT_FOUND` | 404 | File not found |
| `INTERNAL_ERROR` | 500 | Server error |
| `EXTRACT_FAILED` | 500 | YouTube API error |

---

## Endpoints

---

### POST /api/download

Create download job.

#### Request

```json
{
  "url": "https://youtube.com/watch?v=xxx",
  "os": "windows",
  "output": {
    "type": "video",
    "format": "mp4",
    "quality": "1080p"
  },
  "audio": {
    "trackId": "en.xxx",
    "bitrate": "192k"
  },
  "trim": {
    "start": 10,
    "end": 60
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | YouTube URL |
| `os` | string | No | `ios`, `android`, `macos`, `windows`, `linux` |
| `output.type` | string | Yes | `video` or `audio` |
| `output.format` | string | Yes | `mp4`, `webm`, `mkv`, `mp3`, `m4a`, `wav`, `opus`, `flac` |
| `output.quality` | string | No | `2160p`, `1440p`, `1080p`, `720p`, `480p`, `360p` |
| `audio.trackId` | string | No | Audio track ID |
| `audio.bitrate` | string | No | `64k`, `128k`, `192k`, `320k` |
| `trim.start` | number | No | Start time (seconds) |
| `trim.end` | number | No | End time (seconds) |

#### Response

```json
{
  "statusUrl": "https://api.ytconvert.org/api/status/V1StGXR8_Z5jdHi?token=xxx&expires=xxx",
  "title": "Video Title",
  "duration": 213.5,
  "requestedQuality": "1080p",
  "selectedQuality": "720p",
  "qualityChanged": true,
  "qualityChangeReason": "1080p not available, using 720p"
}
```

#### Errors

```json
// 400 - Validation
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "URL is required"
  }
}

// 400 - Invalid URL
{
  "error": {
    "code": "INVALID_URL",
    "message": "Invalid YouTube URL format"
  }
}

// 404 - No stream
{
  "error": {
    "code": "VIDEO_NOT_FOUND",
    "message": "No compatible video stream found"
  }
}

// 500 - Extract failed
{
  "error": {
    "code": "EXTRACT_FAILED",
    "message": "Failed to fetch video metadata"
  }
}
```

---

### GET /api/status/:id

Check job status.

#### Query Parameters

| Param | Required | Description |
|-------|----------|-------------|
| `token` | Yes | Signed URL token |
| `expires` | Yes | Expiration timestamp |

#### Response

##### Pending

```json
{
  "status": "pending",
  "progress": 45,
  "title": "Video Title",
  "duration": 213.5
}
```

##### Completed

```json
{
  "status": "completed",
  "progress": 100,
  "title": "Video Title",
  "duration": 213.5,
  "downloadUrl": "https://api.ytconvert.org/files/xxx/output.mp4?token=xxx&expires=xxx"
}
```

##### Error

```json
{
  "status": "error",
  "progress": 45,
  "title": "Video Title",
  "duration": 213.5,
  "jobError": "Download failed: connection timeout"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `pending`, `completed`, `error` |
| `progress` | number | 0-100 |
| `title` | string | Video title |
| `duration` | number | Duration in seconds |
| `downloadUrl` | string | Download link (only when completed) |
| `jobError` | string | Error message (only when error) |

#### Errors

```json
// 400
{
  "error": {
    "code": "INVALID_JOB_ID",
    "message": "Invalid job ID format"
  }
}

// 401
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Missing token or expires parameter"
  }
}

// 403
{
  "error": {
    "code": "FORBIDDEN",
    "message": "Invalid or expired token"
  }
}

// 404
{
  "error": {
    "code": "JOB_NOT_FOUND",
    "message": "Job not found"
  }
}
```

---

### GET /files/:id/:filename

Download file.

#### Query Parameters

| Param | Required | Description |
|-------|----------|-------------|
| `token` | Yes | Signed URL token |
| `expires` | Yes | Expiration timestamp |

#### Response

Binary file with headers:

```
Content-Type: video/mp4
Content-Disposition: attachment; filename="output.mp4"
```

#### Errors

```json
// 401
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Missing token or expires parameter"
  }
}

// 403
{
  "error": {
    "code": "FORBIDDEN",
    "message": "Invalid or expired download link"
  }
}

// 404
{
  "error": {
    "code": "FILE_NOT_FOUND",
    "message": "File not found"
  }
}
```

---

### GET /stream/:id

Stream video/audio via FFmpeg.

#### Query Parameters

| Param | Required | Description |
|-------|----------|-------------|
| `token` | Yes | Signed URL token |
| `expires` | Yes | Expiration timestamp |

#### Response

Chunked binary stream:

```
Content-Type: video/mp4
Transfer-Encoding: chunked
```

#### Errors

```json
// 400
{
  "error": {
    "code": "JOB_NOT_READY",
    "message": "Job is not ready for streaming"
  }
}

// 403
{
  "error": {
    "code": "FORBIDDEN",
    "message": "Invalid or expired stream link"
  }
}

// 500
{
  "error": {
    "code": "INTERNAL_ERROR",
    "message": "Failed to start stream"
  }
}
```

---

### DELETE /api/jobs/:id

Delete job.

#### Response

```json
{
  "deleted": true
}
```

#### Errors

```json
// 404
{
  "error": {
    "code": "JOB_NOT_FOUND",
    "message": "Job not found"
  }
}
```

---

### GET /health

Health check.

#### Response

```json
{
  "status": "ok",
  "timestamp": 1705123456789
}
```

---

## Client Example

```javascript
async function download(url) {
  // 1. Create job
  const res = await fetch('/api/download', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      url,
      output: { type: 'video', format: 'mp4' }
    })
  });

  const data = await res.json();

  if (!res.ok) {
    throw new Error(data.error.message);
  }

  // 2. Poll status using statusUrl
  pollStatus(data.statusUrl);
}

async function pollStatus(statusUrl) {
  const res = await fetch(statusUrl);
  const data = await res.json();

  if (!res.ok) {
    throw new Error(data.error.message);
  }

  if (data.status === 'completed') {
    window.location.href = data.downloadUrl;
  } else if (data.status === 'error') {
    throw new Error(data.jobError);
  } else {
    // pending - continue polling
    setTimeout(() => pollStatus(statusUrl), 1000);
  }
}
```
