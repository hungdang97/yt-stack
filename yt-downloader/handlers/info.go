package handlers

import (
	"yt-downloader-go/config"
	"yt-downloader-go/models"
	"yt-downloader-go/services"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleInfo handles POST /api/info
// @Summary Get video info and available download options
// @Description Fetch video metadata and the list of available qualities,
// @Description formats and audio languages WITHOUT creating a download job.
// @Tags info
// @Accept json
// @Produce json
// @Param request body models.InfoRequest true "Info request"
// @Success 200 {object} models.InfoResponse
// @Failure 400 {object} utils.ErrorResponse "Validation error"
// @Failure 403 {object} utils.ErrorResponse "Invalid hub token"
// @Failure 500 {object} utils.ErrorResponse "Extraction failed"
// @Router /api/info [post]
func HandleInfo(c *fiber.Ctx) error {
	// Validate hub token - only allow requests from hub
	const hubToken = "1234567890987654321234567890987654321"
	token := c.Get("X-Hub-Token")
	if token != hubToken {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Unauthorized: Invalid or missing hub token",
		})
	}

	var req models.InfoRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid request body")
	}

	if req.URL == "" {
		return utils.BadRequest(c, utils.ErrValidationError, "URL is required")
	}

	// Extract video ID
	videoID, err := utils.ExtractVideoID(req.URL)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidURL, err.Error())
	}

	// Fetch metadata (reuses the same extractor as /api/download)
	data, err := services.Extract(videoID, req.Premium)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, utils.ErrExtractFailed, "Failed to fetch video metadata")
	}

	response := models.InfoResponse{
		VideoID:                 videoID,
		Title:                   data.Title,
		Author:                  data.Author,
		Duration:                data.Duration,
		ThumbnailURL:            data.ThumbnailURL,
		AvailableQualities:      services.GetAvailableQualities(data),
		AvailableAudioLanguages: data.AvailableAudioLanguages,
		VideoFormats:            config.VideoFormats,
		AudioFormats:            config.AudioFormats,
		AudioBitrates:           config.AudioBitrates,
	}

	return c.JSON(response)
}
