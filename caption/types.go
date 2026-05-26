package main

import "encoding/json"

// ===== B1 canonical (input to chunker) =====

type Canonical struct {
	Version     string     `json:"version"`
	Source      SourceMeta `json:"source"`
	DurationSec float64    `json:"duration_sec,omitempty"`
	Words       []Word     `json:"words"`
	Warnings    []Warning  `json:"warnings,omitempty"`
}

type SourceMeta struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	RawFormat string `json:"raw_format"`
}

type Word struct {
	Idx        int      `json:"idx"`
	Text       string   `json:"text"`
	Start      float64  `json:"start"`
	End        float64  `json:"end"`
	Confidence *float64 `json:"confidence,omitempty"`
	Speaker    string   `json:"speaker,omitempty"`
}

type Warning struct {
	Type  string `json:"type"`
	Count int    `json:"count,omitempty"`
	Note  string `json:"note,omitempty"`
}

// ===== Output: utterance format (compatible with translate/edge-tts/render pipeline) =====

type Transcript struct {
	Language   string      `json:"language"`
	Duration   float64     `json:"duration"`
	Utterances []Utterance `json:"utterances"`
}

type Utterance struct {
	Start float64   `json:"start"`
	End   float64   `json:"end"`
	Text  string    `json:"text"`
	Words []UttWord `json:"words"`
}

type UttWord struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// ===== Rules (per-language, override-able) =====

type Rules struct {
	MaxCharsPerLine int     `json:"max_chars_per_line"`
	MaxLines        int     `json:"max_lines"`
	MaxCPS          float64 `json:"max_cps"`
	MinDurationSec  float64 `json:"min_duration_sec"`
	MaxDurationSec  float64 `json:"max_duration_sec"`
	MinWordsPerCue  int     `json:"min_words_per_cue"`
	SoftPauseSec    float64 `json:"soft_pause_sec"`
	HardPauseSec    float64 `json:"hard_pause_sec"`
	CharWeight      float64 `json:"char_weight"`
}

// ===== HTTP request =====

type CaptionRequest struct {
	SourceType string          `json:"source_type"`
	SourceData json.RawMessage `json:"source_data"`
	SourceLang string          `json:"source_lang"`
	Rules      *Rules          `json:"rules,omitempty"`
}
