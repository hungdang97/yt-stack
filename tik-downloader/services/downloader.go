package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"tik-downloader/config"
	"tik-downloader/utils"
)

// Download downloads a file from URL to the job's storage directory
func Download(ctx context.Context, jobID string, downloadURL string, filename string, cookie string) (int64, error) {
	destPath := filepath.Join(utils.GetJobDir(jobID), filename)

	fmt.Printf("[%s] Downloading %s\n", jobID, filename)

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// TikTok CDN requires cookie + headers for auth
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.tiktok.com/")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := config.DownloadClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Create output file
	outFile, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	// Copy with buffer
	buf := make([]byte, config.BufferSize)
	written, err := io.CopyBuffer(outFile, resp.Body, buf)
	if err != nil {
		os.Remove(destPath) // Clean up partial file
		return 0, fmt.Errorf("download interrupted: %w", err)
	}

	fmt.Printf("[%s] ✓ Downloaded %s (%.2f MB)\n", jobID, filename, float64(written)/1024/1024)
	return written, nil
}
