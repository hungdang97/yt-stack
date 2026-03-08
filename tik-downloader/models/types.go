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
	Type      string  `json:"type"` // "video" or "audio"
	Title     string  `json:"title"`
	Duration  float64 `json:"duration"` // seconds
	Thumbnail string  `json:"thumbnail,omitempty"`
}

// StatusResponse for polling
type StatusResponse struct {
	Status      string  `json:"status"`
	Progress    int     `json:"progress"`
	Title       string  `json:"title,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	Thumbnail   string  `json:"thumbnail,omitempty"`
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
// TIK-EXTRACTOR API
// ============================================

// TikExtractRequest sent to tik-extractor POST /tiktok/detail
type TikExtractRequest struct {
	DetailID string `json:"detail_id"`
	Cookie   string `json:"cookie,omitempty"`
	Proxy    string `json:"proxy,omitempty"`
	Source   bool   `json:"source,omitempty"`
}

// TikExtractResponse is the full response from tik-extractor
type TikExtractResponse struct {
	Message string           `json:"message"`
	Data    TikVideoData     `json:"data"`
	Params  TikExtractParams `json:"params"`
	Time    string           `json:"time"`
}

// TikExtractParams contains the extraction parameters echoed back
type TikExtractParams struct {
	Cookie   string `json:"cookie"`
	Proxy    string `json:"proxy"`
	Source   bool   `json:"source"`
	DetailID string `json:"detail_id"`
}

// TikVideoData represents a single video item from tik-extractor
type TikVideoData struct {
	ID              string   `json:"id"`
	Desc            string   `json:"desc"`
	CreateTimestamp int64    `json:"create_timestamp"`
	CreateTime      string   `json:"create_time"`
	TextExtra       []string `json:"text_extra"`
	Type            string   `json:"type"`
	Height          int      `json:"height"`
	Width           int      `json:"width"`
	Downloads       string   `json:"downloads"`
	Duration        string   `json:"duration"`
	URI             string   `json:"uri"`
	DynamicCover    string   `json:"dynamic_cover"`
	StaticCover     string   `json:"static_cover"`
	UID             string   `json:"uid"`
	SecUID          string   `json:"sec_uid"`
	UniqueID        string   `json:"unique_id"`
	Signature       string   `json:"signature"`
	UserAge         int      `json:"user_age"`
	Nickname        string   `json:"nickname"`
	Mark            string   `json:"mark"`
	MusicAuthor     string   `json:"music_author"`
	MusicTitle      string   `json:"music_title"`
	MusicURL        string   `json:"music_url"`
	DiggCount       int      `json:"digg_count"`
	CommentCount    int      `json:"comment_count"`
	CollectCount    int      `json:"collect_count"`
	ShareCount      int      `json:"share_count"`
	PlayCount       int      `json:"play_count"`
	Tag             []string `json:"tag"`
	Extra           string   `json:"extra"`
	ShareURL        string   `json:"share_url"`
	CollectionTime  string   `json:"collection_time"`
}
