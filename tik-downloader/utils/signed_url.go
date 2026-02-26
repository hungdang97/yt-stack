package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"tik-downloader/config"
	"time"
)

// GenerateSignedURL creates a signed URL for file download
func GenerateSignedURL(jobID, filename string) string {
	expires := time.Now().Add(config.SignedURLExpiration).Unix()
	token := generateToken(jobID, filename, expires)
	return fmt.Sprintf("%s%s/files/%s/%s?token=%s&expires=%d",
		config.BaseURL, config.PathPrefix, jobID, filename, token, expires)
}

// GenerateStatusURL creates a signed status URL
func GenerateStatusURL(jobID string) string {
	expires := time.Now().Add(config.SignedURLExpiration).Unix()
	token := generateStatusToken(jobID, expires)
	return fmt.Sprintf("%s%s/api/status/%s?token=%s&expires=%d",
		config.BaseURL, config.PathPrefix, jobID, token, expires)
}

// ValidateSignedURL checks if the token is valid and not expired
func ValidateSignedURL(jobID, filename, token string, expires int64) bool {
	if time.Now().Unix() > expires {
		return false
	}
	expectedToken := generateToken(jobID, filename, expires)
	return hmac.Equal([]byte(token), []byte(expectedToken))
}

// ValidateStatusURL checks if the status token is valid and not expired
func ValidateStatusURL(jobID, token string, expires int64) bool {
	if time.Now().Unix() > expires {
		return false
	}
	expectedToken := generateStatusToken(jobID, expires)
	return hmac.Equal([]byte(token), []byte(expectedToken))
}

func generateToken(jobID, filename string, expires int64) string {
	data := fmt.Sprintf("%s:%s:%d", jobID, filename, expires)
	h := hmac.New(sha256.New, []byte(config.SignedURLSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func generateStatusToken(jobID string, expires int64) string {
	data := fmt.Sprintf("status:%s:%d", jobID, expires)
	h := hmac.New(sha256.New, []byte(config.SignedURLSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// ParseExpires converts expires string to int64
func ParseExpires(expiresStr string) (int64, error) {
	return strconv.ParseInt(expiresStr, 10, 64)
}
