package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
)

// Cache: SHA256-keyed file + in-memory cache for LLM responses.
// Disable by setting CAPTION_CACHE_DIR=off.
type Cache struct {
	dir     string
	enabled bool
	mu      sync.RWMutex
	mem     map[string]string
}

func NewCache() *Cache {
	dir := os.Getenv("CAPTION_CACHE_DIR")
	if dir == "off" {
		return &Cache{enabled: false}
	}
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "caption-cache")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Cache{enabled: false}
	}
	return &Cache{
		dir:     dir,
		enabled: true,
		mem:     map[string]string{},
	}
}

func (c *Cache) Enabled() bool { return c != nil && c.enabled }

// Key produces stable hex hash from arbitrary parts (model, system, user prompts).
func (c *Cache) Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) Get(key string) (string, bool) {
	if !c.Enabled() {
		return "", false
	}
	c.mu.RLock()
	if v, ok := c.mem[key]; ok {
		c.mu.RUnlock()
		return v, true
	}
	c.mu.RUnlock()

	data, err := os.ReadFile(filepath.Join(c.dir, key+".txt"))
	if err != nil {
		return "", false
	}
	c.mu.Lock()
	c.mem[key] = string(data)
	c.mu.Unlock()
	return string(data), true
}

func (c *Cache) Set(key, value string) {
	if !c.Enabled() {
		return
	}
	c.mu.Lock()
	c.mem[key] = value
	c.mu.Unlock()
	_ = os.WriteFile(filepath.Join(c.dir, key+".txt"), []byte(value), 0o644)
}
