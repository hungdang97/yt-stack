package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
	"yt-downloader-go/config"
)

// GenerateSignedURL creates a signed URL with token and expiration
func GenerateSignedURL(jobID, filename string) string {
	expires := time.Now().Add(config.SignedURLExpiration).Unix()
	token := generateToken(jobID, filename, expires)
	return fmt.Sprintf("%s/files/%s/%s?token=%s&expires=%d", config.BaseURL, jobID, filename, token, expires)
}

// GenerateStreamURL creates a signed stream URL
func GenerateStreamURL(jobID string) string {
	expires := time.Now().Add(config.SignedURLExpiration).Unix()
	token := generateStreamToken(jobID, expires)
	return fmt.Sprintf("%s/stream/%s?token=%s&expires=%d", config.BaseURL, jobID, token, expires)
}

// GenerateStatusURL creates a signed status URL
func GenerateStatusURL(jobID string) string {
	expires := time.Now().Add(config.SignedURLExpiration).Unix()
	token := generateStatusToken(jobID, expires)
	return fmt.Sprintf("%s/api/status/%s?token=%s&expires=%d", config.BaseURL, jobID, token, expires)
}

// ValidateStatusURL checks if the status token is valid and not expired
func ValidateStatusURL(jobID, token string, expires int64) bool {
	if time.Now().Unix() > expires {
		return false
	}
	expectedToken := generateStatusToken(jobID, expires)
	return hmac.Equal([]byte(token), []byte(expectedToken))
}

// generateStatusToken creates HMAC-SHA256 token for status URLs
func generateStatusToken(jobID string, expires int64) string {
	data := fmt.Sprintf("status:%s:%d", jobID, expires)
	h := hmac.New(sha256.New, []byte(config.SignedURLSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateStreamURL checks if the stream token is valid and not expired
func ValidateStreamURL(jobID, token string, expires int64) bool {
	if time.Now().Unix() > expires {
		return false
	}
	expectedToken := generateStreamToken(jobID, expires)
	return hmac.Equal([]byte(token), []byte(expectedToken))
}

// generateStreamToken creates HMAC-SHA256 token for stream URLs
func generateStreamToken(jobID string, expires int64) string {
	data := fmt.Sprintf("stream:%s:%d", jobID, expires)
	h := hmac.New(sha256.New, []byte(config.SignedURLSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateSignedURL checks if the token is valid and not expired
func ValidateSignedURL(jobID, filename, token string, expires int64) bool {
	// Check if expired
	if time.Now().Unix() > expires {
		return false
	}

	// Validate token
	expectedToken := generateToken(jobID, filename, expires)
	return hmac.Equal([]byte(token), []byte(expectedToken))
}

// generateToken creates HMAC-SHA256 token
func generateToken(jobID, filename string, expires int64) string {
	data := fmt.Sprintf("%s:%s:%d", jobID, filename, expires)
	h := hmac.New(sha256.New, []byte(config.SignedURLSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// ParseExpires converts expires string to int64
func ParseExpires(expiresStr string) (int64, error) {
	return strconv.ParseInt(expiresStr, 10, 64)
}
