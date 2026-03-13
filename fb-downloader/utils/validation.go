package utils

import (
	"fb-downloader/config"
	"net/url"
	"regexp"
	"strings"
)

var jobIDPattern = regexp.MustCompile(config.JobIDRegex)

// ValidateJobID validates job ID format (21-char nanoid)
func ValidateJobID(id string) bool {
	return jobIDPattern.MatchString(id)
}

// IsFacebookURL checks if the URL is a valid Facebook URL
func IsFacebookURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.Contains(host, "facebook.com") ||
		strings.Contains(host, "fb.watch") ||
		strings.Contains(host, "fb.com")
}
