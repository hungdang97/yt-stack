package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"tik-downloader/config"
	"tik-downloader/models"
	"tik-downloader/services"
	"tik-downloader/utils"
	"time"

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

	var req models.DownloadRequest
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

	downloadURL := videoData.Data.GetDownloads()
	if downloadURL == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No video download URL available"})
	}

	title := videoData.Data.Desc
	if title == "" {
		title = "TikTok Video " + videoID
	}
	duration := services.ParseDuration(videoData.Data.Duration)

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
		Author:    videoData.Data.Nickname,
		Duration:  duration,
		VideoFile: "video.mp4",
		AudioFile: "audio.mp3",
		SourceURL: req.URL,
	}

	if err := utils.WritePrepareMeta(jobID, meta); err != nil {
		utils.DeleteJobDir(jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to write metadata"})
	}

	go processPrepareJob(jobID, videoID, downloadURL, videoData.Data.MusicURL, videoData.Params.Cookie)

	// Build preview response
	proxyVideoURL := utils.GenerateMediaProxyURL(downloadURL)
	var proxyAudioURL string
	if videoData.Data.MusicURL != "" {
		proxyAudioURL = utils.GenerateMediaProxyURL(videoData.Data.MusicURL)
	}
	thumbnail := videoData.Data.StaticCover
	if thumbnail != "" {
		thumbnail = utils.GenerateMediaProxyURL(thumbnail)
	}

	return c.Status(fiber.StatusCreated).JSON(PrepareResponse{
		StatusURL: utils.GeneratePrepareStatusURL(jobID),
		Title:     title,
		Author:    videoData.Data.Nickname,
		Duration:  duration,
		Thumbnail: thumbnail,
		VideoURL:  proxyVideoURL,
		AudioURL:  proxyAudioURL,
		Subtitles: []SubtitleInfo{},
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

func processPrepareJob(jobID, videoID, videoURL, musicURL, cookie string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.DownloadTimeout)
	defer cancel()

	jobDir := utils.GetJobDir(jobID)

	// Download video (TikTok videos already have audio embedded)
	_, err := services.Download(ctx, jobID, videoID, videoURL, "video.mp4", cookie)
	if err != nil {
		fmt.Printf("[Prepare/%s] Video download failed: %v\n", jobID, err)
		utils.UpdatePrepareMetaError(jobID, err.Error())
		return
	}

	// Get audio: prefer separate music URL, else extract from video with ffmpeg
	outputAudioPath := filepath.Join(jobDir, "audio.mp3")
	if musicURL != "" {
		_, err := services.Download(ctx, jobID, videoID, musicURL, "audio.mp3", cookie)
		if err != nil {
			fmt.Printf("[Prepare/%s] Music download failed, extracting from video: %v\n", jobID, err)
			videoPath := filepath.Join(jobDir, "video.mp4")
			if err := extractAudioFromVideo(videoPath, outputAudioPath); err != nil {
				utils.UpdatePrepareMetaError(jobID, fmt.Sprintf("Audio extraction failed: %v", err))
				return
			}
		}
	} else {
		videoPath := filepath.Join(jobDir, "video.mp4")
		if err := extractAudioFromVideo(videoPath, outputAudioPath); err != nil {
			utils.UpdatePrepareMetaError(jobID, fmt.Sprintf("Audio extraction failed: %v", err))
			return
		}
	}

	utils.UpdatePrepareMetaCompleted(jobID)
	fmt.Printf("[Prepare/%s] Job completed\n", jobID)
}

// extractAudioFromVideo extracts audio track from video using ffmpeg
func extractAudioFromVideo(videoPath, audioPath string) error {
	// Remove output if exists
	os.Remove(audioPath)
	cmd := exec.Command("ffmpeg", "-i", videoPath, "-vn", "-acodec", "libmp3lame", "-q:a", "2", audioPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v, output: %s", err, string(output))
	}
	return nil
}
