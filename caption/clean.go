package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const minWordDurSec = 0.05

// stripInvisible removes zero-width characters and other invisible
// Unicode "formatting" runes that survive TrimSpace but render nothing.
// YouTube auto-sub often inserts U+200B around segs as a layout hack.
func stripInvisible(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 0x200B, // zero-width space
			0x200C, // zero-width non-joiner
			0x200D, // zero-width joiner
			0x2060, // word joiner
			0xFEFF: // BOM / zero-width no-break space
			continue
		}
		if unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func Clean(c *Canonical) {
	// 1. Normalize text: strip invisible chars, NFC, trim whitespace.
	for i := range c.Words {
		t := stripInvisible(c.Words[i].Text)
		t = norm.NFC.String(strings.TrimSpace(t))
		c.Words[i].Text = t
	}

	// 2. Drop empty words.
	out := c.Words[:0]
	for _, w := range c.Words {
		if w.Text != "" {
			out = append(out, w)
		}
	}
	c.Words = out

	// 3. Sort by Start ascending — handles upstream non-monotonic data
	//    (multi-seg events with same/missing tOffsetMs etc.).
	sort.SliceStable(c.Words, func(i, j int) bool {
		return c.Words[i].Start < c.Words[j].Start
	})

	// 4. Reindex after sort + drops.
	for i := range c.Words {
		c.Words[i].Idx = i
	}

	const epsilon = 0.001 // 1ms — small enough to be invisible but enforces strict ordering

	// 5. Force strictly monotonic Start: when consecutive words have equal
	//    or inverted starts (degenerate YT json3 with multiple segs at same
	//    tStartMs and no tOffsetMs), bump by epsilon each.
	for i := 1; i < len(c.Words); i++ {
		if c.Words[i].Start <= c.Words[i-1].Start {
			c.Words[i].Start = c.Words[i-1].Start + epsilon
			if c.Words[i].End < c.Words[i].Start {
				c.Words[i].End = c.Words[i].Start + epsilon
			}
		}
	}

	// 6. Clamp end times: start ≤ end, no overlap with next.
	clamped := 0
	for i := 0; i < len(c.Words); i++ {
		if c.Words[i].End <= c.Words[i].Start {
			c.Words[i].End = c.Words[i].Start + minWordDurSec
			clamped++
		}
		if i+1 < len(c.Words) && c.Words[i].End > c.Words[i+1].Start {
			c.Words[i].End = c.Words[i+1].Start
			// At this point start < next.start (monotonic guaranteed),
			// so End ≥ Start is preserved.
			clamped++
		}
	}

	if clamped > 0 {
		c.Warnings = append(c.Warnings, Warning{Type: "clamped_overlap", Count: clamped})
	}

	if n := len(c.Words); n > 0 {
		c.DurationSec = c.Words[n-1].End
	}
}

func Validate(c Canonical) error {
	for i, w := range c.Words {
		if w.Idx != i {
			return fmt.Errorf("idx mismatch at %d: got %d", i, w.Idx)
		}
		if w.Text == "" {
			return fmt.Errorf("empty word at %d", i)
		}
		if w.Start > w.End {
			return fmt.Errorf("negative duration at idx=%d", i)
		}
		if i > 0 {
			prev := c.Words[i-1]
			if prev.End > w.Start {
				return fmt.Errorf("overlap at idx=%d", i)
			}
		}
	}
	return nil
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
