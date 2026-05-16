"""
TTS Dubbing service — Microsoft Edge TTS qua thư viện edge-tts.

POST /submit       { voice, utterances: [{start, end, text}] }   → { job_id }
GET  /status/{id}  → { state, progress, completed, total, output_url? }
GET  /download/{id}                                              → MP3 stream
GET  /                                                            → HTML upload UI
GET  /health                                                      → ok

=== Tư duy chính ===
- Mỗi job chạy ở background (FastAPI BackgroundTasks), trả job_id ngay.
- Worker 3 bước: (1) tính rate per utterance, (2) synth song song có
  semaphore, (3) assemble bằng pydub: overlay segment lên silence track
  ở đúng vị trí start.
- Cache TTS theo md5(text+voice+rate) → tái dùng segment giống nhau.
- Job state lưu in-memory (MVP). Restart = mất state đang chạy.
"""
from __future__ import annotations

import asyncio
import hashlib
import os
import re
import time
import traceback
import uuid
from html import escape as html_escape
from pathlib import Path
from typing import Optional

import edge_tts
from fastapi import BackgroundTasks, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, HTMLResponse
from pydantic import BaseModel
from pydub import AudioSegment, effects
from pydub.silence import detect_leading_silence

# ---------- Config ----------
LISTEN_PORT = int(os.getenv("PORT", "8500"))
OUTPUT_DIR = Path(os.getenv("OUTPUT_DIR", "/tmp/dubbing-output"))
CACHE_DIR = Path(os.getenv("CACHE_DIR", "/tmp/dubbing-cache"))
TTS_CONCURRENCY = int(os.getenv("TTS_CONCURRENCY", "4"))

# === Sync tuning ===
# Thay vì ước lượng rate theo công thức char/s (hay sai vì mỗi voice khác
# tốc độ tự nhiên), giờ MEASURE-AND-RETRY: synth → đo duration → nếu vượt
# slot thì re-synth ở rate cao hơn. RETRY_RATES là dãy rate sẽ thử lần lượt.
# Lần đầu "+0%" (giọng tự nhiên) — 90% câu fit ngay, không tốn synth lại.
RETRY_RATES = [r.strip() for r in os.getenv("RETRY_RATES", "+0%,+25%,+50%,+75%").split(",")]
# Dung sai khi so duration vs slot — 0.2s để bù dao động đo và padding nhỏ.
SLOT_TOLERANCE_SEC = float(os.getenv("SLOT_TOLERANCE_SEC", "0.2"))
# Trim silence Edge TTS chèn ~100-300ms ở đầu/cuối mỗi segment. Threshold dB
# dưới -40 coi như silent — đủ an toàn cho voice Edge (loud, không thì thầm).
SILENCE_TRIM_DB = float(os.getenv("SILENCE_TRIM_DB", "-40"))
# Sau khi retry hết rate mà vẫn over slot, thử atempo stretch tới 1.20x.
# atempo giữ pitch (khác với rate "+75%" đổi pitch nghe lạ); vượt 1.25x thì
# giọng nghe ép. Cap 1.20 là chuẩn của VideoLingo, vẫn tự nhiên.
ATEMPO_MAX = float(os.getenv("ATEMPO_MAX", "1.20"))
# Cross-fade nhỏ ở 2 đầu mỗi segment để bỏ tiếng "click" khi overlay nối tiếp
# nhau. 30ms vừa đủ — dài hơn ngấm vào nội dung speech.
CROSSFADE_MS = int(os.getenv("CROSSFADE_MS", "30"))

# Cleanup TTL (giây). Output đã render xóa sau 1h, cache TTS segments giữ
# lâu hơn (24h) vì cache-hit rate cao khi reuse text/voice giống nhau.
OUTPUT_TTL_SEC = int(os.getenv("OUTPUT_TTL_SEC", "3600"))
CACHE_TTL_SEC = int(os.getenv("CACHE_TTL_SEC", "3600"))
CLEANUP_INTERVAL_SEC = int(os.getenv("CLEANUP_INTERVAL_SEC", "600"))

