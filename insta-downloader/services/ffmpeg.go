package services

import (
	"fmt"
	"os"
	"os/exec"
)

// isH264 checks if the video stream is H.264 codec.
func isH264(videoPath string) bool {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "csv=p=0",
		videoPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	codec := string(output)
	return codec == "h264\n" || codec == "h264"
}

// RemuxVideo remuxes/re-encodes video to mp4 container.
// If already H.264, just remux (fast copy). Otherwise re-encode to H.264.
// Removes the source file after success.
func RemuxVideo(inputPath, outputPath string) error {
	var cmd *exec.Cmd

	if isH264(inputPath) {
		// Already H.264, fast remux
		fmt.Printf("[FFmpeg] Remuxing video (copy): %s → %s\n", inputPath, outputPath)
		cmd = exec.Command("ffmpeg",
			"-i", inputPath,
			"-c", "copy",
			"-movflags", "+faststart",
			"-y",
			outputPath,
		)
	} else {
		// Re-encode to H.264 for compatibility
		fmt.Printf("[FFmpeg] Re-encoding video to H.264: %s → %s\n", inputPath, outputPath)
		cmd = exec.Command("ffmpeg",
			"-i", inputPath,
			"-c:v", "libx264",
			"-preset", "fast",
			"-crf", "18",
			"-c:a", "aac",
			"-b:a", "192k",
			"-movflags", "+faststart",
			"-y",
			outputPath,
		)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	os.Remove(inputPath)

	fmt.Printf("[FFmpeg] ✓ Video processed: %s\n", outputPath)
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

// ExtractAudio uses FFmpeg to extract audio from a video file to MP3.
// Removes the source video after successful extraction.
func ExtractAudio(videoPath, audioPath string) error {
	// Check if video has audio stream
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

	// Remove source video
	os.Remove(videoPath)

	fmt.Printf("[FFmpeg] ✓ Audio extracted: %s\n", audioPath)
	return nil
}

// MergeVideoAudio merges a video-only file with an audio-only file into a single mp4.
// Removes both source files after successful merge.
func MergeVideoAudio(videoPath, audioPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "18",
		"-c:a", "aac",
		"-b:a", "192k",
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

// ConvertToMP3 converts an audio file (m4a/aac) to MP3.
// Removes the source file after successful conversion.
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
