// Video render service — input: video URL + audio URL + .ass URL.
// Output: MP4 với subtitle ASS burn-in và audio mới.
//
//	POST /submit        body {video_url, audio_url, subtitle_url}  → {job_id}
//	GET  /status/{id}   → {state, progress, output_url?}
//	GET  /download/{id} → MP4 stream
//	GET  /health        → ok
//
// === Tư duy chính ===
// 3 input files tải song song → ffmpeg 1 lần để:
//  1. Lấy video stream từ file 0, audio stream từ file 1 (replace audio gốc).
//  2. Burn subtitle ASS vào video qua filter "ass=..." (giữ font/màu/positioning).
//  3. Re-encode H.264 + AAC (subtitle filter bắt buộc re-encode video).
//
// Job state lưu in-memory, output lưu /tmp/video-render/{job_id}/output.mp4.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr = ":8501"
	workRoot   = "/tmp/video-render"
	// Re-encode settings — fast preset = ~3-5x realtime, file size lớn hơn
	// ~10% so với medium, chất lượng dùng cho subtitle burn-in chấp nhận tốt.
	x264Preset = "fast"
	x264CRF    = "23"
	aacBitrate = "128k"

	// Cleanup: xoá job dir + state sau TTL.
	jobTTL          = 1 * time.Hour
	cleanupInterval = 10 * time.Minute
)

// Public URL building. VPS-agent inject BASE_DOMAIN ("ytconvert.org") và
// DOWNLOAD_SUBDOMAIN ("vps-xxxxxx") vào container env. PATH_PREFIX khớp với
// nginx location ("/render"). Khi đủ, response trả absolute URL để client
// gọi thẳng VPS không qua hub (giống pattern signed-URL của downloader).
var (
	publicBaseURL = buildPublicBase()
	pathPrefix    = strings.TrimRight(envOr("PATH_PREFIX", "/render"), "/")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func buildPublicBase() string {
	base := os.Getenv("BASE_DOMAIN")
	sub := os.Getenv("DOWNLOAD_SUBDOMAIN")
	if base != "" && sub != "" {
		return "https://" + sub + "." + base
	}
	return ""
}

// publicURL maps internal handler path ("/status/abc") → externally routable
// URL ("https://vps-xxx.ytconvert.org/render/status/abc") khi container biết
// hostname; nếu không có env, trả prefixed-relative cho local/dev.
func publicURL(internalPath string) string {
	full := pathPrefix + internalPath
	if publicBaseURL != "" {
		return publicBaseURL + full
	}
	return full
}

type renderRequest struct {
	VideoURL    string `json:"video_url"`
	AudioURL    string `json:"audio_url"`
	SubtitleURL string `json:"subtitle_url"`
}

type job struct {
	ID         string
	State      string  // processing | done | failed
	Progress   float64 // 0..1, mostly stage-based: 0.3 = downloaded, 1.0 = encoded
	Stage      string  // "downloading" | "rendering" | ""
	Error      string
	OutputPath string
	CreatedAt  time.Time
}

var (
	jobs   = map[string]*job{}
	jobsMu sync.Mutex
)

func main() {
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		log.Fatalf("mkdir work: %v", err)
	}
	addr := listenAddr
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	go cleanupLoop()

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/submit", handleRender)
	http.HandleFunc("/status/", handleStatus)
	http.HandleFunc("/download/", handleDownload)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		// vps-agent đọc field "version" để hiện trạng thái Ready trên hub dashboard.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})
	log.Printf("video-render listening on %s (work=%s)", addr, workRoot)
	log.Fatal(http.ListenAndServe(addr, corsMiddleware(http.DefaultServeMux)))
}

// corsMiddleware đáp ứng preflight + gắn header * cho mọi origin.
// Cho phép gọi từ file:// local và localhost mà không bị browser block.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	exe, err := os.Executable()
	if err == nil {
		htmlPath := filepath.Join(filepath.Dir(exe), "index.html")
		if data, err := os.ReadFile(htmlPath); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
	}
	// Fallback: cwd-relative (handy during `go run`).
	if data, err := os.ReadFile("index.html"); err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	writeError(w, http.StatusInternalServerError, "index.html not found")
}

func handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req renderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	// audio_url là optional — bỏ trống thì output video silent (subtitle-only).
	if req.VideoURL == "" || req.SubtitleURL == "" {
		writeError(w, http.StatusBadRequest, "video_url and subtitle_url are required (audio_url is optional)")
		return
	}

	id := newJobID()
	j := &job{ID: id, State: "processing", CreatedAt: time.Now()}
	jobsMu.Lock()
	jobs[id] = j
	jobsMu.Unlock()

	go run(id, req)

	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":       id,
		"status_url":   publicURL("/status/" + id),
		"download_url": publicURL("/download/" + id),
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/status/")
	j := getJob(id)
	if j == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	resp := map[string]any{
		"job_id":   id,
		"state":    j.State,
		"stage":    j.Stage,
		"progress": j.Progress,
		"error":    j.Error,
	}
	if j.State == "done" {
		resp["output_url"] = publicURL("/download/" + id)
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/download/")
	j := getJob(id)
	if j == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if j.State != "done" || j.OutputPath == "" {
		writeError(w, http.StatusTooEarly, "job state: "+j.State)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.mp4"`, id))
	http.ServeFile(w, r, j.OutputPath)
}

// run downloads the three inputs in parallel, then runs one ffmpeg pass that
// muxes the new audio in and burns the ASS subtitles onto the video. Subtitle
// burn requires re-encoding video; audio is re-encoded to AAC to keep MP4
// compatibility predictable.
func run(id string, req renderRequest) {
	j := getJob(id)
	if j == nil {
		return
	}

	jobDir := filepath.Join(workRoot, id)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		fail(j, "mkdir: "+err.Error())
		return
	}

	videoPath := filepath.Join(jobDir, "video.mp4")
	subPath := filepath.Join(jobDir, "subtitle.ass")
	// audio_url empty → skip download, render sẽ output silent video.
	audioPath := ""
	downloads := map[string]string{
		videoPath: req.VideoURL,
		subPath:   req.SubtitleURL,
	}
	if req.AudioURL != "" {
		audioPath = filepath.Join(jobDir, "audio.m4a")
		downloads[audioPath] = req.AudioURL
	}

	setState(j, "processing", "downloading", 0.05)
	if err := parallelDownload(downloads); err != nil {
		fail(j, "download: "+err.Error())
		return
	}
	setState(j, "processing", "rendering", 0.3)

	outputPath := filepath.Join(jobDir, "output.mp4")
	if err := renderVideo(j, videoPath, audioPath, subPath, outputPath); err != nil {
		fail(j, "ffmpeg: "+err.Error())
		return
	}

	j.OutputPath = outputPath
	setState(j, "done", "", 1.0)
}

