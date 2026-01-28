package heartbeat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"vps-agent/metrics"
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

	// Collect system metrics
	m, err := metrics.Collect()
	if err != nil {
		log.Printf("[Heartbeat] Failed to collect metrics: %v", err)
		// Continue even if metrics fail, to keep sending heartbeat
		m = &metrics.SystemMetrics{}
	}

	payload, err := json.Marshal(m)
	if err != nil {
		log.Printf("[Heartbeat] Failed to marshal metrics: %v", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("[Heartbeat] Failed to ping hub: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// Only log periodically or on change to avoid noise
		// log.Println("[Heartbeat] Ping successful")
	} else {
		log.Printf("[Heartbeat] Ping failed with status: %d", resp.StatusCode)
	}
}
