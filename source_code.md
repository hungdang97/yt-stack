# Source Code Summary

## Directory Structure
```
./
  .DS_Store
  Makefile
  README.md
  test.html
  .dockerignore
  .gitignore
  .env
  docker-compose.yml
  .env.example
  proxy/
    docker-compose.proxy.yml
    gost.json
  yt-downloader/
    go.mod
    Dockerfile
    Makefile
    go.sum
    .dockerignore
    .gitignore
    yt-downloader-go
    main.go
    config/
      config.go
    .claude/
      settings.local.json
    utils/
      cleanup.go
      response.go
      signed_url.go
      validation.go
      meta.go
      filename.go
    models/
      types.go
    docs/
      API.md
      swagger.yaml
      docs.go
      swagger.json
    storage/
    handlers/
      files.go
      stream.go
      download.go
      health.go
      jobs.go
      status.go
    services/
      downloader.go
      extractor.go
      ffmpeg.go
  .claude/
    settings.local.json
  nginx/
    nginx-http-only.conf
    Dockerfile
    docker-entrypoint.sh
    nginx.conf
  scripts/
    build-and-start.sh
    quick-install.sh
  .vscode/
    settings.json
  yt-extractor/
    requirements.txt
    Dockerfile
    mapper.py
    app.py
    cookie_db.py
    bin/
```


## File Contents