func renderVideo(j *job, video, audio, subtitle, output string) error {
	// ffmpeg's "ass=" filter takes a file path. Escape ':' (drive letter on
	// Windows, or any path char ffmpeg parses as filter separator). On unix
	// paths normally fine, but escape defensively.
	subArg := strings.NewReplacer(`\`, `\\`, `:`, `\:`, `'`, `\'`).Replace(subtitle)

	// Probe video duration so we can map ffmpeg's "out_time_us" into a real
	// progress percentage (30%-100% band, leaving 0-30% for download).
	totalSec := probeDuration(video)
	if totalSec <= 0 {
		totalSec = 1 // avoid div-by-zero; progress will just clamp at 100%
	}

	// Build ffmpeg args động. audio == "" → output silent video (chỉ video
	// stream + subtitle burn-in, không có audio track).
	args := []string{"-y", "-i", video}
	if audio != "" {
		args = append(args, "-i", audio)
	}
	args = append(args, "-map", "0:v:0")
	if audio != "" {
		args = append(args, "-map", "1:a:0")
	}
	args = append(args,
		"-vf", "ass='"+subArg+"'",
		"-c:v", "libx264",
		"-preset", x264Preset,
		"-crf", x264CRF,
	)
	if audio != "" {
		args = append(args, "-c:a", "aac", "-b:a", aacBitrate)
	}
	args = append(args,
		"-movflags", "+faststart",
		// -progress pipe:1 makes ffmpeg emit machine-readable progress lines
		// (key=value, refreshed ~twice per second) on stdout so we can update
		// the job's progress bar in real time instead of jumping 30→100.
		"-progress", "pipe:1",
		output,
	)
	cmd := exec.Command("ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	progressDone := make(chan struct{})
	go trackFFmpegProgress(j, stdout, totalSec, progressDone)

	waitErr := cmd.Wait()
	<-progressDone // make sure we consumed all stdout before returning

	if waitErr != nil {
		return fmt.Errorf("%w; ffmpeg stderr (tail): %s", waitErr, tail(stderr.String(), 800))
	}
	return nil
}

// trackFFmpegProgress reads `key=value` lines from ffmpeg's -progress stream
// and maps `out_time_us` (the encoded timeline position, in microseconds)
// to a 0.3–1.0 job-progress band.
func trackFFmpegProgress(j *job, r io.Reader, totalSec float64, done chan<- struct{}) {
	defer close(done)
	const prefix = "out_time_us="
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		us, err := strconv.ParseInt(line[len(prefix):], 10, 64)
		if err != nil { // "N/A" or partial line during startup
			continue
		}
		ratio := float64(us) / 1e6 / totalSec
		if ratio < 0 {
			ratio = 0
		} else if ratio > 1 {
			ratio = 1
		}
		// 30 % is "downloads done"; ffmpeg fills the remaining 70 %.
		setState(j, "processing", "rendering", 0.3+0.7*ratio)
	}
}

// probeDuration returns the duration of the input file in seconds, or 0 on
// any error (caller decides how to handle the unknown).
func probeDuration(path string) float64 {
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return d
}

// parallelDownload fetches every (path → url) entry concurrently. Returns the
// first error encountered; remaining goroutines still finish to avoid leaking,
// but their results are discarded.
func parallelDownload(pairs map[string]string) error {
	errs := make(chan error, len(pairs))
	var wg sync.WaitGroup
	for path, url := range pairs {
		wg.Add(1)
		go func(path, url string) {
			defer wg.Done()
			errs <- downloadFile(url, path)
		}(path, url)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func downloadFile(url, path string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

// --- job state helpers (small mutex section per call so the HTTP read path
// never blocks the worker for long) ---

func newJobID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func getJob(id string) *job {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	return jobs[id]
}

func setState(j *job, state, stage string, progress float64) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j.State = state
	j.Stage = stage
	j.Progress = progress
}

func fail(j *job, msg string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j.State = "failed"
	j.Error = msg
	log.Printf("job %s failed: %s", j.ID, msg)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// cleanupLoop định kỳ xoá job dir + state cũ hơn jobTTL. Goroutine chạy
// suốt vòng đời container. Nếu một job đang in_progress khi tới TTL nó vẫn
// bị xoá — chấp nhận vì TTL đặt đủ dài (1h, lâu hơn 99% render time).
func cleanupLoop() {
	for range time.Tick(cleanupInterval) {
		cutoff := time.Now().Add(-jobTTL)
		jobsMu.Lock()
		var stale []string
		for id, j := range jobs {
			if j.CreatedAt.Before(cutoff) {
				stale = append(stale, id)
			}
		}
		for _, id := range stale {
			delete(jobs, id)
		}
		jobsMu.Unlock()
		for _, id := range stale {
			dir := filepath.Join(workRoot, id)
			if err := os.RemoveAll(dir); err != nil {
				log.Printf("cleanup %s: %v", id, err)
			}
		}
		if len(stale) > 0 {
			log.Printf("cleanup: removed %d stale jobs", len(stale))
		}
	}
}
