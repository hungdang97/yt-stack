// Deepgram speech-to-text HTTP service.
//
// GET /transcribe?url=<audio-url>
//
// Returns RAW Deepgram Nova-3 response (full schema:
// results.channels[].alternatives[].words[], metadata, utterances, etc.).
// Caller feeds this directly into /api/caption for downstream chunking.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	listenAddr  = ":8502"
	httpTimeout = 10 * time.Minute
	// Tham số Deepgram Nova-3 cho caption quality cao:
	//   smart_format=true   → tự thêm punctuation, viết hoa, số/ngày/tiền
	//   filler_words=false  → bỏ "um", "uh", "you know" — caption sạch hơn
	//   utterances=true     → segment theo câu, có start/end per utterance
	//   utt_split=0.8       → pause ≥0.8s mới tách utterance mới (tránh
	//                          1 câu bị xé vụn vì người nói thở giữa câu)
	//   detect_language=true → auto-detect tiếng nguồn, không cần client báo
	deepgramURL = "https://api.deepgram.com/v1/listen" +
		"?model=nova-3" +
		"&detect_language=true" +
		"&utterances=true" +
		"&utt_split=0.8" +
		"&smart_format=true" +
		"&filler_words=false"
)

// Failover order: try apiKeys[0]; on auth/rate-limit/5xx move to next.
var apiKeys = []string{
	"0cacb1ef2ea705ab110d133d25b2afc6e266fdbf",
	"3eca08b28da7ed4ca62b2f890f57ea71ed7498eb",
	"0409ecc2cb1cb6a8b2b59fb65a798f6f4ce32b6c",
}


func main() {
	addr := listenAddr
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	http.HandleFunc("/transcribe", handleTranscribe)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		// vps-agent đọc "version" để hiện Ready trên hub dashboard.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})
	log.Printf("deepgram listening on %s", addr)
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

func handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	audioURL := extractAudioURL(r)
	if audioURL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	raw, err := callDeepgramRaw(audioURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "deepgram: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

// extractAudioURL returns the audio URL from `?url=...`. If the value was
// properly URL-encoded, the standard parser returns one key with one value
// and we use it. Otherwise the URL's own `&`s leaked into the query parser;
// we recover by taking everything after `url=` in r.URL.RawQuery so any
// inner percent-encoding the proxy needs stays intact.
func extractAudioURL(r *http.Request) string {
	q := r.URL.Query()
	if len(q) == 1 && len(q["url"]) == 1 {
		return strings.TrimSpace(q["url"][0])
	}
	raw := r.URL.RawQuery
	i := strings.Index(raw, "url=")
	if i < 0 || (i > 0 && raw[i-1] != '&') {
		return ""
	}
	return raw[i+len("url="):]
}

// callDeepgramRaw POSTs the audio URL to Deepgram and returns the raw
// response body bytes. Tries API keys in order; failover only on auth/
// rate-limit/server errors. Other 4xx (URL problems, bad audio) abort
// immediately so we don't burn keys on caller mistakes.
func callDeepgramRaw(audioURL string) ([]byte, error) {
	client := &http.Client{Timeout: httpTimeout}
	body, _ := json.Marshal(map[string]string{"url": audioURL})

	var lastErr error
	for i, key := range apiKeys {
		req, _ := http.NewRequest(http.MethodPost, deepgramURL, bytes.NewReader(body))
		req.Header.Set("Authorization", "Token "+key)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("key #%d transport error: %v", i+1, err)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		sErr := fmt.Errorf("status %d: %s", resp.StatusCode, snippet(respBody, 300))
		if shouldFailover(resp.StatusCode) {
			lastErr = sErr
			log.Printf("key #%d failed, trying next: %v", i+1, sErr)
			continue
		}
		return nil, sErr
	}
	if lastErr == nil {
		lastErr = errors.New("all api keys exhausted")
	}
	return nil, lastErr
}

func shouldFailover(status int) bool {
	return status == 401 || status == 403 || status == 429 || status >= 500
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func snippet(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
