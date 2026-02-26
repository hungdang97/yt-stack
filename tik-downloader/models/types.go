package models

// ============================================
// REQUEST / RESPONSE
// ============================================

// DownloadRequest from client (via Hub)
type DownloadRequest struct {
	URL   string `json:"url"`
	Type  string `json:"type"`  // "video" or "audio"
	CTier int    `json:"ctier"` // Customer tier (injected by Hub)
}

// DownloadResponse returned to client
type DownloadResponse struct {
	StatusURL string  `json:"statusUrl"`
	Title     string  `json:"title"`
	Duration  float64 `json:"duration"` // seconds
}

// StatusResponse for polling
type StatusResponse struct {
	Status      string  `json:"status"`
	Progress    int     `json:"progress"`
	Title       string  `json:"title,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	DownloadURL string  `json:"downloadUrl,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// ============================================
// JOB METADATA (stored as meta.json)
// ============================================

const (
	StatusPending     = "pending"
	StatusExtracting  = "extracting"
	StatusDownloading = "downloading"
	StatusCompleted   = "completed"
	StatusError       = "error"
)

type Meta struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	Title        string  `json:"title"`
	Duration     float64 `json:"duration"`
	OutputType   string  `json:"output_type"` // "video" or "audio"
	Output       string  `json:"output"`      // output filename
	Error        string  `json:"error,omitempty"`
	CreatedAt    int64   `json:"created_at"`
	FileSize     int64   `json:"file_size"`
	VideoURL     string  `json:"video_url,omitempty"`
	MusicURL     string  `json:"music_url,omitempty"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
	Author       string  `json:"author,omitempty"`
	SourceURL    string  `json:"source_url"`
}

// ============================================
// TIK-EXTRACTOR RESPONSE
// ============================================

// TikExtractRequest sent to tik-extractor POST /tiktok/detail
type TikExtractRequest struct {
	DetailID string `json:"detail_id"`
	Cookie   string `json:"cookie,omitempty"`
	Proxy    string `json:"proxy,omitempty"`
	Source   bool   `json:"source,omitempty"`
}

// TikExtractResponse from tik-extractor
type TikExtractResponse struct {
	Data []TikVideoData `json:"data"`
}

// TikVideoData represents a single video item from tik-extractor
type TikVideoData struct {
	ID           string `json:"id"`
	Desc         string `json:"desc"`
	Downloads    string `json:"downloads"` // Video download URL
	MusicURL     string `json:"music_url"` // Audio URL
	MusicTitle   string `json:"music_title"`
	MusicAuthor  string `json:"music_author"`
	Duration     string `json:"duration"` // "00:00:30" format
	Height       int    `json:"height"`
	Width        int    `json:"width"`
	Type         string `json:"type"` // "视频" for video
	StaticCover  string `json:"static_cover"`
	DynamicCover string `json:"dynamic_cover"`
	Nickname     string `json:"nickname"`
	UniqueID     string `json:"unique_id"`
	ShareURL     string `json:"share_url"`
	DiggCount    int    `json:"digg_count"`
	PlayCount    int    `json:"play_count"`
}