# Public-facing URL building. VPS-agent injects BASE_DOMAIN ("ytconvert.org")
# and DOWNLOAD_SUBDOMAIN ("vps-xxxxxx") into the container env; PATH_PREFIX
# matches the nginx location ("/tts"). When set, /submit and /status return
# absolute URLs the client can hit directly without round-tripping the hub.
BASE_DOMAIN = os.getenv("BASE_DOMAIN", "")
DOWNLOAD_SUBDOMAIN = os.getenv("DOWNLOAD_SUBDOMAIN", "")
PATH_PREFIX = os.getenv("PATH_PREFIX", "/tts").rstrip("/")
PUBLIC_BASE_URL = (
    f"https://{DOWNLOAD_SUBDOMAIN}.{BASE_DOMAIN}"
    if BASE_DOMAIN and DOWNLOAD_SUBDOMAIN
    else ""
)


def _public_url(internal_path: str) -> str:
    """Map an internal handler path ("/status/abc") to the externally
    routable URL ("https://vps-xxx.ytconvert.org/tts/status/abc") when the
    container knows its public hostname; otherwise fall back to the prefixed
    relative path for local/dev runs.
    """
    path = f"{PATH_PREFIX}{internal_path}"
    return f"{PUBLIC_BASE_URL}{path}" if PUBLIC_BASE_URL else path


OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
CACHE_DIR.mkdir(parents=True, exist_ok=True)


# ---------- Models ----------
class Utterance(BaseModel):
    start: float
    end: float
    text: str


class SubmitRequest(BaseModel):
    voice: str
    utterances: list[Utterance]


class Job:
    """In-memory job state."""

    __slots__ = ("id", "state", "progress", "error", "output_path", "total", "completed", "created_at")

    def __init__(self, job_id: str) -> None:
        self.id = job_id
        self.state = "processing"  # processing → done | failed
        self.progress = 0.0
        self.error: Optional[str] = None
        self.output_path: Optional[Path] = None
        self.total = 0
        self.completed = 0
        self.created_at = time.time()


JOBS: dict[str, Job] = {}
JOBS_LOCK = asyncio.Lock()


# ---------- App ----------
app = FastAPI(title="Edge TTS Dubbing")

# CORS mở cho mọi origin để client local (file://, localhost) gọi thẳng VPS
# không bị browser block. Service không lưu auth cookie nên allow_credentials=False
# là đủ an toàn — endpoints không hỗ trợ auth, payload đã public.
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=False,
    allow_methods=["*"],
    allow_headers=["*"],
    max_age=86400,
)


# Background cleanup task — định kỳ xoá output cũ + cache cũ + job state cũ.
async def _cleanup_loop() -> None:
    while True:
        try:
            now = time.time()
            # 1. Xoá output files cũ + remove job khỏi state map.
            async with JOBS_LOCK:
                stale = [jid for jid, j in JOBS.items() if now - j.created_at > OUTPUT_TTL_SEC]
                for jid in stale:
                    j = JOBS.pop(jid, None)
                    if j and j.output_path and j.output_path.exists():
                        try:
                            j.output_path.unlink()
                        except OSError:
                            pass
            # 2. Xoá cache MP3 segments lâu hơn (cache-hit valuable).
            for f in CACHE_DIR.glob("*.mp3"):
                try:
                    if now - f.stat().st_mtime > CACHE_TTL_SEC:
                        f.unlink()
                except OSError:
                    continue
        except Exception as e:  # noqa: BLE001
            print(f"[cleanup] error: {e}")
        await asyncio.sleep(CLEANUP_INTERVAL_SEC)


@app.on_event("startup")
async def _start_cleanup() -> None:
    asyncio.create_task(_cleanup_loop())


@app.get("/", response_class=HTMLResponse)
async def index() -> str:
    """Trang upload đơn giản — paste JSON utterances, chọn voice, submit."""
    html_path = Path(__file__).parent / "index.html"
    return html_path.read_text(encoding="utf-8")


@app.get("/health")
async def health() -> dict:
    # vps-agent đọc field "version" để hiện trạng thái Ready trên hub dashboard.
    return {"status": "ok", "version": "1.0.0"}


# Lazy 1-hour cache for the voice list. Microsoft's catalog only changes when
# a new voice is published, so refetching on every call would waste a WebSocket
# round-trip for nothing.
_VOICES_CACHE: Optional[list[dict]] = None
_VOICES_CACHE_AT = 0.0
_VOICES_CACHE_TTL = 3600
_VOICES_LOCK = asyncio.Lock()


