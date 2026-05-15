package handlers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"yt-downloader-go/config"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

type Utterance struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type CaptionResponse struct {
	Language   string      `json:"language"`
	Duration   float64     `json:"duration"`
	Utterances []Utterance `json:"utterances"`
}

// HandleCaption handles GET /api/caption — downloads YouTube VTT subtitle, parses and returns clean JSON
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

	// Download VTT from YouTube CDN (via WARP proxy)
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return utils.InternalError(c, "Invalid caption URL")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := config.ProxyMediaClient.Do(req)
	if err != nil {
		return utils.InternalError(c, fmt.Sprintf("Failed to download caption: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return utils.InternalError(c, fmt.Sprintf("Caption download returned status %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return utils.InternalError(c, "Failed to read caption data")
	}

	// Parse WebVTT format
	utterances := parseVTT(string(body))

	if lang == "" {
		lang = detectLanguageFromURL(rawURL)
	}

	return c.JSON(CaptionResponse{
		Language:   lang,
		Duration:   duration,
		Utterances: utterances,
	})
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
