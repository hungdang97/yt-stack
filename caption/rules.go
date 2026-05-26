package main

// defaultRulesByLang: starting presets per language. Override via API request.
var defaultRulesByLang = map[string]Rules{
	"en": {MaxCharsPerLine: 42, MaxLines: 2, MaxCPS: 17, MinDurationSec: 1.0, MaxDurationSec: 7.0,
		MinWordsPerCue: 2, SoftPauseSec: 0.3, HardPauseSec: 1.0, CharWeight: 1.0},
	"vi": {MaxCharsPerLine: 42, MaxLines: 2, MaxCPS: 17, MinDurationSec: 1.0, MaxDurationSec: 7.0,
		MinWordsPerCue: 2, SoftPauseSec: 0.3, HardPauseSec: 1.0, CharWeight: 1.0},
	"zh": {MaxCharsPerLine: 16, MaxLines: 2, MaxCPS: 9, MinDurationSec: 1.0, MaxDurationSec: 7.0,
		MinWordsPerCue: 2, SoftPauseSec: 0.3, HardPauseSec: 1.0, CharWeight: 1.75},
	"ja": {MaxCharsPerLine: 16, MaxLines: 2, MaxCPS: 9, MinDurationSec: 1.0, MaxDurationSec: 7.0,
		MinWordsPerCue: 2, SoftPauseSec: 0.3, HardPauseSec: 1.0, CharWeight: 1.75},
	"ko": {MaxCharsPerLine: 20, MaxLines: 2, MaxCPS: 12, MinDurationSec: 1.0, MaxDurationSec: 7.0,
		MinWordsPerCue: 2, SoftPauseSec: 0.3, HardPauseSec: 1.0, CharWeight: 1.5},
	"th": {MaxCharsPerLine: 35, MaxLines: 2, MaxCPS: 12, MinDurationSec: 1.0, MaxDurationSec: 7.0,
		MinWordsPerCue: 2, SoftPauseSec: 0.3, HardPauseSec: 1.0, CharWeight: 1.0},
}

func GetRules(lang string, override *Rules) Rules {
	base, ok := defaultRulesByLang[lang]
	if !ok {
		base = defaultRulesByLang["en"]
	}
	if override == nil {
		return base
	}
	if override.MaxCharsPerLine > 0 {
		base.MaxCharsPerLine = override.MaxCharsPerLine
	}
	if override.MaxLines > 0 {
		base.MaxLines = override.MaxLines
	}
	if override.MaxCPS > 0 {
		base.MaxCPS = override.MaxCPS
	}
	if override.MinDurationSec > 0 {
		base.MinDurationSec = override.MinDurationSec
	}
	if override.MaxDurationSec > 0 {
		base.MaxDurationSec = override.MaxDurationSec
	}
	if override.MinWordsPerCue > 0 {
		base.MinWordsPerCue = override.MinWordsPerCue
	}
	if override.SoftPauseSec > 0 {
		base.SoftPauseSec = override.SoftPauseSec
	}
	if override.HardPauseSec > 0 {
		base.HardPauseSec = override.HardPauseSec
	}
	if override.CharWeight > 0 {
		base.CharWeight = override.CharWeight
	}
	return base
}
