package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
)

// FFmpegMerge merges video and audio files
// If needsReencode is true, re-encodes to compatible codecs for target format
// If needsReencode is false, copies codecs directly (fast, lossless)
func FFmpegMerge(jobDir string, format string, videoFile string, audioFile string, needsReencode bool) (string, error) {
	outputFile := filepath.Join(jobDir, fmt.Sprintf("output.%s", format))

	var args []string

	if needsReencode {
		// Re-encode to compatible codecs for target format
		videoCodec := config.VideoCodecMap[format]
		if videoCodec == "" {
			videoCodec = "libx264" // fallback to H.264
		}
		audioCodec := config.AudioCodecMap[format]
		if audioCodec == "" {
			audioCodec = "aac" // fallback to AAC
		}

		args = []string{
			"-y",
			"-i", filepath.Join(jobDir, videoFile),
			"-i", filepath.Join(jobDir, audioFile),
			"-map", "0:v:0", // Select video from first input
			"-map", "1:a:0", // Select audio from second input
			"-c:v", videoCodec,
			"-preset", "veryfast", // Faster encoding, less CPU usage
			"-threads", "2", // Limit CPU cores to 2
			"-crf", "23", // Good quality (18-28 range, lower = better)
			"-c:a", audioCodec,
			"-b:a", "192k", // Audio bitrate
			outputFile,
		}
	} else {
		// Fast copy - no re-encoding (lossless, very fast)
		args = []string{
			"-y",
			"-i", filepath.Join(jobDir, videoFile),
			"-i", filepath.Join(jobDir, audioFile),
			"-map", "0:v:0", // Select video from first input
			"-map", "1:a:0", // Select audio from second input
			"-c:v", "copy",
			"-c:a", "copy",
			outputFile,
		}
	}

	if err := runFFmpeg(args); err != nil {
		return "", fmt.Errorf("merge failed: %w", err)
	}

	return filepath.Base(outputFile), nil
}

// FFmpegConvertAudio converts audio to target format
func FFmpegConvertAudio(jobDir string, format string, bitrate string, audioFile string) (string, error) {
	inputPath := filepath.Join(jobDir, audioFile)
	outputFile := filepath.Join(jobDir, fmt.Sprintf("output.%s", format))

	// Determine if we need to encode or can copy
	inputExt := filepath.Ext(audioFile)
	canCopy := canCopyAudio(inputExt, format)

	var args []string
	if canCopy {
		args = []string{
			"-y",
			"-i", inputPath,
			"-c:a", "copy",
			outputFile,
		}
	} else {
		codec := config.AudioCodecMap[format]
		if codec == "" {
			codec = "aac"
		}

		args = []string{
			"-y",
			"-i", inputPath,
			"-threads", "2", // Limit CPU cores to 2
			"-c:a", codec,
		}

		// Add bitrate for lossy codecs
		if bitrate != "" && codec != "pcm_s16le" && codec != "flac" {
			args = append(args, "-b:a", bitrate)
		}

		args = append(args, outputFile)
	}

	if err := runFFmpeg(args); err != nil {
		return "", fmt.Errorf("audio conversion failed: %w", err)
	}

	return filepath.Base(outputFile), nil
}

// ffmpegTrim is the internal trim function for both video and audio
func ffmpegTrim(jobDir string, format string, trim *models.TrimConfig, bitrate string, isVideo bool) (string, error) {
	if trim.End <= trim.Start {
		return "", fmt.Errorf("invalid trim range: end (%.2f) must be greater than start (%.2f)", trim.End, trim.Start)
	}

	inputPath := filepath.Join(jobDir, fmt.Sprintf("output.%s", format))
	outputPath := filepath.Join(jobDir, fmt.Sprintf("output_trimmed.%s", format))
	duration := trim.End - trim.Start

	var args []string
	if trim.Accurate {
		args = []string{
			"-y",
			"-ss", fmt.Sprintf("%.3f", trim.Start),
			"-i", inputPath,
			"-t", fmt.Sprintf("%.3f", duration),
		}

		if isVideo {
			videoCodec := config.VideoCodecMap[format]
			if videoCodec == "" {
				videoCodec = "libx264"
			}
			audioCodec := config.AudioCodecMap[format]
			if audioCodec == "" {
				audioCodec = "aac"
			}
			args = append(args, "-c:v", videoCodec, "-c:a", audioCodec)
			if bitrate != "" {
				args = append(args, "-b:a", bitrate)
			}
		} else {
			audioCodec := config.AudioCodecMap[format]
			if audioCodec == "" {
				audioCodec = "aac"
			}
			args = append(args, "-threads", "2", "-c:a", audioCodec) // Limit CPU cores to 2
			if bitrate != "" && audioCodec != "pcm_s16le" && audioCodec != "flac" {
				args = append(args, "-b:a", bitrate)
			}
		}
		args = append(args, outputPath)
	} else {
		// Fast trim: copy
		args = []string{
			"-y",
			"-ss", fmt.Sprintf("%.3f", trim.Start),
			"-i", inputPath,
			"-t", fmt.Sprintf("%.3f", duration),
		}
		if isVideo {
			args = append(args, "-c", "copy")
		} else {
			args = append(args, "-c:a", "copy")
		}
		args = append(args, outputPath)
	}

	if err := runFFmpeg(args); err != nil {
		return "", fmt.Errorf("trim failed: %w", err)
	}

	_ = os.Remove(inputPath)
	if err := os.Rename(outputPath, inputPath); err != nil {
		return "", fmt.Errorf("failed to rename trimmed file: %w", err)
	}

	return fmt.Sprintf("output.%s", format), nil
}

// FFmpegTrim trims video file
func FFmpegTrim(jobDir string, format string, trim *models.TrimConfig, bitrate string) (string, error) {
	return ffmpegTrim(jobDir, format, trim, bitrate, true)
}

// FFmpegTrimAudio trims audio file
func FFmpegTrimAudio(jobDir string, format string, trim *models.TrimConfig, bitrate string) (string, error) {
	return ffmpegTrim(jobDir, format, trim, bitrate, false)
}

// runFFmpeg executes ffmpeg command
func runFFmpeg(args []string) error {
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg error: %w", err)
	}

	return nil
}

// canCopyAudio checks if audio can be copied without re-encoding
func canCopyAudio(inputExt string, outputFormat string) bool {
	// Remove leading dot
	if len(inputExt) > 0 && inputExt[0] == '.' {
		inputExt = inputExt[1:]
	}

	// Same format: always copy
	if inputExt == outputFormat {
		return true
	}

	// m4a/mp4/mov/aac are compatible (all use AAC codec typically)
	aacFormats := map[string]bool{"m4a": true, "mp4": true, "mov": true, "aac": true}
	if aacFormats[inputExt] && aacFormats[outputFormat] {
		return true
	}

	// webm to opus: YouTube webm audio typically contains Opus codec
	// Note: webm can also contain Vorbis, but YouTube primarily uses Opus for audio
	// If copy fails, FFmpeg will error and user can retry with re-encoding
	if inputExt == "webm" && outputFormat == "opus" {
		return true
	}

	// webm to ogg: YouTube webm can contain Vorbis codec
	// Attempt copy first, will transcode if codec mismatch
	if inputExt == "webm" && outputFormat == "ogg" {
		return true
	}

	// ogg to webm: OGG Vorbis can be copied to webm container
	if inputExt == "ogg" && outputFormat == "webm" {
		return true
	}

	return false
}
