# Hướng dẫn Deploy nhanh

## TL;DR - Deploy trong 3 bước

### Bước 1: Setup VPS lần đầu (chỉ 1 lần)
```bash
ssh root@YOUR_VPS_IP
git clone YOUR_REPO /opt/yt-stack
cd /opt/yt-stack
sudo bash scripts/setup-vps.sh
```

### Bước 2: Cấu hình .env
```bash
# Copy từ .env.example hoặc tạo mới
nano .env
```

Nội dung:
```env
DOMAIN=iloveyou-dl3.ytconvert.org
EMAIL=admin@ytconvert.org
YT_PORT=8080
BASE_URL=https://iloveyou-dl3.ytconvert.org
WARP_USER=wrap
WARP_PASS=1111
DIRECT_USER=server
DIRECT_PASS=2222
```

### Bước 3: Deploy
```bash
./scripts/deploy.sh
```

**XONG!** Services đã chạy tại: `https://iloveyou-dl3.ytconvert.org`

---

## Deploy lần sau (chỉ 1 lệnh)

```bash
./scripts/deploy.sh
```

---

## Kiểm tra nhanh

```bash
# 1. Xem trạng thái
docker-compose ps

# 2. Test health
curl https://iloveyou-dl3.ytconvert.org/health

# 3. Xem logs
./scripts/logs.sh
```

---

## Khắc phục sự cố nhanh

### Service lỗi?
```bash
./scripts/logs.sh SERVICE_NAME
./scripts/restart.sh SERVICE_NAME
```

### SSL không hoạt động?
```bash
# Kiểm tra domain đã trỏ về server chưa
nslookup iloveyou-dl3.ytconvert.org

# Xem logs nginx
./scripts/logs.sh nginx

# Force renew certificate
docker exec nginx certbot renew --force-renewal
./scripts/restart.sh nginx
```

### WARP không kết nối?
```bash
docker exec warp warp-cli status
./scripts/restart.sh warp
```

---

## Workflow làm việc hàng ngày

### Cập nhật code
```bash
# Pull code mới
git pull

# Rebuild và deploy
./scripts/deploy.sh
```

### Monitor
```bash
# Xem logs real-time
./scripts/logs.sh

# Xem logs của 1 service
./scripts/logs.sh yt-downloader
```

### Backup
```bash
# Backup SSL certificates
docker run --rm \
  -v yt-stack_letsencrypt:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/ssl-backup.tar.gz /data
```

---

## Tối ưu hiệu suất

### 1. Tăng worker cho Python service
Edit `docker-compose.yml`:
```yaml
yt-extractor:
  command: uvicorn app:app --host 0.0.0.0 --port 8300 --workers 8
```

### 2. Tăng rate limit
Edit `nginx/nginx.conf`:
```nginx
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;
```

### 3. Enable caching
Thêm vào nginx config:
```nginx
proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=api_cache:10m max_size=1g;
proxy_cache api_cache;
proxy_cache_valid 200 10m;
```

---

## Checklist Deploy Production

- [ ] Domain đã trỏ về IP server
- [ ] Firewall đã config (ports 22, 80, 443, 1111, 2222)
- [ ] .env đã set đúng giá trị
- [ ] Password proxy đã đổi khỏi default
- [ ] Email cho SSL đã đúng
- [ ] Services đều HEALTHY
- [ ] SSL certificate đã được issue
- [ ] HTTP redirect HTTPS hoạt động
- [ ] Proxy authentication hoạt động
- [ ] Health endpoints respond OK
- [ ] Logs không có error nghiêm trọng

---

## Liên hệ Support

Nếu gặp vấn đề, check theo thứ tự:
1. `./scripts/logs.sh` - Xem logs
2. `docker-compose ps` - Xem status
3. README.md - Xem troubleshooting section
4. Tạo issue trên GitHub
