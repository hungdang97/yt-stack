package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	sourceType := flag.String("source-type", "", "youtube | deepgram")
	sourceLang := flag.String("source-lang", "", "ISO lang code, e.g. vi, en")
	inputPath := flag.String("input", "", "Path to source data JSON file")
	outputPath := flag.String("output", "-", "Output path or '-' for stdout")
	flag.Parse()

	if *sourceType != "" || *inputPath != "" {
		runCLI(*sourceType, *sourceLang, *inputPath, *outputPath)
		return
	}

	port := envOrDefault("PORT", "8505")
	http.HandleFunc("/caption", handleCaption)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok\n"))
	})
	log.Printf("caption-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func runCLI(sourceType, sourceLang, inputPath, outputPath string) {
	if sourceType == "" || sourceLang == "" || inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: caption -source-type=<t> -source-lang=<l> -input=<file> [-output=<file>]")
		os.Exit(2)
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		die("read input: %v", err)
	}

	req := CaptionRequest{
		SourceType: sourceType,
		SourceData: raw,
		SourceLang: sourceLang,
	}

	result, err := process(req)
	if err != nil {
		die("process: %v", err)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	out = append(out, '\n')
	if outputPath == "-" {
		os.Stdout.Write(out)
	} else {
		if err := os.WriteFile(outputPath, out, 0o644); err != nil {
			die("write output: %v", err)
		}
	}
}

func handleCaption(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, 405, "POST only")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, 400, "read body: "+err.Error())
		return
	}
	var req CaptionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, 400, "invalid JSON: "+err.Error())
		return
	}
	result, err := process(req)
	if err != nil {
		respondError(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// process: raw source → canonical → chunker → utterance format.
// Punctuation restoration is best-effort (only runs if OPENROUTER_API_KEY is set
// and source has < threshold ratio of punctuated words).
func process(req CaptionRequest) (*Transcript, error) {
	if req.SourceType == "" || req.SourceLang == "" || len(req.SourceData) == 0 {
		return nil, fmt.Errorf("missing required: source_type, source_lang, source_data")
	}

	canonical, err := adaptSource(req.SourceType, req.SourceData)
	if err != nil {
		return nil, fmt.Errorf("adapt source: %v", err)
	}
	Clean(canonical)
	if err := Validate(*canonical); err != nil {
		return nil, fmt.Errorf("validate canonical: %v", err)
	}

	// Optional: punctuation restoration for no-punct sources (e.g. YT auto-sub VN).
	llm := NewLLMClient()
	if llm.APIKey != "" {
		if _, err := MaybeRestorePunctuation(canonical, llm); err != nil {
			log.Printf("punct restoration warning: %v", err)
		}
	}

	rules := GetRules(req.SourceLang, req.Rules)
	return BuildUtterances(canonical, rules, req.SourceLang), nil
}

func adaptSource(sourceType string, rawData []byte) (*Canonical, error) {
	switch sourceType {
	case "youtube":
		return ParseYouTubeJSON3(rawData)
	case "deepgram":
		return ParseDeepgram(rawData)
	default:
		return nil, fmt.Errorf("unknown source_type: %s", sourceType)
	}
}

func respondError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
