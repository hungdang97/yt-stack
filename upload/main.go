// File upload service — nhận file qua multipart POST, lưu disk, sinh public
// URL có signed id. Auto xoá sau 1h. Dùng cho .ass subtitles + bất kỳ file
// nhỏ nào client cần host để truyền vào các service khác (render, edge-tts).
//
//	POST /              multipart/form-data field "file"
//	                    → {"url": "...", "expires_at": 1714060800}
//	GET  /f/{id}.{ext}  → file content
//	GET  /health        → ok
//
// URL trả về là absolute (khi BASE_DOMAIN + DOWNLOAD_SUBDOMAIN set qua
// vps-agent) hoặc relative cho local/dev.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	listenAddr      = ":8504"
	storageDir      = "/tmp/upload-files"
	maxFileSize     = 10 << 20 // 10 MB — đủ cho .ass, .srt, .vtt
	fileTTL         = 1 * time.Hour
	cleanupInterval = 10 * time.Minute
)

var (
	publicBaseURL = buildPublicBase()
	pathPrefix    = strings.TrimRight(envOr("PATH_PREFIX", "/upload"), "/")
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func buildPublicBase() string {
	base := os.Getenv("BASE_DOMAIN")
	sub := os.Getenv("DOWNLOAD_SUBDOMAIN")
	if base != "" && sub != "" {
		return "https://" + sub + "." + base
	}
	return ""
}

func publicURL(relPath string) string {
	full := pathPrefix + relPath
	if publicBaseURL != "" {
		return publicBaseURL + full
	}
	return full
}

func main() {
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		log.Fatalf("mkdir storage: %v", err)
	}

	go cleanupLoop()

	addr := listenAddr
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	http.HandleFunc("/", handleUpload)
	http.HandleFunc("/f/", handleDownload)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})

	log.Printf("upload listening on %s (storage=%s, ttl=%s)", addr, storageDir, fileTTL)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method == http.MethodGet {
		// Tiny landing page để test nhanh không cần frontend.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><meta charset=utf-8>
<title>upload</title><body style="font:14px sans-serif;max-width:600px;margin:40px auto">
<h1>📤 Upload</h1>
<p>POST <code>multipart/form-data</code> field <code>file</code>:</p>
<pre>curl -F "file=@subtitle.ass" ` + pathPrefix + `/</pre>
<form method=post enctype=multipart/form-data action="">
  <input type=file name=file required>
  <button>Upload</button>
</form></body>`))
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	// Limit body size before parsing.
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge,
			"failed to parse multipart (max 10MB): "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field 'file' missing")
		return
	}
	defer file.Close()

	// Sanitize extension — chỉ giữ ký tự an toàn.
	ext := sanitizeExt(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".bin"
	}

	id, err := randomID(12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rand: "+err.Error())
		return
	}
	fname := id + ext
	path := filepath.Join(storageDir, fname)

	out, err := os.Create(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create: "+err.Error())
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		writeError(w, http.StatusInternalServerError, "write: "+err.Error())
		return
	}

	expiresAt := time.Now().Add(fileTTL).Unix()
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        publicURL("/f/" + fname),
		"expires_at": expiresAt,
		"size":       header.Size,
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/f/")
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}
	path := filepath.Join(storageDir, name)
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	defer f.Close()
	stat, _ := f.Stat()

	// Content-Type theo extension; default octet-stream.
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if stat != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}
	_, _ = io.Copy(w, f)
}

func cleanupLoop() {
	for range time.Tick(cleanupInterval) {
		cutoff := time.Now().Add(-fileTTL)
		entries, err := os.ReadDir(storageDir)
		if err != nil {
			continue
		}
		removed := 0
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				if err := os.Remove(filepath.Join(storageDir, e.Name())); err == nil {
					removed++
				}
			}
		}
		if removed > 0 {
			log.Printf("cleanup: removed %d files", removed)
		}
	}
}

func sanitizeExt(ext string) string {
	if ext == "" || len(ext) > 8 {
		return ""
	}
	for _, c := range ext[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return ""
		}
	}
	return strings.ToLower(ext)
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
