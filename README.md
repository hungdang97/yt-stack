# Download Stack

Hệ thống download video đa nền tảng với kiến trúc **Hub → VPS Agent → Docker Stack**. Hỗ trợ YouTube, TikTok, Instagram, Facebook, X/Twitter và 1800+ nguồn khác.

## Kiến trúc

```
Hub (:5000) ── quản lý config, load-balance, proxy API
  │
  └─ VPS Agent (:9000) ── mỗi server, auto-deploy Docker stack
       │
       └─ Docker Compose (19 containers)
            ├─ nginx (:80/443)         ← routing + SSL
            ├─ Downloaders (Go)        ← download/stream/merge
            ├─ Extractors (Python)     ← metadata extraction
            ├─ Render pipeline         ← TTS, subtitle, dubbing
            └─ Proxy layer             ← WARP + Gost
```

## Services

### Downloaders (Go / Fiber v2)

| Service | Port | Nginx route | Chức năng |
|---------|------|-------------|-----------|
| **yt-downloader** | 5001 | `/` | YouTube — stream, signed URLs, ffmpeg merge, cleanup scheduler |
| **tik-downloader** | 5002 | `/tik/` | TikTok — download video/audio, MongoDB cookie pool |
| **insta-downloader** | 5003 | `/insta/` | Instagram — post/reel/story download, MongoDB cookie pool |
| **fb-downloader** | 5004 | `/fb/` | Facebook — video/reel download, ffmpeg merge, signed URLs |
| **x-downloader** | 5005 | `/x/` | X/Twitter — video/audio download, ffmpeg merge, signed URLs |
| **uni-downloader** | 5006 | `/uni/` | Universal — 1800+ nguồn qua yt-dlp |

Mỗi downloader ghép cặp với 1 extractor tương ứng.

### Extractors (Python / FastAPI)

| Service | Port | Chức năng |
|---------|------|-----------|
| **yt-extractor** | 8300 | yt-dlp + Deno runtime, cookie pool từ MongoDB, thread pool 10 workers |
| **tik-extractor** | 5555 | DouK-Downloader, hỗ trợ TikTok + Douyin |
| **insta-extractor** | 8000 | instaloader, hỗ trợ post/reel/profile |
| **fb-extractor** | 8002 | yt-dlp + curl_cffi (impersonate Chrome), cookie qua env `FB_DEFAULT_COOKIE` |
| **x-extractor** | 8003 | yt-dlp + curl_cffi (impersonate Chrome), cookie qua env `X_DEFAULT_COOKIE` |
| **uni-extractor** | 8004 | yt-dlp generic, hỗ trợ 1800+ nguồn |

### Render Pipeline

5 service stateless, chain lại để tạo video dubbed + phụ đề:

```
prepare → deepgram → translate → edge-tts → render
```

| Service | Port | Lang | Chức năng |
|---------|------|------|-----------|
| **upload** | 8504 | Go | File hosting — multipart POST, 10MB max, 1h TTL auto-cleanup |
| **deepgram** | 8502 | Go | Speech-to-text — audio URL → utterances JSON (Deepgram Nova-3) |
| **translate** | 8503 | Go | Dịch utterances sang ngôn ngữ target (OpenRouter LLM) |
| **edge-tts** | 8500 | Python | TTS dubbing — utterances → MP3/M4A qua Microsoft Edge TTS |
| **video-render** | 8501 | Go | Merge video + audio + .ass subtitle → MP4 (h264, AAC, ASS burn-in) |

### Infrastructure

| Service | Port | Chức năng |
|---------|------|-----------|
| **nginx** | 80/443 | SSL (Let's Encrypt), rate limiting (30 req/s), path-based routing |
| **gost** | 1111/2222 | Load-balanced proxy — round-robin 5 WARP instances, auto-failover |
| **warp-1..5** | — | Cloudflare WARP SOCKS5 proxies, mỗi container 1 IP riêng |
| **vps-agent** | 9000 | Orchestration — fetch config từ Hub, generate `.env`, deploy stack, heartbeat |

## Cấu trúc code (Go services)

```
config/     → Load env vars
handlers/   → HTTP route handlers
services/   → Business logic
models/     → Data structures
utils/      → Helpers (cleanup, signing, ...)
```

## Commands

```bash
# Toàn bộ stack
docker-compose build                  # Build all
docker-compose up -d                  # Start all
docker-compose logs -f <service>      # Logs
docker-compose ps                     # Status

# Dev từng service Go
cd yt-downloader && make dev          # Watch mode (air)
cd yt-downloader && go test ./...     # Tests
```

## Config

Config được VPS Agent tự sinh từ Hub. **Không sửa `.env` thủ công** — sửa trên Hub Dashboard rồi restart.

Xem `.env.example` cho danh sách đầy đủ ~43 biến.

## Setup VPS mới

```bash
curl -sSL https://hub.ytconvert.org/install.sh | HUB_URL=https://hub.ytconvert.org bash
```

Tự động: cài Docker → download Agent → detect IP → register Hub → generate `.env` → deploy stack.
