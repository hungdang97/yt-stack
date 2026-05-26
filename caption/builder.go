package main

import (
	"fmt"
	"strings"
)

const (
	translateBatchSize   = 20 // cues/sentences per LLM call
	translateContextSize = 10 // surrounding lines passed as context
)

// ============================================================
// 3 builder functions (1 per mode)
// ============================================================

// BuildSourceOnly creates a cue list containing only the source language.
// Algorithm: greedy chunker on words → 1 line per cue, no LLM.
// Mode = "source_only" (was TH1).
func BuildSourceOnly(canonical *Canonical, rules Rules, sourceLang string) *CueList {
	bounds := chunkWords(canonical.Words, rules)
	cues := make([]Cue, 0, len(bounds))

	for i, b := range bounds {
		chunk := canonical.Words[b[0]:b[1]]
		text := wrapToLines(joinWords(chunk), rules.MaxCharsPerLine, rules.MaxLines)

		cue := Cue{
			Idx:   i,
			Start: chunk[0].Start,
			End:   chunk[len(chunk)-1].End,
			Lines: []Line{
				{
					Role: "source", Lang: sourceLang, Text: text,
					CharCount: runeLen(stripNewlines(text)),
					CPS:       cpsForChunk(text, chunk),
				},
			},
			Kind:            classifyKind(chunk),
			Speaker:         dominantSpeaker(chunk),
			SourceWordRange: [2]int{b[0], b[1]},
		}
		cue.Quality = computeQuality(cue, rules, canonical.Words, b[1]-1)
		cues = append(cues, cue)
	}

	return &CueList{
		Version:     "1.0",
		Mode:        "source_only",
		SourceLang:  sourceLang,
		DurationSec: canonical.DurationSec,
		Cues:        cues,
		Stats:       computeStats(cues, rules),
	}
}

// BuildTranslationOnly creates a cue list with only the target-language translation.
// Algorithm: detect sentences → batch translate with context → re-chunk by translation chars.
// Mode = "translation_only" (was TH2). Requires LLM.
func BuildTranslationOnly(canonical *Canonical, rules Rules, llm *LLMClient,
	sourceLang, targetLang string,
) (*CueList, error) {
	sentences := detectSentences(canonical.Words)
	if len(sentences) == 0 {
		return &CueList{Version: "1.0", Mode: "translation_only",
			SourceLang: sourceLang, TargetLang: targetLang,
			DurationSec: canonical.DurationSec, Cues: []Cue{}}, nil
	}

	srcTexts := make([]string, len(sentences))
	for i, s := range sentences {
		srcTexts[i] = joinWords(canonical.Words[s[0]:s[1]])
	}

	translations, err := batchTranslateWithContext(llm, srcTexts, sourceLang, targetLang, 0)
	if err != nil {
		return nil, fmt.Errorf("translate: %v", err)
	}

	cues := []Cue{}
	visualLimit := rules.MaxCharsPerLine * rules.MaxLines

	for i, s := range sentences {
		chunk := canonical.Words[s[0]:s[1]]
		sentStart := chunk[0].Start
		sentEnd := chunk[len(chunk)-1].End
		translation := translations[i]

		parts := splitByLength(translation, visualLimit)
		for j, part := range parts {
			t0 := sentStart + (sentEnd-sentStart)*float64(j)/float64(len(parts))
			t1 := sentStart + (sentEnd-sentStart)*float64(j+1)/float64(len(parts))
			text := wrapToLines(part, rules.MaxCharsPerLine, rules.MaxLines)

			cue := Cue{
				Idx:   len(cues),
				Start: t0, End: t1,
				Lines: []Line{
					{
						Role: "translation", Lang: targetLang, Text: text,
						CharCount: runeLen(stripNewlines(text)),
						CPS:       cpsForDuration(text, t1-t0),
					},
				},
				Kind:            "speech",
				Speaker:         dominantSpeaker(chunk),
				SourceWordRange: [2]int{s[0], s[1]},
			}
			cue.Quality = computeQuality(cue, rules, canonical.Words, s[1]-1)
			cues = append(cues, cue)
		}
	}

	stats := computeStats(cues, rules)
	stats.LLMCallsMade = llm.Calls()

	return &CueList{
		Version:     "1.0",
		Mode:        "translation_only",
		SourceLang:  sourceLang,
		TargetLang:  targetLang,
		DurationSec: canonical.DurationSec,
		Cues:        cues,
		Stats:       stats,
	}, nil
}

