package services

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"tik-downloader/repository"
)

// CookieItem is a single cookie in memory
type CookieItem struct {
	ID    string
	Value string
}

// cookieRepo is the minimal interface required by CookieProvider
type cookieRepo interface {
	GetRandomActiveCookies(limit int) ([]repository.CookieDoc, error)
}

// CookieProvider manages the active TikTok cookie with in-memory cache
type CookieProvider struct {
	mu       sync.RWMutex
	pool     []CookieItem
	repo     cookieRepo
	stopChan chan struct{}
}

// global provider instance
var globalProvider *CookieProvider

// InitCookieProvider initializes the global cookie provider with DB repo.
// It loads the cookies immediately and starts a background refresh goroutine.
func InitCookieProvider(repo cookieRepo) {
	rand.Seed(time.Now().UnixNano())

	p := &CookieProvider{
		repo:     repo,
		pool:     []CookieItem{},
		stopChan: make(chan struct{}),
	}
	p.refresh()
	globalProvider = p

	// Background refresh every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.refresh()
			case <-p.stopChan:
				return
			}
		}
	}()
}

// StopCookieProvider stops the background refresh goroutine
func StopCookieProvider() {
	if globalProvider != nil {
		close(globalProvider.stopChan)
	}
}

// GetCookie returns a random active cookie from the pool.
// Falls back to empty if DB has no active cookies.
func GetCookie() CookieItem {
	if globalProvider == nil {
		return CookieItem{}
	}

	globalProvider.mu.RLock()
	defer globalProvider.mu.RUnlock()

	if len(globalProvider.pool) == 0 {
		return CookieItem{}
	}

	// Select random cookie
	idx := rand.Intn(len(globalProvider.pool))
	return globalProvider.pool[idx]
}

// refresh loads the latest active cookies from DB into pool
func (p *CookieProvider) refresh() {
	docs, err := p.repo.GetRandomActiveCookies(10) // fetch up to 10
	if err != nil {
		log.Printf("[Cookie] Failed to load from DB: %v — keeping existing cache\n", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(docs) == 0 {
		log.Printf("[Cookie] No active cookie in DB — using fallback\n")
		p.pool = []CookieItem{}
		return
	}

	// Transform new pool
	newPool := []CookieItem{}
	for _, doc := range docs {
		log.Printf("[DEBUG] DB cookie: ID=%s, Cookie length=%d, Status=%s\n", doc.ID.Hex(), len(doc.Cookie), doc.Status)
		newPool = append(newPool, CookieItem{
			ID:    doc.ID.Hex(),
			Value: doc.Cookie,
		})
	}
	p.pool = newPool

	log.Printf("[Cookie] Refreshed pool with %d active cookies\n", len(p.pool))
}
