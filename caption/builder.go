package main

import "strings"

// BuildUtterances groups the canonical word stream into utterances using the
// chunker + scoring rules, then emits the same shape as Deepgram /transcribe
// output: { language, duration, utterances: [{start, end, text, words: [...]}] }.
//
// Each utterance contains its own word-level timing (preserved from B1 input).
// No translation, no quality flags, no line wrapping — caller composes UI.
func BuildUtterances(canonical *Canonical, rules Rules, sourceLang string) *Transcript {
	bounds := chunkWords(canonical.Words, rules)
	utterances := make([]Utterance, 0, len(bounds))

	for _, b := range bounds {
		chunk := canonical.Words[b[0]:b[1]]
		if len(chunk) == 0 {
			continue
		}
		words := make([]UttWord, 0, len(chunk))
		for _, w := range chunk {
			words = append(words, UttWord{
				Text:  w.Text,
				Start: w.Start,
				End:   w.End,
			})
		}
		utterances = append(utterances, Utterance{
			Start: chunk[0].Start,
			End:   chunk[len(chunk)-1].End,
			Text:  joinWords(chunk),
			Words: words,
		})
	}

	return &Transcript{
		Language:   sourceLang,
		Duration:   canonical.DurationSec,
		Utterances: utterances,
	}
}

func joinWords(words []Word) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}
