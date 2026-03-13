package models

// ============================================
// REQUEST / RESPONSE
// ============================================

type OutputConfig struct {
	Type   string `json:"type"`
	Format string `json:"format,omitempty"`
}

type DownloadRequest struct {
	URL    string       `json:"url"`
	OS     string       `json:"os,omitempty"`
	Type   string       `json:"type,omitempty"`
	Output OutputConfig `json:"output,omitempty"`
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
	StatusProcessing  = "processing"
	StatusCompleted   = "completed"
	StatusError       = "error"
)

type Meta struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	Title        string  `json:"title"`
	Duration     float64 `json:"duration,omitempty"`
	OutputType   string  `json:"output_type"`
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
// FB-EXTRACTOR API RESPONSE
// ============================================

type FbMediaItem struct {
	IsVideo             bool   `json:"is_video"`
	VideoURL            string `json:"video_url"`
	VideoProgressiveURL string `json:"video_progressive_url"`
	AudioURL            string `json:"audio_url"`
	DisplayURL          string `json:"display_url"`
}

type FbExtractResponse struct {
	ID             string        `json:"id"`
	Typename       string        `json:"typename"`
	Title          string        `json:"title"`
	Description    string        `json:"description"`
	Caption        string        `json:"caption"`
	OwnerUsername   string        `json:"owner_username"`
	OwnerID        string        `json:"owner_id"`
	IsVideo        bool          `json:"is_video"`
	VideoDuration  *float64      `json:"video_duration"`
	VideoViewCount *int          `json:"video_view_count"`
	MediaCount     int           `json:"media_count"`
	Media          []FbMediaItem `json:"media"`
}

// GetVideoURL returns the best DASH video URL
func (r *FbExtractResponse) GetVideoURL() string {
	for _, m := range r.Media {
		if m.IsVideo && m.VideoURL != "" {
			return m.VideoURL
		}
	}
	return ""
}

// GetVideoProgressiveURL returns the progressive (H.264+audio) video URL
func (r *FbExtractResponse) GetVideoProgressiveURL() string {
	for _, m := range r.Media {
		if m.IsVideo && m.VideoProgressiveURL != "" {
			return m.VideoProgressiveURL
		}
	}
	return ""
}

// GetAudioURL returns the best DASH audio URL
func (r *FbExtractResponse) GetAudioURL() string {
	for _, m := range r.Media {
		if m.AudioURL != "" {
			return m.AudioURL
		}
	}
	return ""
}

// GetImageURL returns the first display URL
func (r *FbExtractResponse) GetImageURL() string {
	for _, m := range r.Media {
		if m.DisplayURL != "" {
			return m.DisplayURL
		}
	}
	return ""
}
