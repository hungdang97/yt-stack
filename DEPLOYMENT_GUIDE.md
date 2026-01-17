# YouTube Downloader Stack - Deployment Guide

Hướng dẫn deploy lên VPS Ubuntu từ A-Z trong 10 phút.

## 📋 Prerequisites

### 1. VPS Requirements
- **OS**: Ubuntu 25.04 hoặc Ubuntu 22.04/24.04 LTS
- **RAM**: Tối thiểu 2GB, khuyến nghị 4GB+
- **CPU**: 2 cores trở lên
- **Disk**: Tối thiểu 20GB
- **Network**: Public IP address

### 2. Domain & DNS
- Một domain hoặc subdomain (ví dụ: `yourdomain.com`)
- Truy cập DNS management (Cloudflare, NameCheap, etc.)

### 3. Local Machine
- Git installed
- SSH access tới VPS

---

## 🚀 Quick Start (3 Commands)

```bash
# 1. SSH vào VPS
ssh root@your-vps-ip

# 2. Chạy auto-setup script
curl -fsSL https://raw.githubusercontent.com/BlueByteVietNam/yt-stack/main/scripts/quick-install.sh | bash

# 3. Cấu hình domain và start
cd /opt/yt-stack
nano .env  # Sửa DOMAIN và EMAIL
docker compose up -d
```

---

## 📖 Detailed Setup Guide

### Step 1: Cấu hình DNS (5 phút)

1. Vào DNS provider (Cloudflare, NameCheap, etc.)
2. Thêm A Record:
   ```
   Type: A
   Name: your-subdomain (hoặc @ cho root domain)
   Value: YOUR_VPS_IP
   TTL: Auto hoặc 300
   ```

3. Verify DNS:
   ```bash
   # Trên máy local hoặc VPS
   dig your-domain.com +short
   # Phải trả về IP VPS
   ```

### Step 2: SSH vào VPS

```bash
ssh root@your-vps-ip
```

### Step 3: Install Docker & Docker Compose

```bash
# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Install Docker Compose Plugin
apt install -y docker-compose-plugin

# Verify installation
docker --version
docker compose version
```

### Step 4: Cấu hình Firewall

```bash
# Allow required ports
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw allow 1111/tcp  # WARP Proxy (optional - nếu muốn public)
ufw allow 2222/tcp  # Direct Proxy (optional - nếu muốn public)

# Enable firewall
ufw enable

# Verify
ufw status numbered
```

### Step 5: Clone Repository

```bash
# Clone vào /opt
cd /opt
git clone https://github.com/BlueByteVietNam/yt-stack.git
cd yt-stack
```

### Step 6: Cấu hình Environment Variables

```bash
# Copy example file
cp .env.example .env

# Edit với domain của bạn
nano .env
```

**Sửa các dòng sau:**
```env
DOMAIN=your-domain.com          # Domain của bạn
EMAIL=your-email@example.com    # Email cho Let's Encrypt

YT_PORT=5001
BASE_URL=https://your-domain.com

# Proxy credentials (có thể thay đổi)
WARP_USER=wrap
WARP_PASS=1111

DIRECT_USER=server
DIRECT_PASS=2222
```

Lưu file: `Ctrl+O`, `Enter`, `Ctrl+X`

### Step 7: Build & Start Services

```bash
# Build tất cả Docker images
docker compose build --no-cache

# Start toàn bộ stack
docker compose up -d

# Check logs
docker compose logs -f
```

### Step 8: Verify Deployment

#### Check Container Status
```bash
docker ps
```

Expected output:
```
CONTAINER ID   IMAGE                    STATUS
xxx            nginx-ssl:latest         Up (healthy)
xxx            yt-downloader:latest     Up (healthy)
xxx            yt-extractor:latest      Up (healthy)
xxx            gost:latest              Up
xxx            warp:latest              Up (healthy)
```

#### Test Health Endpoints
```bash
# Test HTTPS
curl https://your-domain.com/health
# Expected: {"status":"ok","timestamp":...}

# Check SSL certificate
curl -I https://your-domain.com
# Should show: HTTP/2 200
```

#### Test Download API
```bash
curl -X POST https://your-domain.com/api/download \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    "output": {"type": "audio", "format": "mp3"},
    "audio": {"bitrate": "192k"}
  }'
```

Expected response:
```json
{
  "statusUrl": "https://your-domain.com/api/status/...",
  "title": "Rick Astley - Never Gonna Give You Up",
  "duration": 212
}
```

---

## 🔧 Configuration Options

### Proxy Settings

**WARP Proxy** (port 1111):
- Sử dụng Cloudflare WARP network
- Credentials: `WARP_USER:WARP_PASS`
- URL: `http://wrap:1111@your-domain.com:1111`

**Direct Proxy** (port 2222):
- Direct connection, không qua WARP
- Credentials: `DIRECT_USER:DIRECT_PASS`
- URL: `http://server:2222@your-domain.com:2222`

### SSL Certificate

- **Auto-renewal**: Certbot tự động renew mỗi ngày lúc 2AM
- **Manual renewal**:
  ```bash
  docker compose exec nginx certbot renew
  docker compose exec nginx nginx -s reload
  ```

### Service Ports (Internal)

