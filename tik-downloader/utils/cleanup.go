package utils

import (
	"os"
	"tik-downloader/config"
	"time"

	"github.com/robfig/cron/v3"
)

func StartCleanupScheduler() *cron.Cron {
	c := cron.New()
	c.AddFunc(config.CleanupInterval, func() {
		CleanupOldJobs()
	})
	c.Start()
	go CleanupOldJobs()
	return c
}

func CleanupOldJobs() {
	if _, err := os.Stat(config.StorageDir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(config.StorageDir)
	if err != nil {
		return
	}

	now := time.Now()
	processed := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		jobID := entry.Name()

		if !ValidateJobID(jobID) {
			DeleteJobDir(jobID)
			continue
		}

		meta, err := ReadMeta(jobID)
		if err != nil {
			DeleteJobDir(jobID)
			continue
		}

		createdAt := time.UnixMilli(meta.CreatedAt)
		age := now.Sub(createdAt)

		if age > config.MaxJobAge {
			DeleteJobDir(jobID)
		}

		processed++
		if processed >= config.CleanupBatchSize {
			break
		}
	}
}