async def _get_voices() -> list[dict]:
    global _VOICES_CACHE, _VOICES_CACHE_AT
    if _VOICES_CACHE is not None and time.time() - _VOICES_CACHE_AT < _VOICES_CACHE_TTL:
        return _VOICES_CACHE
    async with _VOICES_LOCK:
        # Double-check after acquiring the lock (another coroutine may have
        # populated the cache while we were waiting).
        if _VOICES_CACHE is not None and time.time() - _VOICES_CACHE_AT < _VOICES_CACHE_TTL:
            return _VOICES_CACHE
        raw = await edge_tts.list_voices()
        _VOICES_CACHE = [
            {"name": v["ShortName"], "gender": v["Gender"], "locale": v["Locale"]}
            for v in raw
        ]
        _VOICES_CACHE_AT = time.time()
    return _VOICES_CACHE


@app.get("/voices")
async def voices(locale: str = "") -> list[dict]:
    """List supported voices. Optional ?locale=vi-VN filter (case-insensitive)."""
    vs = await _get_voices()
    if locale:
        loc = locale.lower()
        vs = [v for v in vs if v["locale"].lower() == loc]
    return vs


@app.post("/submit")
async def submit(req: SubmitRequest, bg: BackgroundTasks) -> dict:
    if not req.utterances:
        raise HTTPException(400, "utterances required")
    if not req.voice.strip():
        raise HTTPException(400, "voice required")

    job_id = uuid.uuid4().hex[:12]
    job = Job(job_id)
    job.total = len(req.utterances)
    async with JOBS_LOCK:
        JOBS[job_id] = job
    bg.add_task(run_job, job_id, req)
    return {
        "job_id": job_id,
        "status_url": _public_url(f"/status/{job_id}"),
        "download_url": _public_url(f"/download/{job_id}"),
    }


@app.get("/status/{job_id}")
async def status(job_id: str) -> dict:
    async with JOBS_LOCK:
        job = JOBS.get(job_id)
    if not job:
        raise HTTPException(404, "job not found")
    return {
        "job_id": job_id,
        "state": job.state,
        "progress": round(job.progress, 3),
        "total": job.total,
        "completed": job.completed,
        "error": job.error,
        "output_url": _public_url(f"/download/{job_id}") if job.state == "done" else None,
    }


@app.get("/download/{job_id}")
async def download(job_id: str):
    async with JOBS_LOCK:
        job = JOBS.get(job_id)
    if not job:
        raise HTTPException(404, "job not found")
    if job.state != "done" or not job.output_path:
        raise HTTPException(425, f"job state: {job.state}")
    return FileResponse(
        path=job.output_path,
        media_type="audio/mp4",
        filename=f"{job_id}.m4a",
    )


# ---------- Text Pre-Process ----------
# 2 việc trước khi gửi vào Edge TTS:
#   1. NORMALIZE — chuyển số/%, ngày, $ thành text đầy đủ ("5.2%" → "5 phẩy 2
#      phần trăm"). Edge TTS đôi khi spell-out hoặc skip — explicit là an toàn.
#   2. SSML WRAP — bọc <break time="..."/> ở dấu câu để pause tự nhiên, và
#      <lang xml:lang="en-US"> cho từ tiếng Anh quen thuộc (iPhone, YouTube...)
#      để voice tiếng Việt không spell-out kiểu "i-pho-nê".
#
# Whitelist brand-term thay vì auto-detect (langdetect không reliable cho từ
# đơn, false positive cao). User extend được list khi cần thêm brand.

_EN_BRAND_TERMS = {
    # Tech brands hay xuất hiện trong content Việt
    "iphone", "ipad", "macbook", "mac", "ios", "ipados", "macos",
    "youtube", "google", "apple", "samsung", "microsoft", "sony", "xiaomi",
    "chatgpt", "openai", "gpt", "claude", "gemini",
    "facebook", "instagram", "tiktok", "twitter", "snapchat",
    "windows", "android", "linux",
    "airpods", "airpod", "watch", "vision",
    "pro", "max", "mini", "plus", "ultra", "series", "neural",
    # Tech terms
    "ssd", "hdd", "ram", "rom", "cpu", "gpu", "usb", "hdmi", "wifi", "bluetooth",
    "ai", "ml", "vr", "ar", "api", "url",
}


def _voice_lang(voice: str) -> str:
    """Extract ISO 639-1 lang code from edge-tts voice name.
    'vi-VN-HoaiMyNeural' → 'vi'
    'en-US-JennyNeural'  → 'en'
    """
    return voice.split("-")[0].lower() if voice else "en"


