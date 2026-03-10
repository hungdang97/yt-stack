package handlers

import (
	"context"
	"fmt"
	"insta-downloader/config"
	"insta-downloader/models"
	"insta-downloader/services"
	"insta-downloader/utils"
	"path/filepath"
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

	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "URL is required",
		})
	}

	// Validate type
	if req.Type == "" {
		req.Type = "video"
	}
	if req.Type != "video" && req.Type != "image" && req.Type != "audio" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Type must be 'video', 'image', or 'audio'",
		})
	}

	// Validate Instagram URL
	if !utils.IsInstagramURL(req.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid Instagram URL",
		})
	}

	fmt.Printf("[Instagram] Download request: %s (type=%s)\n", req.URL, req.Type)

	// Extract post metadata from insta-extractor
	postData, err := services.Extract(req.URL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to extract post info: %v", err),
		})
	}

	// Determine download URL and output filename
	var downloadURL, outputFilename string
	switch req.Type {
	case "video":
		downloadURL = postData.GetVideoURL()
		outputFilename = "output.mp4"
		if downloadURL == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No video URL available for this post",
			})
		}
	case "image":
		downloadURL = postData.GetImageURL()
		outputFilename = "output.jpg"
		if downloadURL == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No image URL available for this post",
			})
		}
	case "audio":
		downloadURL = postData.GetVideoURL()
		outputFilename = "output.mp3"
		if downloadURL == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No video URL available (audio requires a video post)",
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

	// Get title
	title := postData.Caption
	if len(title) > 100 {
		title = title[:100]
	}
	if title == "" {
		title = "Instagram " + postData.Shortcode
	}

	// Get duration
	var duration float64
	if postData.VideoDuration != nil {
		duration = *postData.VideoDuration
	}

	// Get thumbnail (first display_url)
	thumbnail := postData.GetImageURL()

	// Create meta
	meta := &models.Meta{
		ID:           jobID,
		Status:       models.StatusDownloading,
		Title:        title,
		Duration:     duration,
		OutputType:   req.Type,
		Output:       outputFilename,
		CreatedAt:    time.Now().UnixMilli(),
		VideoURL:     postData.GetVideoURL(),
		ImageURL:     postData.GetImageURL(),
		ThumbnailURL: thumbnail,
		Author:       postData.OwnerUsername,
		SourceURL:    req.URL,
	}

	if err := utils.WriteMeta(jobID, meta); err != nil {
		utils.DeleteJobDir(jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to write metadata",
		})
	}

	// Start background job
	go processJob(jobID, req.URL, req.Type, downloadURL, outputFilename)

	// Return response
	response := models.DownloadResponse{
		StatusURL: utils.GenerateStatusURL(jobID),
		Type:      req.Type,
		Title:     title,
		Duration:  duration,
		Thumbnail: thumbnail,
	}

	fmt.Printf("[%s] Job created: %s → %s\n", jobID, req.Type, outputFilename)
	return c.Status(fiber.StatusCreated).JSON(response)
}

// processJob runs the download (and optional FFmpeg) in background
func processJob(jobID, postURL, outputType, downloadURL, outputFilename string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.DownloadTimeout)
	defer cancel()

	if outputType == "audio" {
		// For audio: download video first, then extract audio with FFmpeg
		videoFile := "temp_video.mp4"
		fileSize, err := services.Download(ctx, jobID, postURL, downloadURL, videoFile)
		if err != nil {
			fmt.Printf("[%s] ✗ Video download failed: %v\n", jobID, err)
			utils.UpdateMetaError(jobID, err.Error())
			return
		}

		fmt.Printf("[%s] Video downloaded (%.2f MB), extracting audio...\n", jobID, float64(fileSize)/1024/1024)

		// Update status to processing
		utils.UpdateMetaStatus(jobID, models.StatusProcessing)

		// Extract audio with FFmpeg
		videoPath := filepath.Join(utils.GetJobDir(jobID), videoFile)
		audioPath := filepath.Join(utils.GetJobDir(jobID), outputFilename)
		if err := services.ExtractAudio(videoPath, audioPath); err != nil {
			fmt.Printf("[%s] ✗ FFmpeg failed: %v\n", jobID, err)
			utils.UpdateMetaError(jobID, fmt.Sprintf("Audio extraction failed: %v", err))
			return
		}

		// Get audio file size
		audioSize := utils.GetFileSize(audioPath)
		if err := utils.UpdateMetaCompleted(jobID, outputFilename, audioSize); err != nil {
			fmt.Printf("[%s] ✗ Failed to update meta: %v\n", jobID, err)
			return
		}

		fmt.Printf("[%s] ✓ Audio job completed: %s (%.2f MB)\n", jobID, outputFilename, float64(audioSize)/1024/1024)
	} else {
		// For video/image: direct download
		fileSize, err := services.Download(ctx, jobID, postURL, downloadURL, outputFilename)
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
}
