package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload" // Auto-load .env file
)

// ============================================
// LOAD FROM ENV (Primitives Only - ~30 vars)
// ============================================

var (
	// Core Identity
	ServerIP   = mustGetEnv("SERVER_IP")
	ServerName = mustGetEnv("SERVER_NAME")

	// Domain Components
	BaseDomain        = mustGetEnv("BASE_DOMAIN")        // ytconvert.org
	DownloadSubdomain = mustGetEnv("DOWNLOAD_SUBDOMAIN") // vps-103-45...

	Email = mustGetEnv("EMAIL")
	Port  = mustGetEnvInt("PORT")

	// Derived from Components
	Subdomain = DownloadSubdomain // Alias for backward compat
	Domain    = DownloadSubdomain + "." + BaseDomain
	BaseURL   = "https://" + Domain

	// Proxy Credentials
	WARPUser   = mustGetEnv("WARP_USER")
	WARPPass   = mustGetEnv("WARP_PASS")
	DirectUser = mustGetEnv("DIRECT_USER")
	DirectPass = mustGetEnv("DIRECT_PASS")

	// Derived Proxy URLs (constructed in code)
	WARPProxyURL   = fmt.Sprintf("http://%s:%s@gost:1111", WARPUser, WARPPass)
	DirectProxyURL = fmt.Sprintf("http://%s:%s@gost:2222", DirectUser, DirectPass)

	// Storage & Download
	StorageDir      = getEnvOrDefault("STORAGE_DIR", "./storage")
	DownloadThreads = mustGetEnvInt("DOWNLOAD_THREADS")
	ChunkSize       = int64(mustGetEnvInt("CHUNK_SIZE"))
	MaxRetries      = mustGetEnvInt("MAX_RETRIES")
	RetryDelay      = time.Duration(mustGetEnvInt("RETRY_DELAY_MS")) * time.Millisecond
	ChunkTimeout    = time.Duration(mustGetEnvInt("CHUNK_TIMEOUT_S")) * time.Second
	BufferSize      = 128 * 1024 // 128KB - Optimized for better I/O performance

	// Extract API
	ExtractAPIBase    = "http://yt-extractor:8300/api/video" // Python extractor (all yt-dlp platforms)
	ExtractAPITimeout = time.Duration(mustGetEnvInt("EXTRACT_API_TIMEOUT")) * time.Second

	// Cleanup
	CleanupInterval  = mustGetEnv("CLEANUP_INTERVAL")
	MaxJobAge        = time.Duration(mustGetEnvInt("MAX_JOB_AGE_MIN")) * time.Minute
	CleanupBatchSize = mustGetEnvInt("CLEANUP_BATCH_SIZE")

	// Security
	SignedURLSecret     = mustGetEnv("SIGNED_URL_SECRET")
	SignedURLExpiration = time.Duration(mustGetEnvInt("SIGNED_URL_EXPIRATION_MIN")) * time.Minute

	// Job ID - Fixed constants
	JobIDLength = 21
	JobIDRegex  = `^[a-zA-Z0-9_-]{21}$`

	// Limits
	MaxTrimDuration = time.Duration(mustGetEnvInt("MAX_TRIM_DURATION_MIN")) * time.Minute
	MaxFileSize     = int64(mustGetEnvInt("MAX_FILE_SIZE_GB")) * 1024 * 1024 * 1024

	// Feature Flags
	EnableMerge    = mustGetEnvBool("ENABLE_MERGE")
	EnableTrim     = mustGetEnvBool("ENABLE_TRIM")
	EnableReencode = mustGetEnvBool("ENABLE_REENCODE")

	// Tier Config (parsed from JSON)
	TierConfigs = loadTierConfigs()
)

// ============================================
// FIXED CONSTANTS (Not in ENV)
// ============================================

var (
	VideoFormats = []string{"mp4", "webm", "mkv"}
	AudioFormats = []string{"mp3", "m4a", "wav", "opus", "flac", "ogg"}
	Qualities    = []string{"2160p", "1440p", "1080p", "720p", "480p", "360p", "144p"}
	OSTypes      = []string{"ios", "android", "macos", "windows", "linux"}
)

// Quality to height mapping
var QualityToHeight = map[string]int{
	"2160p": 2160,
	"1440p": 1440,
	"1080p": 1080,
	"720p":  720,
	"480p":  480,
	"360p":  360,
	"144p":  144,
}

// Height to quality mapping
var HeightToQuality = map[int]string{
	2160: "2160p",
	1440: "1440p",
	1080: "1080p",
	720:  "720p",
	480:  "480p",
	360:  "360p",
	144:  "144p",
}

// Device profiles
type DeviceProfile struct {
	MaxQuality  string
	VideoCodecs []string
	AudioCodecs []string
}

