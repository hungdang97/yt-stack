package handlers

import (
	"context"
	"fmt"
	"tik-downloader/config"
	"tik-downloader/models"
	"tik-downloader/services"
	"tik-downloader/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// HandleDownload handles POST /api/download
func HandleDownload(c *fiber.Ctx) error {
	// Verify Hub token
	hubToken := c.Get("X-Hub-Token")
	if hubToken != config.HubToken {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	// Parse request
	var req models.DownloadRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate URL
	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "URL is required",
		})
	}

	// Validate type — resolve output.type (Hub/FE shape) then fall back to type
	if req.Output.Type != "" {
		req.Type = req.Output.Type
	}
	if req.Type == "" {
		req.Type = "video" // Default to video
	}
	if req.Type != "video" && req.Type != "audio" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Type must be 'video' or 'audio'",
		})
	}

	// Extract video ID from URL
	videoID, err := utils.ExtractVideoID(req.URL)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid TikTok URL: %v", err),
		})
	}

	fmt.Printf("[TikTok] Download request: %s (type=%s, id=%s)\n", req.URL, req.Type, videoID)

	// Extract video metadata from tik-extractor
	videoData, err := services.Extract(videoID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract video info: %v", err),
		})
	}

	// Determine download URL and output filename
	var downloadURL, outputFilename string
	switch req.Type {
	case "video":
		downloadURL = videoData.Data.GetDownloads()
		outputFilename = "output.mp4"
		if downloadURL == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No video download URL available",
			})
		}
	case "audio":
		downloadURL = videoData.Data.MusicURL
		outputFilename = "output.mp3"
		if downloadURL == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No audio download URL available",
			})
		}
	}

	// Generate job ID
	jobID, err := gonanoid.New(config.JobIDLength)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate job ID",
		})
	}

	// Create job directory
	if err := utils.CreateJobDir(jobID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create job directory",
		})
	}

	// Parse duration
	duration := services.ParseDuration(videoData.Data.Duration)

	// Get title
	title := videoData.Data.Desc
	if title == "" {
		title = "TikTok Video " + videoID
	}

	// Create meta
	meta := &models.Meta{
		ID:           jobID,
		Status:       models.StatusDownloading,
		Title:        title,
		Duration:     duration,
		OutputType:   req.Type,
		Output:       outputFilename,
		CreatedAt:    time.Now().UnixMilli(),
		VideoURL:     videoData.Data.GetDownloads(),
		MusicURL:     videoData.Data.MusicURL,
		ThumbnailURL: videoData.Data.StaticCover,
		Author:       videoData.Data.Nickname,
		SourceURL:    req.URL,
	}

	if err := utils.WriteMeta(jobID, meta); err != nil {
		utils.DeleteJobDir(jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to write metadata",
		})
	}

	// Start background download (with cookie for CDN auth)
	go processJob(jobID, videoID, downloadURL, outputFilename, videoData.Params.Cookie)

	// Return response
	response := models.DownloadResponse{
		StatusURL: utils.GenerateStatusURL(jobID),
		Type:      req.Type,
		Title:     title,
		Duration:  duration,
		Thumbnail: videoData.Data.StaticCover,
	}

	fmt.Printf("[%s] Job created: %s → %s\n", jobID, req.Type, outputFilename)
	return c.Status(fiber.StatusCreated).JSON(response)
}

// processJob runs the download in background
func processJob(jobID, videoID, downloadURL, outputFilename, cookie string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.DownloadTimeout)
	defer cancel()

	fileSize, err := services.Download(ctx, jobID, videoID, downloadURL, outputFilename, cookie)
	if err != nil {
		fmt.Printf("[%s] ✗ Download failed: %v\n", jobID, err)
		utils.UpdateMetaError(jobID, err.Error())
		return
	}

	if err := utils.UpdateMetaCompleted(jobID, outputFilename, fileSize); err != nil {
		fmt.Printf("[%s] ✗ Failed to update meta: %v\n", jobID, err)
		return
	}

	fmt.Printf("[%s] ✓ Job completed: %s (%.2f MB)\n", jobID, outputFilename, float64(fileSize)/1024/1024)
}
