package models

// DownloadRequest represents the incoming download request
// @Description Download request payload
type DownloadRequest struct {
	URL    string       `json:"url" example:"https://youtube.com/watch?v=dQw4w9WgXcQ"`
	OS     string       `json:"os,omitempty" example:"windows" enums:"ios,android,macos,windows,linux"`
	Output OutputConfig `json:"output"`
	Audio  AudioConfig  `json:"audio,omitempty"`
	Trim   *TrimConfig  `json:"trim,omitempty"`
}

// OutputConfig specifies output format and quality
// @Description Output configuration
type OutputConfig struct {
	Type    string `json:"type" example:"video" enums:"video,audio"`
	Format  string `json:"format" example:"mp4" enums:"mp4,webm,mkv,mp3,m4a,wav,opus,flac"`
	Quality string `json:"quality,omitempty" example:"1080p" enums:"2160p,1440p,1080p,720p,480p,360p"`
}

// AudioConfig specifies audio track and bitrate
// @Description Audio configuration
type AudioConfig struct {
	TrackID string `json:"trackId,omitempty" example:"en.vss_abc123"`
	Bitrate string `json:"bitrate,omitempty" example:"192k" enums:"64k,128k,192k,320k"`
}

// TrimConfig specifies trim start and end times
// @Description Trim configuration
type TrimConfig struct {
	Start    float64 `json:"start" example:"10"`
	End      float64 `json:"end" example:"60"`
	Accurate bool    `json:"accurate,omitempty" example:"false"`
}

// DownloadResponse is returned when a job is created
// @Description Response after creating a download job
type DownloadResponse struct {
	StatusURL           string  `json:"statusUrl" example:"https://api.ytconvert.org/api/status/V1StGXR8_Z5jdHi?token=xxx&expires=xxx"`
	Title               string  `json:"title" example:"Rick Astley - Never Gonna Give You Up"`
	Duration            float64 `json:"duration" example:"213.5"`
	RequestedQuality    string  `json:"requestedQuality,omitempty" example:"1080p"`
	SelectedQuality     string  `json:"selectedQuality,omitempty" example:"720p"`
	QualityChanged      bool    `json:"qualityChanged" example:"true"`
	QualityChangeReason string  `json:"qualityChangeReason,omitempty" example:"1080p not available, using 720p"`
	NeedsReencode       bool    `json:"needsReencode" example:"false"`
}

// Job status constants
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusError     = "error"
)

// StatusResponse is returned when checking job status
// @Description Job status response
type StatusResponse struct {
	Status      string  `json:"status" example:"pending" enums:"pending,completed,error"`
	Progress    int     `json:"progress" example:"45"`
	Title       string  `json:"title,omitempty" example:"Rick Astley - Never Gonna Give You Up"`
	Duration    float64 `json:"duration,omitempty" example:"213.5"`
	DownloadURL string  `json:"downloadUrl,omitempty" example:"https://api.ytconvert.org/files/abc123/output.mp4?token=xxx&expires=123"`
	JobError    string  `json:"jobError,omitempty" example:"Download failed: connection timeout"`
}

// Meta represents job metadata stored in meta.json
type Meta struct {
	ID         string      `json:"id"`
	Status     string      `json:"status"` // pending, completed, error
	CreatedAt  int64       `json:"createdAt"`
	VideoID    string      `json:"videoId"`
	Title      string      `json:"title"`
	Duration   float64     `json:"duration"`
	Files      FilesInfo   `json:"files"`
	OutputType string      `json:"outputType"` // video or audio
	Format     string      `json:"format"`
	Quality    string      `json:"quality,omitempty"`
	Bitrate    string      `json:"bitrate,omitempty"`
	Trim       *TrimConfig `json:"trim,omitempty"`
	Output     string      `json:"output,omitempty"`
	StreamOnly bool        `json:"streamOnly,omitempty"` // true = skip merge, stream only
	Error      string      `json:"error,omitempty"`
}

type FilesInfo struct {
	Video *FileInfo `json:"video,omitempty"`
	Audio *FileInfo `json:"audio,omitempty"`
}

type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ExtractResponse from YouTube Extract API
type ExtractResponse struct {
	Title        string   `json:"title"`
	Duration     float64  `json:"duration"`
	VideoStreams []Stream `json:"videoStreams"`
	AudioStreams []Stream `json:"audioStreams"`
}

// Stream represents a video or audio stream
type Stream struct {
	URL           string  `json:"url"`
	MimeType      string  `json:"mimeType"`
	Codec         string  `json:"codec,omitempty"`
	Quality       string  `json:"quality,omitempty"`
	QualityLabel  string  `json:"qualityLabel,omitempty"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	Bitrate       float64 `json:"bitrate,omitempty"`
	ContentLength int64   `json:"fileSize,omitempty"`
	AudioTrackID  string  `json:"audioTrackId,omitempty"`
	IsOriginal    bool    `json:"isOriginal,omitempty"`
	FPS           int     `json:"fps,omitempty"`
}

// VideoSelectionResult contains the selected video stream and metadata
type VideoSelectionResult struct {
	Stream              *Stream
	SelectedQuality     string
	QualityChanged      bool
	QualityChangeReason string
	NeedsReencode       bool
}

// HealthResponse for health check
// @Description Health check response
type HealthResponse struct {
	Status    string `json:"status" example:"ok"`
	Timestamp int64  `json:"timestamp" example:"1705123456789"`
}

// DeleteResponse for job deletion
// @Description Delete job response
type DeleteResponse struct {
	Deleted bool `json:"deleted" example:"true"`
}
