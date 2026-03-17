package utils

import (
	"regexp"
	"uni-downloader/config"
)

var jobIDPattern = regexp.MustCompile(config.JobIDRegex)

// ValidateJobID validates job ID format (21-char nanoid)
func ValidateJobID(id string) bool {
	return jobIDPattern.MatchString(id)
}
