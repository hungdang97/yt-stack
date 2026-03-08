package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"tik-downloader/config"
	"tik-downloader/models"
)

// Extract calls tik-extractor API and returns the full response
func Extract(videoID string) (*models.TikExtractResponse, error) {
	cookieItem := GetCookie()
	cookiePreview := cookieItem.Value
	if len(cookiePreview) > 50 {
		cookiePreview = cookiePreview[:50]
	}
	fmt.Printf("[DEBUG] Cookie ID=%s, Value length=%d, Preview=%q\n", cookieItem.ID, len(cookieItem.Value), cookiePreview)

	reqBody := models.TikExtractRequest{
		DetailID: videoID,
		Cookie:   cookieItem.Value,
		Proxy:    config.WARPProxyURL,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := config.TikExtractorURL + "/tiktok/detail"

	// Log equivalent curl for debugging
	fmt.Printf("[%s] curl -X POST '%s' -H 'Content-Type: application/json' -d '%s'\n", videoID, url, string(bodyBytes))

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := config.ExtractClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extractor API request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("[%s] Response: %d — %s\n", videoID, resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extractor API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var extractResp models.TikExtractResponse
	if err := json.Unmarshal(respBody, &extractResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	video := &extractResp.Data
	if video.GetDownloads() == "" && video.MusicURL == "" {
		return nil, fmt.Errorf("no download URLs in response (message: %s)", extractResp.Message)
	}

	// Ensure cookie is set for CDN download
	if extractResp.Params.Cookie == "" {
		extractResp.Params.Cookie = GetCookie().Value
	}

	fmt.Printf("[%s] ✓ Extracted: %s (duration=%s, downloads=%v, music=%v)\n",
		videoID, video.Desc, video.Duration,
		video.GetDownloads() != "", video.MusicURL != "")

	return &extractResp, nil
}

// ParseDuration converts "00:00:30" format to seconds
func ParseDuration(duration string) float64 {
	parts := strings.Split(duration, ":")
	if len(parts) != 3 {
		return 0
	}
	hours, _ := strconv.ParseFloat(parts[0], 64)
	minutes, _ := strconv.ParseFloat(parts[1], 64)
	seconds, _ := strconv.ParseFloat(parts[2], 64)
	return hours*3600 + minutes*60 + seconds
}
