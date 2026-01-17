package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"yt-downloader-go/config"
)

// HTTPError represents an HTTP error
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// Download downloads a file using streaming (low memory)
func Download(ctx context.Context, downloadURL string, destPath string, totalSize int64) error {
	if totalSize <= config.ChunkSize {
		return downloadSingle(ctx, downloadURL, destPath, totalSize)
	}
	return downloadChunked(ctx, downloadURL, destPath, totalSize)
}

// downloadSingle streams small files directly to disk
func downloadSingle(ctx context.Context, downloadURL string, destPath string, totalSize int64) error {
	tmpPath := destPath + ".tmp"

	resp, err := fetchRange(ctx, downloadURL, 0, totalSize-1)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := streamToFile(resp.Body, tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

// downloadChunked downloads large files using parallel workers with chunk files
func downloadChunked(ctx context.Context, downloadURL string, destPath string, totalSize int64) error {
	// Create chunks directory
	chunksDir := destPath + ".chunks"
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return fmt.Errorf("failed to create chunks dir: %w", err)
	}

	// Calculate number of chunks
	numChunks := int((totalSize + config.ChunkSize - 1) / config.ChunkSize)

	// Error channel for workers
	errChan := make(chan error, config.Threads)
	chunkIndex := int32(-1)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < config.Threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				// Get next chunk atomically
				idx := int(atomic.AddInt32(&chunkIndex, 1))
				if idx >= numChunks {
					return
				}

				// Check context
				select {
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				default:
				}

				// Calculate byte range
				start := int64(idx) * config.ChunkSize
				end := start + config.ChunkSize - 1
				if end >= totalSize {
					end = totalSize - 1
				}

				// Download chunk with retries
				chunkPath := filepath.Join(chunksDir, fmt.Sprintf("chunk_%d", idx))
				if err := downloadChunkWithRetry(ctx, downloadURL, chunkPath, start, end); err != nil {
					errChan <- fmt.Errorf("chunk %d failed: %w", idx, err)
					return
				}
			}
		}()
	}

	// Wait for all workers
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			os.RemoveAll(chunksDir)
			return err
		}
	}

	// Merge chunks
	if err := mergeChunks(chunksDir, destPath, numChunks); err != nil {
		os.RemoveAll(chunksDir)
		return fmt.Errorf("merge failed: %w", err)
	}

	// Cleanup chunks directory
	os.RemoveAll(chunksDir)

	return nil
}

// downloadChunkWithRetry downloads a single chunk with retry logic
func downloadChunkWithRetry(ctx context.Context, downloadURL string, chunkPath string, start, end int64) error {
	tmpPath := chunkPath + ".tmp"

	var lastErr error
	for retry := 0; retry < config.MaxRetries; retry++ {
		resp, err := fetchRange(ctx, downloadURL, start, end)
		if err != nil {
			lastErr = err
			// Don't retry on 403
			if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode == 403 {
				return err
			}
			time.Sleep(config.RetryDelay * time.Duration(retry+1))
			continue
		}

		err = streamToFile(resp.Body, tmpPath)
		resp.Body.Close()

		if err != nil {
			lastErr = err
			os.Remove(tmpPath)
			time.Sleep(config.RetryDelay * time.Duration(retry+1))
			continue
		}

		// Success - rename tmp to final
		if err := os.Rename(tmpPath, chunkPath); err != nil {
			return fmt.Errorf("rename failed: %w", err)
		}
		return nil
	}

	return lastErr
}

// streamToFile streams data from reader to file using buffer pool
func streamToFile(reader io.Reader, destPath string) error {
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer file.Close()

	// Get buffer from pool
	bufPtr := config.BufferPool.Get().(*[]byte)
	defer config.BufferPool.Put(bufPtr)

	// Stream copy with pooled buffer
	_, err = io.CopyBuffer(file, reader, *bufPtr)
	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	return nil
}

// mergeChunks combines all chunk files into the final file
func mergeChunks(chunksDir string, destPath string, numChunks int) error {
	tmpPath := destPath + ".tmp"

	destFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create dest failed: %w", err)
	}
	defer destFile.Close()

	// Get buffer from pool
	bufPtr := config.BufferPool.Get().(*[]byte)
	defer config.BufferPool.Put(bufPtr)

	// Merge in order
	for i := 0; i < numChunks; i++ {
		chunkPath := filepath.Join(chunksDir, fmt.Sprintf("chunk_%d", i))

		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			return fmt.Errorf("open chunk %d failed: %w", i, err)
		}

		_, err = io.CopyBuffer(destFile, chunkFile, *bufPtr)
		chunkFile.Close()

		if err != nil {
			return fmt.Errorf("copy chunk %d failed: %w", i, err)
		}
	}

	// Rename to final
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("final rename failed: %w", err)
	}

	return nil
}

// fetchRange fetches a byte range from URL
func fetchRange(ctx context.Context, downloadURL string, start, end int64) (*http.Response, error) {
	// Add range to URL as query parameter (YouTube style)
	rangeURL := fmt.Sprintf("%s&range=%d-%d", downloadURL, start, end)

	req, err := http.NewRequestWithContext(ctx, "GET", rangeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("Referer", "https://www.youtube.com/")

	resp, err := config.DownloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	return resp, nil
}

// DownloadFile is a simpler download function for small files
func DownloadFile(ctx context.Context, downloadURL string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := config.DownloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &HTTPError{StatusCode: resp.StatusCode, Message: "download failed"}
	}

	return streamToFile(resp.Body, destPath)
}
