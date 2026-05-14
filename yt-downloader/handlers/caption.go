package handlers

import (
	"encoding/json"
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

// YouTube json3 format structures
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

// HandleCaption handles GET /api/caption — downloads YouTube json3 subtitle, parses and returns clean JSON
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

	// Download json3 from YouTube (via WARP proxy)
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

	// Parse json3 format
	var j3 json3Response
	if err := json.Unmarshal(body, &j3); err != nil {
		return utils.InternalError(c, "Failed to parse caption data")
	}

	// Convert to utterances
	utterances := make([]Utterance, 0, len(j3.Events))
	for _, event := range j3.Events {
		if len(event.Segs) == 0 || event.DDurationMs == 0 {
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

	if lang == "" {
		lang = detectLanguageFromURL(rawURL)
	}

	return c.JSON(CaptionResponse{
		Language:   lang,
		Duration:   duration,
		Utterances: utterances,
	})
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
