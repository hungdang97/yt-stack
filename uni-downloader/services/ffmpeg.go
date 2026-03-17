package services

import (
	"fmt"
	"os"
	"os/exec"
)

// DownloadHLS downloads an HLS/m3u8 stream using ffmpeg with optional proxy.
func DownloadHLS(url, outputPath, proxyURL string) error {
	args := []string{}

	// Add proxy via http_proxy option if provided
	if proxyURL != "" {
		args = append(args, "-http_proxy", proxyURL)
	}

	args = append(args,
		"-i", url,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[FFmpeg] Downloading HLS: %s → %s (proxy=%v)\n", url, outputPath, proxyURL != "")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg HLS download failed: %w", err)
	}

	fmt.Printf("[FFmpeg] ✓ HLS downloaded: %s\n", outputPath)
	return nil
}

// RemuxVideo remuxes video to mp4 container (fast copy, no re-encode).
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

// HasAudioStream checks if the input file contains an audio stream.
func HasAudioStream(videoPath string) bool {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		videoPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(output) > 0
}

// ExtractAudio extracts audio from a video file to MP3.
func ExtractAudio(videoPath, audioPath string) error {
	if !HasAudioStream(videoPath) {
		return fmt.Errorf("no audio stream found in video")
	}

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

	os.Remove(videoPath)

	fmt.Printf("[FFmpeg] ✓ Audio extracted: %s\n", audioPath)
	return nil
}

// MergeVideoAudio merges video-only + audio-only into a single mp4.
func MergeVideoAudio(videoPath, audioPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[FFmpeg] Merging video+audio: %s + %s → %s\n", videoPath, audioPath, outputPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg merge failed: %w", err)
	}

	os.Remove(videoPath)
	os.Remove(audioPath)

	fmt.Printf("[FFmpeg] ✓ Merged: %s\n", outputPath)
	return nil
}

// ConvertToMP3 converts an audio file to MP3.
func ConvertToMP3(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-acodec", "libmp3lame",
		"-q:a", "2",
		"-y",
		outputPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[FFmpeg] Converting to MP3: %s → %s\n", inputPath, outputPath)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg convert failed: %w", err)
	}

	os.Remove(inputPath)

	fmt.Printf("[FFmpeg] ✓ Converted to MP3: %s\n", outputPath)
	return nil
}
