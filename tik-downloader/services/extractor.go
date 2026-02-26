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

	reqBody := models.TikExtractRequest{
		DetailID: videoID,
		Cookie:   cookieItem.Value,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := config.TikExtractorURL + "/tiktok/detail"
	fmt.Printf("[%s] Extracting from %s\n", videoID, url)

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

	if resp.StatusCode != http.StatusOK {
		errorMessage := string(respBody)

		// If API returns 401/403 or specific error about auth/cookie, invalidate it
		isBadCookie := resp.StatusCode == 401 || resp.StatusCode == 403 ||
			strings.Contains(strings.ToLower(errorMessage), "login") ||
			strings.Contains(strings.ToLower(errorMessage), "bot") ||
			strings.Contains(strings.ToLower(errorMessage), "sign")

		if isBadCookie {
			InvalidateCookie(cookieItem.ID)
		}

		return nil, fmt.Errorf("extractor API returned %d: %s", resp.StatusCode, errorMessage)
	}

	// Parse directly into typed struct
	var extractResp models.TikExtractResponse
	if err := json.Unmarshal(respBody, &extractResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	video := &extractResp.Data
	if video.Downloads == "" && video.MusicURL == "" {
		errorMessage := extractResp.Message

		// Some TikTok endpoints return empty data with "login required" message inside 200 OK
		isBadCookie := strings.Contains(strings.ToLower(errorMessage), "login") ||
			strings.Contains(strings.ToLower(errorMessage), "bot") ||
			strings.Contains(strings.ToLower(errorMessage), "sign") ||
			strings.Contains(strings.ToLower(errorMessage), "verify")

		if isBadCookie {
			InvalidateCookie(cookieItem.ID)
		}

		return nil, fmt.Errorf("no download URLs in response (message: %s)", errorMessage)
	}

	// Ensure cookie is set for CDN download
	if extractResp.Params.Cookie == "" {
		extractResp.Params.Cookie = GetCookie().Value
	}

	fmt.Printf("[%s] ✓ Extracted: %s (duration=%s, downloads=%v, music=%v)\n",
		videoID, video.Desc, video.Duration,
		video.Downloads != "", video.MusicURL != "")

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
