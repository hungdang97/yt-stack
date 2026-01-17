from fastapi import FastAPI, Query, HTTPException
from fastapi.responses import JSONResponse
from urllib.parse import quote
import yt_dlp
from mapper import map_yt_dlp_to_api
import cookie_db
import asyncio
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

app = FastAPI()

# JavaScript runtime configuration (Deno in bin/)
BASE_DIR = Path(__file__).parent
DENO_PATH = str(BASE_DIR / 'bin' / 'deno')

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
        # Deno JavaScript runtime
        'js_runtimes': {'deno': {'path': DENO_PATH}},
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
