package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"vps-agent/config"
	"vps-agent/control"
	"vps-agent/deployer"
	"vps-agent/heartbeat"
)

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
	log.Printf("  Domain: %v.ytconvert.org", cfg["subdomain"])

	// 3. Generate .env file
	envPath := fmt.Sprintf("%s/.env", projectDir)
	log.Println("Generating .env file...")
	if err := fetcher.GenerateEnvFile(cfg, envPath); err != nil {
		log.Fatalf("Failed to generate .env: %v", err)
	}
	log.Printf("✓ .env file generated: %s", envPath)

	// 4. Build & Deploy service
	dep := deployer.NewDeployer(projectDir)

	log.Println("Building service...")
	if err := dep.Build(); err != nil {
		log.Fatalf("Build failed: %v", err)
	}
	log.Println("✓ Build successful")

	log.Println("Deploying service...")
	if err := dep.Deploy(); err != nil {
		log.Fatalf("Deploy failed: %v", err)
	}
	log.Println("✓ Service deployed successfully")

	// 5. Start Control API
	controlAPI := control.NewControlAPI(fetcher, dep, projectDir)
	controlAPI.SetupRoutes()

	go func() {
		log.Printf("Starting Control API on port %d", agentPort)
		if err := controlAPI.Start(agentPort); err != nil {
			log.Fatalf("Control API failed: %v", err)
		}
	}()

	// 6. Start Heartbeat
	hb := heartbeat.NewHeartbeat(hubURL, serverIP, 30*time.Second)
	go hb.Start()

	log.Println("✓ Agent running successfully")
	log.Println("  Control API: http://localhost:9000")
	log.Println("  Service: Running via docker-compose")

	// Keep running
	select {}
}