// BuildDual creates a cue list with source + translation stacked, sharing timing.
// Algorithm: chunker on source → per-cue translate with constraint → compress fallback.
// Mode = "dual" (was TH3). Requires LLM.
func BuildDual(canonical *Canonical, rules Rules, llm *LLMClient,
	sourceLang, targetLang string,
) (*CueList, error) {
	bounds := chunkWords(canonical.Words, rules)
	if len(bounds) == 0 {
		return &CueList{Version: "1.0", Mode: "dual",
			SourceLang: sourceLang, TargetLang: targetLang,
			DurationSec: canonical.DurationSec, Cues: []Cue{}}, nil
	}

	srcTexts := make([]string, len(bounds))
	for i, b := range bounds {
		srcTexts[i] = joinWords(canonical.Words[b[0]:b[1]])
	}

	visualLimit := rules.MaxCharsPerLine * rules.MaxLines

	// Skip translation for effect-only cues (e.g. "[Music]")
	toTranslate := []string{}
	toTranslateIdx := []int{}
	for i, txt := range srcTexts {
		chunk := canonical.Words[bounds[i][0]:bounds[i][1]]
		if classifyKind(chunk) != "speech" {
			continue
		}
		toTranslate = append(toTranslate, txt)
		toTranslateIdx = append(toTranslateIdx, i)
	}

	translatedSpeech, err := batchTranslateWithContext(llm, toTranslate, sourceLang, targetLang, visualLimit)
	if err != nil {
		return nil, fmt.Errorf("translate: %v", err)
	}

	// Compress any translation still overflowing (per-cue fallback)
	for k, t := range translatedSpeech {
		if runeLen(t) > visualLimit {
			compressed, cerr := CompressLine(llm, t, toTranslate[k], visualLimit)
			if cerr == nil && runeLen(compressed) <= visualLimit && compressed != "" {
				translatedSpeech[k] = compressed
			}
		}
	}

	translations := make([]string, len(bounds))
	compressedFlag := make([]bool, len(bounds))
	for k, idx := range toTranslateIdx {
		translations[idx] = translatedSpeech[k]
		if runeLen(translatedSpeech[k]) > visualLimit {
			compressedFlag[idx] = true
		}
	}

	cues := make([]Cue, 0, len(bounds))
	for i, b := range bounds {
		chunk := canonical.Words[b[0]:b[1]]
		kind := classifyKind(chunk)
		srcText := wrapToLines(srcTexts[i], rules.MaxCharsPerLine, rules.MaxLines)

		lines := []Line{
			{
				Role: "source", Lang: sourceLang, Text: srcText,
				CharCount: runeLen(stripNewlines(srcText)),
				CPS:       cpsForChunk(srcText, chunk),
			},
		}
		if kind == "speech" && translations[i] != "" {
			tgtText := wrapToLines(translations[i], rules.MaxCharsPerLine, rules.MaxLines)
			lines = append(lines, Line{
				Role: "translation", Lang: targetLang, Text: tgtText,
				CharCount: runeLen(stripNewlines(tgtText)),
				CPS:       cpsForChunk(tgtText, chunk),
			})
		}

		cue := Cue{
			Idx:             i,
			Start:           chunk[0].Start,
			End:             chunk[len(chunk)-1].End,
			Lines:           lines,
			Kind:            kind,
			Speaker:         dominantSpeaker(chunk),
			SourceWordRange: [2]int{b[0], b[1]},
		}
		cue.Quality = computeQuality(cue, rules, canonical.Words, b[1]-1)
		if compressedFlag[i] {
			cue.Quality.Flags = appendUnique(cue.Quality.Flags, "compressed")
			cue.Quality.NeedsReview = true
		}
		cues = append(cues, cue)
	}

	stats := computeStats(cues, rules)
	stats.LLMCallsMade = llm.Calls()

	return &CueList{
		Version:     "1.0",
		Mode:        "dual",
		SourceLang:  sourceLang,
		TargetLang:  targetLang,
		DurationSec: canonical.DurationSec,
		Cues:        cues,
		Stats:       stats,
	}, nil
}