- `yt-downloader`: 5001 (not exposed, via nginx)
- `yt-extractor`: 8300 (not exposed, internal only)
- `warp`: 40000 (SOCKS5, internal only)
- `gost`: 1111, 2222 (HTTP proxies, exposed)
- `nginx`: 80, 443 (exposed)

---

## 🛠 Common Operations

### View Logs
```bash
# Tất cả services
docker compose logs -f

# Một service cụ thể
docker compose logs -f nginx
docker compose logs -f yt-downloader
docker compose logs -f yt-extractor
```

### Restart Services
```bash
# Restart tất cả
docker compose restart

# Restart một service
docker compose restart nginx
docker compose restart yt-downloader
```

### Update Code
```bash
cd /opt/yt-stack

# Pull latest code
git pull origin main

# Rebuild changed services
docker compose build --no-cache

# Restart
docker compose down
docker compose up -d
```

### Stop Services
```bash
# Stop tất cả (giữ volumes)
docker compose down

# Stop và xóa volumes
docker compose down -v
```

### Clean Up
```bash
# Remove unused images
docker image prune -a

# Remove unused volumes
docker volume prune

# Complete cleanup
docker system prune -a --volumes
```

---

## 🐛 Troubleshooting

### Issue 1: Container không start

**Check logs:**
```bash
docker compose logs <service-name>
```

**Common causes:**
- Port conflict → Check `ufw status` và `docker ps`
- Memory issue → Check `free -h`
- Permission issue → Check file ownership

### Issue 2: SSL Certificate Failed

**Check nginx logs:**
```bash
docker compose logs nginx | grep -i certbot
```

**Common causes:**
- DNS chưa trỏ đúng IP → `dig your-domain.com`
- Port 80 bị block → `ufw allow 80/tcp`
- Domain không đúng trong `.env`

**Manual fix:**
```bash
# Restart nginx để retry
docker compose restart nginx
```

### Issue 3: "Failed to fetch video metadata"

**Check yt-extractor:**
```bash
# Check logs
docker compose logs yt-extractor

# Test từ yt-downloader
docker compose exec yt-downloader wget -O- http://yt-extractor:8300/health
```

**Common causes:**
- Deno không được cài → Rebuild image
- Cookie expired → Check MongoDB connection
- Network issue → Check Docker network

### Issue 4: WARP Container Unhealthy

**Check WARP logs:**
```bash
docker compose logs warp
```

**Common causes:**
- Missing kernel modules → `modprobe tun`
- Insufficient permissions → Check `device_cgroup_rules`

**Temporary workaround:**
```bash
# Edit docker-compose.yml, thêm:
warp:
  privileged: true
```

### Issue 5: Nginx Crash Loop

**Check config:**
```bash
docker compose exec nginx nginx -t
```

**View detailed logs:**
```bash
docker compose logs nginx --tail 100
```

---

## 📊 Monitoring & Maintenance

### Check Resource Usage
```bash
# System resources
htop

# Docker stats
docker stats

# Disk usage
df -h
docker system df
```

### Backup Important Data
```bash
# Backup .env file
cp /opt/yt-stack/.env /root/env-backup-$(date +%Y%m%d).env

# Backup SSL certificates (if needed)
docker cp nginx:/etc/letsencrypt /root/letsencrypt-backup

# Backup storage
tar -czf /root/storage-backup-$(date +%Y%m%d).tar.gz /opt/yt-stack/storage
```

### Update yt-dlp
```bash
# Rebuild yt-extractor to get latest yt-dlp
docker compose build --no-cache yt-extractor
docker compose up -d yt-extractor
```

---

## 🔒 Security Best Practices

1. **Change Default Credentials**
   ```bash
   # Edit .env và đổi:
   WARP_USER=your-custom-user
   WARP_PASS=your-strong-password
   DIRECT_USER=your-custom-user
   DIRECT_PASS=your-strong-password
   ```

2. **Restrict Proxy Access**
   ```bash
   # Chỉ allow IP cụ thể
   ufw delete allow 1111/tcp
   ufw delete allow 2222/tcp
   ufw allow from YOUR_IP to any port 1111
   ufw allow from YOUR_IP to any port 2222
   ```

3. **Enable SSH Key Only**
   ```bash
   nano /etc/ssh/sshd_config
   # Set: PasswordAuthentication no
   systemctl restart sshd
   ```

4. **Regular Updates**
   ```bash
   # System updates
   apt update && apt upgrade -y

   # Docker images
   docker compose pull
   docker compose up -d
   ```

---

## 📝 Checklist

Sau khi deploy, verify các items sau:

- [ ] DNS A Record trỏ đúng IP
- [ ] `dig your-domain.com` trả về IP VPS
- [ ] `curl https://your-domain.com/health` return 200 OK
- [ ] SSL certificate valid (check browser)
- [ ] Tất cả containers healthy (`docker ps`)
- [ ] Download API test thành công
- [ ] Logs không có errors critical
- [ ] Firewall rules configured
- [ ] `.env` credentials đã thay đổi (nếu cần)

---

## 🆘 Support

- **GitHub Issues**: https://github.com/BlueByteVietNam/yt-stack/issues
- **Documentation**: `/opt/yt-stack/README.md`
- **Architecture**: `/opt/yt-stack/ARCHITECTURE.md`

---

## 📜 License

MIT License - xem file LICENSE để biết chi tiết.
