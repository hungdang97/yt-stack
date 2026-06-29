package models

import (
	"strconv"
	"strings"
)

// FbVideoOption is one selectable video quality (for the /api/info contract).
type FbVideoOption struct {
	Quality   string
	SizeBytes int64
}

// heightFromQuality parses a quality label like "720p" into 720. Returns 0 when
// the label has no numeric height (e.g. Facebook progressive "sd"/"hd").
func heightFromQuality(quality string) int {
	q := strings.ToLower(strings.TrimSpace(quality))
	q = strings.TrimSuffix(q, "p")
	if n, err := strconv.Atoi(q); err == nil {
		return n
	}
	return 0
}

// VideoQualityOptions returns the distinct video qualities, best-first. Streams
// from the extractor are already sorted best-first, so the first occurrence of
// each quality label wins (dedupes multiple codecs at the same resolution).
// SizeBytes estimates the merged output: video stream size plus the best audio
// size for video-only (DASH) streams (progressive streams already include audio).
func (r *FbExtractResponse) VideoQualityOptions() []FbVideoOption {
	// Best (largest) audio size, shared across qualities for the merge estimate.
	var audioSize int64
	for i := range r.AudioStreams {
		if r.AudioStreams[i].FileSize > audioSize {
			audioSize = r.AudioStreams[i].FileSize
		}
	}

	seen := make(map[string]bool)
	out := make([]FbVideoOption, 0, len(r.VideoStreams))
	for i := range r.VideoStreams {
		s := r.VideoStreams[i]
		if s.URL == "" || s.Quality == "" {
			continue
		}
		if seen[s.Quality] {
			continue
		}
		seen[s.Quality] = true
		size := s.FileSize
		if size > 0 && audioSize > 0 && s.VideoOnly {
			size += audioSize
		}
		out = append(out, FbVideoOption{Quality: s.Quality, SizeBytes: size})
	}
	return out
}

// SelectVideoStream picks the video stream for a requested quality and OS.
//
//   - iOS/macOS prefer a progressive (video+audio) stream so playback works
//     without a separate audio merge; falls back to any stream if none exist.
//   - Exact quality match wins; otherwise the closest lower resolution; finally
//     the best available stream.
//
// Returns nil when no videoStreams are present (caller should use the legacy
// media[] URLs).
func (r *FbExtractResponse) SelectVideoStream(quality, os string) *FbStream {
	if len(r.VideoStreams) == 0 {
		return nil
	}

	// Build candidate pool (best-first order is preserved from the extractor).
	pool := make([]*FbStream, 0, len(r.VideoStreams))
	if os == "ios" || os == "macos" {
		for i := range r.VideoStreams {
			if !r.VideoStreams[i].VideoOnly && r.VideoStreams[i].URL != "" {
				pool = append(pool, &r.VideoStreams[i])
			}
		}
	}
	if len(pool) == 0 {
		for i := range r.VideoStreams {
			if r.VideoStreams[i].URL != "" {
				pool = append(pool, &r.VideoStreams[i])
			}
		}
	}
	if len(pool) == 0 {
		return nil
	}

	if quality != "" {
		// 1) exact label match
		for _, s := range pool {
			if strings.EqualFold(s.Quality, quality) {
				return s
			}
		}
		// 2) closest lower resolution (pool is sorted high -> low)
		if reqH := heightFromQuality(quality); reqH > 0 {
			for _, s := range pool {
				if s.Height > 0 && s.Height <= reqH {
					return s
				}
			}
		}
	}

	// 3) best available
	return pool[0]
}
