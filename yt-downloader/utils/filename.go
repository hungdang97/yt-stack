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
	// Multiple spaces/underscores (for classic style)
	multipleSpaces = regexp.MustCompile(`[\s_]+`)
	// Multiple spaces only (for human-readable styles)
	multiSpacesOnly = regexp.MustCompile(`\s{2,}`)
)

// SanitizeFilename removes invalid characters and replaces spaces with underscores (for classic style)
func SanitizeFilename(name string) string {
	name = invalidChars.ReplaceAllString(name, "_")
	name = multipleSpaces.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_ ")
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

// SanitizeHumanFilename removes invalid characters but keeps spaces (for basic/pretty/nerdy styles)
func SanitizeHumanFilename(name string) string {
	name = invalidChars.ReplaceAllString(name, "")
	name = multiSpacesOnly.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

// Filename style constants
const (
	StyleClassic = "classic"
	StyleBasic   = "basic"
	StylePretty  = "pretty"
	StyleNerdy   = "nerdy"
)

// GenerateOutputFilename generates the output filename based on job metadata and style
func GenerateOutputFilename(meta *models.Meta) string {
	style := meta.FilenameStyle
	if style == "" {
		style = StyleBasic
	}

	switch style {
	case StyleClassic:
		return generateClassic(meta)
	case StylePretty:
		return generatePretty(meta)
	case StyleNerdy:
		return generateNerdy(meta)
	default:
		return generateBasic(meta)
	}
}

// classic: youtube_dQw4w9WgXcQ_1080p.mp4 | youtube_dQw4w9WgXcQ_audio.mp3
func generateClassic(meta *models.Meta) string {
	parts := []string{"youtube", meta.VideoID}

	if meta.OutputType == "video" && meta.Quality != "" {
		parts = append(parts, meta.Quality)
	} else if meta.OutputType == "audio" {
		parts = append(parts, "audio")
	}

	if meta.Trim != nil {
		parts = append(parts, fmt.Sprintf("%.0f-%.0fs", meta.Trim.Start, meta.Trim.End))
	}

	return fmt.Sprintf("%s.%s", strings.Join(parts, "_"), meta.Format)
}

// basic: Title - Author (1080p).mp4 | Title - Author.mp3
func generateBasic(meta *models.Meta) string {
	name := buildHumanName(meta)
	info := buildInfoParts(meta, false, false)
	trim := buildHumanTrim(meta)
	return buildFinalName(name, info, trim, meta.Format)
}

// pretty: Title - Author (1080p, youtube).mp4
func generatePretty(meta *models.Meta) string {
	name := buildHumanName(meta)
	info := buildInfoParts(meta, true, false)
	trim := buildHumanTrim(meta)
	return buildFinalName(name, info, trim, meta.Format)
}

// nerdy: Title - Author (1080p, youtube, dQw4w9WgXcQ).mp4
func generateNerdy(meta *models.Meta) string {
	name := buildHumanName(meta)
	info := buildInfoParts(meta, true, true)
	trim := buildHumanTrim(meta)
	return buildFinalName(name, info, trim, meta.Format)
}

// buildHumanName returns "Title - Author" or just "Title"
func buildHumanName(meta *models.Meta) string {
	title := SanitizeHumanFilename(meta.Title)
	if title == "" {
		title = "output"
	}
	author := SanitizeHumanFilename(meta.Author)
	if author != "" {
		return fmt.Sprintf("%s - %s", title, author)
	}
	return title
}

// buildInfoParts builds the parenthesized info like "1080p" or "1080p, youtube, dQw4w9WgXcQ"
func buildInfoParts(meta *models.Meta, includePlatform bool, includeVideoID bool) string {
	var parts []string

	if meta.OutputType == "video" && meta.Quality != "" {
		parts = append(parts, meta.Quality)
	}
	if meta.OutputType == "audio" && meta.Bitrate != "" {
		parts = append(parts, meta.Bitrate)
	}
	if includePlatform {
		parts = append(parts, "youtube")
	}
	if includeVideoID && meta.VideoID != "" {
		parts = append(parts, meta.VideoID)
	}

	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("(%s)", strings.Join(parts, ", "))
}

// buildHumanTrim returns " [0꞉10-1꞉30]" or "" if no trim
func buildHumanTrim(meta *models.Meta) string {
	if meta.Trim == nil {
		return ""
	}
	start := formatTime(meta.Trim.Start)
	end := formatTime(meta.Trim.End)
	return fmt.Sprintf(" [%s-%s]", start, end)
}

// formatTime converts seconds to m꞉ss or h꞉mm꞉ss format using modifier letter colon (꞉ U+A789)
func formatTime(seconds float64) string {
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60

	if h > 0 {
		return fmt.Sprintf("%d꞉%02d꞉%02d", h, m, s)
	}
	return fmt.Sprintf("%d꞉%02d", m, s)
}

// buildFinalName assembles: "name (info) [trim].ext" or "name [trim].ext"
func buildFinalName(name, info, trim, format string) string {
	result := name
	if info != "" {
		result += " " + info
	}
	result += trim
	return fmt.Sprintf("%s.%s", SanitizeHumanFilename(result), format)
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
	case "avi":
		return "video/x-msvideo"
	case "flv":
		return "video/x-flv"
	case "gif":
		return "image/gif"
	case "mov":
		return "video/quicktime"
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
	case "aac":
		return "audio/aac"
	case "alac":
		return "audio/mp4"
	default:
		return "application/octet-stream"
	}
}
