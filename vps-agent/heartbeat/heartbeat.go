package heartbeat

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

type Heartbeat struct {
	hubURL   string
	serverIP string
	interval time.Duration
}

func NewHeartbeat(hubURL, serverIP string, interval time.Duration) *Heartbeat {
	return &Heartbeat{
		hubURL:   hubURL,
		serverIP: serverIP,
		interval: interval,
	}
}

func (h *Heartbeat) Start() {
	ticker := time.NewTicker(h.interval)

	for range ticker.C {
		h.ping()
	}
}

func (h *Heartbeat) ping() {
	url := fmt.Sprintf("%s/api/server-config/%s/heartbeat", h.hubURL, h.serverIP)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("[Heartbeat] Failed to ping hub: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Println("[Heartbeat] Ping successful")
	}
}
