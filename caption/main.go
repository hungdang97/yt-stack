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
	mode := flag.String("mode", "", "source_only | translation_only | dual")
	sourceType := flag.String("source-type", "", "youtube | deepgram")
	sourceLang := flag.String("source-lang", "", "ISO lang code, e.g. vi, en")
	targetLang := flag.String("target-lang", "", "Required for translation_only/dual")
	inputPath := flag.String("input", "", "Path to source data JSON file")
	outputPath := flag.String("output", "-", "Output path or '-' for stdout")
	flag.Parse()

	if *mode != "" {
		runCLI(*mode, *sourceType, *sourceLang, *targetLang, *inputPath, *outputPath)
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

func runCLI(mode, sourceType, sourceLang, targetLang, inputPath, outputPath string) {
	if mode == "" || sourceType == "" || sourceLang == "" || inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: caption-service -mode=<m> -source-type=<t> -source-lang=<l> -input=<file> [-target-lang=<l>] [-output=<file>]")
		os.Exit(2)
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		die("read input: %v", err)
	}

	req := CaptionRequest{
		Mode:       mode,
		SourceType: sourceType,
		SourceData: raw,
		SourceLang: sourceLang,
		TargetLang: targetLang,
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

// process is the single entry point used by both HTTP and CLI.
func process(req CaptionRequest) (*CueList, error) {
	if req.Mode == "" || req.SourceType == "" || req.SourceLang == "" || len(req.SourceData) == 0 {
		return nil, fmt.Errorf("missing required: mode, source_type, source_lang, source_data")
	}
	if (req.Mode == "translation_only" || req.Mode == "dual") && req.TargetLang == "" {
		return nil, fmt.Errorf("target_lang required for mode %s", req.Mode)
	}

	canonical, err := adaptSource(req.SourceType, req.SourceData)
	if err != nil {
		return nil, fmt.Errorf("adapt source: %v", err)
	}
	Clean(canonical)
	if err := Validate(*canonical); err != nil {
		return nil, fmt.Errorf("validate canonical: %v", err)
	}

	// Auto-restore punctuation when source lacks it AND LLM is configured.
	// Safe to call always: noop when source already has punctuation OR no API key.
	llm := NewLLMClient()
	if llm.APIKey != "" {
		if _, err := MaybeRestorePunctuation(canonical, llm); err != nil {
			log.Printf("punct restoration warning: %v", err)
		}
	}

	rules := GetRules(req.SourceLang, req.Rules)

	switch req.Mode {
	case "source_only":
		return BuildSourceOnly(canonical, rules, req.SourceLang), nil
	case "translation_only":
		return BuildTranslationOnly(canonical, rules, llm, req.SourceLang, req.TargetLang)
	case "dual":
		return BuildDual(canonical, rules, llm, req.SourceLang, req.TargetLang)
	default:
		return nil, fmt.Errorf("unknown mode: %s", req.Mode)
	}
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
