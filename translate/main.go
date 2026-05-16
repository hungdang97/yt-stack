// Dịch phụ đề (subtitle) qua OpenRouter, model openai/gpt-oss-20b.
//
// POST /translate
//
//	{
//	  "target": "vi",
//	  "source": "en",        // optional, model tự đoán nếu thiếu
//	  "utterances": [
//	    { "start": 0,   "end": 1.6, "text": "Hello world" },
//	    { "start": 1.7, "end": 3.5, "text": "How are you" }
//	  ]
//	}
//
// Response giữ nguyên shape, mỗi text được dịch, start/end không đổi.
//
// === Tư duy chính ===
//
// Vấn đề: gpt-oss-20b hay "đánh rơi" dòng khi trả về numbered list dài
// (1. ..., 2. ..., 3. ...). Đã có doc nhiều bug structured-output.
//
// Giải pháp: dịch theo "object mode" — gửi/nhận JSON object có key cố định
// (u0, u1, ...). Key là anchor không thể lệch:
//
//	Input:  {"u0":"Hello","u1":"Thanks"}
//	Output: {"u0":"Xin chào","u1":"Cảm ơn"}
//
// 4 lớp phòng thủ chống lỗi:
//  1. Chunk: ≤20 items, ≤8KB mỗi chunk (model output ổn định).
//  2. JSON Schema strict: OpenRouter reject server-side nếu response sai shape.
//  3. Plugin response-healing: tự vá markdown fence, trailing garbage.
//  4. Retry chỉ keys thiếu (max 2 lần) — không retry cả chunk → rẻ.
//
// Chunks chạy song song (sync.WaitGroup + semaphore 100). Empty utterance
// được pass-through nguyên văn, không tốn API call.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr      = ":8503"
	httpTimeout     = 60 * time.Second
	openRouterURL   = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModel = "openai/gpt-oss-20b"

	// Dịch cần consistent, không muốn sáng tạo → temperature = 0.
	temperature = 0.0

	// Giới hạn chunk. Đụng MỘT trong 2 limit → mở chunk mới.
	// Object mode reliable hơn numbered list → cho phép tới 20 items.
	maxChunkChars   = 8000
	maxChunkItems   = 20
	maxOutputTokens = 32000

	// Số goroutine song song (toàn process, share giữa users).
	maxConcurrent = 100

	// Retry chunk: chỉ retry các keys missing, không retry full chunk.
	chunkRetries        = 2
	chunkRetryBaseDelay = 500 * time.Millisecond

	// Chống abuse: 1 request max 5000 utterances.
	maxUtterances = 5000
)

// Thứ tự failover: dùng apiKeys[0] trước, lỗi auth/rate-limit/5xx → key tiếp.
var apiKeys = []string{
	"REDACTED_OPENROUTER_KEY",
}

// Public request/response shape — đúng định dạng API trả về client.
type (
	utterance struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	}
	translateRequest struct {
		Target     string      `json:"target"`
		Source     string      `json:"source,omitempty"`
		Utterances []utterance `json:"utterances"`
	}
	translateResponse struct {
		Target     string      `json:"target"`
		Source     string      `json:"source,omitempty"`
		Utterances []utterance `json:"utterances"`
	}
)

// indexedText: gắn utterance (đã filter empty) với position trong request
// gốc → sau khi chunk + dịch parallel vẫn map lại đúng vị trí ban đầu.
type indexedText struct {
	Pos  int
	Text string
}