var DeviceProfiles = map[string]DeviceProfile{
	"ios": {
		MaxQuality:  "1080p",
		VideoCodecs: []string{"avc1"},
		AudioCodecs: []string{"mp4a"},
	},
	"android": {
		MaxQuality:  "2160p",
		VideoCodecs: []string{"av01", "vp9", "avc1"},
		AudioCodecs: []string{"opus", "mp4a"},
	},
	"macos": {
		MaxQuality:  "1080p",
		VideoCodecs: []string{"avc1"},
		AudioCodecs: []string{"mp4a"},
	},
	"windows": {
		MaxQuality:  "2160p",
		VideoCodecs: []string{"av01", "vp9", "avc1"},
		AudioCodecs: []string{"opus", "mp4a"},
	},
	"linux": {
		MaxQuality:  "2160p",
		VideoCodecs: []string{"av01", "vp9", "avc1"},
		AudioCodecs: []string{"opus", "mp4a"},
	},
}

// Default profile
var DefaultProfile = DeviceProfile{
	MaxQuality:  "1080p",
	VideoCodecs: []string{"avc1"},
	AudioCodecs: []string{"mp4a"},
}

// FFmpeg codec mappings
var AudioCodecMap = map[string]string{
	"mp3": "libmp3lame", "m4a": "aac", "mp4": "aac", "wav": "pcm_s16le",
	"opus": "libopus", "flac": "flac", "webm": "libopus", "ogg": "libvorbis",
}

var VideoCodecMap = map[string]string{
	"mp4": "libx264", "mkv": "libx264", "webm": "libvpx-vp9",
}

// MIME type to extension mapping
var MimeToExt = map[string]string{
	"video/mp4": "mp4", "video/webm": "webm",
	"audio/mp4": "m4a", "audio/webm": "webm", "audio/mpeg": "mp3",
	"audio/ogg": "ogg", "audio/opus": "opus", "audio/flac": "flac",
	"audio/wav": "wav", "audio/x-wav": "wav",
}

// ============================================
// TIER CONFIGURATION
// ============================================

// TierConfig holds configuration for each customer tier
type TierConfig struct {
	Name    string
	Threads int   `json:"threads"`
	Rate    int64 `json:"rate"`
}

func loadTierConfigs() map[int]TierConfig {
	tierJSON := mustGetEnv("TIER_CONFIG")

	var configs map[string]TierConfig
	if err := json.Unmarshal([]byte(tierJSON), &configs); err != nil {
		panic("Invalid TIER_CONFIG: " + err.Error())
	}

	// Convert string keys to int and add names
	result := make(map[int]TierConfig)
	for k, v := range configs {
		tier, _ := strconv.Atoi(k)
		if tier == 0 {
			v.Name = "Standard"
		} else if tier == 1 {
			v.Name = "Tier1"
		} else {
			v.Name = fmt.Sprintf("Tier%d", tier)
		}
		result[tier] = v
	}

	return result
}

// GetTierConfig returns the configuration for a given customer tier
// Falls back to tier 0 (Standard) if tier is not found
func GetTierConfig(ctier int) TierConfig {
	if config, exists := TierConfigs[ctier]; exists {
		return config
	}
	// Default to tier 0 (Standard) for unknown tiers
	return TierConfigs[0]
}

// GetStreamRateLimit returns the stream rate limit based on customer tier
func GetStreamRateLimit(ctier int) int64 {
	return GetTierConfig(ctier).Rate
}

// GetDownloadThreads returns the number of download threads based on customer tier
func GetDownloadThreads(ctier int) int {
	return GetTierConfig(ctier).Threads
}

// ============================================
// HTTP CLIENTS
// ============================================

// BufferPool for reusing buffers (reduces GC pressure)
var BufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, BufferSize)
		return &buf
	},
}

// HTTP Clients (reuse connections via pooling)
var (
	ExtractClient         *http.Client
	DownloadClient        *http.Client // With WARP proxy
	DownloadClientNoProxy *http.Client // Direct IP (no proxy)
)

// Transport for Extract API (no proxy - local API)
var extractTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
}

// HTTP proxy URLs
var warpProxyURL, _ = url.Parse(WARPProxyURL)
var directProxyURL, _ = url.Parse(DirectProxyURL)

// Transport for Download with WARP proxy
var downloadTransport = &http.Transport{
	Proxy:               http.ProxyURL(warpProxyURL),
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
	DisableCompression:  true, // No gzip for downloads - save CPU
}

// Transport for Download without proxy (direct IP)
var downloadTransportNoProxy = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
	DisableCompression:  true, // No gzip for downloads - save CPU
}

func init() {
	ExtractClient = &http.Client{
		Transport: extractTransport,
		Timeout:   ExtractAPITimeout,
	}
	DownloadClient = &http.Client{
		Transport: downloadTransport,
		Timeout:   ChunkTimeout,
	}
	DownloadClientNoProxy = &http.Client{
		Transport: downloadTransportNoProxy,
		Timeout:   ChunkTimeout,
	}
}

// ============================================
// HELPER FUNCTIONS
// ============================================

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic("Required environment variable " + key + " is not set")
	}
	return value
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func mustGetEnvInt(key string) int {
	value := mustGetEnv(key)
	i, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("Invalid integer value for %s: %s", key, value))
	}
	return i
}

func mustGetEnvBool(key string) bool {
	value := mustGetEnv(key)
	return value == "true" || value == "1"
}
