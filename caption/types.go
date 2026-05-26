package main

import "encoding/json"

// ===== B1 canonical (input to B2) =====

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

// ===== B2 cue list (output) =====

type CueList struct {
	Version     string  `json:"version"`
	Mode        string  `json:"mode"` // "source_only" | "translation_only" | "dual"
	SourceLang  string  `json:"source_lang"`
	TargetLang  string  `json:"target_lang,omitempty"`
	DurationSec float64 `json:"duration_sec"`
	Cues        []Cue   `json:"cues"`
	Stats       Stats   `json:"stats"`
}

type Cue struct {
	Idx             int     `json:"idx"`
	Start           float64 `json:"start"`
	End             float64 `json:"end"`
	Lines           []Line  `json:"lines"`
	Kind            string  `json:"kind"` // "speech" | "effect" | "music"
	Speaker         string  `json:"speaker,omitempty"`
	Quality         Quality `json:"quality"`
	SourceWordRange [2]int  `json:"source_word_range"`
}

type Line struct {
	Role      string  `json:"role"` // "source" | "translation"
	Lang      string  `json:"lang"`
	Text      string  `json:"text"`
	CharCount int     `json:"char_count"`
	CPS       float64 `json:"cps"`
}

type Quality struct {
	BoundaryScore  int      `json:"boundary_score"`
	CPSWithinLimit bool     `json:"cps_within_limit"`
	CompressedFrom string   `json:"compressed_from,omitempty"`
	Flags          []string `json:"flags"`
	NeedsReview    bool     `json:"needs_review"`
}

type Stats struct {
	TotalCues        int     `json:"total_cues"`
	AvgCPS           float64 `json:"avg_cps"`
	AvgDurationSec   float64 `json:"avg_duration_sec"`
	MaxCharsPerLine  int     `json:"max_chars_per_line"`
	CuesOverCPSLimit int     `json:"cues_over_cps_limit"`
	CuesNeedReview   int     `json:"cues_need_review"`
	LLMCallsMade     int     `json:"llm_calls_made,omitempty"`
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
	CharWeight      float64 `json:"char_weight"` // 1.0 Latin, 1.75 CJK
}

// ===== HTTP request =====

type CaptionRequest struct {
	Mode       string          `json:"mode"`
	SourceType string          `json:"source_type"`
	SourceData json.RawMessage `json:"source_data"`
	SourceLang string          `json:"source_lang"`
	TargetLang string          `json:"target_lang,omitempty"`
	Rules      *Rules          `json:"rules,omitempty"`
}