// Shape OpenRouter chat-completion (chỉ giữ field mình dùng).
type (
	chatRequest struct {
		Model          string          `json:"model"`
		Messages       []chatMessage   `json:"messages"`
		Temperature    float64         `json:"temperature"`
		MaxTokens      int             `json:"max_tokens,omitempty"`
		Reasoning      *reasoningOpt   `json:"reasoning,omitempty"`
		ResponseFormat *responseFormat `json:"response_format,omitempty"`
		Plugins        []pluginRef     `json:"plugins,omitempty"`
	}
	// Reasoning của gpt-oss-20b: effort=low + exclude=true → model bỏ qua
	// hầu hết chain-of-thought (nhanh hơn 3x, mình cũng không trả tiền
	// cho reasoning tokens ẩn).
	reasoningOpt struct {
		Effort  string `json:"effort,omitempty"`
		Exclude bool   `json:"exclude,omitempty"`
	}
	chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	chatResponse struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	// JSON Schema strict mode: required = mọi key + additionalProperties=false
	// → OpenRouter reject response sai shape ở phía server, không vào tới mình.
	responseFormat struct {
		Type       string     `json:"type"`
		JSONSchema jsonSchema `json:"json_schema"`
	}
	jsonSchema struct {
		Name   string         `json:"name"`
		Strict bool           `json:"strict"`
		Schema map[string]any `json:"schema"`
	}
	// Plugin response-healing: tự vá markdown fence, trailing garbage,
	// missing brackets — failure mode #1 của gpt-oss-20b. Theo benchmark
	// 5M+ requests của OpenRouter: success rate 99.01% → 99.36%.
	pluginRef struct {
		ID string `json:"id"`
	}
)

func main() {
	addr := listenAddr
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	http.HandleFunc("/translate", handleTranslate)
	http.HandleFunc("/cleanup", handleCleanup)
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		// vps-agent đọc "version" để hiện Ready trên hub dashboard.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})
	log.Printf("translate listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, corsMiddleware(http.DefaultServeMux)))
}

// corsMiddleware đáp ứng preflight + gắn header * cho mọi origin.
// Cho phép gọi từ file:// local và localhost mà không bị browser block.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Target) == "" {
		writeError(w, http.StatusBadRequest, "target is required")
		return
	}
	if len(req.Utterances) > maxUtterances {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"too many utterances: %d (max %d)", len(req.Utterances), maxUtterances))
		return
	}
	out, err := translate(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "openrouter: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// translate là entry point chính:
//  1. Tạo response skeleton (giữ start/end + text gốc cho empty utterances).
//  2. Lọc non-empty utterances + nhớ position gốc.
//  3. Chunk theo byte + item count.
//  4. Dịch các chunk song song (goroutines).
//  5. Ghép kết quả về đúng position gốc.
//
// Empty utterance không tốn API call — pass-through text gốc.
func translate(req translateRequest) (translateResponse, error) {
	out := translateResponse{
		Target:     req.Target,
		Source:     req.Source,
		Utterances: make([]utterance, len(req.Utterances)),
	}
	for i, u := range req.Utterances {
		out.Utterances[i] = u
	}

	items := make([]indexedText, 0, len(req.Utterances))
	for i, u := range req.Utterances {
		if strings.TrimSpace(u.Text) != "" {
			items = append(items, indexedText{Pos: i, Text: u.Text})
		}
	}
	if len(items) == 0 {
		return out, nil
	}

	chunks := chunkInputs(items, maxChunkChars, maxChunkItems)
	results := make([][]indexedText, len(chunks))
	errs := make([]error, len(chunks))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c []indexedText) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx], errs[idx] = translateChunk(c, req.Source, req.Target)
		}(i, chunk)
	}
	wg.Wait()

	for i, r := range results {
		if errs[i] != nil {
			return translateResponse{}, fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), errs[i])
		}
		for _, it := range r {
			out.Utterances[it.Pos].Text = it.Text
		}
	}
	return out, nil
}

