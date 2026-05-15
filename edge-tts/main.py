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
- Job state lưu in-memory (MVP). Restart = mất queue.
"""
from __future__ import annotations

import asyncio
import hashlib
import os
import traceback
import uuid
from pathlib import Path
from typing import Optional

import edge_tts
from fastapi import BackgroundTasks, FastAPI, HTTPException
from fastapi.responses import FileResponse, HTMLResponse
from pydantic import BaseModel
from pydub import AudioSegment

# ---------- Config ----------
LISTEN_PORT = int(os.getenv("PORT", "8500"))
OUTPUT_DIR = Path(os.getenv("OUTPUT_DIR", "/tmp/dubbing-output"))
CACHE_DIR = Path(os.getenv("CACHE_DIR", "/tmp/dubbing-cache"))
TTS_CONCURRENCY = int(os.getenv("TTS_CONCURRENCY", "4"))
# Ước lượng tốc độ đọc tự nhiên — dùng để tính rate cần để text fit trong slot.
# 15 char/s là rough cho tiếng Việt; với tiếng Anh có thể cao hơn nhưng
# từ ngắn hơn nên trung bình tương đương.
NATURAL_CHARS_PER_SEC = float(os.getenv("CHARS_PER_SEC", "15"))
# Cap tốc độ tăng — quá nhanh thì giọng dị, người nghe khó tiếp nhận.
MAX_RATE_PCT = int(os.getenv("MAX_RATE_PCT", "50"))

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

    __slots__ = ("id", "state", "progress", "error", "output_path", "total", "completed")

    def __init__(self, job_id: str) -> None:
        self.id = job_id
        self.state = "queued"  # queued → processing → done | failed
        self.progress = 0.0
        self.error: Optional[str] = None
        self.output_path: Optional[Path] = None
        self.total = 0
        self.completed = 0


JOBS: dict[str, Job] = {}
JOBS_LOCK = asyncio.Lock()


# ---------- App ----------
app = FastAPI(title="Edge TTS Dubbing")


@app.get("/", response_class=HTMLResponse)
async def index() -> str:
    """Trang upload đơn giản — paste JSON utterances, chọn voice, submit."""
    html_path = Path(__file__).parent / "index.html"
    return html_path.read_text(encoding="utf-8")


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


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
    return {"job_id": job_id, "status_url": f"/status/{job_id}"}


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
        "output_url": f"/download/{job_id}" if job.state == "done" else None,
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
def _compute_rate(text: str, slot_seconds: float) -> str:
    """Tính rate cần để text vừa khít slot. Format edge-tts: '+N%' hoặc '+0%'."""
    if slot_seconds <= 0:
        return "+0%"
    natural = len(text) / NATURAL_CHARS_PER_SEC
    if natural <= slot_seconds:
        return "+0%"
    pct = min(MAX_RATE_PCT, int(round((natural / slot_seconds - 1) * 100)))
    return f"+{pct}%"


async def run_job(job_id: str, req: SubmitRequest) -> None:
    job = JOBS[job_id]
    job.state = "processing"
    try:
        # === Bước 1: Plan — bỏ utterance rỗng, tính rate cho mỗi utterance ===
        plan: list[tuple[Utterance, str, str]] = []
        for u in req.utterances:
            text = (u.text or "").strip()
            if not text:
                continue
            plan.append((u, text, _compute_rate(text, u.end - u.start)))

        if not plan:
            raise ValueError("no non-empty utterances to synthesize")

        # === Bước 2: Synth song song với semaphore ===
        sem = asyncio.Semaphore(TTS_CONCURRENCY)
        completed_count = 0

        async def synth(u: Utterance, text: str, rate: str) -> tuple[Utterance, Path]:
            nonlocal completed_count
            cache_key = hashlib.md5(f"{text}|{req.voice}|{rate}".encode()).hexdigest()
            cache_path = CACHE_DIR / f"{cache_key}.mp3"
            async with sem:
                if not cache_path.exists():
                    tts = edge_tts.Communicate(text, req.voice, rate=rate)
                    await tts.save(str(cache_path))
            completed_count += 1
            job.completed = completed_count
            # 90% cho synth, 10% còn lại cho assemble.
            job.progress = completed_count / len(plan) * 0.9
            return u, cache_path

        segments = await asyncio.gather(*(synth(u, t, r) for u, t, r in plan))

        # === Bước 3: Assemble — overlay từng segment vào silence track ===
        total_ms = int(max(u.end for u in req.utterances) * 1000)
        track = AudioSegment.silent(duration=total_ms)
        for u, seg_path in segments:
            seg = AudioSegment.from_mp3(str(seg_path))
            track = track.overlay(seg, position=int(u.start * 1000))

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
