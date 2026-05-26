package main

import (
	"encoding/json"
	"fmt"
)

type dgResponse struct {
	Results dgResults `json:"results"`
}

type dgResults struct {
	Channels []dgChannel `json:"channels"`
}

type dgChannel struct {
	Alternatives []dgAlternative `json:"alternatives"`
}

type dgAlternative struct {
	Transcript string   `json:"transcript"`
	Words      []dgWord `json:"words"`
}

type dgWord struct {
	Word           string  `json:"word"`
	PunctuatedWord string  `json:"punctuated_word"`
	Start          float64 `json:"start"`
	End            float64 `json:"end"`
	Confidence     float64 `json:"confidence"`
	Speaker        *int    `json:"speaker,omitempty"`
}

func ParseDeepgram(data []byte) (*Canonical, error) {
	var raw dgResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if len(raw.Results.Channels) == 0 || len(raw.Results.Channels[0].Alternatives) == 0 {
		return nil, fmt.Errorf("deepgram: no channels/alternatives")
	}

	src := raw.Results.Channels[0].Alternatives[0].Words
	words := make([]Word, 0, len(src))

	for _, dw := range src {
		text := dw.PunctuatedWord
		if text == "" {
			text = dw.Word
		}
		conf := dw.Confidence
		w := Word{
			Text:       text,
			Start:      dw.Start,
			End:        dw.End,
			Confidence: &conf,
		}
		if dw.Speaker != nil {
			w.Speaker = fmt.Sprintf("S%d", *dw.Speaker)
		}
		words = append(words, w)
	}

	return &Canonical{
		Version: "1.0",
		Source:  SourceMeta{Type: "deepgram", RawFormat: "deepgram_v1"},
		Words:   words,
	}, nil
}
