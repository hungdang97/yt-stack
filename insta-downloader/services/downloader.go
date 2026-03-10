package services

import (
	"context"
	"fmt"
	"insta-downloader/config"
	"insta-downloader/utils"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Download downloads a file via WARP proxy with retry + URL refresh.
func Download(ctx context.Context, jobID string, postURL string, downloadURL string, filename string) (int64, error) {
	destPath := filepath.Join(utils.GetJobDir(jobID), filename)
	maxRetries := config.MaxRetries

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("[%s] Retry %d/%d — refreshing URL via re-extract...\n", jobID, attempt, maxRetries)

			newData, err := Extract(postURL)
			if err != nil {
				fmt.Printf("[%s] Re-extract failed: %v\n", jobID, err)
				lastErr = fmt.Errorf("re-extract failed: %w", err)
				time.Sleep(config.RetryDelay)
				continue
			}

			// Pick the right URL based on filename
			if strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".png") {
				downloadURL = newData.GetImageURL()
			} else {
				downloadURL = newData.GetVideoURL()
			}

			if downloadURL == "" {
				lastErr = fmt.Errorf("no download URL after re-extract")
				time.Sleep(config.RetryDelay)
				continue
			}

			fmt.Printf("[%s] Got fresh URL after re-extract\n", jobID)
		}

		fmt.Printf("[%s] Attempt %d — WARP Proxy\n", jobID, attempt+1)

		if config.WARPProxyURL == "" {
			return 0, fmt.Errorf("WARP proxy is not configured, but strictly required")
		}

		written, err := doDownload(ctx, downloadURL, destPath)
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

func doDownload(ctx context.Context, downloadURL string, destPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

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
