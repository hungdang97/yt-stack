package main

import (
	"encoding/json"
	"strings"
)

type ytJSON3 struct {
	Events []ytEvent `json:"events"`
}

type ytEvent struct {
	TStartMs    *int64  `json:"tStartMs"`
	DDurationMs *int64  `json:"dDurationMs"`
	Segs        []ytSeg `json:"segs"`
	AAppend     *int    `json:"aAppend"`
	WWinID      *int    `json:"wWinId"`
}

type ytSeg struct {
	UTF8      string `json:"utf8"`
	TOffsetMs *int64 `json:"tOffsetMs"`
	AcAsrConf *int   `json:"acAsrConf"`
}

func ParseYouTubeJSON3(data []byte) (*Canonical, error) {
	var raw ytJSON3
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	words := make([]Word, 0, 256)

	for _, ev := range raw.Events {
		if ev.TStartMs == nil || len(ev.Segs) == 0 {
			continue
		}
		hasContent := false
		for _, s := range ev.Segs {
			if strings.TrimSpace(s.UTF8) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		eventStart := float64(*ev.TStartMs) / 1000.0

		for _, seg := range ev.Segs {
			text := strings.TrimSpace(seg.UTF8)
			if text == "" || text == "\n" {
				continue
			}
			offset := int64(0)
			if seg.TOffsetMs != nil {
				offset = *seg.TOffsetMs
			}
			wordStart := eventStart + float64(offset)/1000.0

			var conf *float64
			if seg.AcAsrConf != nil {
				v := float64(*seg.AcAsrConf) / 255.0
				conf = &v
			}

			if isBracketTag(text) {
				words = append(words, Word{Text: text, Start: wordStart, Confidence: conf})
				continue
			}

			parts := strings.Fields(text)
			for j, p := range parts {
				words = append(words, Word{
					Text:       p,
					Start:      wordStart + float64(j)*0.01,
					Confidence: conf,
				})
			}
		}
	}

	for i := range words {
		if i+1 < len(words) {
			words[i].End = words[i+1].Start
		} else {
			words[i].End = words[i].Start + 0.5
		}
	}

	return &Canonical{
		Version: "1.0",
		Source:  SourceMeta{Type: "youtube", RawFormat: "json3"},
		Words:   words,
	}, nil
}

func isBracketTag(s string) bool {
	if len(s) < 2 {
		return false
	}
	first, last := s[0], s[len(s)-1]
	return (first == '[' && last == ']') ||
		(first == '(' && last == ')') ||
		(strings.HasPrefix(s, "♪") && strings.HasSuffix(s, "♪"))
}
