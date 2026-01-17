# YouTube Stack - Docker Deployment

Kiến trúc hoàn chỉnh cho YouTube downloader services với WARP proxy, SSL/HTTPS và auto-scaling.

## Kiến trúc hệ thống

```
┌─────────────────────────────────────────────────────────┐
│                    Internet (HTTPS)                      │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
              ┌─────────────────┐
              │  Nginx + SSL    │ (Port 80, 443)
              │   (Certbot)     │
              └────────┬────────┘
                       │
         ┌─────────────┴──────────────┐
         ▼                            ▼
┌──────────────────┐        ┌──────────────────┐
│  yt-downloader   │        │  yt-extractor    │
│   (Go:8080)      │        │  (Python:8300)   │
└────────┬─────────┘        └────────┬─────────┘
         │                           │
         └──────────┬────────────────┘
                    │ (Proxy via Gost)
                    ▼
         ┌─────────────────────┐
         │   Gost Proxy Layer  │
         │  Port 1111 (WARP)   │
         │  Port 2222 (Direct) │
         └──────────┬──────────┘
                    │
                    ▼
         ┌─────────────────────┐
         │  Cloudflare WARP    │
         │  (SOCKS5:40000)     │
         └─────────────────────┘
```

## Thành phần

### Services
- **yt-downloader**: Go service cho YouTube download (port 8080)
- **yt-extractor**: Python/FastAPI service cho metadata extraction (port 8300)
- **nginx**: Reverse proxy với SSL tự động (port 80, 443)
- **warp**: Cloudflare WARP SOCKS5 proxy (port 40000)
- **gost**: HTTP proxy gateway với authentication (port 1111, 2222)

### Features
- ✅ SSL/HTTPS tự động với Let's Encrypt (auto-renewal)
- ✅ WARP proxy cho ẩn danh IP
- ✅ Direct proxy với IP server gốc
- ✅ Health checks cho tất cả services
- ✅ Auto-restart on failure
- ✅ Rate limiting
- ✅ Compression (gzip)

## Cấu trúc thư mục

```
yt-stack/
├── .env                    # Environment variables
├── docker-compose.yml      # Main orchestration file
├── yt-downloader/
│   ├── Dockerfile
│   └── [source code]
├── yt-extractor/
│   ├── Dockerfile
│   └── [source code]
├── nginx/
│   ├── Dockerfile
│   ├── nginx.conf
│   └── docker-entrypoint.sh
├── proxy/
│   ├── gost.json
│   └── docker-compose.proxy.yml
└── scripts/
    ├── build.sh          # Build images
    ├── deploy.sh         # Deploy to VPS
    ├── setup-vps.sh      # Initial VPS setup
    ├── logs.sh           # View logs
    └── restart.sh        # Restart services
```

## Yêu cầu

### Local (Development)
- Docker 20.10+
- Docker Compose 1.29+

### VPS (Production)
- Ubuntu 20.04/22.04 hoặc Debian 11+
- RAM tối thiểu: 2GB
- Disk: 20GB
- Domain đã trỏ về IP server

## Setup nhanh

### 1. Setup VPS lần đầu

```bash
# SSH vào VPS
ssh root@your-vps-ip

# Clone repository
git clone YOUR_REPO_URL /opt/yt-stack
cd /opt/yt-stack

# Chạy setup script (cài Docker, Docker Compose, firewall)
sudo bash scripts/setup-vps.sh
```

### 2. Cấu hình môi trường

Tạo file `.env`:

```bash
DOMAIN=iloveyou-dl3.ytconvert.org
EMAIL=admin@ytconvert.org

YT_PORT=8080
BASE_URL=https://iloveyou-dl3.ytconvert.org

WARP_USER=wrap
WARP_PASS=1111

DIRECT_USER=server
DIRECT_PASS=2222
```

### 3. Deploy

```bash
# Build images (nếu build trên local)
./scripts/build.sh

# Deploy toàn bộ stack
./scripts/deploy.sh
```

**Chỉ cần 1 lệnh!** Script sẽ tự động:
- Build/pull images
- Start tất cả services
- Lấy SSL certificate (lần đầu)
- Configure nginx
- Health check

## Quy trình deploy tiếp theo

Sau lần setup đầu tiên, mỗi lần deploy chỉ cần:

```bash
# Trên local: Build images mới
./scripts/build.sh

# Trên VPS: Pull và restart
./scripts/deploy.sh
```

## Quản lý services

### Xem logs
```bash
# All services
./scripts/logs.sh

# Specific service
./scripts/logs.sh yt-downloader
./scripts/logs.sh nginx
```

### Restart services
```bash
# All services
./scripts/restart.sh

# Specific service
./scripts/restart.sh yt-extractor
```

### Stop toàn bộ stack
```bash
docker-compose down
```

### Xem trạng thái
```bash
docker-compose ps
```

## Testing

### Test SSL/HTTPS
```bash
curl https://iloveyou-dl3.ytconvert.org/health
```

### Test WARP proxy
```bash
# Qua WARP (IP ẩn danh)
curl -x http://wrap:1111@YOUR_SERVER_IP:1111 https://ifconfig.me

# Qua IP server gốc
curl -x http://server:2222@YOUR_SERVER_IP:2222 https://ifconfig.me
```

### Test services
```bash
# yt-downloader
curl https://iloveyou-dl3.ytconvert.org/api/download?url=VIDEO_URL

# yt-extractor
curl https://iloveyou-dl3.ytconvert.org/api/youtube/video/VIDEO_ID
```

## SSL Certificate Management

### Auto-renewal
Certificate tự động renew mỗi ngày lúc 2AM (cron job trong nginx container).

### Manual renewal
```bash
docker exec nginx certbot renew --nginx
docker exec nginx nginx -s reload
```

### Thêm domain mới
1. Update `.env`: thêm domain vào `DOMAIN`
2. Update `nginx/nginx.conf`: thêm vào `server_name`
3. Restart nginx: `./scripts/restart.sh nginx`

## Troubleshooting

### Service không start
```bash
# Check logs
./scripts/logs.sh SERVICE_NAME

# Check health
docker inspect --format='{{.State.Health.Status}}' CONTAINER_NAME
```

### SSL certificate lỗi
```bash
# Xóa certificate cũ
docker exec nginx rm -rf /etc/letsencrypt/live/YOUR_DOMAIN

# Restart để lấy certificate mới
./scripts/restart.sh nginx
```

### WARP không kết nối
```bash
# Check WARP status
docker exec warp warp-cli status

# Restart WARP
./scripts/restart.sh warp
```

## Performance tuning

### Tăng số worker cho yt-extractor
Edit `yt-extractor/Dockerfile`:
```dockerfile
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8300", "--workers", "8"]
```

### Tăng rate limit
Edit `nginx/nginx.conf`:
```nginx
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;
```

## Security Notes

- ✅ Services chạy với non-root user
- ✅ Firewall (UFW) chỉ mở port cần thiết
- ✅ Proxy có authentication
- ✅ SSL/TLS 1.2+ only
- ✅ Security headers enabled
- ⚠️  Đổi password proxy trong `.env` trước khi production

## Backup

### Backup volumes
```bash
docker run --rm \
  -v yt-stack_letsencrypt:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/letsencrypt-backup.tar.gz /data
```

### Restore volumes
```bash
docker run --rm \
  -v yt-stack_letsencrypt:/data \
  -v $(pwd):/backup \
  alpine tar xzf /backup/letsencrypt-backup.tar.gz -C /
```

## License

Private project

## Support

Để báo lỗi hoặc yêu cầu feature mới, tạo issue trên GitHub repository.
