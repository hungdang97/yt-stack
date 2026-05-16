# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**download-stack** is a scalable video download system with a Hub-VPS Agent architecture. It handles YouTube, TikTok, Instagram, Facebook, and X/Twitter content via multiple microservices deployed together with Docker Compose.

## Architecture (3 Tiers)

1. **Hub** (external) — Central config dashboard, manages all VPS servers
2. **VPS Agent** (`vps-agent/`, Go) — Runs on each server, auto-discovers IP, fetches config from Hub, generates `.env`, deploys the Docker stack, sends heartbeats. Port 9000.
3. **Download Services** — The actual worker containers on each VPS

### Service Dependency Chain

```
nginx (80/443) → yt-downloader (5001) → yt-extractor (8300)
               → tik-downloader (5002) → tik-extractor (5555)
               → insta-downloader (5003) → insta-extractor (8000)
               → fb-downloader (5004) → fb-extractor (8002)
               → x-downloader (5005) → x-extractor (8003)

gost (1111/2222) → warp-1..5 (SOCKS5 proxies, round-robin)

yt-downloader, tik-downloader, insta-downloader, fb-downloader & x-downloader use gost as HTTP proxy for downloads
```

### Services

| Service | Language | Framework | Port | Purpose |
|---------|----------|-----------|------|---------|
| **yt-downloader** | Go 1.24 | Fiber v2 | 5001 | YouTube download/stream, signed URLs, cleanup scheduler |
| **yt-extractor** | Python 3.11 | FastAPI | 8300 | YouTube metadata via yt-dlp + Deno runtime, cookie pool (MongoDB) |
| **tik-downloader** | Go 1.22 | Fiber v2 | 5002 | TikTok downloads, MongoDB for cookies |
| **tik-extractor** | Python 3.12 | FastAPI | 5555 | TikTok/Douyin metadata extraction (DouK-Downloader) |
| **insta-downloader** | Go 1.24 | Fiber v2 | 5003 | Instagram downloads, MongoDB cookie pool |
| **insta-extractor** | Python | FastAPI | 8000 | Instagram post/reel/profile extraction via instaloader |
| **fb-downloader** | Go 1.24 | Fiber v2 | 5004 | Facebook downloads, ffmpeg merge, signed URLs |
| **fb-extractor** | Python 3.11 | FastAPI | 8002 | Facebook video/reel metadata extraction via yt-dlp + impersonate |
| **x-downloader** | Go 1.24 | Fiber v2 | 5005 | X/Twitter downloads, ffmpeg merge, signed URLs |
| **x-extractor** | Python 3.11 | FastAPI | 8003 | X/Twitter media extraction via yt-dlp + impersonate |
| **edge-tts** | Python 3.11 | FastAPI | 8500 | TTS dubbing — sinh MP3/M4A từ utterances JSON qua Microsoft Edge TTS, ghép timeline bằng pydub + ffmpeg |
| **video-render** | Go 1.24 | stdlib | 8501 | Merge video + audio + .ass subtitle thành MP4 cuối (h264 + AAC, ASS burn-in qua libass) |
| **deepgram** | Go 1.24 | stdlib | 8502 | Speech-to-text — audio URL → utterances JSON qua Deepgram Nova-3 |
| **translate** | Go 1.24 | stdlib | 8503 | Subtitle translation — utterances JSON → dịch sang ngôn ngữ target qua OpenRouter |
| **upload** | Go 1.24 | stdlib | 8504 | File upload host — multipart POST → URL absolute, 10MB max, 1h TTL auto-cleanup |
| **vps-agent** | Go 1.24 | Fiber v2 | 9000 | Orchestration: config fetch, deploy, heartbeat |
| **nginx** | - | nginx:alpine | 80/443 | SSL termination (Let's Encrypt), rate limiting (30 req/s), routes `/` → yt, `/tik/` → tik, `/insta/` → insta, `/fb/` → fb, `/x/` → x |
| **gost** | - | ginuerzh/gost | 1111/2222 | Load-balanced WARP proxy pool (5 instances) + direct proxy |

## Hub API Reference

All client traffic enters at `https://hub.ytconvert.org`. The hub picks a healthy VPS (load-balanced), forwards the request, and either streams the response back or returns absolute VPS URLs the client can hit directly.

### Public — core download

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/health` | Hub + servers summary |
| `GET` | `/stats` | Aggregated stats |
| `GET` | `/api/info?url=…` | Metadata only (no download) |
| `POST` | `/api/download` | Quick download → returns proxy streaming URLs |
| `POST` | `/api/prepare` | Start background download to VPS disk → returns `status_url` + signed file URLs |

### Public — render pipeline

| Method | Path | Body / Query | Purpose |
|---|---|---|---|
| `POST` | `/api/upload` | multipart `file` | Host .ass / .srt / small file, returns absolute URL + 1h TTL |
| `GET` `POST` | `/api/deepgram/transcribe` | `?url=<audio>` or JSON `{"url": "…"}` | Audio → utterances JSON |
| `POST` | `/api/cleanup` | `{utterances[]}` | **(Experimental)** Clean ASR utterances qua LLM — merge câu cắt, fix lỗi, restore punctuation, split câu dài. Không trong main pipeline, dùng riêng để nghiên cứu. |
| `POST` | `/api/translate` | `{target, source?, utterances[]}` | Translate utterances → target lang |
| `GET` | `/api/tts/voices?locale=vi-VN` | — | List Edge TTS voices |
| `POST` | `/api/tts/submit` | `{voice, utterances[]}` | Synthesize dubbed audio → returns `server_id + status_url + download_url` (absolute) |
| `GET` | `/api/tts/status/:server_id/:job_id` | — | TTS job state |
| `GET` | `/api/tts/download/:server_id/:job_id` | — | Stream M4A |
| `POST` | `/api/render/submit` | `{video_url, subtitle_url, audio_url?}` | Merge → MP4 with subtitle burn-in. `audio_url` optional — bỏ trống ⇒ output silent (video + caption, không có tiếng) |
| `GET` | `/api/render/status/:server_id/:job_id` | — | Render progress (download → encoding) |
| `GET` | `/api/render/download/:server_id/:job_id` | — | Stream final MP4 |

### Public — VPS Agent registration

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/server-config/register` | VPS-agent registers itself |
| `GET` | `/api/server-config/:server_ip` | Fetch config to generate `.env` |
| `POST` | `/api/server-config/:server_ip/heartbeat` | Heartbeat + service health + system metrics |

### Admin — Basic Auth at `/admin/*`

| Method | Path | Purpose |
|---|---|---|
| `GET POST PUT DELETE` | `/admin/api/servers[/:id]` | CRUD servers |
| `POST` | `/admin/api/vps/:server_ip/restart/:service` | Pull + rebuild + restart 1 service on 1 VPS |
| `POST` | `/admin/api/restart-all/:service` | Same, across every enabled VPS |
| `GET` | `/admin/api/stats` | System stats |
| `GET POST DELETE` | `/admin/api/cloudflare-domains[/:domain]` | Manage DNS records |
| `GET` | `/admin/api/platform-stats` | Per-platform request counts |

---

## Sequence Diagrams

### Flow 1 — Pure download (1 lệnh ra file cuối)

Use case: client (app mobile, web) chọn link + chất lượng + format → backend tự
chọn stream, transcode/merge nếu cần, trả về **1 file MP4/M4A/MP3** sẵn sàng dùng.

```
Client                 Hub                       VPS-A (yt-downloader)
  │                     │                              │
  │ ① POST /api/download                               │
  │    { url:    "youtube.com/watch?v=…",              │
  │      os:     "windows",         // device          │
  │      license_key: "…",                             │
  │      output: { type:    "video",  // hoặc audio    │
  │                format:  "mp4",    // mp4/mp3/m4a/… │
  │                quality: "1080p" },                 │
  │      audio:   { language: "en" },     // optional  │
  │      trim:    { start: 10, end: 60 }, // optional  │
  │      enableMetadata: false }                       │
  ├────────────────────►│                              │
  │                     │ validate license             │
  │                     │ detectPlatform (yt/tik/fb/…) │
  │                     │ pick VPS (Weighted LC)       │
  │                     │ POST <vps>/api/download      │
  │                     ├─────────────────────────────►│
  │                     │                              │ extract metadata via …-extractor
  │                     │                              │ select best stream khớp quality + audio lang
  │                     │                              │ create job ID
  │                     │                              │ start bg download + transcode + merge
  │                     │ ◄─────────────────────────────┤
  │                     │ 200 { statusUrl (signed),    │
  │                     │       title, duration,       │
  │                     │       selectedQuality,       │
  │                     │       qualityChanged + lý do,│
  │                     │       audioLanguageChanged,  │
  │                     │       needsReencode }        │
  │ ◄───────────────────┤                              │
  │                                                    │
  │ ② poll statusUrl tới done (download + transcode)   │
  ├────────────────────────────────────────────────────►│
  │ ◄──────────────────── { status, progress, file_url } │
  │                                                    │
  │ ③ GET file_url (direct VPS — không qua hub)        │
  ├────────────────────────────────────────────────────►│ /files/<id>/<filename>.mp4
  │ ◄──────────────────── final file stream            │
```

Hub đụng VPS chỉ 1 lần (bước ①). Sau đó client gọi thẳng VPS qua signed URL, **bypass hub** để tiết kiệm bandwidth.

### Flow 2 — Prepare → Render video with caption (full dub pipeline)

Use case: tải video YouTube, thay tiếng dub bằng giọng Vietnamese, burn subtitle vào MP4 cuối.

```
Client            Hub                         VPS (load-balanced — có thể khác nhau mỗi step)
  │                │                               │
  │ ① POST /api/upload  multipart file=subtitle.ass│
  ├───────────────►│                               │
  │                │ pick VPS → POST <vps>/upload/ │
  │                │ ─────────────────────────────►│ upload service
  │                │                               │ save → /tmp/upload-files
  │                │ ◄─────────────────────────────│ { url, expires_at }
  │ ◄──────────────│ ass_url (absolute https URL)  │
  │                                                │
  │ ② POST /api/prepare {url: youtube.com/...}     │
  ├───────────────►│ ─────────────────────────────►│ yt-downloader
  │                │                               │ bg download video+audio
  │                │ ◄─────────────────────────────│ { status_url, video_url,
  │                │                               │   audio_url (signed) }
  │ ◄──────────────│                               │
  │                                                │
  │ ③ poll prepare status tới done                 │
  ├────────────────────────────────────────────────►│
  │ ◄────────────── done                           │
  │                                                │
  │ ④ POST /api/deepgram/transcribe ?url=audio_url │
  ├───────────────►│ ─────────────────────────────►│ deepgram service
  │                │                               │ fetch audio_url
  │                │                               │ Deepgram Nova-3 API
  │                │ ◄─────────────────────────────│ { language, duration,
  │                │                               │   utterances:[…] }
  │ ◄──────────────│ utterances (source lang)      │
  │                                                │
  │ ⑤ POST /api/translate {target:"vi", utterances}│
  ├───────────────►│ ─────────────────────────────►│ translate service
  │                │                               │ chunk + parallel
  │                │                               │ OpenRouter gpt-oss-20b
  │                │ ◄─────────────────────────────│ {target, utterances:[dịch]}
  │ ◄──────────────│ translated utterances         │
  │                                                │
  │ ⑥ POST /api/tts/submit {voice, utterances}     │
  ├───────────────►│ ─────────────────────────────►│ edge-tts service
  │                │                               │ MS Edge TTS WebSocket     │
  │                │                               │ measure-retry rate        │
  │                │                               │ trim silence + overlay    │
  │                │ ◄─────────────────────────────│ {server_id, status_url,
  │                │                               │  download_url (absolute)}
  │ ◄──────────────│                               │
  │                                                │
  │ ⑦ poll /api/tts/status/:server_id/:job_id      │
  ├───────────────►│ ─────────────────────────────►│
  │ ◄────────────── done, dubbed_audio_url         │
  │                                                │
  │ ⑧ POST /api/render/submit                      │
  │      { video_url:    ② video,                  │
  │        audio_url:    ⑦ dubbed_audio_url,       │
  │        subtitle_url: ① ass_url }               │
  ├───────────────►│ ─────────────────────────────►│ video-render service
  │                │                               │ download 3 URLs parallel
  │                │                               │ ffprobe duration
  │                │                               │ ffmpeg burn ASS + AAC
  │                │                               │ -progress pipe:1 → live %
  │                │ ◄─────────────────────────────│ {server_id, status_url,
  │                │                               │  download_url (absolute)}
  │ ◄──────────────│                               │
  │                                                │
  │ ⑨ poll /api/render/status/:server_id/:job_id   │
  ├───────────────►│ ─────────────────────────────►│
  │ ◄────────────── done, output_url (absolute)    │
  │                                                │
  │ ⑩ GET output_url (direct VPS, bypass hub)      │
  ├────────────────────────────────────────────────►│ /render/download/<id>
  │ ◄────────────── final MP4 (h264 + AAC + subs)  │
```

**4 biến thể của Flow 2** — chọn audio nào đi vào render là quyết định chính:

| Biến thể | Audio đi vào render | Subtitle | Khi nào dùng |
|---|---|---|---|
| **2a · Caption only** | `audio_url` từ ② (gốc) | `ass_url` từ ① | Giữ tiếng gốc, chỉ thêm phụ đề. **Skip ④⑤⑥⑦.** |
| **2b · Dub-only (edge-tts riêng)** | dubbed audio từ ⑥-⑦ | `ass_url` từ ① (hoặc bỏ) | Client đã có utterances (vd chép tay, từ DB) → gọi thẳng `/api/tts/submit`. **Skip ④⑤**, vào thẳng ⑥. |
| **2c · Full pipeline** | dubbed audio từ ⑥-⑦ | `ass_url` từ ① | Có audio tiếng gốc, muốn dịch + lồng tiếng + sub. **Chạy đủ ④→⑨.** |
| **2d · Custom audio** | URL audio mp3/m4a do client tự host | `ass_url` từ ① | Đã có file thu âm sẵn (vd voice actor) → upload lên `/api/upload` rồi đưa URL vào ⑧. **Skip ④⑤⑥⑦.** |

Tóm lại: `/api/tts/submit` là một **bước độc lập** — client có thể gọi nó với utterances bất kỳ (tự viết, từ ⑤ translate, từ ④ deepgram trực tiếp, từ DB ngoài, v.v.). Tương tự `/api/upload` cho custom audio. Render chỉ cần 3 URL cuối, không quan tâm chúng đến từ đâu.

---

## Common Commands

### Full Stack (Docker Compose)
```bash
docker-compose build                    # Build all services
docker-compose up -d                    # Start all services
docker-compose logs -f yt-downloader    # Follow logs for a service
docker-compose ps                       # Check service status
./clean_docker.sh                       # Clean up Docker resources
```

### yt-downloader (Go)
```bash
cd yt-downloader
make build          # Compile binary
make run            # Build + run
make dev            # Watch mode (air)
make build-prod     # Optimized production build
go test ./...       # Run all tests
```

### Go services structure (yt-downloader, tik-downloader, insta-downloader, fb-downloader, x-downloader, vps-agent)
```
config/     → Environment variable loading
handlers/   → HTTP route handlers
models/     → Data structures
services/   → Business logic
utils/      → Helpers (cleanup, signing, etc.)
```

### Python services (yt-extractor, tik-extractor, insta-extractor, fb-extractor, x-extractor)
- No Makefile; build/run via Docker
- yt-extractor: `requirements.txt`, Python 3.11
- tik-extractor: `pyproject.toml` (UV package manager), Python 3.12, uses `Dockerfile.api`
- insta-extractor: `requirements.txt`, uses instaloader
- fb-extractor: `requirements.txt`, uses yt-dlp + curl_cffi (impersonate Chrome)
- x-extractor: `requirements.txt`, uses yt-dlp + curl_cffi (impersonate Chrome)

## Environment Configuration

Config is auto-generated by VPS Agent from Hub. See `.env.example` for all ~43 variables. Key categories: core identity, proxy credentials, download tuning, cleanup schedule, security (signed URLs), feature flags, and customer tier config (JSON).

**Do not manually edit `.env` on VPS** — update via Hub Dashboard and restart.

## Key Architectural Details

- **Proxy layer**: 5 Cloudflare WARP containers provide unique IPs; Gost load-balances across them with round-robin, auto-failover (3 failures → 180s cooldown). Auth credentials are auto-generated.
- **yt-extractor** uses a thread pool (10 workers) with cookie pool rotation from MongoDB. Retry logic invalidates bad cookies automatically.
- **Signed URLs**: yt-downloader and tik-downloader use HMAC-signed URLs with configurable expiration for file serving.
- **Customer tiers**: Rate limiting is per-customer-tier (TIER_CONFIG JSON), not per-server.
- **Nginx routing**: `/` routes to yt-downloader, `/tik/` routes to tik-downloader, `/insta/` routes to insta-downloader, `/fb/` routes to fb-downloader, `/x/` routes to x-downloader, all with path stripping.
- **Hub platform detection**: `proxy.go` maps domain → path prefix (`instagram.com` → `/insta`, `tiktok.com` → `/tik`, `facebook.com` → `/fb`, `x.com`/`twitter.com` → `/x`, default → YouTube).
- **fb-extractor** requires `curl_cffi` for browser impersonation (Facebook blocks non-browser requests). Cookie is passed via env var `FB_DEFAULT_COOKIE`, not MongoDB.
- **x-extractor** uses `curl_cffi` for browser impersonation (same as fb-extractor). Cookie is passed via env var `X_DEFAULT_COOKIE` (requires `auth_token` + `ct0`).
