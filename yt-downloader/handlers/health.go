package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

const Version = "6.0.0"

// HandleHealth handles GET /health
// @Summary Health check
// @Description Check if the server is running
// @Tags health
// @Produce json
// @Router /health [get]
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"service":   "yt-downloader",
		"version":   Version,
		"timestamp": time.Now().UnixMilli(),
	})
}
