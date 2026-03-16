package utils

import (
	"net/url"
	"regexp"
	"strings"

	"x-downloader/config"
)

var jobIDPattern = regexp.MustCompile(config.JobIDRegex)

// ValidateJobID validates job ID format (21-char nanoid)
func ValidateJobID(id string) bool {
	return jobIDPattern.MatchString(id)
}

// IsXURL checks if the URL is a valid X/Twitter URL
func IsXURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, "x.com") ||
		strings.Contains(host, "twitter.com") ||
		strings.Contains(host, "t.co")
}
