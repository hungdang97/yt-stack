from fastapi import FastAPI, Query, HTTPException
from fastapi.responses import JSONResponse
from urllib.parse import quote
import asyncio
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import os
import time
import logging
import tempfile
import http.cookiejar
from datetime import datetime, timedelta

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

# ---- Extraction tuning (env-configurable) ----
# YouTube player clients yt-dlp impersonates, in order. 'ios' first is fast and
# usually skips the JS n-sig step; 'web' kept as a reliable fallback.
PLAYER_CLIENTS = [c.strip() for c in os.environ.get("YT_PLAYER_CLIENT", "ios,web").split(",") if c.strip()]
# Keep the deno JS runtime (needed by 'web' for n-sig). Set YT_USE_DENO=false to
# drop it once 'ios' proves reliable on your setup (faster, no JS startup).
USE_DENO = os.environ.get("YT_USE_DENO", "true").lower() in ("1", "true", "yes")

# ---- In-memory metadata cache (videoId+premium -> extract result) ----
# Stream URLs live ~6h; a short TTL makes repeat /api/info and the /api/download
# that follows it near-instant, without serving stale links.
EXTRACT_CACHE_TTL = int(os.environ.get("EXTRACT_CACHE_TTL", "300"))   # seconds; 0 disables
EXTRACT_CACHE_MAX = int(os.environ.get("EXTRACT_CACHE_MAX", "1000"))  # max entries
_extract_cache = {}   # key -> (expires_at, result)
_cache_hits = 0
_cache_misses = 0


def _cache_key(video_id, premium):
    return video_id + (":p" if premium else ":n")


def cache_get(video_id, premium):
    if EXTRACT_CACHE_TTL <= 0:
        return None
    item = _extract_cache.get(_cache_key(video_id, premium))
    if not item:
        return None
    expires_at, result = item
    if expires_at < time.time():
        _extract_cache.pop(_cache_key(video_id, premium), None)
        return None
    return result


def cache_set(video_id, premium, result):
    if EXTRACT_CACHE_TTL <= 0:
        return
    now = time.time()
    if len(_extract_cache) >= EXTRACT_CACHE_MAX:
        # Drop expired entries first, then oldest if still over the cap.
        for k in [k for k, (exp, _) in list(_extract_cache.items()) if exp < now]:
            _extract_cache.pop(k, None)
        while len(_extract_cache) >= EXTRACT_CACHE_MAX:
            _extract_cache.pop(next(iter(_extract_cache)), None)
    _extract_cache[_cache_key(video_id, premium)] = (now + EXTRACT_CACHE_TTL, result)


class CookiePool:
    """Cookie pool with premium/normal separation and periodic refresh."""

    def __init__(self, size=10):
        self.normal_pool = deque()
        self.premium_pool = deque()
        self.size = size
        self._lock = asyncio.Lock()

    async def get(self, premium=False):
        """Get a cookie from pool. Refill if empty."""
        pool = self.premium_pool if premium else self.normal_pool
        async with self._lock:
            if not pool:
                await self._refill(premium)
                pool = self.premium_pool if premium else self.normal_pool

            if pool:
                return pool.popleft()

        # Fallback to direct DB fetch if pool remains empty
        return cookie_db.get(premium=premium)

    async def _refill(self, premium=False):
        """Fetch fresh random cookies."""
        loop = asyncio.get_event_loop()
        cookies = await loop.run_in_executor(None, cookie_db.get_batch, self.size, premium)
        if cookies:
            pool = self.premium_pool if premium else self.normal_pool
            pool.extend(cookies)

    async def start_refresh_loop(self):
        """Background task to refresh pools every 10 seconds."""
        while True:
            await asyncio.sleep(10)
            try:
                async with self._lock:
                    self.normal_pool.clear()
                    self.premium_pool.clear()
                    await self._refill(premium=False)
                    await self._refill(premium=True)
            except Exception as e:
                logger.error(f"Cookie pool refresh failed: {e}")

    async def warm_up(self):
        """Initial fill and start background refresh."""
        await self._refill(premium=False)
        await self._refill(premium=True)
        asyncio.create_task(self.start_refresh_loop())


# Global cookie pool
cookie_pool = CookiePool(size=10)


@app.on_event("startup")
async def startup():
    """Pre-fill cookie pool khi app khởi động."""
    await cookie_pool.warm_up()


