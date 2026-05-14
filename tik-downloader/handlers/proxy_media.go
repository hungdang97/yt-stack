package handlers

import (
	"io"
	"log"
	"net/http"
	"strings"

	"tik-downloader/config"
	"tik-downloader/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleProxyMedia proxies video/audio content through WARP proxy with signed token
func HandleProxyMedia(c *fiber.Ctx) error {
	token := c.Query("token")
	expiresStr := c.Query("expires")
	mediaURL := c.Query("url")

	if token == "" || expiresStr == "" || mediaURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing parameters"})
	}

	expires, err := utils.ParseExpires(expiresStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid expires"})
	}

	if !utils.ValidateMediaProxyURL(mediaURL, token, expires) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "invalid or expired token"})
	}

	if !strings.HasPrefix(mediaURL, "https://") && !strings.HasPrefix(mediaURL, "http://") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid URL scheme"})
	}

	req, err := http.NewRequest("GET", mediaURL, nil)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid URL"})
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	if rangeHeader := c.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := config.DownloadClient.Do(req)
	if err != nil {
		log.Printf("[ProxyMedia] Failed to fetch: %v", err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "failed to fetch media"})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": "upstream error"})
	}

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(h); v != "" {
			c.Set(h, v)
		}
	}
	c.Set("Cache-Control", "public, max-age=3600")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Status(resp.StatusCode)

	_, err = io.Copy(c.Response().BodyWriter(), resp.Body)
	if err != nil {
		log.Printf("[ProxyMedia] Stream error: %v", err)
	}
	return nil
}
