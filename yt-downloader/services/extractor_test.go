package services

import (
	"testing"
	"yt-downloader-go/models"
)

// TestIsVideoCodecCompatible tests codec compatibility checking
func TestIsVideoCodecCompatible(t *testing.T) {
	tests := []struct {
		name         string
		codec        string
		targetFormat string
		expected     bool
	}{
		// MP4 tests
		{"H.264 to MP4", "avc1", "mp4", true},
		{"H.265 to MP4", "hvc1", "mp4", true},
		{"VP9 to MP4", "vp9", "mp4", true},  // Now compatible - modern players support VP9 in MP4
		{"AV1 to MP4", "av01", "mp4", true}, // Now compatible - modern players support AV1 in MP4

		// WebM tests
		{"VP9 to WebM", "vp9", "webm", true},
		{"VP8 to WebM", "vp8", "webm", true},
		{"AV1 to WebM", "av01", "webm", true},
		{"H.264 to WebM", "avc1", "webm", false},

		// MKV tests
		{"VP9 to MKV", "vp9", "mkv", true},
		{"H.264 to MKV", "avc1", "mkv", true},
		{"AV1 to MKV", "av01", "mkv", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVideoCodecCompatible(tt.codec, tt.targetFormat)
			if result != tt.expected {
				t.Errorf("isVideoCodecCompatible(%s, %s) = %v, want %v",
					tt.codec, tt.targetFormat, result, tt.expected)
			}
		})
	}
}

// TestSelectVideo_NeedsReencode tests that NeedsReencode flag is set correctly
func TestSelectVideo_NeedsReencode(t *testing.T) {
	tests := []struct {
		name             string
		targetFormat     string
		videoCodec       string
		expectedReencode bool
	}{
		{"VP9 to MP4 no re-encode", "mp4", "vp9", false}, // VP9 is now compatible with MP4
		{"H.264 to MP4 no re-encode", "mp4", "avc1", false},
		{"VP9 to WebM no re-encode", "webm", "vp9", false},
		{"H.264 to WebM needs re-encode", "webm", "avc1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock extract response
			extractData := &models.ExtractResponse{
				Title:    "Test Video",
				Duration: 100.0,
				VideoStreams: []models.Stream{
					{
						URL:           "https://example.com/video",
						MimeType:      "video/webm",
						Codec:         tt.videoCodec,
						Width:         1920,
						Height:        1080,
						Bitrate:       5000000,
						ContentLength: 100000000,
					},
				},
			}

			result := SelectVideo(extractData, "1080p", "android", tt.targetFormat)

			if result.Stream == nil {
				t.Fatal("Expected stream to be selected")
			}

			if result.NeedsReencode != tt.expectedReencode {
				t.Errorf("NeedsReencode = %v, want %v", result.NeedsReencode, tt.expectedReencode)
			}
		})
	}
}

// TestSelectVideo_PreferCompatibleCodec tests that best quality compatible codec is selected
func TestSelectVideo_PreferCompatibleCodec(t *testing.T) {
	// Create mock extract response with both VP9 and H.264 streams at same quality
	extractData := &models.ExtractResponse{
		Title:    "Test Video",
		Duration: 100.0,
		VideoStreams: []models.Stream{
			{
				URL:           "https://example.com/video-vp9",
				MimeType:      "video/webm",
				Codec:         "vp9",
				Width:         1920,
				Height:        1080,
				Bitrate:       5000000,
				ContentLength: 100000000,
			},
			{
				URL:           "https://example.com/video-h264",
				MimeType:      "video/mp4",
				Codec:         "avc1",
				Width:         1920,
				Height:        1080,
				Bitrate:       4500000, // Slightly lower bitrate
				ContentLength: 95000000,
			},
		},
	}

	// When targeting MP4, should prefer VP9 (higher bitrate) since it's now compatible
	result := SelectVideo(extractData, "1080p", "android", "mp4")

	if result.Stream == nil {
		t.Fatal("Expected stream to be selected")
	}

	codec := GetStreamCodec(result.Stream)
	if codec != "vp9" {
		t.Errorf("Expected VP9 to be preferred for MP4 (higher bitrate, compatible), got %s", codec)
	}

	if result.NeedsReencode {
		t.Error("Expected NeedsReencode to be false for compatible codec")
	}
}

// TestSelectVideo_VerticalVideo tests quality selection for vertical (portrait) videos
func TestSelectVideo_VerticalVideo(t *testing.T) {
	tests := []struct {
		name             string
		width            int
		height           int
		requestedQuality string
		expectedQuality  string
		shouldMatch      bool
	}{
		{
			name:             "Vertical 1080p (1080x1920)",
			width:            1080,
			height:           1920,
			requestedQuality: "1080p",
			expectedQuality:  "1080p",
			shouldMatch:      true,
		},
		{
			name:             "Landscape 1080p (1920x1080)",
			width:            1920,
			height:           1080,
			requestedQuality: "1080p",
			expectedQuality:  "1080p",
			shouldMatch:      true,
		},
		{
			name:             "Vertical 720p (720x1280)",
			width:            720,
			height:           1280,
			requestedQuality: "720p",
			expectedQuality:  "720p",
			shouldMatch:      true,
		},
		{
			name:             "Landscape 720p (1280x720)",
			width:            1280,
			height:           720,
			requestedQuality: "720p",
			expectedQuality:  "720p",
			shouldMatch:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractData := &models.ExtractResponse{
				Title:    "Test Video",
				Duration: 100.0,
				VideoStreams: []models.Stream{
					{
						URL:           "https://example.com/video",
						MimeType:      "video/mp4",
						Codec:         "avc1",
						Width:         tt.width,
						Height:        tt.height,
						Bitrate:       5000000,
						ContentLength: 100000000,
					},
				},
			}

			result := SelectVideo(extractData, tt.requestedQuality, "android", "mp4")

			if result.Stream == nil {
				t.Fatal("Expected stream to be selected")
			}

			if result.SelectedQuality != tt.expectedQuality {
				t.Errorf("SelectedQuality = %s, want %s", result.SelectedQuality, tt.expectedQuality)
			}

			if tt.shouldMatch && result.QualityChanged {
				t.Errorf("Expected exact quality match, but QualityChanged = true, reason: %s",
					result.QualityChangeReason)
			}
		})
	}
}

// TestGetQualityDimension tests the quality dimension calculation
func TestGetQualityDimension(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		height   int
		expected int
	}{
		{"Landscape 1920x1080", 1920, 1080, 1080},
		{"Vertical 1080x1920", 1080, 1920, 1080},
		{"Landscape 1280x720", 1280, 720, 720},
		{"Vertical 720x1280", 720, 1280, 720},
		{"Square 1080x1080", 1080, 1080, 1080},
		{"No width (fallback)", 0, 1080, 1080},
		{"No height (fallback)", 1920, 0, 1920},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &models.Stream{
				Width:  tt.width,
				Height: tt.height,
			}
			result := getQualityDimension(stream)
			if result != tt.expected {
				t.Errorf("getQualityDimension(%dx%d) = %d, want %d",
					tt.width, tt.height, result, tt.expected)
			}
		})
	}
}
