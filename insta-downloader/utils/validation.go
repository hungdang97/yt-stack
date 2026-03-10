package utils

import (
	"insta-downloader/config"
	"net/url"
	"regexp"
	"strings"
)

var jobIDRegex = regexp.MustCompile(config.JobIDRegex)

// ValidateJobID checks if a job ID matches the expected format
func ValidateJobID(jobID string) bool {
	return jobIDRegex.MatchString(jobID)
}

// IsInstagramURL checks if a URL is an Instagram URL
func IsInstagramURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, "instagram.com")
}