// chunkInputs gộp items thành các chunk theo 2 giới hạn: tổng bytes và số
// items — đụng cái nào trước thì mở chunk mới. Một item siêu dài (vượt
// maxChars) vẫn cho 1 chunk riêng thay vì cắt giữa câu.
func chunkInputs(items []indexedText, maxChars, maxItems int) [][]indexedText {
	var chunks [][]indexedText
	var current []indexedText
	size := 0
	for _, it := range items {
		overflow := len(current) > 0 && (size+len(it.Text) > maxChars || len(current) >= maxItems)
		if overflow {
			chunks = append(chunks, current)
			current = nil
			size = 0
		}
		current = append(current, it)
		size += len(it.Text)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// translateChunk dịch 1 chunk bằng object mode (input/output là JSON object
// với key cố định "u0", "u1"…). Key chính là anchor: nếu model bỏ hoặc trả
// rỗng key nào → mình biết CHÍNH XÁC item nào thiếu và **chỉ retry item đó**,
// không retry cả chunk → tiết kiệm token khi fail.
//
// Retry tối đa chunkRetries lần, mỗi lần backoff tăng dần.
func translateChunk(items []indexedText, source, target string) ([]indexedText, error) {
	pending := make(map[string]string, len(items))
	for i, it := range items {
		pending[fmt.Sprintf("u%d", i)] = it.Text
	}

	done := make(map[string]string, len(items))
	var lastErr error

	for attempt := 0; attempt <= chunkRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * chunkRetryBaseDelay)
			log.Printf("chunk retry %d/%d, %d items still pending", attempt, chunkRetries, len(pending))
		}
		result, err := translateChunkOnce(pending, source, target)
		if err != nil {
			lastErr = err
			continue
		}
		for k, v := range result {
			if _, want := pending[k]; want && strings.TrimSpace(v) != "" {
				done[k] = v
				delete(pending, k)
			}
		}
		if len(pending) == 0 {
			break
		}
	}

	if len(pending) > 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("%d items missing after retries", len(pending))
		}
		return nil, lastErr
	}

	out := make([]indexedText, len(items))
	for i, it := range items {
		out[i] = indexedText{Pos: it.Pos, Text: done[fmt.Sprintf("u%d", i)]}
	}
	return out, nil
}

// translateChunkOnce gửi MỘT request object-mode đến OpenRouter.
// Schema strict (required = mọi key + additionalProperties=false) ép
// OpenRouter reject ở server nếu shape sai. Plugin response-healing dọn dẹp
// markdown fence và rác đuôi mà gpt-oss-20b hay sinh ra.
func translateChunkOnce(items map[string]string, source, target string) (map[string]string, error) {
	properties := make(map[string]any, len(items))
	required := make([]string, 0, len(items))
	for k := range items {
		properties[k] = map[string]string{"type": "string"}
		required = append(required, k)
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}

	src := source
	if src == "" {
		src = "the source language (auto-detect)"
	}
	inputJSON, _ := json.Marshal(items)
	prompt := fmt.Sprintf(`Translate the VALUES of this JSON object from %s to %s.
Return a JSON object with EXACTLY the same keys, only the values translated.
Preserve tone, register, and meaning. Keep proper nouns unchanged.
No prose, no markdown — just the JSON object.

Input:
%s`, src, target, string(inputJSON))

	resp, err := callOpenRouter(chatRequest{
		Model:       openRouterModel,
		Temperature: temperature,
		MaxTokens:   maxOutputTokens,
		Reasoning:   &reasoningOpt{Effort: "low", Exclude: true},
		ResponseFormat: &responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchema{
				Name:   "translation",
				Strict: true,
				Schema: schema,
			},
		},
		Plugins:  []pluginRef{{ID: "response-healing"}},
		Messages: []chatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}
	return parseObject(resp.Choices[0].Message.Content)
}

// parseObject lấy JSON object từ giữa text trả về (cắt từ `{` đầu tiên tới
// `}` cuối cùng), chịu được rác đuôi hoặc markdown xung quanh.
func parseObject(s string) (map[string]string, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json object in response: %s", snippet(s, 200))
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("parse json object: %w", err)
	}
	return out, nil
}

// callOpenRouter POST request lên OpenRouter, thử lần lượt các API key.
// Chỉ failover sang key kế khi gặp lỗi auth/rate-limit/5xx; 4xx khác (vd
// 400 bad request) → trả lỗi luôn, không phí key.
func callOpenRouter(body chatRequest) (*chatResponse, error) {
	client := &http.Client{Timeout: httpTimeout}
	bodyBytes, _ := json.Marshal(body)

	var lastErr error
	for i, key := range apiKeys {
		req, _ := http.NewRequest(http.MethodPost, openRouterURL, bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("key #%d transport error: %v", i+1, err)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var cr chatResponse
			if err := json.Unmarshal(respBody, &cr); err != nil {
				return nil, fmt.Errorf("parse response: %w", err)
			}
			return &cr, nil
		}

		sErr := fmt.Errorf("status %d: %s", resp.StatusCode, snippet(string(respBody), 300))
		if shouldFailover(resp.StatusCode) {
			lastErr = sErr
			log.Printf("key #%d failed, trying next: %v", i+1, sErr)
			continue
		}
		return nil, sErr
	}
	if lastErr == nil {
		lastErr = errors.New("all api keys exhausted")
	}
	return nil, lastErr
}

