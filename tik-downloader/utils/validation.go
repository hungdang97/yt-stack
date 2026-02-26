package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"tik-downloader/config"
	"time"
)

var jobIDRegex = regexp.MustCompile(config.JobIDRegex)

// ValidateJobID checks if a job ID matches the expected format
func ValidateJobID(jobID string) bool {
	return jobIDRegex.MatchString(jobID)
}

// ExtractVideoID extracts the numeric video ID from a TikTok URL
// Supports:
//   - https://www.tiktok.com/@user/video/7123456789012345678
//   - https://vm.tiktok.com/ZMxxxxxxxx/
//   - https://www.tiktok.com/t/ZMxxxxxxxx/
func ExtractVideoID(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	// If it's already a numeric ID
	if isNumericID(rawURL) {
		return rawURL, nil
	}

	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Check if it's a short URL that needs redirect resolution
	host := strings.ToLower(u.Hostname())
	if host == "vm.tiktok.com" || strings.Contains(u.Path, "/t/") {
		resolvedURL, err := resolveShortURL(rawURL)
		if err != nil {
			return "", fmt.Errorf("failed to resolve short URL: %w", err)
		}
		u, _ = url.Parse(resolvedURL)
	}

	// Extract ID from path: /@user/video/7123456789
	parts := strings.Split(u.Path, "/")
	for i, part := range parts {
		if part == "video" || part == "photo" {
			if i+1 < len(parts) && isNumericID(parts[i+1]) {
				return parts[i+1], nil
			}
		}
	}

	// Try to find any long numeric sequence in the path
	re := regexp.MustCompile(`(\d{15,})`)
	matches := re.FindStringSubmatch(u.Path)
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("could not extract video ID from URL: %s", rawURL)
}

// isNumericID checks if the string is a valid TikTok numeric ID (15+ digits)
func isNumericID(s string) bool {
	if len(s) < 15 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// resolveShortURL follows redirects to get the final URL
func resolveShortURL(shortURL string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Stop after finding the video URL
			if strings.Contains(req.URL.Path, "/video/") || strings.Contains(req.URL.Path, "/photo/") {
				return http.ErrUseLastResponse
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Head(shortURL)
	if err != nil {
		// Try GET if HEAD fails
		resp, err = client.Get(shortURL)
		if err != nil {
			return "", err
		}
	}
	defer resp.Body.Close()

	// Check Location header or final URL
	if loc := resp.Header.Get("Location"); loc != "" {
		return loc, nil
	}

	return resp.Request.URL.String(), nil
}

// IsTikTokURL checks if a URL is a TikTok URL
func IsTikTokURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, "tiktok.com")
}
