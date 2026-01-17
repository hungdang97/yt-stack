package handlers

import (
	"time"
	"yt-downloader-go/models"

	"github.com/gofiber/fiber/v2"
)

// HandleHealth handles GET /health
// @Summary Health check
// @Description Check if the server is running
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /health [get]
func HandleHealth(c *fiber.Ctx) error {
	return c.JSON(models.HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UnixMilli(),
	})
}
