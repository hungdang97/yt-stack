package handlers

import (
	"context"
	"path/filepath"
	"time"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
	"yt-downloader-go/services"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/jaevor/go-nanoid"
)

var generateID func() string

func init() {
	// Initialize nanoid generator
	var err error
	generateID, err = nanoid.Standard(config.JobIDLength)
	if err != nil {
		panic(err)
	}
}

// HandleDownload handles POST /api/download
// @Summary Create download job
// @Description Create a new download job for a YouTube video or audio
// @Tags download
// @Accept json
// @Produce json
// @Param request body models.DownloadRequest true "Download request"
// @Success 200 {object} models.DownloadResponse
// @Failure 400 {object} utils.ErrorResponse "Validation error"
// @Failure 404 {object} utils.ErrorResponse "No stream found"
// @Failure 500 {object} utils.ErrorResponse "Server error"
// @Router /api/download [post]
func HandleDownload(c *fiber.Ctx) error {
	var req models.DownloadRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid request body")
	}

	// Validate request
	if err := utils.ValidateDownloadRequest(&req); err != nil {
		return utils.BadRequest(c, utils.ErrValidationError, err.Error())
	}

	// Extract video ID
	videoID, err := utils.ExtractVideoID(req.URL)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidURL, err.Error())
	}

	extractData, err := services.Extract(videoID)
	if err != nil {
		return utils.InternalError(c, "Failed to fetch video metadata")
	}

	// Set default values
	osType := req.OS
	if osType == "" {
		osType = "windows"
	}
	bitrate := req.Audio.Bitrate
	if bitrate == "" {
		bitrate = "192k"
	}

	// Calculate thread count based on customer tier
	// Tier 1 (Premium): 4 threads, Others (Standard): 1 thread
	threads := 1
	if req.CTier == 1 {
		threads = 4
	}

	// Select streams
	var videoSelection *models.VideoSelectionResult
	var audioStream *models.Stream

	if req.Output.Type == "video" {
		videoSelection = services.SelectVideo(extractData, req.Output.Quality, osType)
		if videoSelection.Stream == nil {
			return utils.NotFound(c, utils.ErrVideoNotFound, "No compatible video stream found")
		}
		audioStream = services.SelectAudio(extractData, req.Audio.TrackID, osType)
		if audioStream == nil {
			return utils.NotFound(c, utils.ErrAudioNotFound, "No compatible audio stream found")
		}
	} else {
		audioStream = services.SelectAudio(extractData, req.Audio.TrackID, osType)
		if audioStream == nil {
			return utils.NotFound(c, utils.ErrAudioNotFound, "No compatible audio stream found")
		}
	}

	// Generate job ID
	jobID := generateID()

	// Create job directory
	if err := utils.CreateJobDir(jobID); err != nil {
		return utils.InternalError(c, "Failed to create job directory")
	}

	// Prepare metadata
	meta := &models.Meta{
		ID:         jobID,
		Status:     models.StatusPending,
		CreatedAt:  time.Now().UnixMilli(),
		VideoID:    videoID,
		Title:      extractData.Title,
		Duration:   extractData.Duration,
		OutputType: req.Output.Type,
		Format:     req.Output.Format,
		Bitrate:    bitrate,
		Trim:       req.Trim,
		Files:      models.FilesInfo{},
	}

	// Set file info
	if req.Output.Type == "video" {
		videoExt := services.GetExtension(videoSelection.Stream)
		audioExt := services.GetExtension(audioStream)
		meta.Quality = videoSelection.SelectedQuality
		meta.Files.Video = &models.FileInfo{
			Name: "video." + videoExt,
			Size: videoSelection.Stream.ContentLength,
		}
		meta.Files.Audio = &models.FileInfo{
			Name: "audio." + audioExt,
			Size: audioStream.ContentLength,
		}
	} else {
		audioExt := services.GetExtension(audioStream)
		meta.Files.Audio = &models.FileInfo{
			Name: "audio." + audioExt,
			Size: audioStream.ContentLength,
		}
	}

	// Save metadata
	if err := utils.WriteMeta(jobID, meta); err != nil {
		utils.DeleteJobDir(jobID)
		return utils.InternalError(c, "Failed to save job metadata")
	}

	// Start background processing
	go processJob(jobID, meta, videoSelection, audioStream, req.Output.Format, bitrate, threads)

	// Build response
	response := models.DownloadResponse{
		StatusURL: utils.GenerateStatusURL(jobID),
		Title:     extractData.Title,
		Duration:  extractData.Duration,
	}

	if req.Output.Type == "video" && videoSelection != nil {
		response.RequestedQuality = req.Output.Quality
		response.SelectedQuality = videoSelection.SelectedQuality
		response.QualityChanged = videoSelection.QualityChanged
		response.QualityChangeReason = videoSelection.QualityChangeReason
		response.NeedsReencode = videoSelection.NeedsReencode
	}

	return c.JSON(response)
}

