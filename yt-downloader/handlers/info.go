package handlers

import (
	"strconv"
	"strings"

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

	// Video options: one per available quality (mp4), with estimated merged size.
	video := make([]models.InfoOption, 0)
	for _, vo := range services.GetVideoOptions(data) {
		video = append(video, models.InfoOption{
			Label:     vo.Quality + " (.mp4)",
			Type:      "video",
			Format:    "mp4",
			Quality:   vo.Quality,
			SizeBytes: vo.SizeBytes,
		})
	}

	// Audio options: MP3 at each bitrate (high -> low), size estimated from
	// bitrate x duration. config.AudioBitrates is ascending, so iterate reversed.
	audio := make([]models.InfoOption, 0, len(config.AudioBitrates))
	for i := len(config.AudioBitrates) - 1; i >= 0; i-- {
		b := config.AudioBitrates[i]
		var size int64
		if kbps, errConv := strconv.Atoi(strings.TrimSuffix(b, "k")); errConv == nil && kbps > 0 && data.Duration > 0 {
			size = int64(float64(kbps) * 1000 * data.Duration / 8)
		}
		audio = append(audio, models.InfoOption{
			Label:     "MP3 - " + strings.Replace(b, "k", "kbps", 1),
			Type:      "audio",
			Format:    "mp3",
			Bitrate:   b,
			SizeBytes: size,
		})
	}

	// Other options: non-mp3 audio containers (size unknown).
	other := make([]models.InfoOption, 0)
	for _, f := range config.AudioFormats {
		if f == "mp3" {
			continue
		}
		other = append(other, models.InfoOption{
			Label:  strings.ToUpper(f),
			Type:   "audio",
			Format: f,
		})
	}

	response := models.InfoResponse{
		VideoID:        videoID,
		Title:          data.Title,
		Author:         data.Author,
		Duration:       data.Duration,
		ThumbnailURL:   data.ThumbnailURL,
		Video:          video,
		Audio:          audio,
		Other:          other,
		AudioLanguages: data.AvailableAudioLanguages,
	}

	return c.JSON(response)
}
