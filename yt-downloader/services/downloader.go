package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// URLProvider manages the download URL and its refresh logic
type URLProvider struct {
	CurrentURL  string
	RefreshFunc func() (string, error)
	mu          sync.RWMutex
}

func (p *URLProvider) Get() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.CurrentURL
}

func (p *URLProvider) Refresh() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.RefreshFunc == nil {
		return "", fmt.Errorf("no refresh function provided")
	}

	newURL, err := p.RefreshFunc()
	if err != nil {
		return "", err
	}

	p.CurrentURL = newURL
	return newURL, nil
}

// Download downloads a file using streaming (low memory)
// threads: number of parallel download threads (0 = use config.DownloadThreads default)
func Download(ctx context.Context, urlProvider *URLProvider, destPath string, totalSize int64, threads int) error {
	if threads <= 0 {
		threads = config.DownloadThreads
	}
	if totalSize <= config.ChunkSize {
		return downloadSingle(ctx, urlProvider, destPath, totalSize)
	}
	return downloadChunked(ctx, urlProvider, destPath, totalSize, threads)
}

// downloadSingle streams small files directly to disk
func downloadSingle(ctx context.Context, urlProvider *URLProvider, destPath string, totalSize int64) error {
	tmpPath := destPath + ".tmp"
	maxRetries := config.MaxRetries

	var resp *http.Response
	var err error
	var lastErr error

	// Attempt 1: Direct IP
	// Try with retries and refresh for Direct IP
	for i := 0; i <= maxRetries; i++ {
		currentURL := urlProvider.Get()
		// Try Direct IP (useProxy = false)
		resp, err = fetchRangeWithClient(ctx, currentURL, 0, totalSize-1, false)

		if err != nil {
			lastErr = err
			if is403(err) && i < maxRetries {
				fmt.Printf("[Download] 403 in single download (Direct IP), refreshing...\n")
				if _, refreshErr := urlProvider.Refresh(); refreshErr != nil {
					return fmt.Errorf("refresh failed: %w", refreshErr)
				}
				continue
			}
			// Break to fallback if non-403 error or max retries reached
			break
		}

		// Success
		goto Success
	}

	// Attempt 2: Cloudflare Proxy (Fallback)
	fmt.Printf("[Download] Direct IP failed for single download, falling back to Proxy. Last err: %v\n", lastErr)

	// Try Proxy once with potentially refreshed URL
	{
		currentURL := urlProvider.Get()
		// Try Proxy (useProxy = true)
		resp, err = fetchRangeWithClient(ctx, currentURL, 0, totalSize-1, true)
		if err != nil {
			// If proxy fails too, return the proxy error (or wrap both)
			return fmt.Errorf("single download failed (Proxy fallback): %w (Direct err: %v)", err, lastErr)
		}
	}

Success:
	defer resp.Body.Close()

	if err := streamToFile(resp.Body, tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

// downloadChunked downloads large files using parallel workers with chunk files
func downloadChunked(ctx context.Context, urlProvider *URLProvider, destPath string, totalSize int64, threads int) error {
	// Create chunks directory
	chunksDir := destPath + ".chunks"
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return fmt.Errorf("failed to create chunks dir: %w", err)
	}

	// Calculate number of chunks
	numChunks := int((totalSize + config.ChunkSize - 1) / config.ChunkSize)

	// Error channel for workers
	errChan := make(chan error, threads)
	chunkIndex := int32(-1)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < threads; w++ {
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
				if err := downloadChunkWithRetry(ctx, urlProvider, chunkPath, start, end); err != nil {
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
// Strategy: 2 attempts
// - Attempt 1: Direct IP (with retries & refresh)
// - Attempt 2: Cloudflare proxy (fallback)
func downloadChunkWithRetry(ctx context.Context, urlProvider *URLProvider, chunkPath string, start, end int64) error {
	tmpPath := chunkPath + ".tmp"

	// Attempt 1: Use Direct IP
	// Retry loop for 403
	maxRetries := config.MaxRetries
	var lastErr error
	var success bool

	for i := 0; i <= maxRetries; i++ {
		currentURL := urlProvider.Get()
		// useProxy = false for Direct IP
		resp, err := fetchRangeWithClient(ctx, currentURL, start, end, false)
		if err == nil {
			err = streamToFile(resp.Body, tmpPath)
			resp.Body.Close()

			if err == nil {
				// Success - rename tmp to final
				if err := os.Rename(tmpPath, chunkPath); err != nil {
					return fmt.Errorf("rename failed: %w", err)
				}
				success = true
				break
			}
			os.Remove(tmpPath)
			lastErr = err
		} else {
			lastErr = err
			// Check for 403
			if is403(err) && i < maxRetries {
				// fmt.Printf("Chunk 403 (Direct), refreshing...\n")
				if _, refreshErr := urlProvider.Refresh(); refreshErr != nil {
					lastErr = fmt.Errorf("refresh failed: %w", refreshErr)
					break
				}
				continue
			}
		}
		// If not 403, break to fallback
		if !is403(lastErr) {
			break
		}
	}

	if success {
		return nil
	}

	// Attempt 2: Use Cloudflare Proxy (fallback)
	// fmt.Printf("Direct IP failed for chunk, falling back to Proxy...\n")
	time.Sleep(config.RetryDelay)

	// Use potentially refreshed URL
	currentURL := urlProvider.Get()

	// useProxy = true for Cloudflare
	resp, err := fetchRangeWithClient(ctx, currentURL, start, end, true)
	if err != nil {
		return fmt.Errorf("chunk download failed after fallback (Proxy): %w (Direct err: %v)", err, lastErr)
	}
	defer resp.Body.Close()

	err = streamToFile(resp.Body, tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chunk download failed (stream): %w", err)
	}

	// Success - rename tmp to final
	if err := os.Rename(tmpPath, chunkPath); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}

func is403(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "403")
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

// fetchRange fetches a byte range from URL using proxy (default)
func fetchRange(ctx context.Context, downloadURL string, start, end int64) (*http.Response, error) {
	return fetchRangeWithClient(ctx, downloadURL, start, end, true)
}

// fetchRangeWithClient fetches a byte range from URL with optional proxy
// useProxy=true: use Cloudflare proxy, useProxy=false: direct IP
func fetchRangeWithClient(ctx context.Context, downloadURL string, start, end int64, useProxy bool) (*http.Response, error) {
	// Add range to URL as query parameter (YouTube style)
	separator := "&"
	if !strings.Contains(downloadURL, "?") {
		separator = "?"
	}
	rangeURL := fmt.Sprintf("%s%srange=%d-%d", downloadURL, separator, start, end)

	req, err := http.NewRequestWithContext(ctx, "GET", rangeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("Referer", "https://www.youtube.com/")

	// Choose client based on proxy preference
	client := config.DownloadClient
	if !useProxy {
		client = config.DownloadClientNoProxy
	}

	resp, err := client.Do(req)
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
