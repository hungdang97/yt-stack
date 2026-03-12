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

	// Resolve type: prefer output.type (Hub format), fallback to type (simple format)
	if req.Output.Type != "" {
		req.Type = req.Output.Type
	}
	if req.Type == "" {
		req.Type = "video"
	}
	if req.Type != "video" && req.Type != "audio" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Type must be 'video' or 'audio'",
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

	// Get best video and audio URLs
	videoURL := postData.GetVideoURL()
	audioURL := postData.GetAudioURL()

	if videoURL == "" && audioURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No download URL available for this post",
		})
	}

	if req.Type == "audio" && audioURL == "" && videoURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No audio available for this post",
		})
	}

	if req.Type == "video" && videoURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No video URL available for this post",
		})
	}

	var outputFilename string
	switch req.Type {
	case "video":
		outputFilename = "output.mp4"
	case "audio":
		outputFilename = "output.mp3"
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
		VideoURL:     videoURL,
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
	go processJob(jobID, req.URL, req.Type, videoURL, audioURL, outputFilename)

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

// processJob downloads best quality video/audio and processes to requested format
func processJob(jobID, postURL, outputType, videoURL, audioURL, outputFilename string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.DownloadTimeout)
	defer cancel()

	outputPath := filepath.Join(utils.GetJobDir(jobID), outputFilename)

	switch outputType {
	case "audio":
		// Audio request: download audio stream directly, convert to MP3
		if audioURL != "" {
			// Direct audio download (best quality DASH audio)
			audioFile := "temp_audio.m4a"
			fileSize, err := services.Download(ctx, jobID, postURL, audioURL, audioFile)
			if err != nil {
				fmt.Printf("[%s] ✗ Audio download failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, err.Error())
				return
			}
			fmt.Printf("[%s] Audio downloaded (%.2f MB)\n", jobID, float64(fileSize)/1024/1024)

			utils.UpdateMetaStatus(jobID, models.StatusProcessing)
			audioPath := filepath.Join(utils.GetJobDir(jobID), audioFile)
			if err := services.ConvertToMP3(audioPath, outputPath); err != nil {
				fmt.Printf("[%s] ✗ FFmpeg convert to mp3 failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, fmt.Sprintf("Audio conversion failed: %v", err))
				return
			}
		} else {
			// Fallback: download video, extract audio
			videoFile := "temp_video.mp4"
			fileSize, err := services.Download(ctx, jobID, postURL, videoURL, videoFile)
			if err != nil {
				fmt.Printf("[%s] ✗ Video download failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, err.Error())
				return
			}
			fmt.Printf("[%s] Video downloaded for audio extraction (%.2f MB)\n", jobID, float64(fileSize)/1024/1024)

			utils.UpdateMetaStatus(jobID, models.StatusProcessing)
			videoPath := filepath.Join(utils.GetJobDir(jobID), videoFile)
			if err := services.ExtractAudio(videoPath, outputPath); err != nil {
				fmt.Printf("[%s] ✗ FFmpeg audio extract failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, fmt.Sprintf("Audio extraction failed: %v", err))
				return
			}
		}

	case "video":
		// Video request: download best video
		videoFile := "temp_video.mp4"
		fileSize, err := services.Download(ctx, jobID, postURL, videoURL, videoFile)
		if err != nil {
			fmt.Printf("[%s] ✗ Video download failed: %v\n", jobID, err)
			utils.UpdateMetaError(jobID, err.Error())
			return
		}
		fmt.Printf("[%s] Video downloaded (%.2f MB)\n", jobID, float64(fileSize)/1024/1024)

		videoPath := filepath.Join(utils.GetJobDir(jobID), videoFile)

		utils.UpdateMetaStatus(jobID, models.StatusProcessing)

		// Check if video has audio — if not, merge with separate audio stream
		if audioURL != "" && !services.HasAudioStream(videoPath) {
			fmt.Printf("[%s] Video has no audio, downloading separate audio stream...\n", jobID)
			audioFile := "temp_audio.m4a"
			_, err := services.Download(ctx, jobID, postURL, audioURL, audioFile)
			if err != nil {
				fmt.Printf("[%s] ✗ Audio download failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, fmt.Sprintf("Audio download failed: %v", err))
				return
			}
			audioPath := filepath.Join(utils.GetJobDir(jobID), audioFile)
			if err := services.MergeVideoAudio(videoPath, audioPath, outputPath); err != nil {
				fmt.Printf("[%s] ✗ FFmpeg merge failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, fmt.Sprintf("Video+audio merge failed: %v", err))
				return
			}
		} else {
			// Video already has audio, just remux
			if err := services.RemuxVideo(videoPath, outputPath); err != nil {
				fmt.Printf("[%s] ✗ FFmpeg remux failed: %v\n", jobID, err)
				utils.UpdateMetaError(jobID, fmt.Sprintf("Video processing failed: %v", err))
				return
			}
		}
	}

	finalSize := utils.GetFileSize(outputPath)
	if err := utils.UpdateMetaCompleted(jobID, outputFilename, finalSize); err != nil {
		fmt.Printf("[%s] ✗ Failed to update meta: %v\n", jobID, err)
		return
	}

	fmt.Printf("[%s] ✓ Job completed: %s (%.2f MB)\n", jobID, outputFilename, float64(finalSize)/1024/1024)
}
