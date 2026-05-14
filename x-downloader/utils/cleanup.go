package utils

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"x-downloader/config"
	"x-downloader/models"

	"github.com/robfig/cron/v3"
)

// StartCleanupScheduler starts the background cleanup cron job
func StartCleanupScheduler() *cron.Cron {
	c := cron.New()

	_, err := c.AddFunc(config.CleanupInterval, func() {
		cleanupOldJobs()
	})
	if err != nil {
		log.Printf("[Cleanup] Failed to add cron job: %v", err)
		return c
	}

	c.Start()
	log.Printf("[Cleanup] Scheduler started: %s (max age: %v)", config.CleanupInterval, config.MaxJobAge)
	return c
}

func cleanupOldJobs() {
	entries, err := os.ReadDir(config.StorageDir)
	if err != nil {
		log.Printf("[Cleanup] Failed to read storage dir: %v", err)
		return
	}

	now := time.Now()
	cleaned := 0

	for i, entry := range entries {
		if i >= config.CleanupBatchSize {
			break
		}

		if !entry.IsDir() {
			continue
		}

		jobID := entry.Name()
		if !ValidateJobID(jobID) {
			continue
		}

		var createdAt time.Time
		metaPath := filepath.Join(config.StorageDir, jobID, "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			prepareMeta, prepErr := ReadPrepareMeta(jobID)
			if prepErr != nil {
				continue
			}
			createdAt = time.UnixMilli(prepareMeta.CreatedAt)
		} else {
			var meta models.Meta
			if err := json.Unmarshal(data, &meta); err != nil {
				continue
			}
			createdAt = time.UnixMilli(meta.CreatedAt)
		}

		age := now.Sub(createdAt)
		if age > config.MaxJobAge {
			if err := os.RemoveAll(filepath.Join(config.StorageDir, jobID)); err != nil {
				log.Printf("[Cleanup] Failed to remove %s: %v", jobID, err)
				continue
			}
			cleaned++
		}
	}

	if cleaned > 0 {
		log.Printf("[Cleanup] Removed %d expired jobs", cleaned)
	}
}
