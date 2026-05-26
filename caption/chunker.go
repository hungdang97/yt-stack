package main

// chunkWords groups word stream into cue boundaries using greedy + boundary scoring.
// Returns boundaries as [start_idx, end_idx_exclusive] pairs.
func chunkWords(words []Word, rules Rules) [][2]int {
	bounds := [][2]int{}
	n := len(words)
	if n == 0 {
		return bounds
	}

	start := 0
	for start < n {
		end := pickBestCut(words, start, rules)
		if end <= start {
			end = start + 1
		}
		bounds = append(bounds, [2]int{start, end})
		start = end
	}
	return bounds
}

// pickBestCut finds the best end-index (exclusive) for a cue starting at `start`.
// Iterates over candidate end positions within visual/temporal limits,
// picks the one with highest boundary score.
func pickBestCut(words []Word, start int, rules Rules) int {
	n := len(words)
	if start >= n {
		return n
	}

	minEnd := start + rules.MinWordsPerCue
	if minEnd > n {
		minEnd = n
	}
	if minEnd <= start {
		minEnd = start + 1
	}

	bestScore := -1 << 30
	bestEnd := -1
	visualLimit := rules.MaxCharsPerLine * rules.MaxLines

	for end := minEnd; end <= n; end++ {
		chunk := words[start:end]
		chars := chunkChars(chunk, rules.CharWeight)
		if chars > visualLimit {
			break
		}
		dur := chunk[len(chunk)-1].End - chunk[0].Start
		if dur > rules.MaxDurationSec {
			break
		}
		cps := 0.0
		if dur > 0 {
			cps = float64(chars) / dur
		}

		score := boundaryScore(words, end-1, rules)
		if dur < rules.MinDurationSec {
			score -= 3
		}
		if cps > rules.MaxCPS {
			score -= 5
		}

		if score > bestScore {
			bestScore = score
			bestEnd = end
		}
	}

	if bestEnd == -1 {
		// no candidate fit visual/temporal — force cut at minEnd
		bestEnd = minEnd
		if bestEnd > n {
			bestEnd = n
		}
	}
	return bestEnd
}

// chunkChars computes visual character count of a word chunk with per-language weight.
func chunkChars(words []Word, weight float64) int {
	total := 0.0
	for i, w := range words {
		total += float64(runeLen(w.Text)) * weight
		if i > 0 {
			total += 1.0 // space between words (always weight 1)
		}
	}
	return int(total)
}
