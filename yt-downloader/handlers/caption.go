package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
	"yt-downloader-go/config"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

type Utterance struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	Words []Word  `json:"words,omitempty"`
}

// Word — fake word-level timestamp. Native YouTube caption KHÔNG có word
// timing thật (chỉ line-level), nên distribute proportional theo rune count
// trên text. Shape giống deepgram service để FE dùng 1 format thống nhất
// cho karaoke / fine-grained align.
type Word struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type CaptionResponse struct {
	Language   string      `json:"language"`
	Duration   float64     `json:"duration"`
	Utterances []Utterance `json:"utterances"`
}

// YouTube json3 format structures (fallback for manual subtitles)
type json3Response struct {
	Events []json3Event `json:"events"`
}

type json3Event struct {
	TStartMs    int64      `json:"tStartMs"`
	DDurationMs int64      `json:"dDurationMs"`
	Segs        []json3Seg `json:"segs"`
}

type json3Seg struct {
	UTF8 string `json:"utf8"`
}

// HandleCaption handles GET /api/caption — downloads subtitle, parses and returns clean JSON
func HandleCaption(c *fiber.Ctx) error {
	rawURL := c.Query("url")
	token := c.Query("token")
	expiresStr := c.Query("expires")
	lang := c.Query("lang")
	durationStr := c.Query("duration")

	if rawURL == "" || token == "" || expiresStr == "" {
		return utils.BadRequest(c, utils.ErrValidationError, "Missing required parameters")
	}

	expires, err := utils.ParseExpires(expiresStr)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid expires")
	}

	if !utils.ValidateCaptionURL(rawURL, token, expires) {
		return utils.Forbidden(c, "Invalid or expired token")
	}

	var duration float64
	if durationStr != "" {
		duration, _ = strconv.ParseFloat(durationStr, 64)
	}

	// Resolve the actual VTT/json3 URL
	// If URL is an HLS manifest (googlevideo.com), fetch it to extract the real VTT URL inside
	fetchURL := rawURL
	if strings.Contains(rawURL, "googlevideo.com") && strings.Contains(rawURL, "hls_timedtext_playlist") {
		resolvedURL, err := resolveHLSSubtitleURL(rawURL)
		if err != nil {
			return utils.InternalError(c, fmt.Sprintf("Failed to resolve HLS subtitle: %v", err))
		}
		fetchURL = resolvedURL
	}

	// Download subtitle content (via WARP proxy)
	body, statusCode, err := fetchContent(fetchURL)
	if err != nil {
		return utils.InternalError(c, fmt.Sprintf("Failed to download caption: %v", err))
	}
	if statusCode != http.StatusOK {
		return utils.InternalError(c, fmt.Sprintf("Caption download returned status %d", statusCode))
	}

	// Parse based on content format
	var utterances []Utterance
	content := string(body)
	if strings.HasPrefix(strings.TrimSpace(content), "WEBVTT") {
		utterances = parseVTT(content)
	} else {
		utterances = parseJSON3(body)
	}

	// YouTube auto-captions overlap end[i] với start[i+1] để rolling-display
	// trên player của họ — pass-through vào pipeline render thì libass đè dòng
	// lên nhau. Clip lại để 1 lúc chỉ có 1 cue trên màn hình.
	utterances = clipOverlaps(utterances)

	// Estimate word-level timestamps proportional theo rune count.
	// Shape giống /api/deepgram để FE dùng 1 format thống nhất cho
	// karaoke caption / fine align. Field words[] có omitempty nên
	// client cũ chỉ đọc text/start/end vẫn work.
	fillWords(utterances)

	if lang == "" {
		lang = detectLanguageFromURL(rawURL)
	}

	return c.JSON(CaptionResponse{
		Language:   lang,
		Duration:   duration,
		Utterances: utterances,
	})
}

// resolveHLSSubtitleURL fetches an HLS m3u8 manifest and extracts the VTT URL inside
func resolveHLSSubtitleURL(m3u8URL string) (string, error) {
	body, statusCode, err := fetchContent(m3u8URL)
	if err != nil {
		return "", err
	}
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("HLS manifest returned status %d", statusCode)
	}

	// Parse m3u8 — find the first non-comment, non-empty line (the VTT URL)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// This is the VTT segment URL
		return line, nil
	}
	return "", fmt.Errorf("no VTT URL found in HLS manifest")
}

// fetchContent downloads content from a URL via WARP proxy
func fetchContent(url string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := config.ProxyMediaClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// parseJSON3 parses YouTube json3 subtitle format
func parseJSON3(body []byte) []Utterance {
	var j3 json3Response
	if err := json.Unmarshal(body, &j3); err != nil {
		return nil
	}

	utterances := make([]Utterance, 0, len(j3.Events))
	for _, event := range j3.Events {
		if len(event.Segs) == 0 {
			continue
		}

		var textParts []string
		for _, seg := range event.Segs {
			text := strings.TrimSpace(seg.UTF8)
			if text != "" {
				textParts = append(textParts, text)
			}
		}

		text := strings.Join(textParts, " ")
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}

		start := float64(event.TStartMs) / 1000.0
		end := float64(event.TStartMs+event.DDurationMs) / 1000.0

		utterances = append(utterances, Utterance{
			Start: start,
			End:   end,
			Text:  text,
		})
	}
	return utterances
}

