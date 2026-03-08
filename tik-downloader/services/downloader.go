package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"tik-downloader/config"
	"tik-downloader/utils"
	"time"
)

// Download downloads a TikTok video/audio file with retry + URL refresh.
// ALWAYS goes through WARP proxy (100% cloudflare).
//
// Strategy per attempt:
//  1. Try download via WARP Proxy
//  2. If 4xx/5xx → re-extract (refresh URL) and retry from step 1
//
// videoID is used for URL refresh (re-extraction).
// cookie is passed for CDN auth on each request.
func Download(ctx context.Context, jobID string, videoID string, downloadURL string, filename string, cookie string) (int64, error) {
	destPath := filepath.Join(utils.GetJobDir(jobID), filename)
	maxRetries := config.MaxRetries

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("[%s] Retry %d/%d — refreshing URL via re-extract...\n", jobID, attempt, maxRetries)

			// Re-extract to get a fresh signed URL
			newData, err := Extract(videoID)
			if err != nil {
				fmt.Printf("[%s] Re-extract failed: %v\n", jobID, err)
				lastErr = fmt.Errorf("re-extract failed: %w", err)
				time.Sleep(config.RetryDelay)
				continue
			}

			// Pick the right URL based on filename
			if strings.Contains(filename, "mp3") || strings.Contains(filename, "audio") {
				downloadURL = newData.Data.MusicURL
			} else {
				downloadURL = newData.Data.Downloads
			}

			if downloadURL == "" {
				lastErr = fmt.Errorf("no download URL after re-extract")
				time.Sleep(config.RetryDelay)
				continue
			}

			// Update cookie for CDN auth
			cookie = newData.Params.Cookie

			fmt.Printf("[%s] Got fresh URL after re-extract\n", jobID)
		}

		fmt.Printf("[%s] Attempt %d — WARP Proxy\n", jobID, attempt+1)

		if config.WARPProxyURL == "" {
			return 0, fmt.Errorf("WARP proxy is not configured, but strictly required")
		}

		written, err := doDownload(ctx, downloadURL, destPath, cookie)
		if err == nil {
			fmt.Printf("[%s] ✓ Downloaded %s via WARP Proxy (%.2f MB)\n", jobID, filename, float64(written)/1024/1024)
			return written, nil
		}

		fmt.Printf("[%s] WARP Proxy failed: %v\n", jobID, err)
		lastErr = err

		time.Sleep(config.RetryDelay)
	}

	return 0, fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}

// doDownload performs a single HTTP download attempt to destPath.
// Always routes through config.DownloadClient (WARP proxy).
func doDownload(ctx context.Context, downloadURL string, destPath string, cookie string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// TikTok CDN requires browser-like headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.tiktok.com/")
	req.Header.Set("Accept", "*/*")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	// ALWAYS use DownloadClient (which has the WARP proxy)
	client := config.DownloadClient

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Write to temp file first, then rename (atomic)
	tmpPath := destPath + ".tmp"
	outFile, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}

	buf := make([]byte, config.BufferSize)
	written, err := io.CopyBuffer(outFile, resp.Body, buf)
	outFile.Close()

	if err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("download interrupted: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("rename failed: %w", err)
	}

	return written, nil
}
