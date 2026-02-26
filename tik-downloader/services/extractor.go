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

// Extract calls tik-extractor API to get video metadata
func Extract(videoID string) (*models.TikVideoData, error) {
	reqBody := models.TikExtractRequest{
		DetailID: videoID,
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
		return nil, fmt.Errorf("extractor API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extractor API returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - tik-extractor returns nested structure
	var rawResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Try to get data array
	dataField, ok := rawResp["data"]
	if !ok {
		return nil, fmt.Errorf("no 'data' field in response")
	}

	// Re-marshal and unmarshal to extract properly
	dataBytes, err := json.Marshal(dataField)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal data: %w", err)
	}

	// Try as array first
	var dataArray []models.TikVideoData
	if err := json.Unmarshal(dataBytes, &dataArray); err == nil && len(dataArray) > 0 {
		video := &dataArray[0]
		if video.Downloads == "" && video.MusicURL == "" {
			return nil, fmt.Errorf("no download URLs found in response")
		}
		fmt.Printf("[%s] ✓ Extract success: %s (%s)\n", videoID, video.Desc, video.Duration)
		return video, nil
	}

	// Try as single object
	var singleData models.TikVideoData
	if err := json.Unmarshal(dataBytes, &singleData); err == nil {
		if singleData.Downloads != "" || singleData.MusicURL != "" {
			fmt.Printf("[%s] ✓ Extract success: %s (%s)\n", videoID, singleData.Desc, singleData.Duration)
			return &singleData, nil
		}
	}

	return nil, fmt.Errorf("failed to parse video data from response")
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
