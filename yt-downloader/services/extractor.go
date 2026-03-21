package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"yt-downloader-go/config"
	"yt-downloader-go/models"
	"yt-downloader-go/utils"
)

// Extract fetches video metadata using Cloudflare proxy only
// Python Extractor + Cloudflare Proxy (cookie pool + proxy)
func Extract(videoID string, premium bool) (*models.ExtractResponse, error) {
	// Use Cloudflare Proxy only
	result, err := extractFromAPI(videoID, config.ExtractAPIBase, config.WARPProxyURL, premium)
	if err == nil && isValidExtractResponse(result) {
		fmt.Printf("[%s] ✓ Extract success via Python (Cloudflare) premium=%v\n", videoID, premium)
		return result, nil
	}

	// Extraction failed
	fmt.Printf("[%s] Python+Cloudflare failed: %v\n", videoID, err)
	return nil, fmt.Errorf("extraction failed: %w", err)
}

// isValidExtractResponse validates that the response has required data
func isValidExtractResponse(resp *models.ExtractResponse) bool {
	if resp == nil {
		return false
	}

	// Must have title
	if resp.Title == "" {
		return false
	}

	// Must have at least one video stream and one audio stream
	if len(resp.VideoStreams) == 0 || len(resp.AudioStreams) == 0 {
		return false
	}

	return true
}

