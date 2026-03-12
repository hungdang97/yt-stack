package services

import (
	"fmt"
	"os"
	"os/exec"
)

// RemuxVideo remuxes video to mp4 container (fast, no re-encode).
// Removes the source file after successful remux.
func RemuxVideo(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[FFmpeg] Remuxing video: %s → %s\n", inputPath, outputPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg remux failed: %w", err)
	}

	os.Remove(inputPath)

	fmt.Printf("[FFmpeg] ✓ Video remuxed: %s\n", outputPath)
	return nil
}

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
