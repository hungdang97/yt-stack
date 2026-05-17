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
import time
import traceback
import uuid
from pathlib import Path
from typing import Optional

import edge_tts
from fastapi import BackgroundTasks, FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, HTMLResponse
from pydantic import BaseModel, field_validator
from pydub import AudioSegment
from pydub.exceptions import CouldntDecodeError
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
def _parse_timestamp(v) -> float:
    """Chấp nhận nhiều format cho start/end:
      - float / int: 11.98
      - SRT  string: "00:00:11,980"
      - VTT  string: "00:00:11.980"
      - MM:SS string: "01:11.98"
      - Plain string float: "11.98"
    Trả về giây (float). Raise ValueError nếu không parse được.
    """
    if isinstance(v, (int, float)):
        return float(v)
    if not isinstance(v, str):
        raise ValueError(f"timestamp must be number or string, got {type(v).__name__}")
    s = v.strip().replace(",", ".")
    # Plain float ("11.98" / "11")
    try:
        return float(s)
    except ValueError:
        pass
    # Colon-separated: HH:MM:SS hoặc MM:SS
    parts = s.split(":")
    try:
        if len(parts) == 3:
            return float(parts[0]) * 3600 + float(parts[1]) * 60 + float(parts[2])
        if len(parts) == 2:
            return float(parts[0]) * 60 + float(parts[1])
    except ValueError:
        pass
    raise ValueError(f"cannot parse timestamp: {v!r}")


class Utterance(BaseModel):
    start: float
    end: float
    text: str

    @field_validator("start", "end", mode="before")
    @classmethod
    def _coerce_timestamp(cls, v):
        return _parse_timestamp(v)


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
            #    Dọn cả tmp file (kết thúc .tmp.mp3) — rơi rớt nếu synth fail.
            for f in CACHE_DIR.glob("*.mp3"):
                try:
                    age = now - f.stat().st_mtime
                    is_tmp = ".tmp" in f.name
                    # Tmp file: xoá nếu > 5 phút (đủ lâu để chắc chắn rơi rớt).
                    # Cache file: xoá theo TTL chuẩn.
                    if is_tmp and age > 300:
                        f.unlink()
                    elif not is_tmp and age > CACHE_TTL_SEC:
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


async def _synth_raw(text: str, voice: str, rate: str, force: bool = False) -> Path:
    """Synth 1 rate cụ thể, cache theo (text, voice, rate). Không trim.

    Atomic write + verify decode. Tránh 4 issue:
      1. Race: 2 utterance cùng (text, voice, rate) → cùng path; cùng save →
         byte trộn. Mỗi call ghi tmp unique → rename atomic, last writer wins
         nhưng file luôn nguyên vẹn.
      2. Partial write: TTS WebSocket fail giữa chừng → file dở. Tmp + rename
         đảm bảo cache_path chỉ tồn tại khi đã save xong.
      3. Empty output: voice/text lạ → file 0-byte. Reject nếu < 100 bytes.
      4. Output undecodable: byte hợp lệ nhưng không phải MPEG frame. Verify
         bằng AudioSegment.from_mp3() trước khi promote → đảm bảo cache_path
         lúc nào cũng decode được.

    `force=True` skip cache check — dùng để recover khi cache cũ corrupt.
    """
    cache_key = hashlib.md5(f"{text}|{voice}|{rate}".encode()).hexdigest()
    cache_path = CACHE_DIR / f"{cache_key}.mp3"

    if not force and cache_path.exists() and cache_path.stat().st_size > 100:
        return cache_path

    tmp_path = CACHE_DIR / f"{cache_key}.{uuid.uuid4().hex[:8]}.tmp.mp3"
    try:
        await edge_tts.Communicate(text, voice, rate=rate).save(str(tmp_path))
        if not tmp_path.exists() or tmp_path.stat().st_size <= 100:
            raise ValueError(f"empty/tiny TTS output for text {text[:60]!r}")
        # Verify decodable trước khi promote — nếu không decode được thì
        # file cache cũng vô dụng, vứt đi luôn.
        AudioSegment.from_mp3(str(tmp_path))
        tmp_path.replace(cache_path)
    except Exception:
        if tmp_path.exists():
            try:
                tmp_path.unlink()
            except OSError:
                pass
        raise
    return cache_path


async def _synth_fit(
    text: str, voice: str, slot_seconds: float
) -> tuple[AudioSegment, str]:
    """Measure-and-retry: thử RETRY_RATES lần lượt. Synth → trim silence →
    đo duration. Nếu vừa slot (± tolerance) thì dùng. Hết retry vẫn over →
    clip cuối segment (giữ phần đầu câu, mất phần cuối).

    Trả về (segment đã trim, rate dùng) — rate cho logging/debug, segment
    sẵn sàng overlay vào timeline.
    """
    last_seg: Optional[AudioSegment] = None
    last_rate = RETRY_RATES[-1]

    for rate in RETRY_RATES:
        raw_path = await _synth_raw(text, voice, rate)
        try:
            seg = _trim_silence(AudioSegment.from_mp3(str(raw_path)))
        except CouldntDecodeError:
            # File cache từ run cũ corrupt (trước khi có verify-on-write).
            # Force synth lại, lần này sẽ có verify nên đảm bảo decode được.
            raw_path = await _synth_raw(text, voice, rate, force=True)
            seg = _trim_silence(AudioSegment.from_mp3(str(raw_path)))
        last_seg, last_rate = seg, rate
        if seg.duration_seconds <= slot_seconds + SLOT_TOLERANCE_SEC:
            return seg, rate

    # Hết retry mà vẫn dài → clip cuối. Giữ đầu vì câu mở thường quan trọng
    # hơn câu kết; mất 1-2 từ cuối chấp nhận hơn là đè vào câu kế.
    assert last_seg is not None
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
        total_ms = int(max(u.end for u in req.utterances) * 1000)
        track = AudioSegment.silent(duration=total_ms)
        clipped_count = 0
        for u, seg, rate in results:
            track = track.overlay(seg, position=int(u.start * 1000))
            if rate.endswith("+clip"):
                clipped_count += 1

        if clipped_count:
            print(f"[job {job_id}] {clipped_count}/{len(results)} utterances clipped (text too long for slot even at max rate)")

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
