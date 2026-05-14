package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

type CaptionRequest struct {
	URL      string  `json:"url"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

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
	TStartMs   int64        `json:"tStartMs"`
	DDurationMs int64       `json:"dDurationMs"`
	Segs       []json3Seg   `json:"segs"`
}

type json3Seg struct {
	UTF8 string `json:"utf8"`
}

// HandleCaption handles POST /api/caption
func HandleCaption(c *fiber.Ctx) error {
	var req CaptionRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid request body")
	}

	if req.URL == "" {
		return utils.BadRequest(c, utils.ErrValidationError, "URL is required")
	}

	// If it's a proxy URL, extract the raw URL
	rawURL := req.URL
	if strings.Contains(rawURL, "/proxy/media?") {
		extracted := utils.ExtractRawURLFromProxy(rawURL)
		if extracted != "" {
			rawURL = extracted
		}
	}

	// Download json3 from YouTube
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(rawURL)
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
		if len(event.Segs) == 0 {
			continue
		}

		// Combine all segments in this event
		var textParts []string
		for _, seg := range event.Segs {
			text := strings.TrimSpace(seg.UTF8)
			if text != "" {
				textParts = append(textParts, text)
			}
		}

		text := strings.Join(textParts, " ")
		// Clean up newlines within text
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

	language := req.Language
	if language == "" {
		language = detectLanguageFromURL(rawURL)
	}

	return c.JSON(CaptionResponse{
		Language:   language,
		Duration:   req.Duration,
		Utterances: utterances,
	})
}

func detectLanguageFromURL(rawURL string) string {
	// Extract lang= parameter from YouTube timedtext URL
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
