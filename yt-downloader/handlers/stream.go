package handlers

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleStream handles GET /stream/:id
// @Summary Stream video/audio
// @Description Stream video/audio using FFmpeg pipe (realtime remux/convert)
// @Tags stream
// @Produce octet-stream
// @Param id path string true "Job ID"
// @Param token query string true "Signed URL token"
// @Param expires query integer true "Expiration timestamp"
// @Success 200 {file} binary "Media stream"
// @Success 307 "Redirect to download URL"
// @Failure 400 {object} utils.ErrorResponse "Invalid parameters"
// @Failure 401 {object} utils.ErrorResponse "Missing auth"
// @Failure 403 {object} utils.ErrorResponse "Invalid token"
// @Failure 404 {object} utils.ErrorResponse "Not found"
// @Failure 500 {object} utils.ErrorResponse "Stream failed"
// @Router /stream/{id} [get]
func HandleStream(c *fiber.Ctx) error {
	jobID := c.Params("id")
	token := c.Query("token")
	expiresStr := c.Query("expires")

	// Validate job ID
	if !utils.ValidateJobID(jobID) {
		return utils.BadRequest(c, utils.ErrInvalidJobID, "Invalid job ID format")
	}

	// Validate signed URL
	if token == "" || expiresStr == "" {
		return utils.Unauthorized(c, "Missing token or expires parameter")
	}

	expires, err := utils.ParseExpires(expiresStr)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidExpires, "Invalid expires parameter")
	}

	if !utils.ValidateStreamURL(jobID, token, expires) {
		return utils.Forbidden(c, "Invalid or expired stream link")
	}

	// Check if job exists
	if !utils.JobExists(jobID) {
		return utils.NotFound(c, utils.ErrJobNotFound, "Job not found")
	}

	// Read metadata
	meta, err := utils.ReadMeta(jobID)
	if err != nil {
		return utils.InternalError(c, "Failed to read job metadata")
	}

	// Check if job is ready for streaming
	if meta.Status != models.StatusCompleted {
		return utils.BadRequest(c, utils.ErrJobNotReady, "Job is not ready for streaming")
	}

	// If already merged (not stream-only), redirect to file download
	if meta.Output != "" && !meta.StreamOnly {
		downloadURL := utils.GenerateSignedURL(jobID, meta.Output)
		return c.Redirect(downloadURL, fiber.StatusTemporaryRedirect)
	}

	// Stream based on output type
	if meta.OutputType == "video" {
		return streamVideo(c, meta)
	}
	return streamAudio(c, meta)
}

// streamVideo streams merged video+audio using FFmpeg remux
func streamVideo(c *fiber.Ctx, meta *models.Meta) error {
	jobDir := utils.GetJobDir(meta.ID)
	videoPath := filepath.Join(jobDir, meta.Files.Video.Name)
	audioPath := filepath.Join(jobDir, meta.Files.Audio.Name)

	// Check files exist
	if _, err := os.Stat(videoPath); err != nil {
		return utils.NotFound(c, utils.ErrFileNotFound, "Video file not found")
	}
	if _, err := os.Stat(audioPath); err != nil {
		return utils.NotFound(c, utils.ErrFileNotFound, "Audio file not found")
	}

	// Determine output format and content type
	format := meta.Format
	contentType := utils.ContentTypeFromExt(format)

	// Generate filename
	filename := utils.GenerateOutputFilename(meta)
	encodedFilename := url.PathEscape(filename)

	// Set response headers
	c.Set("Content-Type", contentType)
	c.Set("Transfer-Encoding", "chunked")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, encodedFilename))
	c.Set("Cache-Control", "no-cache")

	// Build FFmpeg command for remuxing (no re-encoding, very light CPU)
	args := []string{
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-f", getFFmpegFormat(format),
	}

	// Add movflags for streamable MP4
	if format == "mp4" {
		args = append(args, "-movflags", "frag_keyframe+empty_moov+faststart")
	}

	args = append(args, "pipe:1")

	return runFFmpegStream(c, args)
}

