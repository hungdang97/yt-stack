package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Hardcoded fallback keys (mirror translate service pattern).
// Env OPENROUTER_API_KEY overrides if set; otherwise apiKeys[0] is used.
var apiKeys = []string{
	"REDACTED_OPENROUTER_KEY",
}

type LLMClient struct {
	APIKey string
	Model  string
	URL    string
	Client *http.Client
	Cache  *Cache
	calls  int
	hits   int
}

func NewLLMClient() *LLMClient {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" && len(apiKeys) > 0 {
		key = apiKeys[0]
	}
	return &LLMClient{
		APIKey: key,
		Model:  envOrDefault("CAPTION_LLM_MODEL", "openai/gpt-4o-mini"),
		URL:    "https://openrouter.ai/api/v1/chat/completions",
		Client: &http.Client{Timeout: 120 * time.Second},
		Cache:  NewCache(),
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *LLMClient) Calls() int     { return c.calls }
func (c *LLMClient) CacheHits() int { return c.hits }

func (c *LLMClient) Complete(systemPrompt, userPrompt string) (string, error) {
	// Cache lookup
	var key string
	if c.Cache.Enabled() {
		key = c.Cache.Key(c.Model, systemPrompt, userPrompt)
		if v, ok := c.Cache.Get(key); ok {
			c.hits++
			return v, nil
		}
	}

	if c.APIKey == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY not set")
	}

	c.calls++
	req := chatRequest{
		Model:       c.Model,
		Temperature: 0.3,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", c.URL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://caption-service")
	httpReq.Header.Set("X-Title", "caption-service")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chat chatResponse
	if err := json.Unmarshal(respBody, &chat); err != nil {
		return "", fmt.Errorf("LLM parse: %v", err)
	}
	if chat.Error != nil {
		return "", fmt.Errorf("LLM error: %s", chat.Error.Message)
	}
	if len(chat.Choices) == 0 {
		return "", fmt.Errorf("LLM no choices")
	}

	result := chat.Choices[0].Message.Content
	if key != "" {
		c.Cache.Set(key, result)
	}
	return result, nil
}
