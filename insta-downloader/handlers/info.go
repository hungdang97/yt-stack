package handlers

import (
	"fmt"
	"insta-downloader/config"
	"insta-downloader/services"
	"insta-downloader/utils"
	"net/url"

	"github.com/gofiber/fiber/v2"
)

type InfoRequest struct {
	URL string `json:"url"`
}

type InfoResponse struct {
	Title     string  `json:"title"`
	Caption   string  `json:"caption,omitempty"`
	Author    string  `json:"author,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	VideoURL  string  `json:"videoUrl,omitempty"`
	AudioURL  string  `json:"audioUrl,omitempty"`
	Likes     int     `json:"likes,omitempty"`
	Comments  int     `json:"comments,omitempty"`
}

// HandleInfo handles POST /api/info — returns post metadata for preview without downloading
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

	if !utils.IsInstagramURL(req.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid Instagram URL",
		})
	}

	postData, err := services.Extract(req.URL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract post info: %v", err),
		})
	}

	title := postData.Caption
	if len(title) > 100 {
		title = title[:100]
	}
	if title == "" {
		title = "Instagram " + postData.Shortcode
	}

	var duration float64
	if postData.VideoDuration != nil {
		duration = *postData.VideoDuration
	}

	thumbnail := postData.GetImageURL()
	if thumbnail != "" {
		thumbnail = config.BaseURL + config.PathPrefix + "/proxy/image?url=" + url.QueryEscape(thumbnail)
	}

	// Pick best video URL for preview (prefer progressive for browser compatibility)
	videoURL := postData.GetVideoProgressiveURL()
	if videoURL == "" {
		videoURL = postData.GetVideoURL()
	}

	return c.JSON(InfoResponse{
		Title:     title,
		Caption:   postData.Caption,
		Author:    postData.OwnerUsername,
		Duration:  duration,
		Thumbnail: thumbnail,
		VideoURL:  videoURL,
		AudioURL:  postData.GetAudioURL(),
		Likes:     postData.Likes,
		Comments:  postData.Comments,
	})
}
