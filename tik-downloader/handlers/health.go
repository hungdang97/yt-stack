package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// HandleHealth handles GET /health
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"service":   "tik-downloader",
		"timestamp": time.Now().Unix(),
	})
}