// ============================================================
// Batched translate with context window
// ============================================================

// batchTranslateWithContext: split into batches of translateBatchSize,
// pass translateContextSize surrounding lines as context per batch.
// Uses translateWithMode (picks 1-pass or 3-pass via env).
// On batch failure, splits batch in half and retries; falls back to source text if all retries fail.
func batchTranslateWithContext(llm *LLMClient, sources []string,
	srcLang, tgtLang string, maxCharsPerLine int,
) ([]string, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	out := make([]string, len(sources))
	for i := 0; i < len(sources); i += translateBatchSize {
		end := i + translateBatchSize
		if end > len(sources) {
			end = len(sources)
		}
		batch := sources[i:end]

		ctxStart := i - translateContextSize
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxBefore := sources[ctxStart:i]

		ctxEnd := end + translateContextSize
		if ctxEnd > len(sources) {
			ctxEnd = len(sources)
		}
		ctxAfter := sources[end:ctxEnd]

		result := translateBatchWithRetry(llm, batch, ctxBefore, ctxAfter, srcLang, tgtLang, maxCharsPerLine)
		copy(out[i:end], result)
	}
	return out, nil
}

// translateBatchWithRetry tries the full batch; on count mismatch or LLM error,
// recursively splits batch in half. At size 1, falls back to source text.
// Returns slice of len(batch) — never errors.
func translateBatchWithRetry(llm *LLMClient, batch, ctxBefore, ctxAfter []string,
	srcLang, tgtLang string, maxCharsPerLine int,
) []string {
	if len(batch) == 0 {
		return nil
	}
	result, err := translateWithMode(llm, batch, ctxBefore, ctxAfter, srcLang, tgtLang, maxCharsPerLine)
	if err == nil {
		return result
	}

	// Single item failed even with retries — fall back to source text
	if len(batch) == 1 {
		return []string{batch[0]}
	}

	// Split in half and retry recursively
	mid := len(batch) / 2
	left := translateBatchWithRetry(llm, batch[:mid], ctxBefore, append(append([]string{}, batch[mid:]...), ctxAfter...),
		srcLang, tgtLang, maxCharsPerLine)
	right := translateBatchWithRetry(llm, batch[mid:], append(append([]string{}, ctxBefore...), batch[:mid]...), ctxAfter,
		srcLang, tgtLang, maxCharsPerLine)
	return append(left, right...)
}

// ============================================================
// Helpers
// ============================================================

func joinWords(words []Word) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}

func detectSentences(words []Word) [][2]int {
	bounds := [][2]int{}
	start := 0
	for i, w := range words {
		if endsWithAny(w.Text, ".", "!", "?", "。", "！", "？") {
			bounds = append(bounds, [2]int{start, i + 1})
			start = i + 1
		}
	}
	if start < len(words) {
		bounds = append(bounds, [2]int{start, len(words)})
	}
	if len(bounds) == 0 && len(words) > 0 {
		bounds = [][2]int{{0, len(words)}}
	}
	return bounds
}

