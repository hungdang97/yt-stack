package handlers

import (
	"io"
	"log"
	"net/http"
	"strings"

	"insta-downloader/config"

	"github.com/gofiber/fiber/v2"
)

// allowedHosts restricts proxy to Instagram CDN domains only
var allowedHosts = []string{
	"scontent.cdninstagram.com",
	"scontent-",        // scontent-*.cdninstagram.com
	"instagram.com",
	"cdninstagram.com",
}

func isAllowedHost(host string) bool {
	for _, allowed := range allowedHosts {
		if strings.Contains(host, allowed) {
			return true
		}
	}
	return false
}

// HandleProxyImage proxies Instagram CDN images to bypass CORS
// GET /proxy/image?url=<encoded_instagram_cdn_url>
func HandleProxyImage(c *fiber.Ctx) error {
	imageURL := c.Query("url")
	if imageURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "url parameter is required",
		})
	}

	// Parse and validate the URL
	if !strings.HasPrefix(imageURL, "https://") && !strings.HasPrefix(imageURL, "http://") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid URL scheme",
		})
	}

	// Extract host for validation
	hostStart := strings.Index(imageURL, "://") + 3
	hostEnd := strings.Index(imageURL[hostStart:], "/")
	if hostEnd == -1 {
		hostEnd = len(imageURL) - hostStart
	}
	host := imageURL[hostStart : hostStart+hostEnd]

	if !isAllowedHost(host) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "host not allowed",
		})
	}

	// Fetch image from Instagram CDN
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		log.Printf("[ProxyImage] Failed to create request: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid URL",
		})
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.instagram.com/")

	resp, err := config.DownloadClientNoProxy.Do(req)
	if err != nil {
		log.Printf("[ProxyImage] Failed to fetch image: %v", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "failed to fetch image",
		})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ProxyImage] Upstream returned %d for %s", resp.StatusCode, imageURL)
		return c.Status(resp.StatusCode).JSON(fiber.Map{
			"error": "upstream error",
		})
	}

	// Forward content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		c.Set("Content-Type", contentType)
	} else {
		c.Set("Content-Type", "image/jpeg")
	}

	// Cache for 1 hour
	c.Set("Cache-Control", "public, max-age=3600")

	// Stream response body
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
	if err != nil {
		log.Printf("[ProxyImage] Failed to read response: %v", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "failed to read image",
		})
	}

	return c.Send(body)
}
