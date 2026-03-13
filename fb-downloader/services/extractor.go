package services

import (
	"encoding/json"
	"fmt"
	"fb-downloader/config"
	"fb-downloader/models"
	"io"
	"net/http"
	"net/url"
)

// Extract calls fb-extractor API to get post metadata
func Extract(postURL string) (*models.FbExtractResponse, error) {
	extractURL := config.FbExtractorURL + "/extract"
	params := url.Values{}
	params.Set("url", postURL)
	if config.WARPProxyURL != "" {
		params.Set("proxy", config.WARPProxyURL)
	}
	if config.FbDefaultCookie != "" {
		params.Set("cookie", config.FbDefaultCookie)
	}

	fullURL := extractURL + "?" + params.Encode()
	fmt.Printf("[Extract] GET %s\n", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := config.ExtractClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extractor API request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("[Extract] Response: %d — %s\n", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extractor API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var extractResp models.FbExtractResponse
	if err := json.Unmarshal(respBody, &extractResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(extractResp.Media) == 0 {
		return nil, fmt.Errorf("no media found in response")
	}

	fmt.Printf("[Extract] ✓ Extracted: %s (id=%s, media=%d, is_video=%v)\n",
		extractResp.OwnerUsername, extractResp.ID, len(extractResp.Media), extractResp.IsVideo)

	return &extractResp, nil
}
