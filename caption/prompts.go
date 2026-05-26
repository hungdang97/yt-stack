package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TranslateBatch: 1-pass with surrounding context (rockbenben style).
func TranslateBatch(llm *LLMClient, sources []string, contextBefore, contextAfter []string,
	srcLang, tgtLang string, maxCharsPerLine int,
) ([]string, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	system := fmt.Sprintf(
		"You are a professional subtitle translator translating from %s to %s. "+
			"Use natural, idiomatic %s — never literal/machine-style. "+
			"CRITICAL: output MUST be a JSON array of strings with EXACTLY the same count as input. "+
			"Never merge, split, skip, or add lines. Never add commentary or markdown fences.",
		srcLang, tgtLang, tgtLang)

	var sb strings.Builder
	if len(contextBefore) > 0 {
		sb.WriteString("CONTEXT (preceding lines, do NOT translate, only for reference):\n")
		for _, c := range contextBefore {
			sb.WriteString("  - " + c + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("TRANSLATE these %d lines from %s to %s.\n",
		len(sources), srcLang, tgtLang))
	sb.WriteString(fmt.Sprintf("OUTPUT REQUIREMENT: JSON array of EXACTLY %d strings (1-to-1 mapping with input).\n",
		len(sources)))
	if maxCharsPerLine > 0 {
		sb.WriteString(fmt.Sprintf(
			"HARD CONSTRAINT: each translated line MUST be ≤ %d characters. Compress meaning if needed.\n",
			maxCharsPerLine))
	}
	sb.WriteString("Preserve meaning, tone, idiom.\n")
	sb.WriteString("Return ONLY a JSON array of strings (no markdown, no extra keys, no merging).\n\n")
	sb.WriteString(fmt.Sprintf("Input (%d items):\n", len(sources)))
	indexed, _ := json.Marshal(sources)
	sb.Write(indexed)

	if len(contextAfter) > 0 {
		sb.WriteString("\n\nCONTEXT (following lines, do NOT translate):\n")
		for _, c := range contextAfter {
			sb.WriteString("  - " + c + "\n")
		}
	}

	return parseJSONArrayResponse(llm, system, sb.String(), len(sources))
}

// TranslateThreePass: VideoLingo-style Translate-Reflect-Adapt.
// Pass 1: literal. Pass 2: reflect for naturalness. Pass 3: adapt to char constraint.
// Falls back to previous pass output on failure.
func TranslateThreePass(llm *LLMClient, sources []string, contextBefore, contextAfter []string,
	srcLang, tgtLang string, maxCharsPerLine int,
) ([]string, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	// Pass 1: literal translate
	literal, err := TranslateBatch(llm, sources, contextBefore, contextAfter, srcLang, tgtLang, 0)
	if err != nil {
		return nil, fmt.Errorf("pass1 literal: %v", err)
	}

	// Pass 2: reflect — natural & idiomatic
	reflected, err := reflectPass(llm, sources, literal, srcLang, tgtLang)
	if err != nil {
		// Fallback to literal
		reflected = literal
	}

	// Pass 3: adapt to constraint (only if constraint provided)
	if maxCharsPerLine > 0 {
		adapted, err := adaptPass(llm, sources, reflected, tgtLang, maxCharsPerLine)
		if err != nil {
			return reflected, nil
		}
		return adapted, nil
	}
	return reflected, nil
}

func reflectPass(llm *LLMClient, sources, literal []string, srcLang, tgtLang string) ([]string, error) {
	system := fmt.Sprintf(
		"You are a translation reviewer. Improve naturalness and idiom for %s subtitle output. "+
			"Return JSON array only.", tgtLang)

	type pair struct {
		Source  string `json:"source"`
		Literal string `json:"literal"`
	}
	pairs := make([]pair, len(sources))
	for i := range sources {
		pairs[i] = pair{Source: sources[i], Literal: literal[i]}
	}
	pairsJSON, _ := json.Marshal(pairs)

	user := fmt.Sprintf(
		"For each pair below, review the literal translation and rewrite it to sound natural "+
			"to a native %s speaker. Fix stiff phrasing. Preserve meaning. Keep concise.\n\n"+
			"Return JSON array of EXACTLY %d strings (the improved translations in same order).\n\n"+
			"Pairs (source → literal translation):\n%s",
		tgtLang, len(sources), string(pairsJSON))

	return parseJSONArrayResponse(llm, system, user, len(sources))
}

func adaptPass(llm *LLMClient, sources, reflected []string, tgtLang string, maxCharsPerLine int) ([]string, error) {
	system := fmt.Sprintf(
		"You are a subtitle compressor. Adapt %s translations to fit subtitle character limit. "+
			"Return JSON array only.", tgtLang)

	type pair struct {
		Source      string `json:"source"`
		Translation string `json:"translation"`
		CharCount   int    `json:"char_count"`
	}
	pairs := make([]pair, len(sources))
	for i := range sources {
		pairs[i] = pair{
			Source:      sources[i],
			Translation: reflected[i],
			CharCount:   runeLen(reflected[i]),
		}
	}
	pairsJSON, _ := json.Marshal(pairs)

	user := fmt.Sprintf(
		"Compress each translation below to ≤ %d characters while keeping meaning intact. "+
			"If already within limit, return AS-IS unchanged.\n\n"+
			"Return JSON array of EXACTLY %d strings (in same order).\n\n"+
			"Input:\n%s",
		maxCharsPerLine, len(sources), string(pairsJSON))

	return parseJSONArrayResponse(llm, system, user, len(sources))
}

// CompressLine: fallback single-line compress when batch adapt fails.
func CompressLine(llm *LLMClient, translation, sourceContext string, maxChars int) (string, error) {
	system := "You are a subtitle compressor. Shorten translations to fit screen while preserving meaning."
	user := fmt.Sprintf(
		"The translation below is too long for a subtitle (must be ≤ %d characters).\n\n"+
			"Source: %q\n"+
			"Current translation (%d chars): %q\n\n"+
			"Rewrite to ≤ %d characters. Keep core meaning. "+
			"Return ONLY the compressed text, no quotes, no explanation.",
		maxChars, sourceContext, runeLen(translation), translation, maxChars)

	resp, err := llm.Complete(system, user)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(resp)
	out = strings.Trim(out, `"'`)
	return out, nil
}

// parseJSONArrayResponse: call LLM, parse strict JSON array of N strings.
// On count mismatch, pads/truncates with empty strings (caller may fall back).
func parseJSONArrayResponse(llm *LLMClient, system, user string, expectedLen int) ([]string, error) {
	resp, err := llm.Complete(system, user)
	if err != nil {
		return nil, err
	}

	r := strings.TrimSpace(resp)
	r = strings.TrimPrefix(r, "```json")
	r = strings.TrimPrefix(r, "```")
	r = strings.TrimSuffix(r, "```")
	r = strings.TrimSpace(r)

	var result []string
	if err := json.Unmarshal([]byte(r), &result); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %v\nresponse: %.200s", err, r)
	}
	if len(result) != expectedLen {
		return nil, fmt.Errorf("returned %d items, expected %d", len(result), expectedLen)
	}
	return result, nil
}

// translateWithMode picks 1-pass or 3-pass based on env var CAPTION_TRANSLATE_MODE.
func translateWithMode(llm *LLMClient, sources []string, contextBefore, contextAfter []string,
	srcLang, tgtLang string, maxCharsPerLine int,
) ([]string, error) {
	mode := os.Getenv("CAPTION_TRANSLATE_MODE")
	if mode == "3pass" {
		return TranslateThreePass(llm, sources, contextBefore, contextAfter, srcLang, tgtLang, maxCharsPerLine)
	}
	return TranslateBatch(llm, sources, contextBefore, contextAfter, srcLang, tgtLang, maxCharsPerLine)
}