def parse_cookies_to_netscape(cookie_string):
    """
    Parse cookie string to Netscape cookie file format.

    Input format: "name1=value1; name2=value2; ..."
    Output: Netscape cookie file content
    """
    if not cookie_string:
        return None

    lines = ["# Netscape HTTP Cookie File\n"]

    # Parse cookie string
    for cookie in cookie_string.split(';'):
        cookie = cookie.strip()
        if not cookie or '=' not in cookie:
            continue

        name, value = cookie.split('=', 1)
        name = name.strip()
        value = value.strip()

        # Netscape format: domain flag domain path secure expiration name value
        # For YouTube cookies, use .youtube.com domain
        expiration = int((datetime.now() + timedelta(days=365)).timestamp())
        line = f".youtube.com\tTRUE\t/\tTRUE\t{expiration}\t{name}\t{value}\n"
        lines.append(line)

    return ''.join(lines)


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


def build_ydl_opts(cookie_file_path=None, proxy=None):
    """Build yt-dlp options with optional cookie file and proxy."""
    opts = {
        'quiet': True,
        'no_warnings': True,
        'skip_download': True,
        'extract_flat': True,
        'no_check_formats': True,
        'dump_single_json': True,
        'extractor_args': {
            'youtube': {
                'player_client': PLAYER_CLIENTS,
                'skip': ['hls', 'dash', 'translated_subs'],
            },
        },
    }
    # Deno JS runtime (found via PATH) — required by web/mweb clients for n-sig.
    if USE_DENO:
        opts['js_runtime'] = 'deno'

    # Point the bgutil PO Token provider plugin at the provider service so the
    # mweb client can obtain a GVS PO Token (recovers audio formats on flagged IPs).
    # https://github.com/yt-dlp/yt-dlp/wiki/PO-Token-Guide
    pot_base_url = os.environ.get('BGUTIL_POT_BASE_URL')
    if pot_base_url:
        opts['extractor_args']['youtubepot-bgutilhttp'] = {'base_url': [pot_base_url]}

    if cookie_file_path:
        opts['cookiefile'] = cookie_file_path
    if proxy:
        opts['proxy'] = proxy
    return opts


def extract_sync(video_id: str, proxy: str, profile: str, cookies: str):
    """Blocking extraction - runs in thread pool."""
    url = f'https://www.youtube.com/watch?v={video_id}'

    # Create temporary cookie file
    cookie_content = parse_cookies_to_netscape(cookies)
    if not cookie_content:
        return None, "Invalid cookie format"

    cookie_file = None
    try:
        # Write cookies to temporary file
        cookie_file = tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False)
        cookie_file.write(cookie_content)
        cookie_file.close()

        # Build yt-dlp options with cookie file
        ydl_opts = build_ydl_opts(cookie_file.name, proxy)

        t0 = time.time()
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(url, download=False)
        logger.info(f"[{video_id}] yt-dlp extract_info: {(time.time() - t0) * 1000:.0f}ms (cookie)")

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
            return None, error_msg

        return result, None
    except Exception as e:
        return None, str(e)
    finally:
        # Cleanup temporary cookie file
        if cookie_file:
            try:
                os.unlink(cookie_file.name)
            except:
                pass


def extract_no_cookie_sync(video_id: str, proxy: str):
    """Extraction without cookies - uses Cloudflare IP only. Runs in thread pool."""
    url = f'https://www.youtube.com/watch?v={video_id}'

    try:
        ydl_opts = build_ydl_opts(cookie_file_path=None, proxy=proxy)

        t0 = time.time()
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(url, download=False)
        logger.info(f"[{video_id}] yt-dlp extract_info: {(time.time() - t0) * 1000:.0f}ms (no-cookie)")

        result = map_yt_dlp_to_api(info)

        has_video = bool(result.get('videoStreams'))
        has_audio = bool(result.get('audioStreams'))

        if not has_video or not has_audio:
            missing = []
            if not has_video:
                missing.append('video')
            if not has_audio:
                missing.append('audio')
            return None, f"Missing streams: {', '.join(missing)}"

        return result, None
    except Exception as e:
        return None, str(e)


