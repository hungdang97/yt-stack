package main

import "strings"

// boundaryScore: score quality of cutting AFTER word at index i.
// Higher = better cut point. Negative = bad linguistic break.
func boundaryScore(words []Word, i int, rules Rules) int {
	if i < 0 || i >= len(words) {
		return 0
	}
	score := 0
	text := words[i].Text

	if endsWithAny(text, ".", "!", "?", "。", "！", "？") {
		score += 10
	} else if endsWithAny(text, ",", ";", ":", "，", "；", "：") {
		score += 7
	}

	if i+1 < len(words) {
		gap := words[i+1].Start - words[i].End
		switch {
		case gap >= rules.HardPauseSec:
			score += 5
		case gap >= rules.SoftPauseSec:
			score += 3
		}
		if words[i].Speaker != "" && words[i+1].Speaker != "" && words[i].Speaker != words[i+1].Speaker {
			score += 1000
		}
		if isBracketTag(words[i].Text) != isBracketTag(words[i+1].Text) {
			score += 50
		}
	}

	if isPreposition(text) {
		score -= 5
	}

	return score
}

func endsWithAny(s string, suffixes ...string) bool {
	for _, sfx := range suffixes {
		if strings.HasSuffix(s, sfx) {
			return true
		}
	}
	return false
}

var (
	enPrepositions = map[string]bool{"in": true, "on": true, "at": true, "to": true, "for": true, "with": true, "of": true, "by": true, "from": true, "into": true}
	viPrepositions = map[string]bool{"ở": true, "tại": true, "đến": true, "cho": true, "với": true, "của": true, "bởi": true, "từ": true, "trong": true, "trên": true}
)

func isPreposition(word string) bool {
	w := strings.ToLower(strings.TrimRight(word, ".,;:!?"))
	return enPrepositions[w] || viPrepositions[w]
}
