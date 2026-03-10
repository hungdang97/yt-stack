package models

// ============================================
// REQUEST / RESPONSE
// ============================================

type DownloadRequest struct {
	URL  string `json:"url"`
	Type string `json:"type"` // "video", "image", or "audio"
}

type DownloadResponse struct {
	StatusURL string  `json:"statusUrl"`
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Duration  float64 `json:"duration,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
}

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
	StatusProcessing  = "processing" // FFmpeg extracting audio
	StatusCompleted   = "completed"
	StatusError       = "error"
)

type Meta struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	Title        string  `json:"title"`
	Duration     float64 `json:"duration,omitempty"`
	OutputType   string  `json:"output_type"` // "video", "image", or "audio"
	Output       string  `json:"output"`
	Error        string  `json:"error,omitempty"`
	CreatedAt    int64   `json:"created_at"`
	FileSize     int64   `json:"file_size"`
	VideoURL     string  `json:"video_url,omitempty"`
	ImageURL     string  `json:"image_url,omitempty"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
	Author       string  `json:"author,omitempty"`
	SourceURL    string  `json:"source_url"`
}

// ============================================
// INSTA-EXTRACTOR API RESPONSE
// ============================================

type InstaMediaItem struct {
	IsVideo    bool   `json:"is_video"`
	VideoURL   string `json:"video_url"`
	DisplayURL string `json:"display_url"`
}

type InstaExtractResponse struct {
	Shortcode            string           `json:"shortcode"`
	MediaID              int64            `json:"media_id"`
	Typename             string           `json:"typename"`
	Caption              string           `json:"caption"`
	CaptionHashtags      []string         `json:"caption_hashtags"`
	CaptionMentions      []string         `json:"caption_mentions"`
	TaggedUsers          []string         `json:"tagged_users"`
	Likes                int              `json:"likes"`
	Comments             int              `json:"comments"`
	DateUTC              string           `json:"date_utc"`
	DateLocal            string           `json:"date_local"`
	IsVideo              bool             `json:"is_video"`
	IsPinned             bool             `json:"is_pinned"`
	IsSponsored          bool             `json:"is_sponsored"`
	VideoDuration        *float64         `json:"video_duration"`
	VideoViewCount       *int             `json:"video_view_count"`
	VideoPlayCount       *int             `json:"video_play_count"`
	Title                string           `json:"title"`
	AccessibilityCaption string           `json:"accessibility_caption"`
	OwnerUsername        string           `json:"owner_username"`
	OwnerID              int64            `json:"owner_id"`
	Location             interface{}      `json:"location"`
	MediaCount           int              `json:"media_count"`
	Media                []InstaMediaItem `json:"media"`
}

// GetVideoURL returns the first video URL from media items
func (r *InstaExtractResponse) GetVideoURL() string {
	for _, m := range r.Media {
		if m.IsVideo && m.VideoURL != "" {
			return m.VideoURL
		}
	}
	return ""
}

// GetImageURL returns the first display URL from media items
func (r *InstaExtractResponse) GetImageURL() string {
	for _, m := range r.Media {
		if m.DisplayURL != "" {
			return m.DisplayURL
		}
	}
	return ""
}
