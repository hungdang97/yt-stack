package handlers

import (
	"context"
	"fmt"
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
	// Validate hub token - only allow requests from hub
	const hubToken = "1234567890987654321234567890987654321"
	token := c.Get("X-Hub-Token")
	if token != hubToken {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Unauthorized: Invalid or missing hub token",
		})
	}

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

	extractData, err := services.Extract(videoID, req.Premium)
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
	threads := config.GetDownloadThreads(req.CTier)

	// Select streams
	var videoSelection *models.VideoSelectionResult
	var audioSelection *models.AudioSelectionResult
	var audioStream *models.Stream

	if req.Output.Type == "video" {
		videoSelection = services.SelectVideo(extractData, req.Output.Quality, osType, req.Output.Format)
		if videoSelection.Stream == nil {
			return utils.NotFound(c, utils.ErrVideoNotFound, "No compatible video stream found")
		}
		audioSelection = services.SelectAudio(extractData, req.Audio.TrackID, osType, req.Output.Format)
		if audioSelection.Stream == nil {
			return utils.NotFound(c, utils.ErrAudioNotFound, "No compatible audio stream found")
		}
		audioStream = audioSelection.Stream
	} else {
		audioSelection = services.SelectAudio(extractData, req.Audio.TrackID, osType, req.Output.Format)
		if audioSelection.Stream == nil {
			return utils.NotFound(c, utils.ErrAudioNotFound, "No compatible audio stream found")
		}
		audioStream = audioSelection.Stream
	}

	// Generate job ID
	jobID := generateID()

	// Create job directory
	if err := utils.CreateJobDir(jobID); err != nil {
		return utils.InternalError(c, "Failed to create job directory")
	}

	// Prepare metadata
	meta := &models.Meta{
		ID:             jobID,
		Status:         models.StatusPending,
		CreatedAt:      time.Now().UnixMilli(),
		VideoID:        videoID,
		CTier:          req.CTier,
		Title:          extractData.Title,
		Author:         extractData.Author,
		Duration:       extractData.Duration,
		OutputType:     req.Output.Type,
		Format:         req.Output.Format,
		Bitrate:        bitrate,
		Trim:           req.Trim,
		FilenameStyle:  req.FilenameStyle,
		EnableMetadata: req.EnableMetadata,
		ThumbnailURL:   extractData.ThumbnailURL,
		Files:          models.FilesInfo{},
		NeedsReencode:  calculateNeedsReencode(videoSelection, audioStream, req.Output.Format),
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
	go processJob(jobID, meta, videoSelection, audioStream, req.Output.Format, bitrate, threads, req.Premium)

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

	// Add audio language information
	if audioSelection != nil {
		response.AvailableAudioLanguages = audioSelection.AvailableAudioLanguages
		response.AudioLanguageChanged = audioSelection.AudioLanguageChanged
		response.AudioLanguageChangeReason = audioSelection.AudioLanguageChangeReason
	}

	return c.JSON(response)
}

