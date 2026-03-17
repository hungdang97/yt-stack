package handlers

import (
	"io"
	"log"
	"net/http"
	"strings"

	"uni-downloader/config"

	"github.com/gofiber/fiber/v2"
)

// HandleProxyImage proxies CDN images to bypass CORS
// No domain whitelist since we don't know CDN domains for all sites
// GET /proxy/image?url=<encoded_cdn_url>
func HandleProxyImage(c *fiber.Ctx) error {
	imageURL := c.Query("url")
	if imageURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "url parameter is required",
		})
	}

	if !strings.HasPrefix(imageURL, "https://") && !strings.HasPrefix(imageURL, "http://") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid URL scheme",
		})
	}

	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		log.Printf("[ProxyImage] Failed to create request: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid URL",
		})
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

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

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		c.Set("Content-Type", contentType)
	} else {
		c.Set("Content-Type", "image/jpeg")
	}

	c.Set("Cache-Control", "public, max-age=3600")

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		log.Printf("[ProxyImage] Failed to read response: %v", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "failed to read image",
		})
	}

	return c.Send(body)
}
