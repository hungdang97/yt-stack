package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"vps-agent/config"
	"vps-agent/control"
	"vps-agent/deployer"
	"vps-agent/heartbeat"
)

// waitForDNS waits until the domain resolves (A record exists)
func waitForDNS(domain string) {
	log.Printf("Checking DNS for %s...", domain)
	for {
		ips, err := net.LookupHost(domain)
		if err == nil && len(ips) > 0 {
			log.Printf("  ✓ Found records: %v", ips)
			return
		}
		log.Printf("  ... waiting for DNS propagation for %s (Retrying in 10s)", domain)
		time.Sleep(10 * time.Second)
	}
}

func main() {
	hubURL := os.Getenv("HUB_URL")
	projectDir := os.Getenv("PROJECT_DIR")
	agentPort := 9000

	if hubURL == "" {
		log.Fatal("HUB_URL environment variable is required")
	}
	if projectDir == "" {
		projectDir = "/opt/yt-stack"
	}

	log.Println("=== VPS Agent Starting ===")
	log.Printf("Hub URL: %s", hubURL)
	log.Printf("Project Dir: %s", projectDir)

	// 1. Create config fetcher
	fetcher := config.NewConfigFetcher(hubURL)
	serverIP := fetcher.GetServerIP()
	log.Printf("Detected Server IP: %s", serverIP)

	// 2. Check for existing config
	log.Println("Checking for existing config on Hub...")
	existingConfig, err := fetcher.FetchConfig()
	if err == nil && existingConfig != nil {
		log.Println("✓ Found existing config on Hub. Preserving configuration.")
	} else {
		log.Println("! No existing config found. Generating new config...")
	}

	// 3. Register with Hub (using existing or auto-generating)
	log.Println("Registering with Hub...")
	cfg, err := fetcher.RegisterWithHub(existingConfig)
	if err != nil {
		log.Fatalf("Failed to register: %v", err)
	}

	log.Printf("✓ Registered successfully!")
	log.Printf("  Server Name: %v", cfg["name"])
	log.Printf("  Subdomain: %v", cfg["subdomain"])
	// Show actual domain from config
	domain := "ytconvert.org"
	if d, ok := cfg["domain"]; ok && d != nil && d != "" {
		domain = fmt.Sprintf("%v", d)
	}
	log.Printf("  Domain: %v.%s", cfg["subdomain"], domain)

	// 3. Generate .env file
	envPath := fmt.Sprintf("%s/.env", projectDir)
	log.Println("Generating .env file...")
	if err := fetcher.GenerateEnvFile(cfg, envPath); err != nil {
		log.Fatalf("Failed to generate .env: %v", err)
	}
	log.Printf("✓ .env file generated: %s", envPath)

	// 3.5 Verify DNS Propagation
	// Read domain from Hub config (same logic as fetcher.go)
	baseDomain := "ytconvert.org" // Default fallback
	if domain, ok := cfg["domain"]; ok && domain != nil && domain != "" {
		baseDomain = fmt.Sprintf("%v", domain)
	}
	subdomain := fmt.Sprintf("%v", cfg["subdomain"])

	downloadDomain := fmt.Sprintf("%s.%s", subdomain, baseDomain)

	log.Println("Verifying DNS records...")
	waitForDNS(downloadDomain)
	log.Println("✓ DNS records verified")

	// 4. Create deployer and Control API (start API early so dashboard can connect)
	dep := deployer.NewDeployer(projectDir)
	controlAPI := control.NewControlAPI(fetcher, dep, projectDir)
	controlAPI.SetupRoutes()

	go func() {
		log.Printf("Starting Control API on port %d", agentPort)
		if err := controlAPI.Start(agentPort); err != nil {
			log.Fatalf("Control API failed: %v", err)
		}
	}()

	// 5. Start Heartbeat (early, so Hub knows agent is alive)
	hb := heartbeat.NewHeartbeat(hubURL, serverIP, projectDir, 10*time.Second)
	go hb.Start()

	// 6. Build & Deploy service (Control API already running, dashboard can monitor)
	log.Println("Building service...")
	controlAPI.SetBuildStatus("building", "Building services...")
	if err := dep.Build(); err != nil {
		controlAPI.SetBuildStatus("error", "Build failed: "+err.Error())
		log.Fatalf("Build failed: %v", err)
	}
	log.Println("✓ Build successful")

	log.Println("Deploying service...")
	controlAPI.SetBuildStatus("deploying", "Deploying services...")
	if err := dep.Deploy(); err != nil {
		controlAPI.SetBuildStatus("error", "Deploy failed: "+err.Error())
		log.Fatalf("Deploy failed: %v", err)
	}
	controlAPI.SetBuildStatus("success", "Services deployed successfully")
	log.Println("✓ Service deployed successfully")

	log.Println("✓ Agent running successfully")
	log.Println("  Control API: http://localhost:9000")
	log.Println("  Service: Running via docker-compose")

	// Keep running
	select {}
}
