package handlers

import (
	"fmt"
	"tik-downloader/config"
	"tik-downloader/services"
	"tik-downloader/utils"

	"github.com/gofiber/fiber/v2"
)

type InfoRequest struct {
	URL string `json:"url"`
}

type InfoResponse struct {
	Title     string  `json:"title"`
	Caption   string  `json:"caption,omitempty"`
	Author    string  `json:"author,omitempty"`
	Duration  float64 `json:"duration"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	VideoURL  string  `json:"videoUrl,omitempty"`
	AudioURL  string  `json:"audioUrl,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
}

// HandleInfo handles POST /api/info — returns video metadata for preview without downloading
func HandleInfo(c *fiber.Ctx) error {
	hubToken := c.Get("X-Hub-Token")
	if hubToken != config.HubToken {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	var req InfoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "URL is required",
		})
	}

	videoID, err := utils.ExtractVideoID(req.URL)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid TikTok URL: %v", err),
		})
	}

	videoData, err := services.Extract(videoID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract video info: %v", err),
		})
	}

	title := videoData.Data.Desc
	if title == "" {
		title = "TikTok Video " + videoID
	}

	return c.JSON(InfoResponse{
		Title:     title,
		Caption:   videoData.Data.Desc,
		Author:    videoData.Data.Nickname,
		Duration:  services.ParseDuration(videoData.Data.Duration),
		Thumbnail: videoData.Data.StaticCover,
		VideoURL:  videoData.Data.GetDownloads(),
		AudioURL:  videoData.Data.MusicURL,
		Width:     videoData.Data.Width,
		Height:    videoData.Data.Height,
	})
}
