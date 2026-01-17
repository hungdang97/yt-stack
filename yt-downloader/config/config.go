package config

import (
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload" // Auto-load .env file
)

const (
	// Server
	Port = 5001

	// Storage
	StorageDir = "./storage"

	// Download settings
	Threads      = 4
	ChunkSize    = 10_000_000 // 10MB
	MaxRetries   = 3
	RetryDelay   = 100 * time.Millisecond
	ChunkTimeout = 30 * time.Second
	BufferSize   = 64 * 1024 // 64KB - optimal for io.CopyBuffer

	// Extract API
	ExtractAPIBase    = "http://127.0.0.1:8300/api/youtube/video"
	ExtractAPITimeout = 15 * time.Second

	// Cleanup
	CleanupInterval  = "*/5 * * * *" // Every 5 minutes
	MaxJobAge        = 30 * time.Minute
	CleanupBatchSize = 5000
	// Job ID
	JobIDLength = 21
	JobIDRegex  = `^[a-zA-Z0-9_-]{21}$`

	// Signed URL
	SignedURLSecret     = "18072001aA@"
	SignedURLExpiration = 30 * time.Minute

	// Limits
	MaxTrimDuration = 24 * time.Hour

	// Stream rate limit (bytes per second)
	// 0 = unlimited, otherwise limits FFmpeg output read speed
	// This creates backpressure to prevent FFmpeg from processing faster than needed
	StreamRateLimit = 1 * 1024 * 1024 // 1MB/s
)

// Base URL for download links (required env)
var BaseURL = mustGetEnv("BASE_URL")

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic("Required environment variable " + key + " is not set")
	}
	return value
}

// Supported formats
var (
	VideoFormats = []string{"mp4", "webm", "mkv"}
	AudioFormats = []string{"mp3", "m4a", "wav", "opus", "flac"}
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
	"mp3":  "libmp3lame",
	"m4a":  "aac",
	"mp4":  "aac",
	"wav":  "pcm_s16le",
	"opus": "libopus",
	"flac": "flac",
	"webm": "libopus",
}

var VideoCodecMap = map[string]string{
	"mp4":  "libx264",
	"mkv":  "libx264",
	"webm": "libvpx-vp9",
}

// MIME type to extension mapping
var MimeToExt = map[string]string{
	"video/mp4":   "mp4",
	"video/webm":  "webm",
	"audio/mp4":   "m4a",
	"audio/webm":  "webm",
	"audio/mpeg":  "mp3",
	"audio/ogg":   "ogg",
	"audio/opus":  "opus",
	"audio/flac":  "flac",
	"audio/wav":   "wav",
	"audio/x-wav": "wav",
}

// WARP Proxy config
const WARPProxyURL = "http://wrap:1111@0.0.0.0:1111"

// BufferPool for reusing buffers (reduces GC pressure)
var BufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, BufferSize)
		return &buf
	},
}

// HTTP Clients (reuse connections via pooling)
var (
	ExtractClient  *http.Client
	DownloadClient *http.Client
)

// Transport for Extract API (no proxy - local API)
var extractTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
}

// HTTP proxy URL for downloads
var proxyURL, _ = url.Parse(WARPProxyURL)

// Transport for Download (gzip disabled for raw streaming)
var downloadTransport = &http.Transport{
	Proxy:               http.ProxyURL(proxyURL),
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
}
