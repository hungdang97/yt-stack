# insta-downloader - Design

## Overview
Go service (Fiber v2) tương tự tik-downloader, xử lý download Instagram video/image/audio.
Port: 5003

## API Endpoints

### POST /api/download
```json
// Request
{
  "url": "https://www.instagram.com/reel/ABC123/",
  "type": "video" | "image" | "audio"
}
// Header: X-Hub-Token

// Response
{
  "statusUrl": "https://domain/insta/api/status/{id}?token=...&expires=...",
  "type": "video|image|audio",
  "title": "...",
  "thumbnail": "..."
}
```

### GET /api/status/:id
```json
{
  "status": "extracting|downloading|processing|completed|error",
  "progress": 0-100,
  "title": "...",
  "downloadUrl": "https://domain/insta/files/{id}/{filename}?token=...&expires=..."
}
```
> Status "processing" là bước mới (so với tik-downloader) — dùng khi FFmpeg đang extract audio.

### GET /files/:id/:filename
Serve file với signed URL validation.

### GET /health
Health check.

## Download Flow

### type = "video"
1. Gọi insta-extractor: GET http://insta-extractor:8000/extract?url=...&proxy=...
2. Lấy video_url từ media[0] (hoặc media đầu tiên có is_video=true)
3. Download video_url → output.mp4
4. Done

### type = "image"
1. Gọi insta-extractor
2. Lấy display_url từ media[0]
3. Download display_url → output.jpg
4. Done

### type = "audio"
1. Gọi insta-extractor
2. Lấy video_url (phải có video, nếu không → error)
3. Download video → temp.mp4
4. FFmpeg extract: `ffmpeg -i temp.mp4 -vn -acodec libmp3lame -q:a 2 output.mp3`
5. Xóa temp.mp4
6. Done

### type = "carousel" (bonus, optional)
1. Gọi insta-extractor → trả về nhiều media items
2. Download tất cả → output_1.jpg, output_2.mp4, ...
3. Trả về danh sách files

## Project Structure
```
insta-downloader/
├── main.go                 # Fiber server, routes, cleanup scheduler
├── config/
│   └── config.go           # Env vars: PORT, STORAGE_DIR, INSTA_EXTRACTOR_URL,
│                           # WARP_USER, WARP_PASS, SIGNED_URL_SECRET, etc.
├── handlers/
│   ├── download.go         # POST /api/download
│   ├── status.go           # GET /api/status/:id
│   ├── files.go            # GET /files/:id/:filename
│   └── health.go           # GET /health
├── models/
│   └── types.go            # InstaExtractResponse, JobMeta, DownloadRequest, etc.
├── services/
│   ├── extractor.go        # Gọi insta-extractor API
│   ├── downloader.go       # Download file qua WARP proxy
│   └── ffmpeg.go           # Extract audio từ video (ffmpeg)
├── utils/
│   ├── signed_url.go       # HMAC signed URL (copy từ tik-downloader)
│   ├── meta.go             # Job metadata read/write
│   ├── cleanup.go          # Cron cleanup old jobs
│   └── validation.go       # Parse Instagram URL → shortcode
├── Dockerfile              # Multi-stage, CẦN FFmpeg (khác tik-downloader)
├── go.mod
└── go.sum
```

## Khác biệt so với tik-downloader

| Feature | tik-downloader | insta-downloader |
|---------|---------------|-----------------|
| Extractor | POST tik-extractor:5555 | GET insta-extractor:8000/extract |
| Music URL | Có sẵn từ extractor | Không có → FFmpeg extract |
| Image download | Không hỗ trợ | Hỗ trợ |
| FFmpeg | Không cần | Cần (cho audio extract) |
| Cookie pool | MongoDB | Không cần (hoặc optional) |
| Proxy | Qua insta-extractor query param | Qua insta-extractor query param |
| Port | 5002 | 5003 |
| Nginx route | /tik/ | /insta/ |

## Docker Compose Addition
```yaml
insta-downloader:
  platform: linux/amd64
  build:
    context: ./insta-downloader
    dockerfile: Dockerfile
  image: insta-downloader:latest
  container_name: insta-downloader
  restart: always
  ports:
    - "5003:5003"
  environment:
    - PORT=5003
    - STORAGE_DIR=./storage
    - SIGNED_URL_SECRET=${SIGNED_URL_SECRET}
    - SIGNED_URL_EXPIRATION_MIN=${SIGNED_URL_EXPIRATION_MIN:-30}
    - BASE_DOMAIN=${BASE_DOMAIN}
    - DOWNLOAD_SUBDOMAIN=${DOWNLOAD_SUBDOMAIN}
    - PATH_PREFIX=/insta
    - INSTA_EXTRACTOR_URL=http://insta-extractor:8000
    - HUB_TOKEN=${HUB_TOKEN}
    - CLEANUP_INTERVAL=*/5 * * * *
    - MAX_JOB_AGE_MIN=15
    - WARP_USER=${WARP_USER}
    - WARP_PASS=${WARP_PASS}
  volumes:
    - insta_storage:/app/storage
  depends_on:
    insta-extractor:
      condition: service_healthy
  healthcheck:
    test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:5003/health"]
    interval: 30s
    timeout: 10s
    retries: 3
    start_period: 10s
  networks:
    - backend
```

## Nginx Addition (thêm route /insta/)
```nginx
location /insta/ {
    rewrite ^/insta/(.*) /$1 break;
    proxy_pass http://insta_downloader;
}
```

## Env vars cần thêm
Không cần thêm env mới — dùng chung SIGNED_URL_SECRET, WARP_USER/PASS, HUB_TOKEN từ .env hiện tại.
