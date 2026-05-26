package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultPunctThreshold = 0.05
const restoreChunkSize = 150 // words per LLM call

// MaybeRestorePunctuation auto-detects no-punct sources and triggers LLM restore.
// Updates canonical.Words in place. Idempotent (safe to call multiple times).
// Returns true if restoration ran, false if skipped.
func MaybeRestorePunctuation(c *Canonical, llm *LLMClient) (bool, error) {
	threshold := defaultPunctThreshold
	if v := os.Getenv("CAPTION_RESTORE_PUNCT_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}
	if threshold <= 0 {
		return false, nil // disabled
	}

	if len(c.Words) == 0 {
		return false, nil
	}

	punctCount := 0
	for _, w := range c.Words {
		if endsWithAny(w.Text, ".", ",", "!", "?", ";", ":", "。", "！", "？", "，", "；", "：") {
			punctCount++
		}
	}
	ratio := float64(punctCount) / float64(len(c.Words))
	if ratio >= threshold {
		return false, nil // already has punctuation
	}

	// Restore in chunks (avoid token limit on long videos)
	restoredCount := 0
	for start := 0; start < len(c.Words); start += restoreChunkSize {
		end := start + restoreChunkSize
		if end > len(c.Words) {
			end = len(c.Words)
		}
		restoredWords, err := restorePunctChunk(llm, c.Words[start:end])
		if err != nil {
			continue // skip failing chunk, don't break whole pipeline
		}
		for i, rw := range restoredWords {
			c.Words[start+i].Text = rw
		}
		restoredCount += len(restoredWords)
	}

	if restoredCount > 0 {
		c.Warnings = append(c.Warnings, Warning{
			Type:  "punctuation_restored",
			Count: restoredCount,
		})
	}
	return restoredCount > 0, nil
}

// restorePunctChunk asks LLM to add punctuation to a word list, preserving order/count.
func restorePunctChunk(llm *LLMClient, words []Word) ([]string, error) {
	raw := make([]string, len(words))
	for i, w := range words {
		raw[i] = w.Text
	}
	inputJSON, _ := json.Marshal(raw)

	system := "You are a punctuation restoration tool. Return JSON array only — no markdown, no commentary."
	user := fmt.Sprintf(
		"Add punctuation (. , ! ? ; :) and proper capitalization to these words.\n"+
			"STRICT RULES:\n"+
			"- Return a JSON array of EXACTLY %d strings (same count).\n"+
			"- Each string is one word, optionally with punctuation appended (e.g. 'hello,' 'world.').\n"+
			"- DO NOT add, remove, or reorder words.\n"+
			"- DO NOT translate.\n\n"+
			"Input (%d words): %s\n\n"+
			"Output (JSON array of %d strings):",
		len(words), len(words), string(inputJSON), len(words))

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
		return nil, fmt.Errorf("parse restore JSON: %v", err)
	}
	if len(result) != len(words) {
		return nil, fmt.Errorf("word count mismatch: got %d, expected %d", len(result), len(words))
	}
	return result, nil
}