// streamAudio streams audio, with transcoding if needed
func streamAudio(c *fiber.Ctx, meta *models.Meta) error {
	jobDir := utils.GetJobDir(meta.ID)
	audioPath := filepath.Join(jobDir, meta.Files.Audio.Name)

	// Check file exists
	if _, err := os.Stat(audioPath); err != nil {
		return utils.NotFound(c, utils.ErrFileNotFound, "Audio file not found")
	}

	// Determine output format
	format := meta.Format
	contentType := utils.ContentTypeFromExt(format)

	// Generate filename
	filename := utils.GenerateOutputFilename(meta)
	encodedFilename := url.PathEscape(filename)

	// Set response headers
	c.Set("Content-Type", contentType)
	c.Set("Transfer-Encoding", "chunked")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, encodedFilename))
	c.Set("Cache-Control", "no-cache")

	// Check if transcoding is needed
	inputExt := filepath.Ext(meta.Files.Audio.Name)
	if len(inputExt) > 0 && inputExt[0] == '.' {
		inputExt = inputExt[1:]
	}

	var args []string

	if canCopyAudioStream(inputExt, format) {
		args = []string{
			"-y",
			"-i", audioPath,
			"-c:a", "copy",
			"-f", getFFmpegFormat(format),
			"pipe:1",
		}
	} else {
		codec := config.AudioCodecMap[format]
		if codec == "" {
			codec = "aac"
		}

		bitrate := meta.Bitrate
		if bitrate == "" {
			bitrate = "192k"
		}

		args = []string{
			"-y",
			"-i", audioPath,
			"-vn",
			"-c:a", codec,
		}

		// Add bitrate for lossy codecs
		if codec != "pcm_s16le" && codec != "flac" {
			args = append(args, "-b:a", bitrate)
		}

		args = append(args, "-f", getFFmpegFormat(format), "pipe:1")
	}

	return runFFmpegStream(c, args)
}

func runFFmpegStream(c *fiber.Ctx, args []string) error {
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return utils.InternalError(c, "Failed to start stream")
	}

	if err := cmd.Start(); err != nil {
		return utils.InternalError(c, "Failed to start stream")
	}

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			stdout.Close()
			cmd.Wait()
		}()

		buf := make([]byte, 64*1024)
		rateLimit := config.StreamRateLimit

		var startTime time.Time
		var totalBytes int64

		if rateLimit > 0 {
			startTime = time.Now()
		}

		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					cmd.Process.Kill()
					return
				}
				w.Flush()

				if rateLimit > 0 {
					totalBytes += int64(n)
					expectedDuration := time.Duration(totalBytes) * time.Second / time.Duration(rateLimit)
					actualDuration := time.Since(startTime)
					if expectedDuration > actualDuration {
						time.Sleep(expectedDuration - actualDuration)
					}
				}
			}
			if err != nil {
				return
			}
		}
	})

	return nil
}

// getFFmpegFormat returns the FFmpeg format name for a given extension
func getFFmpegFormat(ext string) string {
	switch ext {
	case "mp4":
		return "mp4"
	case "webm":
		return "webm"
	case "mkv":
		return "matroska"
	case "mp3":
		return "mp3"
	case "m4a":
		return "ipod" // FFmpeg uses "ipod" for m4a
	case "opus":
		return "opus"
	case "wav":
		return "wav"
	case "flac":
		return "flac"
	default:
		return ext
	}
}

// canCopyAudioStream checks if audio can be copied without re-encoding
func canCopyAudioStream(inputExt, outputFormat string) bool {
	if inputExt == outputFormat {
		return true
	}
	if (inputExt == "m4a" || inputExt == "mp4") && (outputFormat == "m4a" || outputFormat == "mp4") {
		return true
	}
	if inputExt == "webm" && outputFormat == "opus" {
		return true
	}
	return false
}