// shouldFailover: status nào nên thử key kế (chỉ lỗi quota/auth/server).
func shouldFailover(status int) bool {
	return status == 401 || status == 403 || status == 429 || status >= 500
}

// =================== CLEANUP endpoint ===================
//
// POST /cleanup
//
//	{ "utterances": [{start, end, text}, ...] }
//
// Mục đích: làm sạch transcript từ ASR / YouTube auto-caption TRƯỚC khi đi
// translate hoặc TTS. ASR raw output thường có:
//   - Câu cắt giữa chừng (YouTube cắt theo time, không theo grammar)
//   - Thiếu dấu câu
//   - Lỗi ASR rõ ràng ("đọc bass" cho "đọc pass")
//   - Số/đơn vị malformed ("11.7 triệu000đ")
//   - Filler ("um", "uh", "kiểu kiểu")
//   - Proper noun không nhất quán
//
// LLM xử lý 5 việc cùng lúc, GIỮ NGUYÊN ngôn ngữ gốc (không dịch). Output
// mỗi utterance là MẢNG sub-utterances (1+ phần tử) — nếu câu sạch sẵn thì
// trả mảng 1 phần tử. Nếu câu dài, LLM split thành 2-3 chunk ≤80 chars.
//
// Backend Go re-distribute timestamp proportional theo char count cho từng
// sub-utterance — đảm bảo timeline khớp nhịp speaker gốc.
type (
	cleanupRequest struct {
		Utterances []utterance `json:"utterances"`
	}
	cleanupResponse struct {
		Utterances []utterance `json:"utterances"`
	}
	// cleanedItem: kết quả cleanup cho 1 utterance source.
	// Pos = vị trí gốc trong request. Chunks = 1 hoặc nhiều sub-utterance.
	cleanedItem struct {
		Pos    int
		Chunks []string
	}
)

func handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req cleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if len(req.Utterances) > maxUtterances {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"too many utterances: %d (max %d)", len(req.Utterances), maxUtterances))
		return
	}
	if len(req.Utterances) == 0 {
		writeJSON(w, http.StatusOK, cleanupResponse{Utterances: []utterance{}})
		return
	}
	out, err := cleanup(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "openrouter: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// cleanup: tái dùng cùng pattern chunk-parallel của translate.
// 1. Filter non-empty utterances + nhớ Pos gốc.
// 2. Chunk theo byte + item count (cùng limit translate).
// 3. Chạy cleanupChunk song song.
// 4. Rebuild output: empty array của LLM = "key này absorbed bởi key trước".
//    Backend gộp timestamp của các key absorbed vào key chứa cleaned text.
func cleanup(req cleanupRequest) (cleanupResponse, error) {
	items := make([]indexedText, 0, len(req.Utterances))
	for i, u := range req.Utterances {
		if strings.TrimSpace(u.Text) != "" {
			items = append(items, indexedText{Pos: i, Text: u.Text})
		}
	}
	if len(items) == 0 {
		return cleanupResponse{Utterances: req.Utterances}, nil
	}

	chunks := chunkInputs(items, maxChunkChars, maxChunkItems)
	results := make([][]cleanedItem, len(chunks))
	errs := make([]error, len(chunks))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c []indexedText) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx], errs[idx] = cleanupChunk(c)
		}(i, chunk)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return cleanupResponse{}, fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), err)
		}
	}

	// Pos → chunks. Empty slice (LLM trả []) = "key này absorbed bởi key trước".
	cleaned := make(map[int][]string, len(items))
	for _, r := range results {
		for _, item := range r {
			cleaned[item.Pos] = item.Chunks
		}
	}

	// Rebuild: dùng index-loop để khi gặp key có nội dung, "nuốt" các key
	// absorbed (empty array) phía sau → timestamp range mở rộng tương ứng.
	out := make([]utterance, 0, len(req.Utterances))
	i := 0
	for i < len(req.Utterances) {
		u := req.Utterances[i]
		subChunks, ok := cleaned[i]

		// Source utterance rỗng (không gửi LLM) → pass-through.
		if !ok {
			out = append(out, u)
			i++
			continue
		}

		// Key absorbed nhưng không có previous key chứa nội dung — edge case
		// (LLM bug). Pass-through để không mất utterance.
		if len(subChunks) == 0 {
			out = append(out, u)
			i++
			continue
		}

		// Look ahead: bao nhiêu key kế tiếp được absorbed vào key này?
		endIdx := i
		for j := i + 1; j < len(req.Utterances); j++ {
			next, hasNext := cleaned[j]
			if hasNext && len(next) == 0 {
				endIdx = j
			} else {
				break
			}
		}
		rangeEnd := req.Utterances[endIdx].End

		// Distribute subChunks theo char count trên timestamp range
		// [u.Start, rangeEnd]. 1 chunk thì giữ nguyên range.
		if len(subChunks) == 1 {
			out = append(out, utterance{Start: u.Start, End: rangeEnd, Text: subChunks[0]})
		} else {
			totalRunes := 0
			runeCounts := make([]int, len(subChunks))
			for j, c := range subChunks {
				runeCounts[j] = len([]rune(c))
				totalRunes += runeCounts[j]
			}
			if totalRunes == 0 {
				out = append(out, u)
			} else {
				duration := rangeEnd - u.Start
				cursor := u.Start
				for j, c := range subChunks {
					subDur := duration * float64(runeCounts[j]) / float64(totalRunes)
					end := cursor + subDur
					if j == len(subChunks)-1 {
						end = rangeEnd
					}
					out = append(out, utterance{Start: cursor, End: end, Text: c})
					cursor = end
				}
			}
		}

		i = endIdx + 1
	}

	return cleanupResponse{Utterances: out}, nil
}

