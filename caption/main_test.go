package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestB1AdapterYouTube(t *testing.T) {
	data, err := os.ReadFile("testdata/youtube_sample.json")
	if err != nil {
		t.Skip("no test data")
	}
	c, err := ParseYouTubeJSON3(data)
	if err != nil {
		t.Fatal(err)
	}
	Clean(c)
	if err := Validate(*c); err != nil {
		t.Fatal(err)
	}
	if len(c.Words) == 0 {
		t.Fatal("no words")
	}
}

func TestB1AdapterDeepgram(t *testing.T) {
	data, err := os.ReadFile("testdata/deepgram_sample.json")
	if err != nil {
		t.Skip("no test data")
	}
	c, err := ParseDeepgram(data)
	if err != nil {
		t.Fatal(err)
	}
	Clean(c)
	if err := Validate(*c); err != nil {
		t.Fatal(err)
	}
}

func TestBuildUtterancesFromDeepgram(t *testing.T) {
	data, err := os.ReadFile("testdata/deepgram_sample.json")
	if err != nil {
		t.Skip("no test data")
	}
	c, _ := ParseDeepgram(data)
	Clean(c)
	result := BuildUtterances(c, GetRules("en", nil), "en")

	if result.Language != "en" {
		t.Fatalf("language wrong: %s", result.Language)
	}
	if len(result.Utterances) == 0 {
		t.Fatal("no utterances")
	}
	for _, u := range result.Utterances {
		if u.Text == "" {
			t.Fatal("empty utterance text")
		}
		if len(u.Words) == 0 {
			t.Fatal("utterance has no words")
		}
		if u.Start >= u.End {
			t.Fatalf("invalid utterance timing: %+v", u)
		}
	}
}

func TestChunkerInvariants(t *testing.T) {
	data, err := os.ReadFile("testdata/deepgram_sample.json")
	if err != nil {
		t.Skip("no test data")
	}
	c, _ := ParseDeepgram(data)
	Clean(c)
	result := BuildUtterances(c, GetRules("en", nil), "en")

	for i := 0; i < len(result.Utterances)-1; i++ {
		if result.Utterances[i].End > result.Utterances[i+1].Start {
			t.Errorf("utterance overlap at idx=%d", i)
		}
	}
}

func TestRulesOverride(t *testing.T) {
	merged := GetRules("en", &Rules{MaxCPS: 25, MaxCharsPerLine: 50})
	if merged.MaxCPS != 25 || merged.MaxCharsPerLine != 50 {
		t.Errorf("override not applied: %+v", merged)
	}
	if merged.MaxLines != 2 {
		t.Errorf("fallback MaxLines should be 2: %d", merged.MaxLines)
	}
}

func TestCachePersistence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CAPTION_CACHE_DIR", tmp)

	c := NewCache()
	if !c.Enabled() {
		t.Fatal("cache should be enabled")
	}
	key := c.Key("model-x", "system", "user")
	c.Set(key, "cached-result")
	c2 := NewCache()
	got, ok := c2.Get(key)
	if !ok || got != "cached-result" {
		t.Fatalf("cache miss after persistence: got=%q ok=%v", got, ok)
	}
	if _, err := os.Stat(filepath.Join(tmp, key+".txt")); err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
}
