package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
	"yt-downloader-go/utils"
)

// Extract fetches video metadata from YouTube Extract API
func Extract(videoID string) (*models.ExtractResponse, error) {
	apiURL := fmt.Sprintf("%s/%s?proxy=%s", config.ExtractAPIBase, videoID, config.WARPProxyURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := config.ExtractClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result models.ExtractResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// SelectVideo selects the best video stream based on quality and device
func SelectVideo(data *models.ExtractResponse, requestedQuality string, osType string) *models.VideoSelectionResult {
	result := &models.VideoSelectionResult{}

	// Get device profile
	profile, ok := config.DeviceProfiles[osType]
	if !ok {
		profile = config.DefaultProfile
	}

	// Filter streams by supported codecs
	var compatibleStreams []models.Stream
	for _, stream := range data.VideoStreams {
		codec := getStreamCodec(&stream)
		if isCodecSupported(codec, profile.VideoCodecs) {
			compatibleStreams = append(compatibleStreams, stream)
		}
	}

	if len(compatibleStreams) == 0 {
		return result
	}

	// Sort by height (descending) then by bitrate (descending)
	sort.Slice(compatibleStreams, func(i, j int) bool {
		if compatibleStreams[i].Height != compatibleStreams[j].Height {
			return compatibleStreams[i].Height > compatibleStreams[j].Height
		}
		return compatibleStreams[i].Bitrate > compatibleStreams[j].Bitrate
	})

	// Get max quality for device
	maxHeight := config.QualityToHeight[profile.MaxQuality]
	requestedHeight := 0
	if requestedQuality != "" {
		requestedHeight = config.QualityToHeight[requestedQuality]
	}

	// Limit requested height to device max
	if requestedHeight > maxHeight {
		requestedHeight = maxHeight
		result.QualityChanged = true
		result.QualityChangeReason = fmt.Sprintf("Device max quality is %s", profile.MaxQuality)
	}

	// Find best matching stream
	var selectedStream *models.Stream

	if requestedHeight > 0 {
		// Try exact match first
		for i := range compatibleStreams {
			if compatibleStreams[i].Height == requestedHeight {
				selectedStream = &compatibleStreams[i]
				break
			}
		}

		// If no exact match, find closest lower quality
		if selectedStream == nil {
			for i := range compatibleStreams {
				if compatibleStreams[i].Height < requestedHeight {
					selectedStream = &compatibleStreams[i]
					result.QualityChanged = true
					result.QualityChangeReason = fmt.Sprintf("Requested %s not available, using %s",
						requestedQuality, config.HeightToQuality[compatibleStreams[i].Height])
					break
				}
			}
		}

		// If still no match, use highest available
		if selectedStream == nil && len(compatibleStreams) > 0 {
			selectedStream = &compatibleStreams[0]
			result.QualityChanged = true
			result.QualityChangeReason = fmt.Sprintf("Using highest available: %s",
				config.HeightToQuality[compatibleStreams[0].Height])
		}
	} else {
		// No quality specified, use highest within device limits
		for i := range compatibleStreams {
			if compatibleStreams[i].Height <= maxHeight {
				selectedStream = &compatibleStreams[i]
				break
			}
		}
		if selectedStream == nil && len(compatibleStreams) > 0 {
			selectedStream = &compatibleStreams[0]
		}
	}

	if selectedStream != nil {
		result.Stream = selectedStream
		result.SelectedQuality = config.HeightToQuality[selectedStream.Height]
		if result.SelectedQuality == "" {
			result.SelectedQuality = fmt.Sprintf("%dp", selectedStream.Height)
		}
	}

	return result
}

// SelectAudio selects the best audio stream based on device and track
func SelectAudio(data *models.ExtractResponse, trackID string, osType string) *models.Stream {
	// Get device profile
	profile, ok := config.DeviceProfiles[osType]
	if !ok {
		profile = config.DefaultProfile
	}

	// Filter streams by supported codecs
	var compatibleStreams []models.Stream
	for _, stream := range data.AudioStreams {
		codec := getStreamCodec(&stream)
		if isCodecSupported(codec, profile.AudioCodecs) {
			compatibleStreams = append(compatibleStreams, stream)
		}
	}

	if len(compatibleStreams) == 0 {
		return nil
	}

	// Filter by track ID if specified
	if trackID != "" {
		var filtered []models.Stream
		for _, stream := range compatibleStreams {
			if stream.AudioTrackID == trackID {
				filtered = append(filtered, stream)
			}
		}
		if len(filtered) > 0 {
			compatibleStreams = filtered
		}
	} else {
		// Prefer original audio track
		var originals []models.Stream
		for _, stream := range compatibleStreams {
			if stream.IsOriginal {
				originals = append(originals, stream)
			}
		}
		if len(originals) > 0 {
			compatibleStreams = originals
		}
	}

	// Sort by codec priority, then bitrate (higher is better)
	sort.Slice(compatibleStreams, func(i, j int) bool {
		codecI := getStreamCodec(&compatibleStreams[i])
		codecJ := getStreamCodec(&compatibleStreams[j])
		priorityI := codecPriority(codecI, profile.AudioCodecs)
		priorityJ := codecPriority(codecJ, profile.AudioCodecs)

		if priorityI != priorityJ {
			return priorityI < priorityJ
		}
		return compatibleStreams[i].Bitrate > compatibleStreams[j].Bitrate
	})

	if len(compatibleStreams) > 0 {
		return &compatibleStreams[0]
	}

	return nil
}

// getStreamCodec returns the codec from Stream, preferring Codec field over mimeType extraction
func getStreamCodec(stream *models.Stream) string {
	// Prefer direct codec field if available
	if stream.Codec != "" {
		// Extract base codec: "avc1.4d400c" -> "avc1", "mp4a.40.2" -> "mp4a"
		codec := stream.Codec
		if dotIdx := strings.Index(codec, "."); dotIdx != -1 {
			codec = codec[:dotIdx]
		}
		return codec
	}
	// Fallback to extracting from mimeType
	return extractCodec(stream.MimeType)
}

// extractCodec extracts codec identifier from MIME type
func extractCodec(mimeType string) string {
	// Example: "video/mp4; codecs=\"avc1.640028\"" -> "avc1"
	// Example: "audio/webm; codecs=\"opus\"" -> "opus"

	if idx := strings.Index(mimeType, "codecs="); idx != -1 {
		codec := mimeType[idx+7:]
		codec = strings.Trim(codec, "\"' ")
		// Get first part before dot: "avc1.640028" -> "avc1"
		if dotIdx := strings.Index(codec, "."); dotIdx != -1 {
			codec = codec[:dotIdx]
		}
		return codec
	}

	// Fallback: extract from MIME type
	// "video/mp4" -> "mp4", "audio/webm" -> "webm"
	if parts := strings.Split(mimeType, "/"); len(parts) == 2 {
		return strings.Split(parts[1], ";")[0]
	}

	return ""
}

// isCodecSupported checks if codec is in supported list
func isCodecSupported(codec string, supported []string) bool {
	for _, s := range supported {
		if strings.HasPrefix(codec, s) {
			return true
		}
	}
	return false
}

// codecPriority returns priority (lower is better)
func codecPriority(codec string, priorityList []string) int {
	for i, c := range priorityList {
		if strings.HasPrefix(codec, c) {
			return i
		}
	}
	return len(priorityList)
}

// GetExtension returns file extension for a stream
func GetExtension(stream *models.Stream) string {
	return utils.GetExtFromMimeType(stream.MimeType)
}

// NeedsReencode checks if video/audio need re-encoding for target format
func NeedsReencode(videoStream *models.Stream, audioStream *models.Stream, targetFormat string) bool {
	if videoStream == nil {
		return false
	}

	videoCodec := getStreamCodec(videoStream)
	audioCodec := ""
	if audioStream != nil {
		audioCodec = getStreamCodec(audioStream)
	}

	switch targetFormat {
	case "mp4":
		// MP4 supports H.264/H.265 video and AAC audio
		videoOK := slices.Contains([]string{"avc1", "hvc1", "hev1"}, videoCodec)
		audioOK := audioCodec == "" || strings.HasPrefix(audioCodec, "mp4a")
		return !(videoOK && audioOK)
	case "webm":
		// WebM supports VP8/VP9/AV1 video and Opus/Vorbis audio
		videoOK := slices.Contains([]string{"vp8", "vp9", "vp09", "av01"}, videoCodec)
		audioOK := audioCodec == "" || slices.Contains([]string{"opus", "vorbis"}, audioCodec)
		return !(videoOK && audioOK)
	case "mkv":
		// MKV supports almost everything
		return false
	}

	return false
}
