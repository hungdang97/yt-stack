package handlers

import (
	"fmt"

	"tik-downloader/config"
	"tik-downloader/models"
	"tik-downloader/services"
	"tik-downloader/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleInfo handles POST /api/info — returns metadata + available download
// options (unified contract) WITHOUT creating a job. TikTok serves a single
// rendition, so we expose at most one Video and one Audio option.
func HandleInfo(c *fiber.Ctx) error {
	// Verify Hub token
	if c.Get("X-Hub-Token") != config.HubToken {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req models.InfoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "URL is required"})
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

	video := make([]models.InfoOption, 0, 1)
	audio := make([]models.InfoOption, 0, 1)
	if videoData.Data.GetDownloads() != "" {
		video = append(video, models.InfoOption{Label: "Video (.mp4)", Type: "video", Format: "mp4"})
	}
	if videoData.Data.MusicURL != "" {
		audio = append(audio, models.InfoOption{Label: "Audio (.mp3)", Type: "audio", Format: "mp3"})
	}

	return c.JSON(models.InfoResponse{
		VideoID:      videoID,
		Title:        title,
		Author:       videoData.Data.Nickname,
		Duration:     services.ParseDuration(videoData.Data.Duration),
		ThumbnailURL: videoData.Data.StaticCover,
		Video:        video,
		Audio:        audio,
		Other:        []models.InfoOption{},
	})
}
