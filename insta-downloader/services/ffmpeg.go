package services

import (
	"fmt"
	"os"
	"os/exec"
)

// ExtractAudio uses FFmpeg to extract audio from a video file to MP3.
// Removes the source video after successful extraction.
func ExtractAudio(videoPath, audioPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-q:a", "2",
		"-y",
		audioPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[FFmpeg] Extracting audio: %s → %s\n", videoPath, audioPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Remove source video
	os.Remove(videoPath)

	fmt.Printf("[FFmpeg] ✓ Audio extracted: %s\n", audioPath)
	return nil
}
