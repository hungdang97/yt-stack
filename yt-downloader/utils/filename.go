package utils

import (
	"fmt"
	"regexp"
	"strings"
	"yt-downloader-go/models"
)

var (
	// Characters not allowed in filenames
	invalidChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	// Multiple spaces/underscores
	multipleSpaces = regexp.MustCompile(`[\s_]+`)
)

// SanitizeFilename removes invalid characters from filename
func SanitizeFilename(name string) string {
	// Replace invalid characters with underscore
	name = invalidChars.ReplaceAllString(name, "_")
	// Replace multiple spaces/underscores with single underscore
	name = multipleSpaces.ReplaceAllString(name, "_")
	// Trim leading/trailing underscores and spaces
	name = strings.Trim(name, "_ ")
	// Limit length
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

// GenerateOutputFilename generates the output filename based on job metadata
func GenerateOutputFilename(meta *models.Meta) string {
	title := SanitizeFilename(meta.Title)
	if title == "" {
		title = "output"
	}

	var parts []string
	parts = append(parts, title)

	// Add quality for video
	if meta.OutputType == "video" && meta.Quality != "" {
		parts = append(parts, meta.Quality)
	}

	// Add bitrate for audio
	if meta.OutputType == "audio" && meta.Bitrate != "" {
		parts = append(parts, meta.Bitrate)
	}

	// Add trim info
	if meta.Trim != nil {
		trimInfo := fmt.Sprintf("%.0f-%.0fs", meta.Trim.Start, meta.Trim.End)
		parts = append(parts, trimInfo)
	}

	filename := strings.Join(parts, "_")
	return fmt.Sprintf("%s.%s", filename, meta.Format)
}

// GetExtFromMimeType extracts file extension from MIME type
func GetExtFromMimeType(mimeType string) string {
	// Remove codec info: "video/mp4; codecs=..." -> "video/mp4"
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	// Map common MIME types
	switch mimeType {
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "audio/mp4":
		return "m4a"
	case "audio/webm":
		return "webm"
	case "audio/mpeg":
		return "mp3"
	case "audio/ogg":
		return "ogg"
	case "audio/opus":
		return "opus"
	case "audio/flac":
		return "flac"
	case "audio/wav", "audio/x-wav":
		return "wav"
	default:
		// Try to extract from MIME type
		parts := strings.Split(mimeType, "/")
		if len(parts) == 2 {
			return parts[1]
		}
		return "bin"
	}
}

// ContentTypeFromExt returns content type for file extension
func ContentTypeFromExt(ext string) string {
	switch ext {
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mkv":
		return "video/x-matroska"
	case "mp3":
		return "audio/mpeg"
	case "m4a":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/opus"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}
