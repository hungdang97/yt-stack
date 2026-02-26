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

// Default TikTok cookie for API extraction
const defaultCookie = "_ttp=2hqtHChFe7UTighTpqgKHHA1iSp; uid_tt_ss=38a4b5ae41a9360b3d04a610cc8b06830a0a45b4556798c29169fd69d0c9c6ee; sessionid_ss=6b6f7985871b49380232a4376f4d3755; tta_attr_id_mirror=0.1761013677.7563496152593137680; passport_csrf_token=109af37c1be17247496b5d7ba0bc0876; tt_chain_token=bapr1TRV9NPhvWDkAWJ3LQ==; tiktok_webapp_theme_source=auto; delay_guest_mode_vid=5; uid_tt=38a4b5ae41a9360b3d04a610cc8b06830a0a45b4556798c29169fd69d0c9c6ee; sid_tt=6b6f7985871b49380232a4376f4d3755; sessionid=6b6f7985871b49380232a4376f4d3755; sid_guard=6b6f7985871b49380232a4376f4d3755%7C1772008103%7C15552000%7CMon%2C+24-Aug-2026+08%3A28%3A23+GMT; tt_csrf_token=byDvtQzK-oVU6u0ap9ODaWUZJpFEA-JV-ztQ; odin_tt=8a1ed1eee0055d5d177f1dd2d5a6a13dd44ce985f13fcfbfe9a5932d721fd44ad5fd4ed914c0aa396606946689d9fc79331de4baf45e4deb8eb10daaffbac9194f04f2a2f2d7ebccd85fae08b24e1f3e; msToken=MOl4FTrX27v8KTbUkP5ZKLoTOt-HqgPytFQVBd_c2pFgA_WR7CZc0dYC7nTCoJLgCxj41nxvdZtmSpx71obYJjqqD_KSDlx3bx6pAeAw3xM0u3608V4HT19eVshzSMBy0b5t9H6VK8TkUs7BsBSVFEmA"

// Extract calls tik-extractor API to get video metadata
func Extract(videoID string) (*models.TikVideoData, error) {
	reqBody := models.TikExtractRequest{
		DetailID: videoID,
		Cookie:   defaultCookie,
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