def normalize_text(text: str, lang: str) -> str:
    """Expand số / %, $ / ngày / abbreviation → text đầy đủ để Edge TTS đọc
    tự nhiên thay vì spell-out hoặc skip. Chỉ xử lý case STRUCTURAL (có ký
    hiệu đặc biệt như %, $, /), không động đến số đơn lẻ vì Edge TTS Vietnamese
    voice đọc số trong context khá ổn.
    """
    if not text:
        return text
    out = text

    if lang == "vi":
        # Currency: $50 / $1.5 → "50 đô la"
        out = re.sub(r"\$\s*(\d+(?:[.,]\d+)?)", r"\1 đô la", out)
        # Percent decimal: 5.2% → "5 phẩy 2 phần trăm"
        out = re.sub(
            r"(\d+)[.,](\d+)\s*%",
            lambda m: f"{m.group(1)} phẩy {m.group(2)} phần trăm",
            out,
        )
        # Percent integer: 50% → "50 phần trăm"
        out = re.sub(r"(\d+)\s*%", r"\1 phần trăm", out)
        # Date dd/mm/yyyy: 15/3/2024 → "ngày 15 tháng 3 năm 2024"
        out = re.sub(
            r"\b(\d{1,2})/(\d{1,2})/(\d{2,4})\b",
            r"ngày \1 tháng \2 năm \3",
            out,
        )
        # Date dd/mm
        out = re.sub(r"\b(\d{1,2})/(\d{1,2})\b", r"ngày \1 tháng \2", out)
        # K suffix: 200k → "200 nghìn"
        out = re.sub(r"\b(\d+)k\b", r"\1 nghìn", out, flags=re.IGNORECASE)
        # Hz: "60 Hz" / "60Hz" → "60 héc"
        out = re.sub(r"\b(\d+)\s*Hz\b", r"\1 héc", out, flags=re.IGNORECASE)
    elif lang == "en":
        out = re.sub(r"\$\s*(\d+(?:[.,]\d+)?)", r"\1 dollars", out)
        out = re.sub(r"(\d+(?:[.,]\d+)?)\s*%", r"\1 percent", out)
        out = re.sub(r"\bDr\.", "Doctor", out)
        out = re.sub(r"\bMr\.", "Mister", out)
        out = re.sub(r"\bMrs\.", "Misses", out)
        out = re.sub(r"\betc\.", "et cetera", out)
        out = re.sub(r"\b(\d+)k\b", r"\1 thousand", out, flags=re.IGNORECASE)
        out = re.sub(r"\b(\d+)\s*Hz\b", r"\1 hertz", out, flags=re.IGNORECASE)

    return out


def _wrap_brand_terms(text: str) -> str:
    """Tìm brand terms trong whitelist và bọc <lang xml:lang='en-US'> để
    Edge TTS Vietnamese voice tạm switch sang giọng Anh đọc đúng chính tả.
    Case-insensitive match nhưng giữ nguyên case gốc trong output.
    """
    pattern = r"\b(" + "|".join(re.escape(t) for t in _EN_BRAND_TERMS) + r")\b"
    return re.sub(
        pattern,
        lambda m: f'<lang xml:lang="en-US">{m.group(1)}</lang>',
        text,
        flags=re.IGNORECASE,
    )


def wrap_ssml(text: str, voice_lang: str = "vi") -> str:
    """Wrap text trong <speak>...</speak> với 2 enhancement:
      • <break time="..."/> sau dấu câu để pause tự nhiên
      • <lang xml:lang="en-US"> cho brand terms (chỉ khi voice_lang='vi')

    Escape XML chars trước khi inject tag để tránh hỏng SSML khi text có '<>&'.
    """
    if not text:
        return text
    # Escape XML special chars TRƯỚC khi inject tag (tag mình thêm sau ko bị escape)
    safe = html_escape(text, quote=False)

    # Break sau dấu kết câu (.?!) → 400ms reset intonation
    safe = re.sub(r"([.?!])(\s|$)", r'\1<break time="400ms"/>\2', safe)
    # Break sau dấu phẩy/ngắt nhịp → 200ms breath
    safe = re.sub(r"([,;:])(\s|$)", r'\1<break time="200ms"/>\2', safe)

    # Brand terms — chỉ wrap khi voice là Vietnamese (Edge TTS English voice
    # đọc brand terms native rồi, không cần <lang> tag).
    if voice_lang == "vi":
        safe = _wrap_brand_terms(safe)

    return f"<speak>{safe}</speak>"