// cleanupChunk: mỗi key trả về MẢNG sub-utterance (≥1). Retry pattern giống
// translateChunk — chỉ retry keys thiếu chứ không retry cả chunk.
func cleanupChunk(items []indexedText) ([]cleanedItem, error) {
	pending := make(map[string]string, len(items))
	keyToPos := make(map[string]int, len(items))
	for i, it := range items {
		k := fmt.Sprintf("u%d", i)
		pending[k] = it.Text
		keyToPos[k] = it.Pos
	}

	done := make(map[string][]string, len(items))
	var lastErr error

	for attempt := 0; attempt <= chunkRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * chunkRetryBaseDelay)
			log.Printf("cleanup chunk retry %d/%d, %d items still pending", attempt, chunkRetries, len(pending))
		}
		result, err := cleanupChunkOnce(pending)
		if err != nil {
			lastErr = err
			continue
		}
		for k, v := range result {
			if _, want := pending[k]; !want {
				continue
			}
			// EMPTY array = "key này absorbed bởi key trước" (merge marker).
			// Đây là response hợp lệ — backend sẽ xử lý ở stage rebuild.
			if len(v) == 0 {
				done[k] = []string{}
				delete(pending, k)
				continue
			}
			// Lọc trim empty strings trong array (LLM đôi khi sinh chunk rỗng).
			clean := make([]string, 0, len(v))
			for _, s := range v {
				if strings.TrimSpace(s) != "" {
					clean = append(clean, strings.TrimSpace(s))
				}
			}
			if len(clean) > 0 {
				done[k] = clean
				delete(pending, k)
			}
			// Nếu sau filter còn array rỗng (tất cả strings rỗng) → giữ pending
			// để retry.
		}
		if len(pending) == 0 {
			break
		}
	}

	if len(pending) > 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("%d items missing after retries", len(pending))
		}
		return nil, lastErr
	}

	out := make([]cleanedItem, len(items))
	for i, it := range items {
		k := fmt.Sprintf("u%d", i)
		out[i] = cleanedItem{Pos: it.Pos, Chunks: done[k]}
	}
	return out, nil
}

