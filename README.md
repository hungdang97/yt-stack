# yt-stack — Multi-platform Video Downloader Stack

Stack tải video đa nền tảng (**YouTube, TikTok, Instagram, Facebook, X/Twitter, Universal**)
chạy bằng Docker, theo kiến trúc **Hub ⇄ VPS Agent**. Mỗi VPS chạy 6 cặp
downloader (Go) + extractor (Python), một pool **WARP + gost** (IP sạch Cloudflare),
và **nginx** (reverse proxy + SSL tự động).

## Kiến trúc
```
Client → Hub (/api/download) → chọn VPS → nginx(VPS) → downloader → extractor → gost → WARP → Internet
                                  ▲ register / heartbeat / control
                              vps-agent (mỗi VPS)
```
- **Hub** (repo `yt-downloader-hub`): load balancer + dashboard + registry, inject config cho agent.
- **vps-agent**: detect IP → đăng ký Hub → sinh `.env` → `docker compose up` toàn stack.

## Mô hình PORT (quan trọng)
Chỉ **2 cổng** mở ra ngoài trên VPS (+1 cho agent). Mọi service khác chỉ chạy
trong **docker network**, KHÔNG publish ra host → **không trùng cổng với service khác**.

| Cổng | Dùng cho | Public? |
|------|----------|---------|
| `80` / `443` | nginx (entry + SSL Let's Encrypt) | ✅ public |
| `9000` | vps-agent control API | chỉ mở cho IP Hub |
| 5001-5006, 8300, 5555, 8000, 8002, 8003, 8004, 1111, 2223 | downloader/extractor/gost | ❌ nội bộ docker, KHÔNG ra host |

→ Downloader gọi extractor/proxy qua tên service (`yt-extractor:8300`, `gost:1111`),
không qua cổng host. nginx route theo prefix: `/`→yt, `/tik`,`/insta`,`/fb`,`/x`,`/uni`.

## Yêu cầu VPS
- Ubuntu 20.04/22.04 hoặc Debian 11+, quyền root.
- RAM ≥ 8GB nếu chạy đủ 30 WARP (giảm WARP nếu VPS nhỏ).
- **Cổng 80/443 phải trống** (nginx chạy trong docker — đừng cài nginx host song song).
- DNS: `<subdomain>.<domain>` trỏ về IP VPS (Hub tự tạo nếu đã cấu hình Cloudflare).

## Cài đặt (1 lệnh, trên VPS)
```bash
curl -sSL https://<hub-domain>/downloads/install.sh | \
  HUB_URL=https://<hub-domain> \
  GIT_TOKEN=<github_token_đọc_repo> \
  bash
```
- `GIT_TOKEN`: token GitHub có quyền **đọc** repo này (vì repo private).
- Tùy chọn override: `GIT_REPO`, `GIT_BRANCH` (mặc định `main`), `PROJECT_DIR`.

Agent sẽ: cài Docker/Go → clone repo → đăng ký Hub → Hub trả config (đã inject
Mongo/HUB_TOKEN/cookie) → sinh `.env` → build & deploy.

## Cấu hình `.env`
Xem `.env.example` để biết toàn bộ biến. Trên VPS file `.env` **do agent tự sinh**
— đừng sửa tay (sẽ bị ghi đè khi deploy lại). Muốn đổi giá trị global
(Mongo, HUB_TOKEN, cookie, email) thì sửa **env của Hub** rồi để agent nhận lại.

Các biến chính: `BASE_DOMAIN`/`DOWNLOAD_SUBDOMAIN`, `WARP_*`, `SIGNED_URL_SECRET`,
`MONGO_URI`/`MONGO_DB` (cookie pool), `HUB_TOKEN`, `FB/X_DEFAULT_COOKIE`, `TIER_CONFIG`.

## Vận hành
```bash
cd /opt/yt-stack
docker compose ps
docker compose logs -f yt-downloader
journalctl -u vps-agent -f        # log agent
```
Restart/Rebuild/Update từ **Hub Dashboard** (gọi agent control API :9000).

## Chạy thử ít WARP (VPS nhỏ)
Dùng `docker-compose.3warp.yml` (3 WARP thay vì 30):
```bash
docker compose -f docker-compose.3warp.yml up -d --build
```

## Bảo mật
- Firewall: chỉ mở `80/443` công khai; `9000` chỉ cho IP Hub; chặn phần còn lại.
- Đổi `HUB_TOKEN`, `SIGNED_URL_SECRET`, mật khẩu Mongo khỏi giá trị mặc định.
- `.env` chứa secret → đã `.gitignore`, không commit.