// wrapToLines word-wraps text into at most maxLines lines of maxChars each.
// If the greedy wrap produces more than maxLines, excess lines are merged into
// the last allowed line (accepts char overflow there — caller flags it).
// max_lines is HARD: result always has ≤ maxLines lines.
func wrapToLines(text string, maxChars, maxLines int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	if maxLines <= 1 {
		// Single-line mode: never wrap.
		return strings.Join(words, " ")
	}

	lines := []string{}
	current := ""
	for _, w := range words {
		candidate := current
		if candidate != "" {
			candidate += " "
		}
		candidate += w
		if runeLen(candidate) > maxChars && current != "" {
			lines = append(lines, current)
			current = w
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}

	// HARD enforce max_lines: merge any excess into the last allowed line.
	// This may push the last line over maxChars — quality.flags will catch it.
	if len(lines) > maxLines {
		merged := strings.Join(lines[maxLines-1:], " ")
		lines = append(lines[:maxLines-1], merged)
	}

	return strings.Join(lines, "\n")
}

func splitByLength(text string, maxChars int) []string {
	if runeLen(text) <= maxChars {
		return []string{text}
	}
	words := strings.Fields(text)
	out := []string{}
	current := ""
	for _, w := range words {
		candidate := current
		if candidate != "" {
			candidate += " "
		}
		candidate += w
		if runeLen(candidate) > maxChars && current != "" {
			out = append(out, current)
			current = w
		} else {
			current = candidate
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func cpsForChunk(text string, words []Word) float64 {
	if len(words) == 0 {
		return 0
	}
	dur := words[len(words)-1].End - words[0].Start
	return cpsForDuration(text, dur)
}

func cpsForDuration(text string, dur float64) float64 {
	if dur <= 0 {
		return 0
	}
	return float64(runeLen(stripNewlines(text))) / dur
}

func stripNewlines(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

func classifyKind(words []Word) string {
	if len(words) == 0 {
		return "speech"
	}
	allBrackets := true
	for _, w := range words {
		if !isBracketTag(w.Text) {
			allBrackets = false
			break
		}
	}
	if !allBrackets {
		return "speech"
	}
	text := strings.ToLower(joinWords(words))
	if strings.Contains(text, "music") || strings.Contains(text, "nhạc") || strings.Contains(text, "♪") {
		return "music"
	}
	return "effect"
}

func dominantSpeaker(words []Word) string {
	counts := map[string]int{}
	for _, w := range words {
		if w.Speaker != "" {
			counts[w.Speaker]++
		}
	}
	best := ""
	bestCount := 0
	for s, c := range counts {
		if c > bestCount {
			bestCount = c
			best = s
		}
	}
	return best
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// ============================================================
// Quality + Stats
// ============================================================

func computeQuality(cue Cue, rules Rules, words []Word, cutAfterIdx int) Quality {
	q := Quality{Flags: []string{}}

	if cutAfterIdx >= 0 && cutAfterIdx < len(words) {
		q.BoundaryScore = boundaryScore(words, cutAfterIdx, rules)
		if q.BoundaryScore < 5 {
			q.Flags = append(q.Flags, "weak_boundary")
		}
	}

	maxCPS := 0.0
	for _, l := range cue.Lines {
		if l.CPS > maxCPS {
			maxCPS = l.CPS
		}
	}
	q.CPSWithinLimit = maxCPS <= rules.MaxCPS
	if !q.CPSWithinLimit {
		q.Flags = append(q.Flags, "cps_exceeded")
	}

	for _, l := range cue.Lines {
		for _, line := range strings.Split(l.Text, "\n") {
			if runeLen(line) > rules.MaxCharsPerLine {
				q.Flags = appendUnique(q.Flags, "line_overflow")
			}
		}
	}

	dur := cue.End - cue.Start
	if dur < rules.MinDurationSec {
		q.Flags = appendUnique(q.Flags, "too_short")
	}

	q.NeedsReview = len(q.Flags) > 0
	return q
}

func computeStats(cues []Cue, rules Rules) Stats {
	s := Stats{TotalCues: len(cues)}
	if len(cues) == 0 {
		return s
	}

	totalCPS, totalDur, lineCount := 0.0, 0.0, 0
	maxChars := 0
	for _, c := range cues {
		for _, l := range c.Lines {
			totalCPS += l.CPS
			lineCount++
			for _, line := range strings.Split(l.Text, "\n") {
				rl := runeLen(line)
				if rl > maxChars {
					maxChars = rl
				}
			}
		}
		totalDur += c.End - c.Start
		if !c.Quality.CPSWithinLimit {
			s.CuesOverCPSLimit++
		}
		if c.Quality.NeedsReview {
			s.CuesNeedReview++
		}
	}
	if lineCount > 0 {
		s.AvgCPS = totalCPS / float64(lineCount)
	}
	s.AvgDurationSec = totalDur / float64(len(cues))
	s.MaxCharsPerLine = maxChars
	return s
}