// processJob handles the background download and processing
func processJob(jobID string, meta *models.Meta, videoSelection *models.VideoSelectionResult, audioStream *models.Stream, format string, bitrate string, threads int) {
	// Timeout: 30 minutes max per job to prevent zombie goroutines
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	jobDir := utils.GetJobDir(jobID)

	defer func() {
		if r := recover(); r != nil {
			utils.UpdateMetaError(jobID, "Internal error")
		}
	}()

	if meta.OutputType == "video" {
		// Download video and audio in parallel
		errChan := make(chan error, 2)

		go func() {
			videoPath := jobDir + "/" + meta.Files.Video.Name
			errChan <- services.Download(ctx, videoSelection.Stream.URL, videoPath, videoSelection.Stream.ContentLength, threads)
		}()

		go func() {
			audioPath := jobDir + "/" + meta.Files.Audio.Name
			errChan <- services.Download(ctx, audioStream.URL, audioPath, audioStream.ContentLength, threads)
		}()

		for i := 0; i < 2; i++ {
			if err := <-errChan; err != nil {
				utils.UpdateMetaError(jobID, "Download failed: "+err.Error())
				return
			}
		}
	} else {
		audioPath := jobDir + "/" + meta.Files.Audio.Name
		if err := services.Download(ctx, audioStream.URL, audioPath, audioStream.ContentLength, threads); err != nil {
			utils.UpdateMetaError(jobID, "Download failed: "+err.Error())
			return
		}
	}

	if !shouldMerge(meta) {
		utils.UpdateMetaStreamOnly(jobID)
		return
	}

	// Process with FFmpeg
	var outputFile string
	var err error

	if meta.OutputType == "video" {
		outputFile, err = services.FFmpegMerge(jobDir, format, meta.Files.Video.Name, meta.Files.Audio.Name)
		if err != nil {
			utils.UpdateMetaError(jobID, "Processing failed: "+err.Error())
			return
		}

		if meta.Trim != nil {
			outputFile, err = services.FFmpegTrim(jobDir, format, meta.Trim, bitrate)
			if err != nil {
				utils.UpdateMetaError(jobID, "Trim failed: "+err.Error())
				return
			}
		}
	} else {
		outputFile, err = services.FFmpegConvertAudio(jobDir, format, bitrate, meta.Files.Audio.Name)
		if err != nil {
			utils.UpdateMetaError(jobID, "Conversion failed: "+err.Error())
			return
		}

		if meta.Trim != nil {
			outputFile, err = services.FFmpegTrimAudio(jobDir, format, meta.Trim, bitrate)
			if err != nil {
				utils.UpdateMetaError(jobID, "Trim failed: "+err.Error())
				return
			}
		}
	}

	utils.CleanupTempFiles(jobID)
	utils.UpdateMetaOutput(jobID, outputFile)
}

// shouldMerge determines if the job should be pre-merged or stream-only
// Strategy: minimize CPU usage
// - Heavy tasks (transcode): threshold 15 minutes
// - Light tasks (remux/copy): threshold 4 hours
func shouldMerge(meta *models.Meta) bool {
	const (
		maxDurationTranscode = 15 * 60.0  // 15 minutes - heavy CPU (transcode)
		maxDurationRemux     = 4 * 3600.0 // 4 hours - light CPU (remux/copy)
	)

	// Check if this job needs transcoding (heavy CPU)
	transcode := needsTranscode(meta)

	if transcode {
		return meta.Duration <= maxDurationTranscode
	}
	return meta.Duration <= maxDurationRemux
}

// needsTranscode checks if the job requires transcoding (heavy CPU)
// Returns true for:
// - Audio format conversion (e.g., webm→mp3)
// - Video with accurate trim (requires re-encoding)
func needsTranscode(meta *models.Meta) bool {
	// Video with accurate trim needs re-encoding
	if meta.OutputType == "video" && meta.Trim != nil && meta.Trim.Accurate {
		return true
	}

	// Audio: check if format conversion is needed
	if meta.OutputType == "audio" {
		return needsAudioTranscode(meta)
	}

	return false
}

// needsAudioTranscode checks if audio format requires transcoding
func needsAudioTranscode(meta *models.Meta) bool {
	if meta.Files.Audio == nil {
		return false
	}

	inputExt := filepath.Ext(meta.Files.Audio.Name)
	if len(inputExt) > 0 && inputExt[0] == '.' {
		inputExt = inputExt[1:]
	}

	outputFormat := meta.Format

	// Same format: no transcode
	if inputExt == outputFormat {
		return false
	}
	// m4a/mp4 compatible: no transcode
	if (inputExt == "m4a" || inputExt == "mp4") && (outputFormat == "m4a" || outputFormat == "mp4") {
		return false
	}
	// webm to opus: no transcode (YouTube webm contains Opus)
	if inputExt == "webm" && outputFormat == "opus" {
		return false
	}

	return true
}
