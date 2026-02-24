package services

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"yt-downloader-go/models"
)

// FFmpegEmbedMetadata embeds metadata (title, artist) and thumbnail into the output file.
// Only runs when EnableMetadata is true. Returns the output filename (unchanged).
func FFmpegEmbedMetadata(jobDir string, meta *models.Meta) error {
	outputFile := filepath.Join(jobDir, meta.Output)
	format := meta.Format

	// Download thumbnail if available
	thumbnailPath := ""
	if meta.ThumbnailURL != "" {
		path, err := downloadThumbnail(jobDir, meta.ThumbnailURL)
		if err == nil {
			thumbnailPath = path
			defer os.Remove(thumbnailPath) // cleanup thumbnail after embedding
		}
		// If thumbnail download fails, continue without it
	}

	// Build FFmpeg args
	tempOutput := filepath.Join(jobDir, fmt.Sprintf("output_meta.%s", format))

	var args []string
	args = append(args, "-y", "-i", outputFile)

	// Add thumbnail input if available and format supports it
	canEmbedThumb := thumbnailPath != "" && canEmbedThumbnail(format)
	if canEmbedThumb {
		args = append(args, "-i", thumbnailPath)
	}

	// Map streams
	args = append(args, "-map", "0") // map all streams from input
	if canEmbedThumb {
		args = append(args, "-map", "1") // map thumbnail
	}

	// Copy all codecs (no re-encoding)
	args = append(args, "-c", "copy")

	// Add metadata tags
	if meta.Title != "" {
		args = append(args, "-metadata", fmt.Sprintf("title=%s", meta.Title))
	}
	if meta.Author != "" {
		args = append(args, "-metadata", fmt.Sprintf("artist=%s", meta.Author))
	}

	// Set thumbnail disposition and ID3v2 for MP3
	if canEmbedThumb {
		if format == "mp3" {
			args = append(args, "-id3v2_version", "3")
		} else {
			args = append(args, "-disposition:v:1", "attached_pic")
		}
	}

	args = append(args, tempOutput)

	if err := runFFmpeg(args); err != nil {
		// If metadata embedding fails, just keep the original file
		os.Remove(tempOutput)
		return fmt.Errorf("metadata embedding failed: %w", err)
	}

	// Replace original with metadata version
	if err := os.Remove(outputFile); err != nil {
		os.Remove(tempOutput)
		return fmt.Errorf("failed to remove original: %w", err)
	}
	if err := os.Rename(tempOutput, outputFile); err != nil {
		return fmt.Errorf("failed to rename metadata file: %w", err)
	}

	return nil
}

// canEmbedThumbnail checks if the format supports embedded thumbnail
// Updated: Always attempt to embed, let FFmpeg decide
func canEmbedThumbnail(format string) bool {
	return true
}

// canEmbedTextMetadata checks if the format supports text metadata
// Updated: Always attempt to embed, let FFmpeg decide
func canEmbedTextMetadata(format string) bool {
	return true
}

// downloadThumbnail downloads thumbnail image to jobDir
func downloadThumbnail(jobDir string, url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("thumbnail download returned status %d", resp.StatusCode)
	}

	// Determine extension from content type
	ext := "jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = "png"
	} else if strings.Contains(ct, "webp") {
		ext = "webp"
	}

	thumbnailPath := filepath.Join(jobDir, fmt.Sprintf("thumbnail.%s", ext))
	f, err := os.Create(thumbnailPath)
	if err != nil {
		return "", fmt.Errorf("failed to create thumbnail file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(thumbnailPath)
		return "", fmt.Errorf("failed to write thumbnail: %w", err)
	}

	return thumbnailPath, nil
}
