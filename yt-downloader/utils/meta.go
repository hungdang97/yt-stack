package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
)

// GetJobDir returns the directory path for a job
func GetJobDir(jobID string) string {
	return filepath.Join(config.StorageDir, jobID)
}

// GetMetaPath returns the meta.json path for a job
func GetMetaPath(jobID string) string {
	return filepath.Join(GetJobDir(jobID), "meta.json")
}

// ReadMeta reads the meta.json file for a job
func ReadMeta(jobID string) (*models.Meta, error) {
	data, err := os.ReadFile(GetMetaPath(jobID))
	if err != nil {
		return nil, err
	}

	var meta models.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// WriteMeta writes the meta.json file for a job
func WriteMeta(jobID string, meta *models.Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(GetMetaPath(jobID), data, 0644)
}

// UpdateMetaStatus updates the status field
func UpdateMetaStatus(jobID string, status string) error {
	meta, err := ReadMeta(jobID)
	if err != nil {
		return err
	}
	meta.Status = status
	return WriteMeta(jobID, meta)
}

// UpdateMetaError updates status to error with message
func UpdateMetaError(jobID string, errMsg string) error {
	meta, err := ReadMeta(jobID)
	if err != nil {
		return err
	}
	meta.Status = models.StatusError
	meta.Error = errMsg
	return WriteMeta(jobID, meta)
}

// UpdateMetaOutput updates the output filename
func UpdateMetaOutput(jobID string, output string) error {
	meta, err := ReadMeta(jobID)
	if err != nil {
		return err
	}
	meta.Status = models.StatusCompleted
	meta.Output = output
	return WriteMeta(jobID, meta)
}

// UpdateMetaStreamOnly marks the job as completed for streaming (no merge)
func UpdateMetaStreamOnly(jobID string) error {
	meta, err := ReadMeta(jobID)
	if err != nil {
		return err
	}
	meta.Status = models.StatusCompleted
	meta.StreamOnly = true
	return WriteMeta(jobID, meta)
}

// CreateJobDir creates the job directory
func CreateJobDir(jobID string) error {
	return os.MkdirAll(GetJobDir(jobID), 0755)
}

// DeleteJobDir deletes the job directory and all contents
func DeleteJobDir(jobID string) error {
	return os.RemoveAll(GetJobDir(jobID))
}

// JobExists checks if a job directory exists
func JobExists(jobID string) bool {
	_, err := os.Stat(GetJobDir(jobID))
	return err == nil
}

// GetFileSize returns the size of a file, or 0 if it doesn't exist
func GetFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// getDownloadedSize calculates total downloaded bytes for a file
// Priority: final file > chunks dir (downloading) > tmp file (merging)
func getDownloadedSize(jobDir, fileName string, expectedSize int64) int64 {
	basePath := filepath.Join(jobDir, fileName)

	// 1. Final file exists = done
	if size := GetFileSize(basePath); size > 0 {
		return expectedSize
	}

	// 2. Chunks dir exists = downloading
	chunksDir := basePath + ".chunks"
	if info, err := os.Stat(chunksDir); err == nil && info.IsDir() {
		var total int64
		entries, _ := os.ReadDir(chunksDir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if fileInfo, err := entry.Info(); err == nil {
				total += fileInfo.Size()
			}
		}
		return total
	}

	// 3. Tmp file exists (no chunks dir) = merging = download done
	if GetFileSize(basePath+".tmp") > 0 {
		return expectedSize
	}

	return 0
}

// CalculateProgress calculates download progress from file sizes
func CalculateProgress(meta *models.Meta) int {
	if meta.Status == models.StatusCompleted {
		return 100
	}
	if meta.Status == models.StatusError {
		return 0
	}

	jobDir := GetJobDir(meta.ID)

	if meta.OutputType == "video" && meta.Files.Video != nil && meta.Files.Audio != nil {
		// Video + Audio download
		videoSize := getDownloadedSize(jobDir, meta.Files.Video.Name, meta.Files.Video.Size)
		audioSize := getDownloadedSize(jobDir, meta.Files.Audio.Name, meta.Files.Audio.Size)

		videoProgress := 0
		audioProgress := 0
		if meta.Files.Video.Size > 0 {
			videoProgress = int(float64(videoSize) / float64(meta.Files.Video.Size) * 100)
		}
		if meta.Files.Audio.Size > 0 {
			audioProgress = int(float64(audioSize) / float64(meta.Files.Audio.Size) * 100)
		}

		videoProgress = min(videoProgress, 100)
		audioProgress = min(audioProgress, 100)

		// Weighted progress: video 70%, audio 30%
		progress := int(float64(videoProgress)*0.7 + float64(audioProgress)*0.3)
		return min(progress, 100)
	} else if meta.Files.Audio != nil {
		// Audio only
		audioSize := getDownloadedSize(jobDir, meta.Files.Audio.Name, meta.Files.Audio.Size)

		audioProgress := 0
		if meta.Files.Audio.Size > 0 {
			audioProgress = int(float64(audioSize) / float64(meta.Files.Audio.Size) * 100)
		}

		return min(audioProgress, 100)
	}

	return 0
}