// processJob handles the background download and processing
func processJob(jobID string, meta *models.Meta, videoSelection *models.VideoSelectionResult, audioStream *models.Stream, format string, bitrate string, threads int, premium bool) {
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
			errChan <- services.Download(ctx, makeURLProvider(meta.VideoID, videoSelection.Stream, true, premium), videoPath, videoSelection.Stream.ContentLength, threads)
		}()

		go func() {
			audioPath := jobDir + "/" + meta.Files.Audio.Name
			errChan <- services.Download(ctx, makeURLProvider(meta.VideoID, audioStream, false, premium), audioPath, audioStream.ContentLength, threads)
		}()

		for i := 0; i < 2; i++ {
			if err := <-errChan; err != nil {
				utils.UpdateMetaError(jobID, "Download failed: "+err.Error())
				return
			}
		}
	} else {
		audioPath := jobDir + "/" + meta.Files.Audio.Name
		if err := services.Download(ctx, makeURLProvider(meta.VideoID, audioStream, false, premium), audioPath, audioStream.ContentLength, threads); err != nil {
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
		outputFile, err = services.FFmpegMerge(jobDir, format, meta.Files.Video.Name, meta.Files.Audio.Name, meta.NeedsReencode)
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

	// Embed metadata (title, artist, thumbnail) if enabled
	if meta.EnableMetadata {
		// Update meta.Output before embedding so FFmpegEmbedMetadata knows the file
		meta.Output = outputFile
		if err := services.FFmpegEmbedMetadata(jobDir, meta); err != nil {
			fmt.Printf("[%s] Warning: metadata embedding failed: %v\n", jobID, err)
			// Non-fatal: continue without metadata
		}
	}

	utils.CleanupTempFiles(jobID)
	utils.UpdateMetaOutput(jobID, outputFile)
}

// shouldMerge determines if the job should be pre-merged or stream-only
// Strategy: Prevent server overload from heavy transcoding
// - All transcoding jobs → STREAM (except audio < 5 minutes)
// - Audio < 5 minutes → can merge even if needs transcoding
// - Non-transcoding: Audio ≤ 3 min merge, Video ≤ 4 hours merge
func shouldMerge(meta *models.Meta) bool {
	const (
		maxDurationAudio = 5 * 60.0   // 5 minutes for audio
		maxDurationVideo = 4 * 3600.0 // 4 hours for video
	)

	// Check if needs transcoding (heavy CPU)
	transcode := needsTranscode(meta)

	if transcode {
		// Exception: Audio < 5 minutes can still merge even with transcoding
		if meta.OutputType == "audio" && meta.Duration < maxDurationAudio {
			return true
		}
		// All other transcoding jobs must stream to prevent server overload
		return false
	}

	// No transcoding needed - use duration thresholds
	if meta.OutputType == "audio" {
		return meta.Duration <= maxDurationAudio
	}

	// Video
	return meta.Duration <= maxDurationVideo
}

// needsTranscode checks if the job requires transcoding (heavy CPU)
// Returns true for:
// - Video codec re-encoding for format compatibility (e.g., VP9→H.264 for MP4)
// - Audio format conversion (e.g., webm→mp3)
// - Video with accurate trim (requires re-encoding)
func needsTranscode(meta *models.Meta) bool {
	// Video with accurate trim needs re-encoding
	if meta.OutputType == "video" && meta.Trim != nil && meta.Trim.Accurate {
		return true
	}

	// Video needs re-encoding for format compatibility
	if meta.OutputType == "video" && meta.NeedsReencode {
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

// calculateNeedsReencode determines if re-encoding is needed based on video and audio codec compatibility
func calculateNeedsReencode(videoSelection *models.VideoSelectionResult, audioStream *models.Stream, targetFormat string) bool {
	// For audio-only downloads, no video codec check needed
	if videoSelection == nil {
		return false
	}

	// Check video codec compatibility
	if videoSelection.NeedsReencode {
		return true
	}

	// Check audio codec compatibility
	if audioStream != nil {
		audioCodec := services.GetStreamCodec(audioStream)
		if !services.IsAudioCodecCompatible(audioCodec, targetFormat) {
			return true
		}
	}

	return false
}

// makeURLProvider creates a URLProvider with refresh logic
func makeURLProvider(videoID string, targetStream *models.Stream, isVideo bool, premium bool) *services.URLProvider {
	return &services.URLProvider{
		CurrentURL: targetStream.URL,
		RefreshFunc: func() (string, error) {
			fmt.Printf("[Refresh] Refreshing URL for %s (isVideo=%v)...\n", videoID, isVideo)
			newData, err := services.Extract(videoID, premium)
			if err != nil {
				return "", fmt.Errorf("extract failed: %w", err)
			}

			var newStream *models.Stream
			if isVideo {
				newStream = services.FindEquivalentVideoStream(targetStream, newData.VideoStreams)
			} else {
				newStream = services.FindEquivalentAudioStream(targetStream, newData.AudioStreams)
			}

			if newStream == nil {
				return "", fmt.Errorf("matching stream not found")
			}
			fmt.Printf("[Refresh] Success! New URL: %s...\n", newStream.URL[:50])
			return newStream.URL, nil
		},
	}
}
