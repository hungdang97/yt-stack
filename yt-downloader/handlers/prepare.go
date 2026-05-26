package handlers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
	"yt-downloader-go/services"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

type PrepareResponse struct {
	StatusURL         string         `json:"statusUrl"`
	Title             string         `json:"title"`
	Caption           string         `json:"caption,omitempty"`
	Author            string         `json:"author,omitempty"`
	Duration          float64        `json:"duration"`
	Thumbnail         string         `json:"thumbnail,omitempty"`
	OriginalThumbnail string         `json:"originalThumbnail,omitempty"`
	VideoURL          string         `json:"videoUrl,omitempty"`
	OriginalVideoURL  string         `json:"originalVideoUrl,omitempty"`
	AudioURL          string         `json:"audioUrl,omitempty"`
	OriginalAudioURL  string         `json:"originalAudioUrl,omitempty"`
	Subtitles         []SubtitleInfo `json:"subtitles"` // always [] or [single original-lang sub]
}

type PrepareStatusResponse struct {
	Status        string             `json:"status"`
	Progress      int                `json:"progress"`
	VideoURL      string             `json:"videoUrl,omitempty"`
	AudioURL      string             `json:"audioUrl,omitempty"`
	VideoProgress *PrepareFileStatus `json:"videoProgress"`
	AudioProgress *PrepareFileStatus `json:"audioProgress"`
	Error         string             `json:"error,omitempty"`
}

type PrepareFileStatus struct {
	Progress int    `json:"progress"`
	URL      string `json:"url,omitempty"`
}

// HandlePrepare handles POST /api/prepare — extracts metadata, starts background download of video+audio
func HandlePrepare(c *fiber.Ctx) error {

	var req models.DownloadRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid request body")
	}

	if req.URL == "" {
		return utils.BadRequest(c, utils.ErrValidationError, "URL is required")
	}

	videoID, err := utils.ExtractVideoID(req.URL)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidURL, err.Error())
	}

	extractData, err := services.Extract(videoID, req.Premium)
	if err != nil {
		return utils.InternalError(c, "Failed to fetch video metadata")
	}

	// Pick best H.264 video (widest compatibility)
	osType := req.OS
	if osType == "" {
		osType = "windows"
	}
	videoSelection := services.SelectVideo(extractData, "1080p", osType, "mp4")
	if videoSelection.Stream == nil {
		return utils.NotFound(c, utils.ErrVideoNotFound, "No compatible video stream found")
	}

	audioSelection := services.SelectAudio(extractData, "", osType, "mp4")
	if audioSelection.Stream == nil {
		return utils.NotFound(c, utils.ErrAudioNotFound, "No compatible audio stream found")
	}

	// Generate job ID
	jobID := generateID()
	if err := utils.CreateJobDir(jobID); err != nil {
		return utils.InternalError(c, "Failed to create job directory")
	}

	// Determine file extensions
	videoExt := services.GetExtension(videoSelection.Stream)
	audioExt := services.GetExtension(audioSelection.Stream)

	// Save prepare meta
	meta := &models.PrepareMeta{
		ID:        jobID,
		Status:    models.StatusPending,
		CreatedAt: time.Now().UnixMilli(),
		VideoID:   videoID,
		Title:     extractData.Title,
		Author:    extractData.Author,
		Duration:  extractData.Duration,
		VideoFile: "video." + videoExt,
		AudioFile: "audio." + audioExt,
		VideoSize: videoSelection.Stream.ContentLength,
		AudioSize: audioSelection.Stream.ContentLength,
	}

	if err := utils.WritePrepareMeta(jobID, meta); err != nil {
		utils.DeleteJobDir(jobID)
		return utils.InternalError(c, "Failed to save job metadata")
	}

	// Start background download
	threads := config.GetDownloadThreads(req.CTier)
	go processPrepareJob(jobID, meta, videoSelection, audioSelection.Stream, threads, req.Premium)

	// Build preview response (same as info + statusUrl)
	originalVideoURL := videoSelection.Stream.URL
	originalAudioURL := audioSelection.Stream.URL
	originalThumbnail := extractData.ThumbnailURL

	proxyVideoURL := utils.GenerateMediaProxyURL(originalVideoURL)
	proxyAudioURL := utils.GenerateMediaProxyURL(originalAudioURL)

	thumbnail := originalThumbnail
	if thumbnail != "" {
		thumbnail = utils.GenerateMediaProxyURL(thumbnail)
	}

	subtitles := pickOriginalSubtitle(extractData)

	return c.Status(fiber.StatusCreated).JSON(PrepareResponse{
		StatusURL:         utils.GeneratePrepareStatusURL(jobID),
		Title:             extractData.Title,
		Author:            extractData.Author,
		Duration:          extractData.Duration,
		Thumbnail:         thumbnail,
		OriginalThumbnail: originalThumbnail,
		VideoURL:          proxyVideoURL,
		OriginalVideoURL:  originalVideoURL,
		AudioURL:          proxyAudioURL,
		OriginalAudioURL:  originalAudioURL,
		Subtitles:         subtitles,
	})
}

