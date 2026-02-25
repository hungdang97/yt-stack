package utils

import (
	"fmt"
	"regexp"
	"slices"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
)

var (
	// YouTube URL patterns
	youtubeURLPattern = regexp.MustCompile(`(?:youtube\.com\/(?:watch\?v=|embed\/|v\/|shorts\/)|youtu\.be\/)([a-zA-Z0-9_-]{11})`)
	bitratePattern    = regexp.MustCompile(`^\d{1,3}k$`)
	jobIDPattern      = regexp.MustCompile(config.JobIDRegex)
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ExtractVideoID extracts the video ID from a YouTube URL
func ExtractVideoID(url string) (string, error) {
	matches := youtubeURLPattern.FindStringSubmatch(url)
	if len(matches) < 2 {
		return "", ValidationError{Field: "url", Message: "Invalid YouTube URL"}
	}
	return matches[1], nil
}

// ValidateDownloadRequest validates the download request
func ValidateDownloadRequest(req *models.DownloadRequest) error {
	// Validate URL
	if req.URL == "" {
		return ValidationError{Field: "url", Message: "URL is required"}
	}
	if _, err := ExtractVideoID(req.URL); err != nil {
		return err
	}

	// Validate OS if provided
	if req.OS != "" && !slices.Contains(config.OSTypes, req.OS) {
		return ValidationError{Field: "os", Message: fmt.Sprintf("Invalid OS type. Must be one of: %v", config.OSTypes)}
	}

	// Validate output type
	if req.Output.Type != "video" && req.Output.Type != "audio" {
		return ValidationError{Field: "output.type", Message: "Must be 'video' or 'audio'"}
	}

	// Validate format
	if req.Output.Type == "video" {
		if !slices.Contains(config.VideoFormats, req.Output.Format) {
			return ValidationError{Field: "output.format", Message: fmt.Sprintf("Invalid video format. Must be one of: %v", config.VideoFormats)}
		}
	} else {
		if !slices.Contains(config.AudioFormats, req.Output.Format) {
			return ValidationError{Field: "output.format", Message: fmt.Sprintf("Invalid audio format. Must be one of: %v", config.AudioFormats)}
		}
	}

	// Validate quality for video
	if req.Output.Type == "video" && req.Output.Quality != "" {
		if !slices.Contains(config.Qualities, req.Output.Quality) {
			return ValidationError{Field: "output.quality", Message: fmt.Sprintf("Invalid quality. Must be one of: %v", config.Qualities)}
		}
	}

	// Validate bitrate if provided
	if req.Audio.Bitrate != "" && !bitratePattern.MatchString(req.Audio.Bitrate) {
		return ValidationError{Field: "audio.bitrate", Message: "Invalid bitrate format. Must be like '192k'"}
	}

	// Validate trim if provided
	if req.Trim != nil {
		if req.Trim.Start < 0 {
			return ValidationError{Field: "trim.start", Message: "Start time must be >= 0"}
		}
		if req.Trim.End <= req.Trim.Start {
			return ValidationError{Field: "trim.end", Message: "End time must be greater than start time"}
		}
		duration := req.Trim.End - req.Trim.Start
		if duration > config.MaxTrimDuration.Seconds() {
			return ValidationError{Field: "trim", Message: fmt.Sprintf("Trim duration must be <= %v", config.MaxTrimDuration)}
		}
	}

	return nil
}

// ValidateJobID validates the job ID format
func ValidateJobID(jobID string) bool {
	return jobIDPattern.MatchString(jobID)
}

// ValidateFilename validates the filename to prevent path traversal
func ValidateFilename(filename string) bool {
	if filename == "" {
		return false
	}
	// Check for path traversal attempts
	for _, char := range []string{"..", "/", "\\"} {
		if contains(filename, char) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
