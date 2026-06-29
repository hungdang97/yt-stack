package handlers

import (
	"fmt"
	"net/url"

	"fb-downloader/config"
	"fb-downloader/models"
	"fb-downloader/services"
	"fb-downloader/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleInfo handles POST /api/info — returns metadata + available download
// options (unified contract) WITHOUT creating a job. Facebook serves a single
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
	if !utils.IsFacebookURL(req.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Facebook URL"})
	}

	postData, err := services.Extract(req.URL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract post info: %v", err),
		})
	}

	// Title / author / duration
	title := postData.Title
	if title == "" {
		title = postData.Caption
	}
	if len(title) > 100 {
		title = title[:100]
	}
	if title == "" {
		title = "Facebook " + postData.ID
	}
	var duration float64
	if postData.VideoDuration != nil {
		duration = *postData.VideoDuration
	}

	// Thumbnail — proxy through our server to bypass Facebook CORP
	thumbnail := postData.GetImageURL()
	if thumbnail != "" {
		thumbnail = config.BaseURL + config.PathPrefix + "/proxy/image?url=" + url.QueryEscape(thumbnail)
	}

	hasVideo := postData.GetVideoURL() != "" || postData.GetVideoProgressiveURL() != ""

	video := make([]models.InfoOption, 0)
	audio := make([]models.InfoOption, 0, 1)

	// Prefer the rich videoStreams (multiple qualities, like YouTube). Fall back
	// to the legacy single-option shape when the extractor didn't provide them.
	if qopts := postData.VideoQualityOptions(); len(qopts) > 0 {
		for _, q := range qopts {
			video = append(video, models.InfoOption{
				Label:     q.Quality + " (.mp4)",
				Type:      "video",
				Format:    "mp4",
				Quality:   q.Quality,
				SizeBytes: q.SizeBytes,
			})
		}
		audio = append(audio, models.InfoOption{Label: "Audio (.mp3)", Type: "audio", Format: "mp3"})
	} else if hasVideo {
		video = append(video, models.InfoOption{Label: "Video (.mp4)", Type: "video", Format: "mp4"})
		// Audio can be extracted from the video even if there's no separate audio URL.
		audio = append(audio, models.InfoOption{Label: "Audio (.mp3)", Type: "audio", Format: "mp3"})
	} else if postData.GetAudioURL() != "" {
		audio = append(audio, models.InfoOption{Label: "Audio (.mp3)", Type: "audio", Format: "mp3"})
	}

	return c.JSON(models.InfoResponse{
		VideoID:      postData.ID,
		Title:        title,
		Author:       postData.OwnerUsername,
		Duration:     duration,
		ThumbnailURL: thumbnail,
		Video:        video,
		Audio:        audio,
		Other:        []models.InfoOption{},
	})
}