// parseVTTTimestamp parses "HH:MM:SS.mmm" or "MM:SS.mmm" to seconds
func parseVTTTimestamp(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}

	var hours, minutes int
	var seconds float64
	var err error

	if len(parts) == 3 {
		hours, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, false
		}
		parts = parts[1:]
	}

	minutes, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}

	seconds, err = strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, false
	}

	return float64(hours)*3600 + float64(minutes)*60 + seconds, true
}

// parseVTT parses WebVTT content into utterances
func parseVTT(content string) []Utterance {
	var utterances []Utterance
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()

		// Look for timestamp lines: "00:00:01.000 --> 00:00:04.000"
		if !strings.Contains(line, "-->") {
			continue
		}

		// Strip position/alignment settings after timestamp
		timePart := line
		if idx := strings.Index(timePart, " align:"); idx != -1 {
			timePart = timePart[:idx]
		}
		if idx := strings.Index(timePart, " position:"); idx != -1 {
			timePart = timePart[:idx]
		}
		if idx := strings.Index(timePart, " size:"); idx != -1 {
			timePart = timePart[:idx]
		}
		if idx := strings.Index(timePart, " line:"); idx != -1 {
			timePart = timePart[:idx]
		}

		parts := strings.SplitN(timePart, "-->", 2)
		if len(parts) != 2 {
			continue
		}

		start, ok1 := parseVTTTimestamp(parts[0])
		end, ok2 := parseVTTTimestamp(parts[1])
		if !ok1 || !ok2 {
			continue
		}

		// Collect text lines until blank line
		var textLines []string
		for scanner.Scan() {
			textLine := scanner.Text()
			if textLine == "" {
				break
			}
			textLines = append(textLines, textLine)
		}

		// Join and clean text (strip VTT tags like <c>, </c>, etc.)
		text := strings.Join(textLines, " ")
		text = stripVTTTags(text)
		text = strings.Join(strings.Fields(text), " ")

		if text == "" {
			continue
		}

		utterances = append(utterances, Utterance{
			Start: start,
			End:   end,
			Text:  text,
		})
	}

	return utterances
}

// stripVTTTags removes WebVTT formatting tags like <c>, </c>, <00:00:01.000>, etc.
func stripVTTTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// clipOverlaps: nếu end[i] > start[i+1] thì set end[i] = start[i+1]. Mục đích
// để 1 thời điểm chỉ có 1 cue hiển thị — tránh libass đè 2 dòng lên nhau khi
// burn-in. YouTube auto-caption mặc định overlap ~1-2s liên tục, không sửa
// thì render ra ký tự chồng nhau.
func clipOverlaps(utts []Utterance) []Utterance {
	for i := 0; i < len(utts)-1; i++ {
		if utts[i].End > utts[i+1].Start {
			utts[i].End = utts[i+1].Start
		}
	}
	return utts
}

// distributeWords: estimate word-level timestamps bằng cách chia time
// proportional theo rune count. Native YT caption không có word timing
// thật, nên đây là approximation đủ tốt cho karaoke / sync use cases.
//
// Algorithm:
//   1. Split text bằng whitespace → tokens
//   2. Đếm rune mỗi token (UTF-8 aware cho VN/JA/ZH)
//   3. Chia duration cho mỗi token theo tỉ lệ rune_count / total_runes
//   4. Token cuối lấy end = utt.End (tránh sai số float dồn)
//
// Nếu text rỗng hoặc không có whitespace → trả nil (omitempty bỏ field).
func distributeWords(text string, start, end float64) []Word {
	tokens := strings.Fields(text)
	if len(tokens) == 0 {
		return nil
	}
	runeCounts := make([]int, len(tokens))
	totalRunes := 0
	for i, t := range tokens {
		runeCounts[i] = utf8.RuneCountInString(t)
		totalRunes += runeCounts[i]
	}
	if totalRunes == 0 {
		return nil
	}
	duration := end - start
	if duration <= 0 {
		return nil
	}
	words := make([]Word, len(tokens))
	cursor := start
	for i, t := range tokens {
		subDur := duration * float64(runeCounts[i]) / float64(totalRunes)
		wEnd := cursor + subDur
		if i == len(tokens)-1 {
			wEnd = end // anchor cuối, tránh drift do float
		}
		words[i] = Word{Text: t, Start: cursor, End: wEnd}
		cursor = wEnd
	}
	return words
}

// fillWords: gắn words[] estimate cho mọi utterance trong slice.
// Chạy SAU clipOverlaps để timestamps đã chuẩn.
func fillWords(utts []Utterance) {
	for i := range utts {
		utts[i].Words = distributeWords(utts[i].Text, utts[i].Start, utts[i].End)
	}
}

func detectLanguageFromURL(rawURL string) string {
	idx := strings.Index(rawURL, "lang=")
	if idx == -1 {
		return ""
	}
	rest := rawURL[idx+5:]
	end := strings.IndexAny(rest, "&# ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}
