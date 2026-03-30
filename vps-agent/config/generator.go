package config

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateConfig auto-generates full config from server IP
func GenerateConfig(serverIP string) map[string]interface{} {
	subdomain := generateSubdomain(serverIP)

	return map[string]interface{}{
		// Core Identity
		"server_ip": serverIP,
		"name":      subdomain,
		"subdomain": subdomain,
		"email":     "admin@ytconvert.org",
		"port":      5001,

		// Proxy Credentials (random)
		"warp_user":   generateRandomString(8),
		"warp_pass":   generateRandomString(16),
		"direct_user": generateRandomString(8),
		"direct_pass": generateRandomString(16),

		// Storage & Download (defaults)
		"storage_dir":      "./storage",
		"download_threads": 4,
		"chunk_size":       10000000,
		"max_retries":      3,
		"retry_delay_ms":   100,
		"chunk_timeout_s":  30,

		// Extract API
		"extract_api_timeout": 10,

		// Cleanup
		"cleanup_interval":   "*/3 * * * *",
		"max_job_age_min":    60,
		"cleanup_batch_size": 5000,

		// Security (random secret)
		"signed_url_secret":         generateRandomString(32),
		"signed_url_expiration_min": 60,

		// Limits
		"max_trim_duration_min": 1440,
		"max_file_size_gb":      5,

		// Feature Flags
		"enable_merge":    true,
		"enable_trim":     true,
		"enable_reencode": true,

		// Tier Config (JSON string) - Customer tier config
		"tier_config": `{"0":{"threads":2,"rate":1048576},"1":{"threads":4,"rate":2097152}}`,

		// Agent Info
		"agent_version": "2.0.0",
	}
}

// generateSubdomain creates random subdomain
// vps-x7z9q2w1
func generateSubdomain(ip string) string {
	return "vps-" + generateRandomString(8)
}

// generateRandomString generates cryptographically secure random string
func generateRandomString(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}
