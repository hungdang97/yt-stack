# Kiến trúc hệ thống YT Stack

## Tổng quan

Hệ thống được thiết kế theo kiểu microservices với Docker, bao gồm 5 services chính:

```
┌───────────────────────────────────────────────────────────────┐
│                          Internet                              │
└────────────────────────┬──────────────────────────────────────┘
                         │ HTTPS (443)
                         │ HTTP (80) → Redirect to HTTPS
                         ▼
                ┌─────────────────────┐
                │   Nginx + Certbot   │
                │  - Reverse Proxy    │
                │  - SSL Termination  │
                │  - Rate Limiting    │
                │  - Compression      │
                └──────────┬──────────┘
                           │
            ┌──────────────┴────────────────┐
            │                               │
            ▼                               ▼
   ┌─────────────────┐           ┌─────────────────┐
   │ yt-downloader   │           │  yt-extractor   │
   │   (Go:8080)     │           │ (Python:8300)   │
   │                 │           │                 │
   │ - Download API  │           │ - Extract meta  │
   │ - Stream video  │           │ - Cookie pool   │
   │ - File mgmt     │           │ - Async workers │
   └────────┬────────┘           └────────┬────────┘
            │                             │
            │  (Outbound HTTP via Proxy)  │
            └──────────────┬──────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │  Gost Proxy     │
                  │                 │
                  │ Port 1111 ──┐   │
                  │ Port 2222 ──┼───┼──> External clients
                  │             │   │
                  └─────────────┴───┘
                           │
                ┌──────────┴─────────┐
                │  (via SOCKS5)      │ (direct)
                ▼                    ▼
       ┌─────────────────┐      ┌──────────┐
       │ Cloudflare WARP │      │ Internet │
       │  (SOCKS5:40000) │      │ (Direct) │
       └─────────────────┘      └──────────┘
```

## Services chi tiết

### 1. Nginx (nginx)
**Image**: Custom build từ nginx:alpine
**Ports**: 80, 443
**Vai trò**:
- Reverse proxy cho backend services
- SSL/TLS termination với Let's Encrypt
- Auto-redirect HTTP → HTTPS
- Rate limiting (30 req/s default)
- Gzip compression
- Security headers

**Cấu hình chính**:
- `/` → yt-downloader:8080
- `/api/youtube/` → yt-extractor:8300
- `/health` → health check endpoint

**Volumes**:
- `letsencrypt:/etc/letsencrypt` - SSL certificates
- `certbot_webroot:/var/www/certbot` - ACME challenge

**Auto-renewal**: Cron job chạy daily lúc 2AM

---

### 2. yt-downloader (Go service)
**Image**: Custom build từ golang:1.24-alpine
**Port**: 8080 (internal)
**Language**: Go 1.24
**Framework**: Fiber v2

**Chức năng**:
- YouTube video download API
- Stream video trực tiếp
- File management
- Background cleanup jobs
- Swagger documentation

**Dependencies**:
- yt-dlp (installed trong image)
- ffmpeg (video processing)

**Storage**:
- `/app/storage` - Mounted volume cho downloaded files

**Health check**: `GET /health` mỗi 30s

---

### 3. yt-extractor (Python service)
**Image**: Custom build từ python:3.11-slim
**Port**: 8300 (internal)
**Language**: Python 3.11
**Framework**: FastAPI + Uvicorn

**Chức năng**:
- Extract YouTube video metadata
- Cookie pool management (5-10 pre-fetched cookies)
- Async processing với ThreadPoolExecutor
- MongoDB cookie database integration

**Dependencies**:
- yt-dlp + yt-dlp-ejs
- Deno runtime (JavaScript engine, installed in /app/bin/)
- FastAPI + Uvicorn (4 workers default)

**Optimization**:
- Cookie pooling để giảm DB queries
- Background refill mechanism
- Thread pool (10 workers) cho blocking operations

**Health check**: `GET /health` mỗi 30s

---

### 4. Cloudflare WARP (warp)
**Image**: caomingjun/warp:latest
**Port**: 40000 (SOCKS5, internal only)
**Vai trò**:
- Cung cấp SOCKS5 proxy qua Cloudflare WARP
- Ẩn IP server khi gọi API bên ngoài

**Requirements**:
- `CAP_ADD: NET_ADMIN` - Network management capability
- `sysctls` - IPv6 và routing config

**Health check**:
- Curl qua SOCKS5 tới cloudflare.com/cdn-cgi/trace
- Start period: 40s (WARP cần thời gian connect)

---

### 5. Gost Proxy (gost)
**Image**: ginuerzh/gost:latest
**Ports**:
- 1111 (HTTP proxy via WARP, public)
- 2222 (HTTP proxy direct, public)

**Vai trò**:
- HTTP proxy gateway với authentication
- Route 1111 → WARP (ẩn IP)
- Route 2222 → Direct (IP server gốc)

**Authentication**:
- Port 1111: `${WARP_USER}:${WARP_PASS}`
- Port 2222: `${DIRECT_USER}:${DIRECT_PASS}`

