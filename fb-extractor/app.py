from __future__ import annotations
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from typing import Optional
import yt_dlp
from yt_dlp.networking.impersonate import ImpersonateTarget
import re
import json
import urllib.parse
import logging
import tempfile
import os

from mapper import map_fb_response

logger = logging.getLogger("fb-extractor")

app = FastAPI(
    title="Facebook Extractor",
    version="2.0.0",
    description="Extract media links from Facebook videos, reels, stories. Powered by yt-dlp.",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ============================================
# HELPERS
# ============================================

def _safe_int(value) -> int:
    if value is None:
        return 0
    try:
        return int(value)
    except (ValueError, TypeError):
        return 0


def _is_facebook_url(url: str) -> bool:
    return bool(re.search(
        r"(?:facebook\.com|fb\.watch|fb\.com|facebookwkhpilnemxj7asaniu7vnjjbiltxjqhye3mhbshg7kx5tfyd\.onion)",
        url, re.IGNORECASE,
    ))


# ============================================
# YT-DLP EXTRACTION
# ============================================

def _build_ydl_opts(proxy: Optional[str] = None, cookie: Optional[str] = None) -> tuple[dict, Optional[str]]:
    opts = {
        "quiet": True,
        "no_warnings": True,
        "skip_download": True,
        "extract_flat": False,
        "no_check_formats": True,
        "dump_single_json": True,
        "impersonate": ImpersonateTarget.from_str("chrome"),
    }

    if proxy:
        opts["proxy"] = proxy

    cookie_file = None
    if cookie:
        try:
            cookies_dict = json.loads(urllib.parse.unquote(cookie))
        except (json.JSONDecodeError, ValueError):
            cookies_dict = {"c_user": cookie} if cookie.isdigit() else {"xs": cookie}

        cookie_file = tempfile.NamedTemporaryFile(
            mode="w", suffix=".txt", delete=False, prefix="fb_cookie_"
        )
        cookie_file.write("# Netscape HTTP Cookie File\n")
        for name, value in cookies_dict.items():
            cookie_file.write(f".facebook.com\tTRUE\t/\tTRUE\t0\t{name}\t{value}\n")
        cookie_file.close()
        opts["cookiefile"] = cookie_file.name

    return opts, cookie_file.name if cookie_file else None


def _extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None) -> dict:
    opts, cookie_path = _build_ydl_opts(proxy, cookie)

    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            info = ydl.extract_info(url, download=False)
    finally:
        if cookie_path and os.path.exists(cookie_path):
            os.unlink(cookie_path)

    if not info:
        raise Exception("yt-dlp returned empty result")

    return map_fb_response(info)


# ============================================
# ROUTES
# ============================================

@app.get("/health")
def health():
    return {"status": "ok", "service": "fb-extractor", "version": "2.0.0"}


@app.get("/extract", summary="Extract media from Facebook video/reel")
def extract(url: str, proxy: Optional[str] = None, cookie: Optional[str] = None):
    if not _is_facebook_url(url):
        raise HTTPException(status_code=400, detail="Not a valid Facebook URL")

    try:
        logger.info(f"[Extract] yt-dlp attempt for {url}")
        result = _extract(url, proxy=proxy, cookie=cookie)
        logger.info(f"[Extract] yt-dlp success for {url}")
        return result
    except Exception as e:
        logger.error(f"[Extract] yt-dlp failed for {url}: {e}")
        raise HTTPException(status_code=400, detail=f"Extraction failed: {e}")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8002)