### test.html

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>YT Downloader Test</title>
  <style>
    * { box-sizing: border-box; font-family: system-ui, -apple-system, sans-serif; }
    body { max-width: 640px; margin: 40px auto; padding: 20px; background: #f8fafc; color: #1e293b; }
    h1 { font-size: 1.5rem; margin-bottom: 24px; color: #0f172a; }
    .form-group { margin-bottom: 16px; }
    label { display: block; margin-bottom: 6px; font-weight: 500; font-size: 0.9rem; color: #475569; }
    input, select { width: 100%; padding: 10px 12px; border: 1px solid #cbd5e1; border-radius: 6px; font-size: 1rem; background: #fff; }
    input:focus, select:focus { outline: none; border-color: #3b82f6; box-shadow: 0 0 0 3px rgba(59,130,246,0.1); }
    .row { display: flex; gap: 12px; }
    .row > * { flex: 1; }
    button { width: 100%; padding: 12px; background: #3b82f6; color: white; border: none; border-radius: 6px; cursor: pointer; font-size: 1rem; font-weight: 500; transition: background 0.2s; }
    button:hover { background: #2563eb; }
    button:disabled { background: #94a3b8; cursor: not-allowed; }

    #status { margin-top: 24px; padding: 16px; background: #fff; border-radius: 8px; border: 1px solid #e2e8f0; display: none; }
    #status.show { display: block; }

    .status-header { font-weight: 600; margin-bottom: 8px; }
    .status-info { font-size: 0.9rem; color: #64748b; margin-bottom: 12px; }

    .progress-container { margin: 16px 0; }
    .progress-label { display: flex; justify-content: space-between; font-size: 0.85rem; color: #64748b; margin-bottom: 4px; }
    .progress { height: 8px; background: #e2e8f0; border-radius: 4px; overflow: hidden; }
    .progress-bar { height: 100%; background: #3b82f6; transition: width 0.3s ease; border-radius: 4px; }
    .progress-bar.done { background: #22c55e; }

    .detail-progress { margin-top: 12px; display: flex; gap: 16px; font-size: 0.85rem; }
    .detail-item { display: flex; align-items: center; gap: 6px; color: #64748b; }
    .detail-bar { width: 60px; height: 4px; background: #e2e8f0; border-radius: 2px; overflow: hidden; }
    .detail-bar-fill { height: 100%; background: #3b82f6; transition: width 0.3s; }

    .error { color: #dc2626; }
    .success { color: #16a34a; }
    .warning { color: #d97706; background: #fef3c7; padding: 8px 12px; border-radius: 6px; font-size: 0.85rem; margin-top: 8px; }

    .download-link { margin-top: 16px; }
    .download-link a { display: inline-flex; align-items: center; gap: 8px; background: #22c55e; color: white; padding: 10px 20px; border-radius: 6px; text-decoration: none; font-weight: 500; }
    .download-link a:hover { background: #16a34a; }

    #trimFields { display: none; margin-top: 8px; }
    #trimFields.show { display: block; }
    #qualityGroup { display: none; }
    #qualityGroup.show { display: block; }

    .checkbox-label { display: flex; align-items: center; gap: 8px; cursor: pointer; }
    .checkbox-label input { width: auto; }

    .info-box { background: #eff6ff; border: 1px solid #bfdbfe; padding: 10px 12px; border-radius: 6px; margin-bottom: 16px; font-size: 0.85rem; color: #1e40af; }
  </style>
</head>
<body>
  <h1>YT Downloader Test - Updated</h1>

  <div class="form-group">
    <label>YouTube URL</label>
    <input type="text" id="url" placeholder="https://www.youtube.com/watch?v=...">
  </div>

  <div class="row">
    <div class="form-group">
      <label>Platform</label>
      <select id="platform">
        <option value="">Auto</option>
        <option value="ios">iOS</option>
        <option value="android">Android</option>
        <option value="macos">macOS</option>
        <option value="windows">Windows</option>
        <option value="linux">Linux</option>
      </select>
    </div>
    <div class="form-group">
      <label>Type</label>
      <select id="type">
        <option value="video">Video</option>
        <option value="audio">Audio</option>
      </select>
    </div>
  </div>

  <div id="platformInfo" class="info-box" style="display: none;"></div>

  <div class="row">
    <div class="form-group">
      <label>Format</label>
      <select id="format"></select>
    </div>
    <div class="form-group" id="qualityGroup">
      <label>Quality</label>
      <select id="quality">
        <option value="">Auto (Best)</option>
        <option value="2160p">4K (2160p)</option>
        <option value="1440p">1440p</option>
        <option value="1080p">1080p</option>
        <option value="720p">720p</option>
        <option value="480p">480p</option>
        <option value="360p">360p</option>
      </select>
    </div>
  </div>

  <div class="form-group">
    <label>Audio Bitrate</label>
    <select id="bitrate">
      <option value="192k">192 kbps</option>
      <option value="128k">128 kbps</option>
      <option value="320k">320 kbps</option>
      <option value="64k">64 kbps</option>
    </select>
  </div>

  <div class="form-group">
    <label class="checkbox-label">
      <input type="checkbox" id="enableTrim">
      <span>Enable Trim</span>
    </label>
  </div>

  <div id="trimFields">
    <div class="row">
      <div class="form-group">
        <label>Start (seconds)</label>
        <input type="number" id="trimStart" value="0" min="0">
      </div>
      <div class="form-group">
        <label>End (seconds)</label>
        <input type="number" id="trimEnd" value="60" min="1">
      </div>
    </div>
  </div>

  <button id="downloadBtn">Download</button>

  <div id="status">
    <div class="status-header" id="statusTitle"></div>
    <div class="status-info" id="statusInfo"></div>

    <div class="progress-container">
      <div class="progress-label">
        <span id="statusPhase">Downloading...</span>
        <span id="statusPercent">0%</span>
      </div>
      <div class="progress">
        <div class="progress-bar" id="progressBar"></div>
      </div>
    </div>

    <div class="detail-progress" id="detailProgress" style="display: none;">
      <div class="detail-item">
        <span>Video:</span>
        <div class="detail-bar"><div class="detail-bar-fill" id="videoProgress"></div></div>
        <span id="videoPercent">0d%</span>
      </div>
      <div class="detail-item">
        <span>Audio:</span>
        <div class="detail-bar"><div class="detail-bar-fill" id="audioProgress"></div></div>
        <span id="audioPercent">0%</span>
      </div>
    </div>

    <div id="warningBox" class="warning" style="display: none;"></div>
    <div id="errorBox" class="error" style="display: none;"></div>
    <div id="downloadLink" class="download-link"></div>
  </div>

  <script>
    const API = 'https://vps-69764722.ytconvert.org';
    let currentStatusUrl = null;
    let pollInterval = null;

    const platformInfo = {
      ios: 'iOS: Max 1080p, H.264 codec, AAC audio',
      android: 'Android: Up to 4K, AV1/VP9/H.264, Opus/AAC',
      macos: 'macOS: Max 1080p, H.264 codec, AAC audio',
      windows: 'Windows: Up to 4K, AV1/VP9/H.264, Opus/AAC',
      linux: 'Linux: Up to 4K, AV1/VP9/H.264, Opus/AAC'
    };

    const videoFormats = ['mp4', 'webm', 'mkv'];
    const audioFormats = ['mp3', 'm4a', 'wav', 'opus', 'flac', 'ogg'];

    // Elements 
    const els = {
      url: document.getElementById('url'),
      platform: document.getElementById('platform'),
      platformInfo: document.getElementById('platformInfo'),
      type: document.getElementById('type'),
      format: document.getElementById('format'),
      quality: document.getElementById('quality'),
      qualityGroup: document.getElementById('qualityGroup'),
      bitrate: document.getElementById('bitrate'),
      enableTrim: document.getElementById('enableTrim'),
      trimFields: document.getElementById('trimFields'),
      trimStart: document.getElementById('trimStart'),
      trimEnd: document.getElementById('trimEnd'),
      downloadBtn: document.getElementById('downloadBtn'),
      status: document.getElementById('status'),
      statusTitle: document.getElementById('statusTitle'),
      statusInfo: document.getElementById('statusInfo'),
      statusPhase: document.getElementById('statusPhase'),
      statusPercent: document.getElementById('statusPercent'),
      progressBar: document.getElementById('progressBar'),
      detailProgress: document.getElementById('detailProgress'),
      videoProgress: document.getElementById('videoProgress'),
      audioProgress: document.getElementById('audioProgress'),
      videoPercent: document.getElementById('videoPercent'),
      audioPercent: document.getElementById('audioPercent'),
      warningBox: document.getElementById('warningBox'),
      errorBox: document.getElementById('errorBox'),
      downloadLink: document.getElementById('downloadLink')
    };

    // Platform change
    els.platform.addEventListener('change', () => {
      const p = els.platform.value;
      if (p && platformInfo[p]) {
        els.platformInfo.textContent = platformInfo[p];
        els.platformInfo.style.display = 'block';
      } else {
        els.platformInfo.style.display = 'none';
      }
    });

    // Type change
    els.type.addEventListener('change', () => {
      const isVideo = els.type.value === 'video';
      els.format.innerHTML = '';
      (isVideo ? videoFormats : audioFormats).forEach(f => {
        els.format.innerHTML += `<option value="${f}">${f.toUpperCase()}</option>`;
      });
      els.qualityGroup.classList.toggle('show', isVideo);
    });

    // Trim toggle
    els.enableTrim.addEventListener('change', () => {
      els.trimFields.classList.toggle('show', els.enableTrim.checked);
    });

    // Init
    els.type.dispatchEvent(new Event('change'));

    // Format duration
    function formatDuration(sec) {
      const m = Math.floor(sec / 60);
      const s = Math.floor(sec % 60);
      return `${m}:${s.toString().padStart(2, '0')}`;
    }

    // Get error message from response
    function getErrorMessage(data) {
      if (data.error && data.error.message) {
        return data.error.message;
      }
      return 'Unknown error';
    }

    // Reset status UI
    function resetStatus() {
      els.progressBar.style.width = '0%';
      els.progressBar.classList.remove('done');
      els.statusPhase.textContent = 'Starting...';
      els.statusPercent.textContent = '0%';
      els.detailProgress.style.display = 'none';
      els.videoProgress.style.width = '0%';
      els.audioProgress.style.width = '0%';
      els.videoPercent.textContent = '0%';
      els.audioPercent.textContent = '0%';
      els.warningBox.style.display = 'none';
      els.errorBox.style.display = 'none';
      els.downloadLink.innerHTML = '';
    }

    // Download click
    els.downloadBtn.addEventListener('click', async () => {
      const url = els.url.value.trim();
      if (!url) return alert('Please enter a YouTube URL');

      const body = {
        url,
        output: { type: els.type.value, format: els.format.value },
        audio: { bitrate: els.bitrate.value }
      };

      if (els.platform.value) body.os = els.platform.value;
      if (els.type.value === 'video' && els.quality.value) {
        body.output.quality = els.quality.value;
      }
      if (els.enableTrim.checked) {
        body.trim = {
          start: parseFloat(els.trimStart.value) || 0,
          end: parseFloat(els.trimEnd.value) || 60
        };
      }

      els.downloadBtn.disabled = true;
      els.status.classList.add('show');
      resetStatus();

      try {
        const res = await fetch(`${API}/api/download`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        });

        const data = await res.json();
        if (!res.ok) throw new Error(getErrorMessage(data));

        currentStatusUrl = data.statusUrl;
        els.statusTitle.textContent = data.title;
        els.statusInfo.textContent = `Duration: ${formatDuration(data.duration)}` +
          (data.selectedQuality ? ` | Quality: ${data.selectedQuality}` : '');

        if (data.qualityChanged && data.qualityChangeReason) {
          els.warningBox.textContent = data.qualityChangeReason;
          els.warningBox.style.display = 'block';
        }

        pollInterval = setInterval(checkStatus, 1000);
      } catch (err) {
        els.errorBox.textContent = err.message;
        els.errorBox.style.display = 'block';
        els.downloadBtn.disabled = false;
      }
    });

    // Check status
    async function checkStatus() {
      if (!currentStatusUrl) return;

      try {
        const res = await fetch(currentStatusUrl);
        const data = await res.json();
        if (!res.ok) throw new Error(getErrorMessage(data));

        // Update progress
        els.progressBar.style.width = `${data.progress}%`;
        els.statusPercent.textContent = `${data.progress}%`;

        // Show detail progress for video downloads (only when pending)
        if (data.status === 'pending' && data.detail) {
          if (data.detail.video !== undefined || data.detail.audio !== undefined) {
            els.detailProgress.style.display = 'flex';
            if (data.detail.video !== undefined) {
              els.videoProgress.style.width = `${data.detail.video}%`;
              els.videoPercent.textContent = `${data.detail.video}%`;
            }
            if (data.detail.audio !== undefined) {
              els.audioProgress.style.width = `${data.detail.audio}%`;
              els.audioPercent.textContent = `${data.detail.audio}%`;
            }
          }
          els.statusPhase.textContent = 'Downloading...';
        } else if (data.status === 'completed') {
          // Completed
          clearInterval(pollInterval);
          els.progressBar.style.width = '100%';
          els.progressBar.classList.add('done');
          els.statusPhase.textContent = 'Complete!';
          els.statusPercent.textContent = '100%';
          els.detailProgress.style.display = 'none';

          if (data.downloadUrl) {
            const filename = data.downloadUrl.split('/').pop().split('?')[0];
            els.downloadLink.innerHTML = `<a href="${data.downloadUrl}" download>
              <svg width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>
              </svg>
              Download ${decodeURIComponent(filename)}
            </a>`;
          }
          els.downloadBtn.disabled = false;
        } else if (data.status === 'error') {
          // Error
          clearInterval(pollInterval);
          els.statusPhase.textContent = 'Error';
          els.errorBox.textContent = data.jobError || 'Unknown error';
          els.errorBox.style.display = 'block';
          els.downloadBtn.disabled = false;
        }
      } catch (err) {
        clearInterval(pollInterval);
        els.errorBox.textContent = err.message;
        els.errorBox.style.display = 'block';
        els.downloadBtn.disabled = false;
      }
    }
  </script>
</body>
</html>

```

---


### docker-compose.yml

```yaml
services:
  # Cloudflare WARP - SOCKS5 proxy on port 40000
  warp:
    image: caomingjun/warp:latest
    container_name: warp
    restart: always
    device_cgroup_rules:
      - 'c 10:200 rwm'
    cap_add:
      - NET_ADMIN
      - MKNOD
      - AUDIT_WRITE
    sysctls:
      - net.ipv6.conf.all.disable_ipv6=0
      - net.ipv4.conf.all.src_valid_mark=1
    volumes:
      - warp_data:/var/lib/cloudflare-warp
    environment:
      - WARP_SLEEP=2
      - GOST_ARGS=-L :40000
    logging:
      driver: "none"
    healthcheck:
      test: ["CMD", "curl", "-fsS", "--socks5", "127.0.0.1:40000", "https://www.cloudflare.com/cdn-cgi/trace"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    networks:
      - backend

  # Gost - HTTP proxy with authentication
  gost:
    image: ginuerzh/gost:latest
    container_name: gost
    restart: always
    depends_on:
      warp:
        condition: service_healthy
    ports:
      - "1111:1111"  # WARP proxy (public)
      - "2222:2222"  # Direct proxy (public)
    environment:
      - WARP_USER=${WARP_USER}
      - WARP_PASS=${WARP_PASS}
      - DIRECT_USER=${DIRECT_USER}
      - DIRECT_PASS=${DIRECT_PASS}
    logging:
      driver: "none"
    command: >
      -L=http://${WARP_USER}:${WARP_PASS}@:1111?probeResist=code:400
      -F=socks5://warp:40000
      -L=http://${DIRECT_USER}:${DIRECT_PASS}@:2222?probeResist=code:400
    networks:
      - backend

  # YouTube Downloader Go service
  yt-downloader:
    platform: linux/amd64
    build:
      context: ./yt-downloader
      dockerfile: Dockerfile
    image: yt-downloader:latest
    container_name: yt-downloader
    restart: always
    expose:
      - "5001"
    environment:
      - BASE_URL=${BASE_URL}
      - PORT=5001
    volumes:
      - yt_storage:/app/storage
    logging:
      driver: "none"
    depends_on:
      gost:
        condition: service_started
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:5001/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    networks:
      - backend

  # YouTube Extractor Python service
  yt-extractor:
    platform: linux/amd64
    build:
      context: ./yt-extractor
      dockerfile: Dockerfile
    image: yt-extractor:latest
    container_name: yt-extractor
    restart: always
    environment:
      - PYTHONUNBUFFERED=1
    logging:
      driver: "none"
    depends_on:
      gost:
        condition: service_started
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "-O", "/dev/null", "http://localhost:8300/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 15s
    networks:
      - backend

  # Nginx reverse proxy with SSL
  nginx:
    platform: linux/amd64
    build:
      context: ./nginx
      dockerfile: Dockerfile
    image: nginx-ssl:latest
    container_name: nginx
    restart: always
    ports:
      - "80:80"
      - "443:443"
    environment:
      - DOMAIN=${DOMAIN}
      - EMAIL=${EMAIL}
    volumes:
      - letsencrypt:/etc/letsencrypt
      - certbot_webroot:/var/www/certbot
    logging:
      driver: "none"
    depends_on:
      yt-downloader:
        condition: service_healthy
      yt-extractor:
        condition: service_healthy
    networks:
      - backend

networks:
  backend:
    driver: bridge

volumes:
  warp_data:
  yt_storage:
  letsencrypt:
  certbot_webroot:

```

---


### proxy/docker-compose.proxy.yml

```yaml
version: '3.8'

services:
  # Cloudflare WARP - provides SOCKS5 proxy on port 40000
  warp:
    image: caomingjun/warp:latest
    container_name: warp
    restart: always
    cap_add:
      - NET_ADMIN
    sysctls:
      - net.ipv6.conf.all.disable_ipv6=0
      - net.ipv4.conf.all.src_valid_mark=1
    volumes:
      - warp_data:/var/lib/cloudflare-warp
    environment:
      - WARP_SLEEP=2
    healthcheck:
      test: ["CMD", "curl", "-fsS", "--socks5", "127.0.0.1:40000", "https://www.cloudflare.com/cdn-cgi/trace"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    networks:
      - proxy_network

  # Gost - HTTP proxy with authentication
  gost:
    image: ginuerzh/gost:latest
    container_name: gost
    restart: always
    depends_on:
      warp:
        condition: service_healthy
    ports:
      - "1111:1111"  # WARP proxy (public)
      - "2222:2222"  # Direct proxy (public)
    volumes:
      - ./gost.json:/etc/gost/gost.json:ro
    environment:
      - WARP_USER=${WARP_USER}
      - WARP_PASS=${WARP_PASS}
      - DIRECT_USER=${DIRECT_USER}
      - DIRECT_PASS=${DIRECT_PASS}
    command: -C /etc/gost/gost.json
    networks:
      - proxy_network

networks:
  proxy_network:
    driver: bridge

volumes:
  warp_data:

```

---


### yt-downloader/docs/swagger.yaml

```yaml
basePath: /
definitions:
  models.AudioConfig:
    description: Audio configuration
    properties:
      bitrate:
        enum:
        - 64k
        - 128k
        - 192k
        - 320k
        example: 192k
        type: string
      trackId:
        example: en.vss_abc123
        type: string
    type: object
  models.DeleteResponse:
    description: Delete job response
    properties:
      deleted:
        example: true
        type: boolean
    type: object
  models.DownloadRequest:
    description: Download request payload
    properties:
      audio:
        $ref: '#/definitions/models.AudioConfig'
      os:
        enum:
        - ios
        - android
        - macos
        - windows
        - linux
        example: windows
        type: string
      output:
        $ref: '#/definitions/models.OutputConfig'
      trim:
        $ref: '#/definitions/models.TrimConfig'
      url:
        example: https://youtube.com/watch?v=dQw4w9WgXcQ
        type: string
    type: object
  models.DownloadResponse:
    description: Response after creating a download job
    properties:
      duration:
        example: 213.5
        type: number
      needsReencode:
        example: false
        type: boolean
      qualityChangeReason:
        example: 1080p not available, using 720p
        type: string
      qualityChanged:
        example: true
        type: boolean
      requestedQuality:
        example: 1080p
        type: string
      selectedQuality:
        example: 720p
        type: string
      statusUrl:
        example: https://api.ytconvert.org/api/status/V1StGXR8_Z5jdHi?token=xxx&expires=xxx
        type: string
      title:
        example: Rick Astley - Never Gonna Give You Up
        type: string
    type: object
  models.HealthResponse:
    description: Health check response
    properties:
      status:
        example: ok
        type: string
      timestamp:
        example: 1705123456789
        type: integer
    type: object
  models.OutputConfig:
    description: Output configuration
    properties:
      format:
        enum:
        - mp4
        - webm
        - mkv
        - mp3
        - m4a
        - wav
        - opus
        - flac
        - ogg
        example: mp4
        type: string
      quality:
        enum:
        - 2160p
        - 1440p
        - 1080p
        - 720p
        - 480p
        - 360p
        example: 1080p
        type: string
      type:
        enum:
        - video
        - audio
        example: video
        type: string
    type: object
  models.StatusResponse:
    description: Job status response
    properties:
      downloadUrl:
        example: https://api.ytconvert.org/files/abc123/output.mp4?token=xxx&expires=123
        type: string
      duration:
        example: 213.5
        type: number
      jobError:
        example: 'Download failed: connection timeout'
        type: string
      progress:
        example: 45
        type: integer
      status:
        enum:
        - pending
        - completed
        - error
        example: pending
        type: string
      title:
        example: Rick Astley - Never Gonna Give You Up
        type: string
    type: object
  models.TrimConfig:
    description: Trim configuration
    properties:
      accurate:
        example: false
        type: boolean
      end:
        example: 60
        type: number
      start:
        example: 10
        type: number
    type: object
  utils.ErrorDetail:
    properties:
      code:
        type: string
      message:
        type: string
    type: object
  utils.ErrorResponse:
    properties:
      error:
        $ref: '#/definitions/utils.ErrorDetail'
    type: object
host: api.ytconvert.org
info:
  contact:
    email: support@ytconvert.org
    name: API Support
  description: API for downloading YouTube videos and audio
  license:
    name: MIT
    url: https://opensource.org/licenses/MIT
  termsOfService: http://swagger.io/terms/
  title: YT Downloader API
  version: "2.0"
paths:
  /api/download:
    post:
      consumes:
      - application/json
      description: Create a new download job for a YouTube video or audio
      parameters:
      - description: Download request
        in: body
        name: request
        required: true
        schema:
          $ref: '#/definitions/models.DownloadRequest'
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            $ref: '#/definitions/models.DownloadResponse'
        "400":
          description: Validation error
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "404":
          description: No stream found
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "500":
          description: Server error
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
      summary: Create download job
      tags:
      - download
  /api/jobs/{id}:
    delete:
      description: Delete a job and its associated files
      parameters:
      - description: Job ID
        in: path
        name: id
        required: true
        type: string
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            $ref: '#/definitions/models.DeleteResponse'
        "400":
          description: Invalid job ID
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "404":
          description: Job not found
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "500":
          description: Delete failed
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
      summary: Delete job
      tags:
      - jobs
  /api/status/{id}:
    get:
      description: Check the status and progress of a download job
      parameters:
      - description: Job ID
        in: path
        name: id
        required: true
        type: string
      - description: Signed URL token
        in: query
        name: token
        required: true
        type: string
      - description: Expiration timestamp
        in: query
        name: expires
        required: true
        type: string
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            $ref: '#/definitions/models.StatusResponse'
        "400":
          description: Invalid job ID
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "401":
          description: Missing token or expires
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "403":
          description: Invalid or expired token
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "404":
          description: Job not found
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "500":
          description: Server error
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
      summary: Get job status
      tags:
      - status
  /files/{id}/{filename}:
    get:
      description: Download the merged output file
      parameters:
      - description: Job ID
        in: path
        name: id
        required: true
        type: string
      - description: Output filename
        in: path
        name: filename
        required: true
        type: string
      - description: Signed URL token
        in: query
        name: token
        required: true
        type: string
      - description: Expiration timestamp
        in: query
        name: expires
        required: true
        type: integer
      produces:
      - application/octet-stream
      responses:
        "200":
          description: Output file
          schema:
            type: file
        "400":
          description: Invalid parameters
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "401":
          description: Missing auth
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "403":
          description: Invalid token
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "404":
          description: Not found
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
      summary: Download file
      tags:
      - files
  /health:
    get:
      description: Check if the server is running
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            $ref: '#/definitions/models.HealthResponse'
      summary: Health check
      tags:
      - health
  /stream/{id}:
    get:
      description: Stream video/audio using FFmpeg pipe (realtime remux/convert)
      parameters:
      - description: Job ID
        in: path
        name: id
        required: true
        type: string
      - description: Signed URL token
        in: query
        name: token
        required: true
        type: string
      - description: Expiration timestamp
        in: query
        name: expires
        required: true
        type: integer
      produces:
      - application/octet-stream
      responses:
        "200":
          description: Media stream
          schema:
            type: file
        "307":
          description: Redirect to download URL
        "400":
          description: Invalid parameters
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "401":
          description: Missing auth
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "403":
          description: Invalid token
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "404":
          description: Not found
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
        "500":
          description: Stream failed
          schema:
            $ref: '#/definitions/utils.ErrorResponse'
      summary: Stream video/audio
      tags:
      - stream
schemes:
- https
- http
swagger: "2.0"

```

---


### yt-extractor/mapper.py

```py
"""
Mapper module to convert yt-dlp info to API response format.

yt-dlp format types:
- AUDIO_ONLY: vcodec='none', acodec != 'none' (e.g., m4a, webm audio)
- VIDEO_ONLY: vcodec != 'none', acodec='none' (e.g., mp4/webm video without audio)
- VIDEO+AUDIO: vcodec != 'none', acodec != 'none' (e.g., format_id=18, 360p mp4)
- STORYBOARD: vcodec='none', acodec='none' (skip these)
"""

from urllib.parse import urlparse, parse_qs


def parse_audio_lang_from_url(url):
    """
    Extract language code and audio content type from URL xtags parameter.

    URL contains xtags like: acont=dubbed:lang=vi or acont=original:lang=en

    Returns:
        tuple: (lang_code, audio_content_type) e.g. ('vi', 'dubbed') or ('en', 'original')
               Returns (None, None) if not found
    """
    if not url:
        return None, None

    try:
        parsed = urlparse(url)
        params = parse_qs(parsed.query)
        xtags = params.get('xtags', [''])[0]

        lang_code = None
        acont_type = None

        for part in xtags.split(':'):
            if part.startswith('lang='):
                lang_code = part.split('=', 1)[1]
            elif part.startswith('acont='):
                acont_type = part.split('=', 1)[1]

        return lang_code, acont_type
    except Exception:
        return None, None


FORMAT_NAMES = {
    'mp4': 'MPEG-4',
    'webm': 'WebM',
    'm4a': 'M4A',
    'opus': 'Opus',
}


def get_format_name(ext):
    """Convert extension to format name."""
    return FORMAT_NAMES.get(ext, ext.upper() if ext else None)


def get_filesize(fmt):
    """Get filesize from format, preferring exact size over approximate."""
    return fmt.get('filesize') or fmt.get('filesize_approx')


def get_best_thumbnail(thumbnails):
    """Get best quality thumbnail URL from thumbnails list."""
    if not thumbnails:
        return None

    # Prefer maxresdefault or hqdefault
    for thumb in thumbnails:
        thumb_id = thumb.get('id', '')
        if 'maxres' in thumb_id or 'hq' in thumb_id:
            return thumb.get('url')

    # Fallback to last thumbnail (usually highest quality)
    return thumbnails[-1].get('url')


def build_codec_string(vcodec, acodec):
    """Build codec string from video and audio codecs."""
    if vcodec and vcodec != 'none' and acodec and acodec != 'none':
        return f"{vcodec}, {acodec}"
    if vcodec and vcodec != 'none':
        return vcodec
    return None


def map_video_stream(fmt):
    """Map yt-dlp format to API video stream format."""
    vcodec = fmt.get('vcodec')
    acodec = fmt.get('acodec')
    ext = fmt.get('ext', 'mp4')

    return {
        'url': fmt.get('url'),
        'quality': fmt.get('format_note') or f"{fmt.get('height', 0)}p",
        'format': get_format_name(ext),
        'mimeType': f"video/{ext}",
        'bitrate': fmt.get('vbr') or fmt.get('tbr'),
        'fileSize': get_filesize(fmt),
        'codec': build_codec_string(vcodec, acodec),
        'width': fmt.get('width'),
        'height': fmt.get('height'),
        'fps': fmt.get('fps'),
        'videoOnly': acodec == 'none' or acodec is None,
    }


def map_audio_stream(fmt):
    """Map yt-dlp format to API audio stream format."""
    ext = fmt.get('ext', 'm4a')
    url = fmt.get('url')
    lang_code, acont_type = parse_audio_lang_from_url(url)

    return {
        'url': url,
        'quality': fmt.get('format_note') or f"{int(fmt.get('abr') or 0)}kbps",
        'format': get_format_name(ext),
        'mimeType': f"audio/{ext}",
        'bitrate': fmt.get('abr') or fmt.get('tbr'),
        'fileSize': get_filesize(fmt),
        'codec': fmt.get('acodec'),
        'audioTrackId': lang_code,
        'audioTrackType': acont_type,
    }


def map_yt_dlp_to_api(info):
    """
    Map yt-dlp info to API response format.

    Args:
        info: yt-dlp extract_info result

    Returns:
        dict with videoStreams, audioStreams, and metadata
    """
    video_streams = []
    audio_streams = []

    for fmt in info.get('formats', []):
        vcodec = fmt.get('vcodec', 'none')
        acodec = fmt.get('acodec', 'none')

        # Skip storyboards (no video or audio codec)
        if vcodec == 'none' and acodec == 'none':
            continue

        if vcodec != 'none':
            video_streams.append(map_video_stream(fmt))
        elif acodec != 'none':
            audio_streams.append(map_audio_stream(fmt))

    categories = info.get('categories')

    return {
        'id': info.get('id'),
        'title': info.get('title'),
        'uploaderName': info.get('uploader') or info.get('channel'),
        'uploaderUrl': info.get('uploader_url') or info.get('channel_url'),
        'thumbnailUrl': get_best_thumbnail(info.get('thumbnails', [])),
        'duration': info.get('duration'),
        'videoStreams': video_streams,
        'audioStreams': audio_streams,
        'subtitles': [],
        'category': categories[0] if categories else None,
        'tags': info.get('tags', []),
    }

```

---


### yt-extractor/app.py

```py
from fastapi import FastAPI, Query, HTTPException
from fastapi.responses import JSONResponse
from urllib.parse import quote
import asyncio
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import os

# Set Deno path BEFORE importing yt_dlp
BASE_DIR = Path(__file__).parent
DENO_BIN_DIR = str(BASE_DIR / 'bin')
os.environ["PATH"] = f"{DENO_BIN_DIR}:" + os.environ.get("PATH", "")

# Now import yt_dlp - it will find deno in PATH
import yt_dlp
from mapper import map_yt_dlp_to_api
import cookie_db

app = FastAPI()

# Thread pool for blocking yt-dlp operations
executor = ThreadPoolExecutor(max_workers=10)


class CookiePool:
    """Pre-fetched cookie pool để giảm tải DB mỗi request."""

    def __init__(self, min_size=5, max_size=10):
        self.pool = deque()
        self.min_size = min_size
        self.max_size = max_size
        self._lock = asyncio.Lock()
        self._refilling = False

    async def get(self):
        """Lấy cookie từ pool, trigger refill nếu cần."""
        async with self._lock:
            if self.pool:
                cookie = self.pool.popleft()
                # Trigger background refill nếu pool thấp
                if len(self.pool) < self.min_size and not self._refilling:
                    asyncio.create_task(self._refill())
                return cookie
        # Fallback: lấy trực tiếp từ DB nếu pool rỗng
        return cookie_db.get()

    async def _refill(self):
        """Background fetch thêm cookies vào pool."""
        if self._refilling:
            return
        self._refilling = True
        try:
            needed = self.max_size - len(self.pool)
            if needed > 0:
                loop = asyncio.get_event_loop()
                cookies = await loop.run_in_executor(None, cookie_db.get_batch, needed)
                async with self._lock:
                    self.pool.extend(cookies)
        finally:
            self._refilling = False

    async def warm_up(self):
        """Initial fill khi startup."""
        await self._refill()


# Global cookie pool
cookie_pool = CookiePool(min_size=5, max_size=10)


@app.on_event("startup")
async def startup():
    """Pre-fill cookie pool khi app khởi động."""
    await cookie_pool.warm_up()


def build_proxy_url(proxy):
    """
    Build proxy URL with authentication.

    Supports formats:
    - user:pass:host:port -> http://user:pass@host:port
    - http://host:port (no auth)
    - http://user:pass@host:port (already formatted)
    """
    if not proxy:
        return None

    # Already a proper URL format
    if '://' in proxy:
        return proxy

    # Parse format: user:pass:host:port
    parts = proxy.split(':')
    if len(parts) == 4:
        user, password, host, port = parts
        encoded_user = quote(user, safe='')
        encoded_pass = quote(password, safe='')
        return f"http://{encoded_user}:{encoded_pass}@{host}:{port}"

    # Fallback: assume it's host:port only
    if len(parts) == 2:
        return f"http://{proxy}"

    return proxy


def build_ydl_opts(cookies, proxy=None):
    """Build yt-dlp options with cookies and optional proxy."""
    opts = {
        'quiet': True,
        'no_warnings': True,
        'skip_download': True,
        'extract_flat': True,
        'no_check_formats': True,
        'formats': 'none',
        'dump_single_json': True,
        'http_headers': {'Cookie': cookies},
        'extractor_args': {
            'youtube': {
                'skip': ['hls', 'dash', 'translated_subs'],
                'player_skip': ['webpage', 'configs'],
                'player_client': ['TVHTML5'],
            }
        },
        # Deno JavaScript runtime (found via PATH)
        'js_runtime': 'deno',
    }
    if proxy:
        opts['proxy'] = proxy
    return opts


def extract_sync(video_id: str, proxy: str, profile: str, cookies: str):
    """Blocking extraction - runs in thread pool."""
    url = f'https://www.youtube.com/watch?v={video_id}'
    ydl_opts = build_ydl_opts(cookies, proxy)

    try:
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(url, download=False)
            result = map_yt_dlp_to_api(info)

            # Check if both audio and video streams exist
            has_video = bool(result.get('videoStreams'))
            has_audio = bool(result.get('audioStreams'))

            if not has_video or not has_audio:
                missing = []
                if not has_video:
                    missing.append('video')
                if not has_audio:
                    missing.append('audio')
                error_msg = f"Missing streams: {', '.join(missing)}"
                if profile:
                    cookie_db.invalidate(profile)
                return None, error_msg

            return result, None
    except Exception as e:
        if profile and cookie_db.is_bad(e):
            cookie_db.invalidate(profile)
        return None, str(e)


@app.get('/api/youtube/video/{video_id}')
async def extract(video_id: str, proxy: str = Query(None)):
    profile, cookies = await cookie_pool.get()
    if not cookies:
        return JSONResponse({'error': 'No active cookie'}, status_code=503)

    proxy_url = build_proxy_url(proxy)

    # Run blocking yt-dlp in thread pool
    loop = asyncio.get_event_loop()
    result, error = await loop.run_in_executor(
        executor, extract_sync, video_id, proxy_url, profile, cookies
    )

    if error:
        return JSONResponse({'error': error}, status_code=500)
    return result


@app.get('/health')
async def health():
    return {'status': 'UP'}


if __name__ == '__main__':
    import uvicorn
    uvicorn.run(app, host='0.0.0.0', port=8300)

```

---


### yt-extractor/cookie_db.py

```py
import os
from datetime import datetime
from threading import Thread
from pymongo import MongoClient

# Error patterns indicating bad/expired cookies
BAD_COOKIE_ERRORS = ('sign in', 'login required', 'bot', 'please sign in', 'login', 'sign')

# MongoDB connection
_MONGO_URI = os.environ.get(
    'MONGO_URI',
    'mongodb://cookie:cookie123456789@85.10.196.119:27017/cookie'
)
_col = MongoClient(_MONGO_URI)['cookie']['cookies']


def _update_last_used(doc_id):
    """Background update - không block main thread."""
    _col.update_one({'_id': doc_id}, {'$set': {'last_used': datetime.now()}})


def get():
    """Get active cookie with least recent usage (round-robin)."""
    doc = _col.find_one({'status': 'active'}, sort=[('last_used', 1)])
    if doc:
        Thread(target=_update_last_used, args=(doc['_id'],), daemon=True).start()
        return doc['profile_name'], doc['cookie_string']
    return None, None


def get_batch(limit=10):
    """Get multiple active cookies for pool pre-fetching."""
    docs = list(_col.find({'status': 'active'}, sort=[('last_used', 1)]).limit(limit))
    if docs:
        # Update last_used for all fetched docs in background
        doc_ids = [doc['_id'] for doc in docs]
        Thread(target=_update_batch_last_used, args=(doc_ids,), daemon=True).start()
        return [(doc['profile_name'], doc['cookie_string']) for doc in docs]
    return []


def _update_batch_last_used(doc_ids):
    """Background batch update."""
    _col.update_many({'_id': {'$in': doc_ids}}, {'$set': {'last_used': datetime.now()}})


def invalidate(profile):
    """Mark a cookie profile as inactive."""
    _col.update_one(
        {'profile_name': profile},
        {'$set': {'status': 'inactive', 'updated_at': datetime.now()}}
    )


def is_bad(error):
    """Check if error indicates bad/expired cookie."""
    error_msg = str(error).lower()
    return any(pattern in error_msg for pattern in BAD_COOKIE_ERRORS)

```

---