// extractFromAPI performs the actual extraction from specified API with optional proxy
func extractFromAPI(videoID string, apiBase string, proxy string, premium bool) (*models.ExtractResponse, error) {
	// Build API URL
	apiURL := fmt.Sprintf("%s/%s", apiBase, videoID)
	params := url.Values{}
	if proxy != "" {
		params.Set("proxy", proxy)
	}
	if premium {
		params.Set("premium", "1")
	}
	if len(params) > 0 {
		apiURL = fmt.Sprintf("%s?%s", apiURL, params.Encode())
	}

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

// getQualityDimension returns the dimension used for quality matching
// Uses the shorter dimension (min of width and height) which is the industry standard
// Examples:
//   - Landscape 1920×1080 → 1080 (shorter edge) → "1080p"
//   - Vertical 1080×1920 → 1080 (shorter edge) → "1080p"
func getQualityDimension(stream *models.Stream) int {
	// Handle edge cases where one dimension is missing
	if stream.Width == 0 {
		return stream.Height
	}
	if stream.Height == 0 {
		return stream.Width
	}
	// Return the shorter dimension
	if stream.Width < stream.Height {
		return stream.Width
	}
	return stream.Height
}

// SelectVideo selects the best video stream based on quality, device, and target format
func SelectVideo(data *models.ExtractResponse, requestedQuality string, osType string, targetFormat string) *models.VideoSelectionResult {
	result := &models.VideoSelectionResult{}

	// Get device profile
	profile, ok := config.DeviceProfiles[osType]
	if !ok {
		profile = config.DefaultProfile
	}

	// Filter streams by supported codecs
	var compatibleStreams []models.Stream
	for _, stream := range data.VideoStreams {
		codec := GetStreamCodec(&stream)
		if isCodecSupported(codec, profile.VideoCodecs) {
			compatibleStreams = append(compatibleStreams, stream)
		}
	}

	if len(compatibleStreams) == 0 {
		return result
	}

	// Sort by quality dimension (descending), then format compatibility, then bitrate (descending)
	sort.Slice(compatibleStreams, func(i, j int) bool {
		// 1. Quality dimension priority (higher is better)
		// Use shorter edge for quality comparison to handle both landscape and vertical videos
		dimI := getQualityDimension(&compatibleStreams[i])
		dimJ := getQualityDimension(&compatibleStreams[j])
		if dimI != dimJ {
			return dimI > dimJ
		}

		// 2. Format compatibility priority (compatible codecs preferred)
		codecI := GetStreamCodec(&compatibleStreams[i])
		codecJ := GetStreamCodec(&compatibleStreams[j])
		compatI := isVideoCodecCompatible(codecI, targetFormat)
		compatJ := isVideoCodecCompatible(codecJ, targetFormat)

		if compatI != compatJ {
			return compatI // true comes first
		}

		// 3. Bitrate priority (higher is better)
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
		// Try exact match first using quality dimension
		for i := range compatibleStreams {
			streamDim := getQualityDimension(&compatibleStreams[i])
			if streamDim == requestedHeight {
				selectedStream = &compatibleStreams[i]
				break
			}
		}

		// If no exact match, find closest lower quality
		if selectedStream == nil {
			for i := range compatibleStreams {
				streamDim := getQualityDimension(&compatibleStreams[i])
				if streamDim < requestedHeight {
					selectedStream = &compatibleStreams[i]
					result.QualityChanged = true
					result.QualityChangeReason = fmt.Sprintf("Requested %s not available, using %s",
						requestedQuality, config.HeightToQuality[streamDim])
					break
				}
			}
		}

		// If still no match, use highest available
		if selectedStream == nil && len(compatibleStreams) > 0 {
			selectedStream = &compatibleStreams[0]
			streamDim := getQualityDimension(selectedStream)
			result.QualityChanged = true
			result.QualityChangeReason = fmt.Sprintf("Using highest available: %s",
				config.HeightToQuality[streamDim])
		}
	} else {
		// No quality specified, use highest within device limits
		for i := range compatibleStreams {
			streamDim := getQualityDimension(&compatibleStreams[i])
			if streamDim <= maxHeight {
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
		streamDim := getQualityDimension(selectedStream)
		result.SelectedQuality = config.HeightToQuality[streamDim]
		if result.SelectedQuality == "" {
			result.SelectedQuality = fmt.Sprintf("%dp", streamDim)
		}

		// Set NeedsReencode flag based on codec compatibility
		videoCodec := GetStreamCodec(selectedStream)
		result.NeedsReencode = !isVideoCodecCompatible(videoCodec, targetFormat)
	}

	return result
}

// SelectAudio selects the best audio stream based on device, track, and target format
// Returns AudioSelectionResult with language validation and fallback information
func SelectAudio(data *models.ExtractResponse, trackID string, osType string, targetFormat string) *models.AudioSelectionResult {
	result := &models.AudioSelectionResult{
		AvailableAudioLanguages: data.AvailableAudioLanguages,
	}

	// Get device profile
	profile, ok := config.DeviceProfiles[osType]
	if !ok {
		profile = config.DefaultProfile
	}

	// Filter streams by supported codecs
	var compatibleStreams []models.Stream
	for _, stream := range data.AudioStreams {
		codec := GetStreamCodec(&stream)
		if isCodecSupported(codec, profile.AudioCodecs) {
			compatibleStreams = append(compatibleStreams, stream)
		}
	}

	if len(compatibleStreams) == 0 {
		return result
	}

	// Validate requested trackID (language) against available languages
	requestedLangAvailable := false
	if trackID != "" {
		fmt.Printf("[SelectAudio] Requested trackID: %s\n", trackID)
		fmt.Printf("[SelectAudio] Available languages: %v\n", data.AvailableAudioLanguages)
		for _, lang := range data.AvailableAudioLanguages {
			if lang == trackID {
				requestedLangAvailable = true
				break
			}
		}
		fmt.Printf("[SelectAudio] Language available: %v\n", requestedLangAvailable)
	}

	// Filter by track ID if specified and available
	if trackID != "" && requestedLangAvailable {
		var filtered []models.Stream
		fmt.Printf("[SelectAudio] Filtering %d compatible streams by trackID=%s\n", len(compatibleStreams), trackID)
		for i, stream := range compatibleStreams {
			fmt.Printf("[SelectAudio] Stream %d: AudioTrackID=%s, IsOriginal=%v, Codec=%s\n",
				i, stream.AudioTrackID, stream.IsOriginal, GetStreamCodec(&stream))
			if stream.AudioTrackID == trackID {
				filtered = append(filtered, stream)
				fmt.Printf("[SelectAudio] ✓ Matched stream %d\n", i)
			}
		}
		fmt.Printf("[SelectAudio] Filtered count: %d\n", len(filtered))
		if len(filtered) > 0 {
			compatibleStreams = filtered
		} else {
			// No streams matched - this shouldn't happen if language is available
			fmt.Printf("[SelectAudio] WARNING: No streams matched trackID=%s even though language is available!\n", trackID)
		}
	} else if trackID != "" && !requestedLangAvailable {
		// Requested language not available - fall back to original
		result.AudioLanguageChanged = true
		result.AudioLanguageChangeReason = fmt.Sprintf("Requested language '%s' not available, using default language", trackID)
		fmt.Printf("[SelectAudio] Language not available, falling back to original\n")

		// Find original audio track
		var originals []models.Stream
		for _, stream := range compatibleStreams {
			if stream.IsOriginal {
				originals = append(originals, stream)
			}
		}
		if len(originals) > 0 {
			compatibleStreams = originals
		}
	} else {
		// No trackID specified - prefer original audio track
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

	// Sort by format compatibility first, then bitrate (higher is better)
	// This prioritizes codecs that don't need transcoding for the target format
	sort.Slice(compatibleStreams, func(i, j int) bool {
		codecI := GetStreamCodec(&compatibleStreams[i])
		codecJ := GetStreamCodec(&compatibleStreams[j])

		// 1. Format compatibility priority (compatible codecs preferred)
		compatI := IsAudioCodecCompatible(codecI, targetFormat)
		compatJ := IsAudioCodecCompatible(codecJ, targetFormat)

		if compatI != compatJ {
			return compatI // true comes first
		}

		// 2. Bitrate priority (higher is better)
		return compatibleStreams[i].Bitrate > compatibleStreams[j].Bitrate
	})

	fmt.Printf("[SelectAudio] After sorting, top streams:\n")
	for i := 0; i < len(compatibleStreams) && i < 5; i++ {
		fmt.Printf("[SelectAudio]   %d: AudioTrackID=%s, Codec=%s, Bitrate=%.0f, IsOriginal=%v\n",
			i, compatibleStreams[i].AudioTrackID, GetStreamCodec(&compatibleStreams[i]),
			compatibleStreams[i].Bitrate, compatibleStreams[i].IsOriginal)
	}

	if len(compatibleStreams) > 0 {
		result.Stream = &compatibleStreams[0]
		fmt.Printf("[SelectAudio] ✓ FINAL SELECTED: AudioTrackID=%s, Codec=%s, Bitrate=%.0f\n",
			result.Stream.AudioTrackID, GetStreamCodec(result.Stream), result.Stream.Bitrate)
	}

	return result
}

// GetStreamCodec returns the codec from Stream, preferring Codec field over mimeType extraction
func GetStreamCodec(stream *models.Stream) string {
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

// isVideoCodecCompatible checks if video codec is compatible with target format
func isVideoCodecCompatible(videoCodec string, targetFormat string) bool {
	switch targetFormat {
	case "mp4":
		// MP4 supports H.264/H.265/VP9/AV1 video
		// Modern players (2016+) support VP9 and AV1 in MP4 container
		// This avoids extremely heavy re-encoding for 4K videos
		return slices.Contains([]string{
			"avc1",         // H.264
			"hvc1", "hev1", // H.265
			"vp09", "vp9", // VP9 (widely supported in modern browsers/players)
			"av01", // AV1 (modern codec, supported 2018+)
		}, videoCodec)
	case "webm":
		// WebM supports VP8/VP9/AV1 video
		return slices.Contains([]string{"vp8", "vp9", "vp09", "av01"}, videoCodec)
	case "mkv":
		// MKV supports almost everything
		return true
	default:
		return false
	}
}

// IsAudioCodecCompatible checks if audio codec is compatible with target format
func IsAudioCodecCompatible(audioCodec string, targetFormat string) bool {
	switch targetFormat {
	case "mp4", "m4a":
		// MP4/M4A support AAC audio
		return strings.HasPrefix(audioCodec, "mp4a") // AAC codec
	case "webm":
		// WebM supports Opus and Vorbis
		return audioCodec == "opus" || audioCodec == "vorbis"
	case "mkv":
		// MKV supports almost everything
		return true
	case "opus":
		// Opus format expects Opus codec
		return audioCodec == "opus"
	case "ogg":
		// OGG typically uses Vorbis or Opus
		return audioCodec == "vorbis" || audioCodec == "opus"
	case "mp3", "wav", "flac":
		// These formats require transcoding from any source
		return false
	default:
		return false
	}
}

// GetExtension returns file extension for a stream
func GetExtension(stream *models.Stream) string {
	return utils.GetExtFromMimeType(stream.MimeType)
}

// FindEquivalentVideoStream finds a stream in the new list that matches the target stream's properties
func FindEquivalentVideoStream(target *models.Stream, streams []models.Stream) *models.Stream {
	if target == nil {
		return nil
	}

	for i := range streams {
		s := &streams[i]
		// Match resolution and mime-type base
		if s.Width == target.Width && s.Height == target.Height && isSameMimeType(s.MimeType, target.MimeType) {
			return s
		}
	}
	return nil
}

// FindEquivalentAudioStream finds a stream in the new list that matches the target stream's properties
func FindEquivalentAudioStream(target *models.Stream, streams []models.Stream) *models.Stream {
	if target == nil {
		return nil
	}

	// Check AudioTrackID first
	if target.AudioTrackID != "" {
		for i := range streams {
			if streams[i].AudioTrackID == target.AudioTrackID {
				return &streams[i]
			}
		}
	}

	// Check IsOriginal + MimeType
	if target.IsOriginal {
		for i := range streams {
			s := &streams[i]
			if s.IsOriginal && isSameMimeType(s.MimeType, target.MimeType) {
				return s
			}
		}
	}

	// Fallback to closest bitrate with same mime type
	for i := range streams {
		s := &streams[i]
		if isSameMimeType(s.MimeType, target.MimeType) {
			// Naive fallback: return first matching mime type
			// Ideally we could match bitrate but this is usually sufficient for same-format refreshes
			return s
		}
	}

	return nil
}

func isSameMimeType(a, b string) bool {
	// "video/mp4; codecs=..." vs "video/mp4"
	return strings.Split(a, ";")[0] == strings.Split(b, ";")[0]
}
