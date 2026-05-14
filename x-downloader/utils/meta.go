package utils

import (
	"encoding/json"
	"os"
	"path/filepath"

	"x-downloader/config"
	"x-downloader/models"
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

// UpdateMetaCompleted marks job as completed with output filename and file size
func UpdateMetaCompleted(jobID string, output string, fileSize int64) error {
	meta, err := ReadMeta(jobID)
	if err != nil {
		return err
	}
	meta.Status = models.StatusCompleted
	meta.Output = output
	meta.FileSize = fileSize
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

// --- Prepare Meta ---

func GetPrepareMetaPath(jobID string) string {
	return filepath.Join(GetJobDir(jobID), "prepare_meta.json")
}

func ReadPrepareMeta(jobID string) (*models.PrepareMeta, error) {
	data, err := os.ReadFile(GetPrepareMetaPath(jobID))
	if err != nil {
		return nil, err
	}
	var meta models.PrepareMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func WritePrepareMeta(jobID string, meta *models.PrepareMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetPrepareMetaPath(jobID), data, 0644)
}

func UpdatePrepareMetaError(jobID string, errMsg string) {
	meta, err := ReadPrepareMeta(jobID)
	if err != nil {
		return
	}
	meta.Status = models.StatusError
	meta.Error = errMsg
	WritePrepareMeta(jobID, meta)
}

func UpdatePrepareMetaCompleted(jobID string) {
	meta, err := ReadPrepareMeta(jobID)
	if err != nil {
		return
	}
	meta.Status = models.StatusCompleted
	WritePrepareMeta(jobID, meta)
}

func CalculatePrepareProgress(meta *models.PrepareMeta) int {
	if meta.Status == models.StatusCompleted {
		return 100
	}
	if meta.Status == models.StatusError {
		return 0
	}
	if meta.Status == models.StatusProcessing {
		return 80
	}
	if meta.Status == models.StatusDownloading {
		return 30
	}
	return 10
}

// CalculateProgress calculates download progress from file sizes
func CalculateProgress(meta *models.Meta) int {
	if meta.Status == models.StatusCompleted {
		return 100
	}
	if meta.Status == models.StatusError {
		return 0
	}
	if meta.Status == models.StatusExtracting {
		return 5
	}
	if meta.Status == models.StatusProcessing {
		return 80
	}

	if meta.FileSize > 0 {
		outputPath := filepath.Join(GetJobDir(meta.ID), meta.Output)
		currentSize := GetFileSize(outputPath)
		if currentSize > 0 {
			progress := int(float64(currentSize) / float64(meta.FileSize) * 100)
			if progress > 99 {
				progress = 99
			}
			return progress
		}
	}

	if meta.Status == models.StatusDownloading {
		return 10
	}

	return 0
}