# ---------- Worker ----------
def _trim_silence(seg: AudioSegment) -> AudioSegment:
    """Cắt silence đầu + cuối segment. Edge TTS chèn ~100-300ms padding mỗi
    bên — không trim thì 100 câu liên tiếp lệch dồn về cảm giác sync.
    Threshold -40dB an toàn cho voice Edge (loud); nếu segment toàn silent
    (hỏng) thì giữ nguyên để gọi biết mà error.
    """
    start_trim = detect_leading_silence(seg, silence_threshold=SILENCE_TRIM_DB)
    end_trim = detect_leading_silence(seg.reverse(), silence_threshold=SILENCE_TRIM_DB)
    if start_trim + end_trim >= len(seg):
        return seg
    return seg[start_trim : len(seg) - end_trim]


async def _synth_raw(text: str, voice: str, rate: str) -> Path:
    """Synth 1 rate cụ thể, cache theo (text, voice, rate). Không trim."""
    cache_key = hashlib.md5(f"{text}|{voice}|{rate}".encode()).hexdigest()
    cache_path = CACHE_DIR / f"{cache_key}.mp3"
    if not cache_path.exists():
        await edge_tts.Communicate(text, voice, rate=rate).save(str(cache_path))
    return cache_path


async def _atempo_stretch(seg: AudioSegment, factor: float) -> AudioSegment:
    """Speed up segment bằng ffmpeg atempo filter (giữ pitch). Khác với rate
    cao của Edge TTS — rate đổi pitch (giọng cao the thé), atempo chỉ thay
    tempo. ffmpeg atempo cap 0.5-2.0; mình thực tế dùng ≤1.20.

    Implementation: export segment ra file tạm → ffmpeg subprocess → load lại.
    Phép tử subprocess vì pydub không có atempo built-in.
    """
    if factor <= 1.0:
        return seg  # no stretch needed
    # Atempo factor được validate trước khi gọi, nhưng safety:
    factor = min(factor, 2.0)

    tmp_in = CACHE_DIR / f"_atempo_in_{uuid.uuid4().hex[:8]}.mp3"
    tmp_out = CACHE_DIR / f"_atempo_out_{uuid.uuid4().hex[:8]}.mp3"
    try:
        seg.export(str(tmp_in), format="mp3")
        proc = await asyncio.create_subprocess_exec(
            "ffmpeg", "-y", "-i", str(tmp_in),
            "-af", f"atempo={factor:.3f}",
            "-loglevel", "error",
            str(tmp_out),
            stdout=asyncio.subprocess.DEVNULL,
            stderr=asyncio.subprocess.PIPE,
        )
        _, stderr = await proc.communicate()
        if proc.returncode != 0:
            print(f"[atempo] ffmpeg failed: {stderr.decode(errors='replace')[:200]}")
            return seg
        return AudioSegment.from_mp3(str(tmp_out))
    finally:
        for p in (tmp_in, tmp_out):
            try:
                p.unlink(missing_ok=True)
            except OSError:
                pass