// pickOriginalSubtitle returns a slice containing at most ONE SubtitleInfo:
// the subtitle in the video's original audio language. Empty slice if no
// suitable subtitle found.
//
// Selection priority:
//  1. Manual subtitle in original lang (best quality)
//  2. Auto-generated subtitle in original lang
//  3. Manual subtitle in any language (fallback)
//  4. Auto-generated subtitle in any language (last resort)
//
// URL is a simple media proxy — caller GETs raw json3 bytes for downstream
// processing (e.g. POST to /api/caption with source_type=youtube).
//
// Original language detection: prefer AudioStream where IsOriginal=true,
// fallback to AvailableAudioLanguages[0]. Match supports prefix (vi vs vi-VN).
func pickOriginalSubtitle(extractData *models.ExtractResponse) []SubtitleInfo {
	if extractData == nil || len(extractData.Subtitles) == 0 {
		return []SubtitleInfo{}
	}

	// 1. Detect original audio language
	originalLang := ""
	for _, s := range extractData.AudioStreams {
		if s.IsOriginal && s.AudioTrackID != "" {
			originalLang = s.AudioTrackID
			break
		}
	}
	if originalLang == "" && len(extractData.AvailableAudioLanguages) > 0 {
		originalLang = extractData.AvailableAudioLanguages[0]
	}

	// 2. Score each subtitle and pick best by priority
	var (
		manualMatch *models.Subtitle
		autoMatch   *models.Subtitle
		manualAny   *models.Subtitle
		autoAny     *models.Subtitle
	)
	for i := range extractData.Subtitles {
		s := &extractData.Subtitles[i]
		if s.URL == "" {
			continue
		}
		isLangMatch := originalLang != "" && langMatches(s.Lang, originalLang)

		if isLangMatch && !s.AutoGenerated && manualMatch == nil {
			manualMatch = s
		}
		if isLangMatch && s.AutoGenerated && autoMatch == nil {
			autoMatch = s
		}
		if !s.AutoGenerated && manualAny == nil {
			manualAny = s
		}
		if s.AutoGenerated && autoAny == nil {
			autoAny = s
		}
	}

	pick := manualMatch
	if pick == nil {
		pick = autoMatch
	}
	if pick == nil {
		pick = manualAny
	}
	if pick == nil {
		pick = autoAny
	}
	if pick == nil {
		return []SubtitleInfo{}
	}

	return []SubtitleInfo{{
		Lang:          pick.Lang,
		URL:           utils.GenerateMediaProxyURL(pick.URL),
		OriginalURL:   pick.URL,
		AutoGenerated: pick.AutoGenerated,
	}}
}

// langMatches returns true for exact match or prefix match (vi == vi-VN, en == en-US).
func langMatches(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+"-") || strings.HasPrefix(b, a+"-")
}

// HandlePrepareStatus handles GET /api/prepare/status/:id
func HandlePrepareStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if !utils.ValidateJobID(jobID) {
		return utils.BadRequest(c, utils.ErrInvalidJobID, "Invalid job ID format")
	}

	token := c.Query("token")
	expiresStr := c.Query("expires")
	if token == "" || expiresStr == "" {
		return utils.Unauthorized(c, "Missing token or expires parameter")
	}

	expires, err := utils.ParseExpires(expiresStr)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid expires format")
	}

	if !utils.ValidatePrepareStatusURL(jobID, token, expires) {
		return utils.Forbidden(c, "Invalid or expired token")
	}

	meta, err := utils.ReadPrepareMeta(jobID)
	if err != nil {
		return utils.NotFound(c, utils.ErrJobNotFound, "Job not found")
	}

	videoProgress, audioProgress := utils.CalculatePrepareProgressSeparate(meta)

	videoStatus := &PrepareFileStatus{Progress: videoProgress}
	audioStatus := &PrepareFileStatus{Progress: audioProgress}

	// Return signed URL as soon as each file is done (progress == 100)
	if videoProgress == 100 {
		videoStatus.URL = utils.GenerateSignedURL(jobID, meta.VideoFile)
	}
	if audioProgress == 100 {
		audioStatus.URL = utils.GenerateSignedURL(jobID, meta.AudioFile)
	}

	overallProgress := int(float64(videoProgress)*0.7 + float64(audioProgress)*0.3)

	response := PrepareStatusResponse{
		Status:        meta.Status,
		Progress:      min(overallProgress, 100),
		VideoProgress: videoStatus,
		AudioProgress: audioStatus,
	}

	if meta.Status == models.StatusCompleted {
		response.Progress = 100
		response.VideoURL = videoStatus.URL
		response.AudioURL = audioStatus.URL
	}

	if meta.Status == models.StatusError {
		response.Error = meta.Error
	}

	return c.JSON(response)
}

func processPrepareJob(jobID string, meta *models.PrepareMeta, videoSelection *models.VideoSelectionResult, audioStream *models.Stream, threads int, premium bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	jobDir := utils.GetJobDir(jobID)

	defer func() {
		if r := recover(); r != nil {
			utils.UpdatePrepareMetaError(jobID, "Internal error")
		}
	}()

	// Download video and audio in parallel
	errChan := make(chan error, 2)

	go func() {
		videoPath := filepath.Join(jobDir, meta.VideoFile)
		errChan <- services.Download(ctx, makeURLProvider(meta.VideoID, videoSelection.Stream, true, premium), videoPath, videoSelection.Stream.ContentLength, threads)
	}()

	go func() {
		audioPath := filepath.Join(jobDir, meta.AudioFile)
		errChan <- services.Download(ctx, makeURLProvider(meta.VideoID, audioStream, false, premium), audioPath, audioStream.ContentLength, threads)
	}()

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			utils.UpdatePrepareMetaError(jobID, "Download failed: "+err.Error())
			return
		}
	}

	utils.UpdatePrepareMetaCompleted(jobID)
	fmt.Printf("[Prepare/%s] Job completed: video=%s audio=%s\n", jobID, meta.VideoFile, meta.AudioFile)
}
