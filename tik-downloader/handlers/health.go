package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

const Version = "2.0.0"

// HandleHealth handles GET /health
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"service":   "tik-downloader",
		"version":   Version,
		"timestamp": time.Now().Unix(),
	})
}
