package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
)

var (
	// Server
	Port = getEnvIntOrDefault("PORT", 5006)

	// Domain
	BaseDomain        = getEnvOrDefault("BASE_DOMAIN", "ytconvert.org")
	DownloadSubdomain = getEnvOrDefault("DOWNLOAD_SUBDOMAIN", "localhost")
	PathPrefix        = getEnvOrDefault("PATH_PREFIX", "/uni")

	// Derived
	Domain  = DownloadSubdomain + "." + BaseDomain
	BaseURL = "https://" + Domain

	// Storage
	StorageDir = getEnvOrDefault("STORAGE_DIR", "./storage")

	// Universal Extractor
	UniExtractorURL     = getEnvOrDefault("UNI_EXTRACTOR_URL", "http://uni-extractor:8004")
	UniExtractorTimeout = time.Duration(getEnvIntOrDefault("EXTRACT_API_TIMEOUT", 15)) * time.Second

	// Universal Cookie (from env, optional)
	UniDefaultCookie = getEnvOrDefault("UNI_DEFAULT_COOKIE", "")

	// Cleanup
	CleanupInterval  = getEnvOrDefault("CLEANUP_INTERVAL", "*/5 * * * *")
	MaxJobAge        = time.Duration(getEnvIntOrDefault("MAX_JOB_AGE_MIN", 60)) * time.Minute
	CleanupBatchSize = getEnvIntOrDefault("CLEANUP_BATCH_SIZE", 5000)

	// Security
	SignedURLSecret     = getEnvOrDefault("SIGNED_URL_SECRET", "default-secret-change-me")
	SignedURLExpiration = time.Duration(getEnvIntOrDefault("SIGNED_URL_EXPIRATION_MIN", 60)) * time.Minute
	HubToken           = getEnvOrDefault("HUB_TOKEN", "1234567890987654321234567890987654321")

	// Download
	DownloadTimeout = time.Duration(getEnvIntOrDefault("DOWNLOAD_TIMEOUT_S", 120)) * time.Second
	MaxFileSize     = int64(getEnvIntOrDefault("MAX_FILE_SIZE_MB", 500)) * 1024 * 1024
	BufferSize      = 128 * 1024 // 128KB
	MaxRetries      = getEnvIntOrDefault("MAX_RETRIES", 3)
	RetryDelay      = time.Duration(getEnvIntOrDefault("RETRY_DELAY_MS", 1000)) * time.Millisecond

	// Job ID
	JobIDLength = 21
	JobIDRegex  = `^[a-zA-Z0-9_-]{21}$`

	// Proxy Credentials
	WARPUser = getEnvOrDefault("WARP_USER", "")
	WARPPass = getEnvOrDefault("WARP_PASS", "")

	// Derived Proxy URL
	WARPProxyURL = ""
)

func initProxyURL() {
	if WARPUser != "" && WARPPass != "" {
		WARPProxyURL = fmt.Sprintf("http://%s:%s@gost:1111", WARPUser, WARPPass)
	}
}

var (
	ExtractClient         *http.Client
	DownloadClient        *http.Client
	DownloadClientNoProxy *http.Client
)

func init() {
	initProxyURL()

	ExtractClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: UniExtractorTimeout,
	}

	downloadTransportNoProxy := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	DownloadClientNoProxy = &http.Client{
		Transport: downloadTransportNoProxy,
		Timeout:   DownloadTimeout,
	}

	downloadTransport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	if WARPProxyURL != "" {
		proxyURL, _ := url.Parse(WARPProxyURL)
		downloadTransport.Proxy = http.ProxyURL(proxyURL)
	}
	DownloadClient = &http.Client{
		Transport: downloadTransport,
		Timeout:   DownloadTimeout,
	}
}

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