async def _synth_fit(
    text: str, voice: str, slot_seconds: float
) -> tuple[AudioSegment, str]:
    """Measure-and-retry: thử RETRY_RATES lần lượt. Synth → trim silence →
    đo duration. Nếu vừa slot (± tolerance) thì dùng. Hết retry vẫn over →
    clip cuối segment (giữ phần đầu câu, mất phần cuối).

    Pre-process text MỘT LẦN trước retry loop (rate-independent):
      • normalize số/%/ngày → text đầy đủ
      • SSML wrap với <break> ở dấu câu + <lang> tag cho brand terms

    Trả về (segment đã trim, rate dùng) — rate cho logging/debug, segment
    sẵn sàng overlay vào timeline.
    """
    lang = _voice_lang(voice)
    processed = wrap_ssml(normalize_text(text, lang), lang)

    last_seg: Optional[AudioSegment] = None
    last_rate = RETRY_RATES[-1]

    for rate in RETRY_RATES:
        raw_path = await _synth_raw(processed, voice, rate)
        seg = _trim_silence(AudioSegment.from_mp3(str(raw_path)))
        last_seg, last_rate = seg, rate
        if seg.duration_seconds <= slot_seconds + SLOT_TOLERANCE_SEC:
            return seg, rate

    # Hết retry rate mà vẫn dài → thử atempo stretch (giữ pitch). Atempo chỉ
    # dùng được khi ratio ≤ ATEMPO_MAX (1.20x), vượt thì giọng nghe ép.
    assert last_seg is not None
    ratio = last_seg.duration_seconds / max(slot_seconds, 0.001)
    if ratio <= ATEMPO_MAX:
        stretched = await _atempo_stretch(last_seg, ratio)
        if stretched.duration_seconds <= slot_seconds + SLOT_TOLERANCE_SEC:
            return stretched, f"{last_rate}+atempo{ratio:.2f}"
        # Atempo không đạt expected (ffmpeg lỗi hoặc rounding) → tiếp tục clip
        last_seg = stretched

    # Cuối cùng: clip cuối. Giữ đầu vì câu mở thường quan trọng hơn câu kết;
    # mất 1-2 từ cuối chấp nhận hơn là đè vào câu kế.
    clipped = last_seg[: int(slot_seconds * 1000)]
    return clipped, f"{last_rate}+clip"


async def run_job(job_id: str, req: SubmitRequest) -> None:
    job = JOBS[job_id]
    try:
        # === Bước 1: Plan — bỏ utterance rỗng ===
        plan: list[Utterance] = []
        for u in req.utterances:
            text = (u.text or "").strip()
            if text:
                plan.append(Utterance(start=u.start, end=u.end, text=text))

        if not plan:
            raise ValueError("no non-empty utterances to synthesize")

        # === Bước 2: Synth song song với semaphore (measure-and-retry) ===
        sem = asyncio.Semaphore(TTS_CONCURRENCY)
        completed_count = 0

        async def synth_one(u: Utterance) -> tuple[Utterance, AudioSegment, str]:
            nonlocal completed_count
            async with sem:
                seg, rate = await _synth_fit(u.text, req.voice, u.end - u.start)
            completed_count += 1
            job.completed = completed_count
            # 90% cho synth, 10% còn lại cho assemble.
            job.progress = completed_count / len(plan) * 0.9
            return u, seg, rate

        results = await asyncio.gather(*(synth_one(u) for u in plan))

        # === Bước 3: Assemble — overlay segment ĐÃ TRIM vào silence track ===
        # Vì đã trim silence đầu/cuối + đảm bảo fit slot ở bước 2, mỗi segment
        # đặt đúng u.start là speech bắt đầu ngay đó, không drift.
        # Mỗi segment có fade_in/fade_out CROSSFADE_MS để bỏ "click" boundary
        # khi câu trước/sau dính sát nhau.
        total_ms = int(max(u.end for u in req.utterances) * 1000)
        track = AudioSegment.silent(duration=total_ms)
        clipped_count = 0
        stretched_count = 0
        for u, seg, rate in results:
            if CROSSFADE_MS > 0 and len(seg) > 2 * CROSSFADE_MS:
                seg = seg.fade_in(CROSSFADE_MS).fade_out(CROSSFADE_MS)
            track = track.overlay(seg, position=int(u.start * 1000))
            if rate.endswith("+clip"):
                clipped_count += 1
            elif "+atempo" in rate:
                stretched_count += 1

        if clipped_count or stretched_count:
            print(
                f"[job {job_id}] {clipped_count} clipped, {stretched_count} atempo-stretched "
                f"out of {len(results)} utterances"
            )

        # Volume normalize TOÀN TRACK — peak ~ -1dBFS. Tránh câu to câu nhỏ
        # khi nhiều segment merge lại (mỗi segment volume hơi khác do TTS).
        track = effects.normalize(track)

        # Output M4A (AAC) — drop-in cho mp4 video, merge bằng `-c copy`
        # không cần re-encode (nhanh hơn ~10-30x so với MP3-in-MP4).
        output_path = OUTPUT_DIR / f"{job_id}.m4a"
        track.export(str(output_path), format="ipod", codec="aac", bitrate="128k")

        job.output_path = output_path
        job.progress = 1.0
        job.state = "done"
    except Exception as e:
        job.state = "failed"
        job.error = f"{type(e).__name__}: {e}"
        traceback.print_exc()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=LISTEN_PORT, log_level="info")