**Security**:
- `probeResist=code:400` - Chống port scanning

---

## Data Flow

### 1. User request → Download video
```
User (HTTPS)
  → Nginx:443
    → yt-downloader:8080
      → Gost:1111
        → WARP:40000
          → YouTube API
```

### 2. External client → Sử dụng proxy
```
Client
  → http://wrap:1111@SERVER_IP:1111
    → Gost
      → WARP
        → Target website (IP ẩn danh)

Client
  → http://server:2222@SERVER_IP:2222
    → Gost
      → Target website (IP server gốc)
```

### 3. Extract video metadata
```
User (HTTPS)
  → Nginx:443
    → yt-extractor:8300
      → Cookie Pool (pre-fetched)
        → yt-dlp (Deno runtime)
          → YouTube API
```

---

## Networking

### Docker Networks
- **backend**: Bridge network cho tất cả services
  - Cho phép inter-service communication
  - DNS resolution tự động (service name → IP)

### Port Mapping
| Service       | Internal Port | External Port | Access      |
|---------------|---------------|---------------|-------------|
| nginx         | 80, 443       | 80, 443       | Public      |
| yt-downloader | 8080          | -             | Internal    |
| yt-extractor  | 8300          | -             | Internal    |
| warp          | 40000         | -             | Internal    |
| gost          | 1111, 2222    | 1111, 2222    | Public      |

---

## Volumes

### Persistent Volumes
1. **warp_data**: Cloudflare WARP configuration
2. **yt_storage**: Downloaded video files
3. **letsencrypt**: SSL certificates
4. **certbot_webroot**: ACME challenge files
5. **nginx_logs**: Nginx access/error logs

### Volume Lifecycle
- Tự động tạo khi `docker-compose up`
- Persist qua container restarts
- Backup: Sử dụng `docker run` với volume mount

---

## Security

### 1. Network Security
- Firewall (UFW): Chỉ mở ports 22, 80, 443, 1111, 2222
- Services nội bộ không expose ra ngoài
- SSL/TLS 1.2+ only
- HSTS header enabled

### 2. Application Security
- Non-root user trong containers
- Proxy authentication required
- Rate limiting (30 req/s)
- Security headers (X-Frame-Options, CSP, etc.)

### 3. Certificate Management
- Auto-renewal với Certbot
- Let's Encrypt production CA
- 90-day certificate validity
- Daily renewal check

---

## Scalability

### Horizontal Scaling
**yt-extractor**: Có thể scale workers
```yaml
yt-extractor:
  command: uvicorn app:app --workers 8  # Tăng từ 4 lên 8
```

**yt-downloader**: Go service, có thể add replicas
```yaml
yt-downloader:
  deploy:
    replicas: 3
```

**Nginx**: Load balancing
```nginx
upstream yt_downloader {
    server yt-downloader-1:8080;
    server yt-downloader-2:8080;
    server yt-downloader-3:8080;
}
```

### Vertical Scaling
Tăng resource limits:
```yaml
services:
  yt-extractor:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
```

---

## Monitoring

### Health Checks
Tất cả services có health check:
- **Interval**: 30s
- **Timeout**: 10s
- **Retries**: 3

### Logs
```bash
# Real-time logs
docker-compose logs -f

# Service-specific
docker-compose logs -f yt-downloader

# Last 100 lines
docker-compose logs --tail=100
```

### Metrics
Có thể tích hợp:
- Prometheus (metrics collection)
- Grafana (visualization)
- Loki (log aggregation)

---

## Deployment Strategy

### Blue-Green Deployment
1. Build image mới với tag khác
2. Start stack mới
3. Switch traffic tại nginx
4. Stop stack cũ

### Rolling Update
```bash
docker-compose up -d --no-deps --build yt-downloader
```

### Zero-downtime
- Health checks đảm bảo service ready
- Nginx connection draining
- Graceful shutdown

---

## Troubleshooting Flow

```
Issue detected
    │
    ▼
Check service status
    │── docker-compose ps
    │
    ▼
Check logs
    │── docker-compose logs SERVICE
    │
    ▼
Check health
    │── docker inspect CONTAINER
    │
    ▼
Restart if needed
    │── docker-compose restart SERVICE
    │
    ▼
Verify fix
    └── curl https://DOMAIN/health
```

---

## Future Improvements

### Phase 2
- [ ] Redis caching layer
- [ ] MongoDB cho metadata storage
- [ ] CDN integration
- [ ] Prometheus + Grafana monitoring
- [ ] ELK Stack cho log aggregation

### Phase 3
- [ ] Kubernetes migration
- [ ] Auto-scaling based on CPU/Memory
- [ ] Multi-region deployment
- [ ] Message queue (RabbitMQ/Kafka)
- [ ] Object storage (S3/MinIO)

---

## References

- Docker Compose: https://docs.docker.com/compose/
- Nginx: https://nginx.org/en/docs/
- Cloudflare WARP: https://developers.cloudflare.com/warp-client/
- Gost: https://github.com/ginuerzh/gost
- Let's Encrypt: https://letsencrypt.org/docs/
