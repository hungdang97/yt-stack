package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

// ============================================
// LOAD FROM ENV
// ============================================

var (
	// Server
	Port = getEnvIntOrDefault("PORT", 5002)

	// Domain
	BaseDomain        = getEnvOrDefault("BASE_DOMAIN", "ytconvert.org")
	DownloadSubdomain = getEnvOrDefault("DOWNLOAD_SUBDOMAIN", "localhost")
	PathPrefix        = getEnvOrDefault("PATH_PREFIX", "/tik")

	// Derived
	Domain  = DownloadSubdomain + "." + BaseDomain
	BaseURL = "https://" + Domain

	// Storage
	StorageDir = getEnvOrDefault("STORAGE_DIR", "./storage")

	// TikTok Extractor
	TikExtractorURL     = getEnvOrDefault("TIK_EXTRACTOR_URL", "http://tik-extractor:5555")
	TikExtractorTimeout = time.Duration(getEnvIntOrDefault("EXTRACT_API_TIMEOUT", 30)) * time.Second

	// Cleanup
	CleanupInterval  = getEnvOrDefault("CLEANUP_INTERVAL", "*/5 * * * *")
	MaxJobAge        = time.Duration(getEnvIntOrDefault("MAX_JOB_AGE_MIN", 15)) * time.Minute
	CleanupBatchSize = getEnvIntOrDefault("CLEANUP_BATCH_SIZE", 5000)

	// Security
	SignedURLSecret     = getEnvOrDefault("SIGNED_URL_SECRET", "default-secret-change-me")
	SignedURLExpiration = time.Duration(getEnvIntOrDefault("SIGNED_URL_EXPIRATION_MIN", 30)) * time.Minute
	HubToken            = getEnvOrDefault("HUB_TOKEN", "1234567890987654321234567890987654321")

	// MongoDB
	MongoURI = getEnvOrDefault("MONGO_URI", "mongodb://cookie:cookie123456789@85.10.196.119:27017/cookie")
	MongoDB  = getEnvOrDefault("MONGO_DB", "cookie")

	// Download
	DownloadTimeout = time.Duration(getEnvIntOrDefault("DOWNLOAD_TIMEOUT_S", 120)) * time.Second
	MaxFileSize     = int64(getEnvIntOrDefault("MAX_FILE_SIZE_MB", 500)) * 1024 * 1024
	BufferSize      = 128 * 1024 // 128KB

	// Job ID
	JobIDLength = 21
	JobIDRegex  = `^[a-zA-Z0-9_-]{21}$`
)

// ============================================
// HTTP CLIENTS
// ============================================

var (
	ExtractClient  *http.Client
	DownloadClient *http.Client
)

func init() {
	ExtractClient = &http.Client{
		Timeout: TikExtractorTimeout,
	}
	DownloadClient = &http.Client{
		Timeout: DownloadTimeout,
	}
}

// ============================================
// HELPERS
// ============================================

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(value)
	if err != nil {
		fmt.Printf("[Config] Warning: invalid int for %s=%s, using default %d\n", key, value, defaultValue)
		return defaultValue
	}
	return i
}