async def _try_cookie_extract(video_id, proxy_url, premium, attempt, total):
    """Try extraction with a cookie from pool. Returns (result, error)."""
    label = 'premium' if premium else 'normal'
    profile, cookies = await cookie_pool.get(premium=premium)
    if not cookies:
        logger.error(f"[{video_id}] No active {label} cookie available")
        return None, f"No active {label} cookie available"

    logger.info(f"[{video_id}] Attempt {attempt}/{total} | Using {label} cookie: {profile}")

    loop = asyncio.get_event_loop()
    result, error = await loop.run_in_executor(
        executor, extract_sync, video_id, proxy_url, profile, cookies
    )

    if not error:
        logger.info(f"[{video_id}] Extraction successful | video={len(result.get('videoStreams', []))} | audio={len(result.get('audioStreams', []))}")
        result['extractInfo'] = {
            'cookie': profile,
            'proxy': proxy_url,
            'premium': premium,
            'attempt': attempt,
        }
        return result, None

    logger.warning(f"[{video_id}] Attempt {attempt} failed: {error} (cookie invalidation disabled)")
    return None, error


async def _try_no_cookie_extract(video_id, proxy_url, attempt, total):
    """Try extraction without cookies (Cloudflare IP only). Returns (result, error)."""
    logger.info(f"[{video_id}] Attempt {attempt}/{total} | No cookie, Cloudflare IP only")

    loop = asyncio.get_event_loop()
    result, error = await loop.run_in_executor(
        executor, extract_no_cookie_sync, video_id, proxy_url
    )

    if not error:
        logger.info(f"[{video_id}] Extraction successful (no cookie) | video={len(result.get('videoStreams', []))} | audio={len(result.get('audioStreams', []))}")
        result['extractInfo'] = {
            'cookie': None,
            'proxy': proxy_url,
            'premium': False,
            'attempt': attempt,
        }
        return result, None

    logger.warning(f"[{video_id}] Attempt {attempt} (no cookie) failed: {error}")
    return None, error


@app.get('/api/youtube/video/{video_id}')
async def extract(video_id: str, proxy: str = Query(None), premium: str = Query(None)):
    is_premium = premium == '1'
    t_start = time.time()
    logger.info(f"[{video_id}] Extraction request received | proxy={proxy} | premium={is_premium}")

    # Serve from cache when we extracted this video recently — big speed win for
    # repeat / trending links, and for the /api/download that follows /api/info.
    global _cache_hits, _cache_misses
    cached = cache_get(video_id, is_premium)
    if cached is not None:
        _cache_hits += 1
        logger.info(f"[{video_id}] cache HIT ({(time.time() - t_start) * 1000:.0f}ms)")
        return cached
    _cache_misses += 1

    proxy_url = build_proxy_url(proxy)

    last_error = None
    last_attempt = 0

    # Build attempt plan:
    # Premium:  premium cookie → premium cookie → normal cookie → normal cookie → no-cookie
    # Normal:   normal cookie → normal cookie → no-cookie
    attempts = []
    if is_premium:
        attempts.append(('cookie', True))    # premium cookie
        attempts.append(('cookie', True))    # premium cookie
    attempts.append(('cookie', False))       # normal cookie
    attempts.append(('cookie', False))       # normal cookie
    if proxy_url:
        attempts.append(('no_cookie', False))  # Cloudflare IP only

    total = len(attempts)

    for i, (method, use_premium) in enumerate(attempts, 1):
        last_attempt = i

        if method == 'cookie':
            result, error = await _try_cookie_extract(video_id, proxy_url, use_premium, i, total)
        else:
            result, error = await _try_no_cookie_extract(video_id, proxy_url, i, total)

        if result:
            cache_set(video_id, is_premium, result)
            logger.info(f"[{video_id}] extracted in {(time.time() - t_start) * 1000:.0f}ms (attempt {i}/{total})")
            return result
        last_error = error

    logger.error(f"[{video_id}] Extraction finally failed after {last_attempt} attempts: {last_error}")
    return JSONResponse({'error': last_error}, status_code=500)


@app.get('/health')
async def health():
    return {
        'status': 'UP',
        'service': 'yt-extractor',
        'version': '2.0.0',
        'cache': {
            'ttl': EXTRACT_CACHE_TTL,
            'size': len(_extract_cache),
            'hits': _cache_hits,
            'misses': _cache_misses,
        },
        'playerClients': PLAYER_CLIENTS,
        'useDeno': USE_DENO,
    }


if __name__ == '__main__':
    import uvicorn
    uvicorn.run(app, host='0.0.0.0', port=8300)
