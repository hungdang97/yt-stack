package services

import (
	"encoding/json"
	"fmt"
	"fb-downloader/config"
	"fb-downloader/models"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// validFbCookie drops placeholder/garbage cookie values so we never forward them
// to the extractor (which would send a junk cookie to Facebook and get HTTP 400).
// The default env value is a comment like "# cookie Facebook thật (điền nếu cần)".
func validFbCookie(c string) string {
	c = strings.TrimSpace(c)
	if c == "" || strings.HasPrefix(c, "#") {
		return ""
	}
	// Must look like a real cookie: JSON object/array, a name=value pair, or a bare id.
	if strings.HasPrefix(c, "{") || strings.HasPrefix(c, "[") || strings.Contains(c, "=") || isAllDigits(c) {
		return c
	}
	return ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Extract calls fb-extractor API to get post metadata
func Extract(postURL string) (*models.FbExtractResponse, error) {
	extractURL := config.FbExtractorURL + "/extract"
	params := url.Values{}
	params.Set("url", postURL)
	if config.WARPProxyURL != "" {
		params.Set("proxy", config.WARPProxyURL)
	}
	if c := validFbCookie(config.FbDefaultCookie); c != "" {
		params.Set("cookie", c)
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
