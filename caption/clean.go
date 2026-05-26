package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const minWordDurSec = 0.05

func Clean(c *Canonical) {
	for i := range c.Words {
		c.Words[i].Text = norm.NFC.String(strings.TrimSpace(c.Words[i].Text))
	}

	out := c.Words[:0]
	for _, w := range c.Words {
		if w.Text != "" {
			out = append(out, w)
		}
	}
	c.Words = out

	for i := range c.Words {
		c.Words[i].Idx = i
	}

	clamped := 0
	for i := 0; i < len(c.Words); i++ {
		if c.Words[i].End <= c.Words[i].Start {
			c.Words[i].End = c.Words[i].Start + minWordDurSec
			clamped++
		}
		if i+1 < len(c.Words) && c.Words[i].End > c.Words[i+1].Start {
			c.Words[i].End = c.Words[i+1].Start
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
