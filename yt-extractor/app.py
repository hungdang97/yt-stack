from fastapi import FastAPI, Query, HTTPException
from fastapi.responses import JSONResponse
from urllib.parse import quote
import asyncio
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import os
import logging

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[logging.StreamHandler()]
)
logger = logging.getLogger(__name__)

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
    """Random cookie pool with periodic refresh."""

    def __init__(self, size=10):
        self.pool = deque()
        self.size = size
        self._lock = asyncio.Lock()

    async def get(self):
        """Get a cookie from pool. Refill if empty."""
        async with self._lock:
            if not self.pool:
                await self._refill()
            
            if self.pool:
                return self.pool.popleft()
        
        # Fallback to direct DB fetch if pool remains empty
        return cookie_db.get()

    async def _refill(self):
        """Fetch fresh random cookies."""
        loop = asyncio.get_event_loop()
        cookies = await loop.run_in_executor(None, cookie_db.get_batch, self.size)
        if cookies:
            self.pool.extend(cookies)

    async def start_refresh_loop(self):
        """Background task to refresh pool every 10 seconds."""
        while True:
            await asyncio.sleep(10)
            try:
                async with self._lock:
                    self.pool.clear()
                    await self._refill()
                # logger.debug("Cookie pool refreshed")
            except Exception as e:
                logger.error(f"Cookie pool refresh failed: {e}")

    async def warm_up(self):
        """Initial fill and start background refresh."""
        await self._refill()
        asyncio.create_task(self.start_refresh_loop())


# Global cookie pool
cookie_pool = CookiePool(size=10)


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
                
                # Treat missing streams as a potential cookie/bot issue
                # logger.error(f"[{video_id}] Missing streams: {error_msg} | proxy={proxy}")
                return None, error_msg

            return result, None
    except Exception as e:
        # logger.error(f"[{video_id}] Extraction failed: {str(e)} | proxy={proxy}")
        return None, str(e)


@app.get('/api/youtube/video/{video_id}')
async def extract(video_id: str, proxy: str = Query(None)):
    logger.info(f"[{video_id}] Extraction request received | proxy={proxy}")
    proxy_url = build_proxy_url(proxy)
    
    # Retry mechanism: Attempt 1 + 1 Retry = 2 Attempts
    max_attempts = 2
    last_error = None
    
    for attempt in range(1, max_attempts + 1):
        profile, cookies = await cookie_pool.get()
        if not cookies:
            logger.error(f"[{video_id}] No active cookie available")
            return JSONResponse({'error': 'No active cookie'}, status_code=503)

        logger.info(f"[{video_id}] Attempt {attempt}/{max_attempts} | Using cookie: {profile}")

        # Run blocking yt-dlp in thread pool
        loop = asyncio.get_event_loop()
        result, error = await loop.run_in_executor(
            executor, extract_sync, video_id, proxy_url, profile, cookies
        )

        if not error:
            logger.info(f"[{video_id}] Extraction successful | video={len(result.get('videoStreams', []))} | audio={len(result.get('audioStreams', []))}")
            return result
        
        # Handle Error
        last_error = error
        is_bad = cookie_db.is_bad(error) or "Missing streams" in error
        
        if is_bad:
            logger.warning(f"[{video_id}] Attempt {attempt} failed with BAD COOKIE error: {error}. Invalidating {profile}.")
            cookie_db.invalidate(profile)
            # Loop continues to next attempt
        else:
            logger.error(f"[{video_id}] Attempt {attempt} failed with non-cookie error: {error}. No retry.")
            break # Non-cookie error (e.g. 404, network), do not retry randomly

    # If we exhausted retries or broke early
    logger.error(f"[{video_id}] Extraction finally failed after {attempt} attempts: {last_error}")
    return JSONResponse({'error': last_error}, status_code=500)


@app.get('/health')
async def health():
    return {'status': 'UP'}


if __name__ == '__main__':
    import uvicorn
    uvicorn.run(app, host='0.0.0.0', port=8300)
