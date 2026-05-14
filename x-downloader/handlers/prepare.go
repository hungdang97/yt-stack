package handlers

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"x-downloader/config"
	"x-downloader/models"
	"x-downloader/services"
	"x-downloader/utils"

	"github.com/gofiber/fiber/v2"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

type PrepareResponse struct {
	StatusURL string         `json:"statusUrl"`
	Title     string         `json:"title"`
	Author    string         `json:"author,omitempty"`
	Duration  float64        `json:"duration"`
	Thumbnail string         `json:"thumbnail,omitempty"`
	VideoURL  string         `json:"videoUrl,omitempty"`
	AudioURL  string         `json:"audioUrl,omitempty"`
	Subtitles []SubtitleInfo `json:"subtitles"`
}

type PrepareStatusResponse struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	VideoURL string `json:"videoUrl,omitempty"`
	AudioURL string `json:"audioUrl,omitempty"`
	Error    string `json:"error,omitempty"`
}

// HandlePrepare handles POST /api/prepare
func HandlePrepare(c *fiber.Ctx) error {
	hubToken := c.Get("X-Hub-Token")
	if hubToken != config.HubToken {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req models.DownloadRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "URL is required"})
	}
	if !utils.IsXURL(req.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid X/Twitter URL"})
	}

	postData, err := services.Extract(req.URL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract post info: %v", err),
		})
	}

	// Pick video URL (prefer progressive — X audio is HLS, can't download separately)
	videoURL := postData.GetVideoProgressiveURL()
	if videoURL == "" {
		videoURL = postData.GetVideoURL()
	}
	if videoURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No video URL available"})
	}

	title := postData.Title
	if title == "" {
		title = postData.Caption
	}
	if len(title) > 100 {
		title = title[:100]
	}
	if title == "" {
		title = "X " + postData.ID
	}

	var duration float64
	if postData.VideoDuration != nil {
		duration = *postData.VideoDuration
	}

	jobID, err := gonanoid.New(config.JobIDLength)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate job ID"})
	}
	if err := utils.CreateJobDir(jobID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create job directory"})
	}

	meta := &models.PrepareMeta{
		ID:        jobID,
		Status:    models.StatusDownloading,
		CreatedAt: time.Now().UnixMilli(),
		Title:     title,
		Author:    postData.OwnerUsername,
		Duration:  duration,
		VideoFile: "video.mp4",
		AudioFile: "audio.m4a",
		SourceURL: req.URL,
	}

	if err := utils.WritePrepareMeta(jobID, meta); err != nil {
		utils.DeleteJobDir(jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to write metadata"})
	}

	go processPrepareJob(jobID, req.URL, videoURL)

	// Build preview response
	thumbnail := postData.GetImageURL()
	if thumbnail != "" {
		thumbnail = config.BaseURL + config.PathPrefix + "/proxy/image?url=" + url.QueryEscape(thumbnail)
	}

	proxyVideoURL := utils.GenerateMediaProxyURL(videoURL)

	return c.Status(fiber.StatusCreated).JSON(PrepareResponse{
		StatusURL: utils.GeneratePrepareStatusURL(jobID),
		Title:     title,
		Author:    postData.OwnerUsername,
		Duration:  duration,
		Thumbnail: thumbnail,
		VideoURL:  proxyVideoURL,
		Subtitles: mapSubtitles(postData.Subtitles),
	})
}

// HandlePrepareStatus handles GET /api/prepare/status/:id
func HandlePrepareStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if !utils.ValidateJobID(jobID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid job ID"})
	}

	token := c.Query("token")
	expiresStr := c.Query("expires")
	if token != "" && expiresStr != "" {
		expires, err := utils.ParseExpires(expiresStr)
		if err != nil || !utils.ValidatePrepareStatusURL(jobID, token, expires) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Invalid or expired token"})
		}
	}

	meta, err := utils.ReadPrepareMeta(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Job not found"})
	}

	response := PrepareStatusResponse{
		Status:   meta.Status,
		Progress: utils.CalculatePrepareProgress(meta),
	}

	if meta.Status == models.StatusCompleted {
		response.Progress = 100
		response.VideoURL = utils.GenerateSignedURL(jobID, meta.VideoFile)
		response.AudioURL = utils.GenerateSignedURL(jobID, meta.AudioFile)
	}
	if meta.Status == models.StatusError {
		response.Error = meta.Error
	}

	return c.JSON(response)
}

func processPrepareJob(jobID, postURL, videoURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.DownloadTimeout)
	defer cancel()

	jobDir := utils.GetJobDir(jobID)

	// Download video (X/Twitter: always use progressive, audio is HLS)
	_, err := services.Download(ctx, jobID, postURL, videoURL, "temp_video.mp4")
	if err != nil {
		fmt.Printf("[Prepare/%s] Video download failed: %v\n", jobID, err)
		utils.UpdatePrepareMetaError(jobID, err.Error())
		return
	}

	videoPath := filepath.Join(jobDir, "temp_video.mp4")
	outputVideoPath := filepath.Join(jobDir, "video.mp4")

	if err := services.RemuxVideo(videoPath, outputVideoPath); err != nil {
		utils.UpdatePrepareMetaError(jobID, fmt.Sprintf("Video processing failed: %v", err))
		return
	}

	// Extract audio from video (X/Twitter audio is always HLS, can't download directly)
	outputAudioPath := filepath.Join(jobDir, "audio.m4a")
	if err := services.ExtractAudio(outputVideoPath, outputAudioPath); err != nil {
		utils.UpdatePrepareMetaError(jobID, fmt.Sprintf("Audio extraction failed: %v", err))
		return
	}

	utils.UpdatePrepareMetaCompleted(jobID)
	fmt.Printf("[Prepare/%s] Job completed\n", jobID)
}