// cleanupChunkOnce: gửi 1 LLM call. Schema yêu cầu mỗi value là array of
// string. minItems=0 để cho phép empty array = "key này absorbed bởi key
// trước đó". Strict mode để OpenRouter reject response sai shape.
func cleanupChunkOnce(items map[string]string) (map[string][]string, error) {
	properties := make(map[string]any, len(items))
	required := make([]string, 0, len(items))
	for k := range items {
		properties[k] = map[string]any{
			"type":     "array",
			"items":    map[string]string{"type": "string"},
			"minItems": 0,
		}
		required = append(required, k)
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}

	inputJSON, _ := json.Marshal(items)
	prompt := fmt.Sprintf(`You are cleaning ASR / YouTube auto-caption transcript utterances.
The input is a JSON object where each value is a raw transcript chunk. Chunks
are often cut mid-word or mid-sentence (auto-caption cuts by time, not grammar),
have ASR typos, missing punctuation, or inconsistent proper noun spellings.

Goal: produce CLEAN, GRAMMATICALLY-COMPLETE sentences.

For each input key, return a JSON ARRAY (possibly empty). The array semantics:

  • 1 item: this key holds 1 clean sentence (its own, possibly merged from
    next keys; or split into multiple — see SPLIT below).
  • multiple items: this key's content was a long sentence that you split into
    sub-sentences (each ≤80 chars).
  • EMPTY [] : this key's content was ABSORBED by a previous key (merged into
    that key's sentence). Backend will extend the previous key's timestamp to
    cover this absorbed key.

RULES:

1. MERGE across keys when needed.
   If a key ends mid-word/mid-sentence AND the next key is its continuation,
   merge them into ONE complete sentence. Put the result in the FIRST key's
   array, return [] for the absorbed key.

   Example:
     Input:  {"u0": "Chào mừng các bạn đã quay trở lại với ng",
              "u1": "channel. Sau khá là nhiều tranh cãi về"}
     Output: {"u0": ["Chào mừng các bạn đã quay trở lại với kênh Channel.",
                     "Sau khá là nhiều tranh cãi về..."],
              "u1": []}

2. SPLIT long sentences.
   If cleaned text > 80 chars, split into 2-3 sub-sentences at natural grammar
   boundary (after period, comma, conjunction). Each ≤80 chars. Put split
   chunks as multiple items in the same key's array.

3. FIX obvious errors.
   • Punctuation: restore missing commas/periods if obvious from context.
   • ASR typos: "11.7 triệu000đ" → "11.7 triệu đồng"; "120 H" → "120 Hz" if
     context clearly indicates hertz.
   • Filler removal: drop "um", "uh", "ờ", "à", "kiểu kiểu" when clearly speech
     disfluency, NOT meaningful content.
   • Proper nouns: normalize inconsistent spellings within this batch
     (e.g. "Onway"/"Oneway" → "Oneway"; pick one canonical form).

4. ALREADY CLEAN.
   If a key's text is already a complete clean sentence ≤80 chars, return
   [original_text] as 1-item array (no change needed).

CRITICAL:
• DO NOT translate to any other language. Keep ORIGINAL language.
• Cover every key's content EXACTLY ONCE across all output arrays (either
  in its own array, or absorbed into a previous key's array).
• Process keys in input order; don't reorder.
• Return JSON only, no prose, no markdown.

Input:
%s`, string(inputJSON))

	resp, err := callOpenRouter(chatRequest{
		Model:       openRouterModel,
		Temperature: temperature,
		MaxTokens:   maxOutputTokens,
		Reasoning:   &reasoningOpt{Effort: "low", Exclude: true},
		ResponseFormat: &responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchema{
				Name:   "cleanup",
				Strict: true,
				Schema: schema,
			},
		},
		Plugins:  []pluginRef{{ID: "response-healing"}},
		Messages: []chatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}
	return parseObjectArray(resp.Choices[0].Message.Content)
}

// parseObjectArray: parse map[string][]string (cleanup response shape).
// Tolerant với rác đuôi / markdown — cắt từ '{' đầu tới '}' cuối.
func parseObjectArray(s string) (map[string][]string, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json object in response: %s", snippet(s, 200))
	}
	var out map[string][]string
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("parse json object: %w", err)
	}
	return out, nil
}

// =================== end CLEANUP ===================

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func snippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
