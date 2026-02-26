package control

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"vps-agent/config"
	"vps-agent/deployer"

	"github.com/gofiber/fiber/v2"
)

type BuildState string

const (
	StateIdle        BuildState = "idle"
	StateConfiguring BuildState = "configuring"
	StatePulling     BuildState = "pulling"
	StateStopping    BuildState = "stopping"
	StateBuilding    BuildState = "building"
	StateDeploying   BuildState = "deploying"
	StateSuccess     BuildState = "success"
	StateError       BuildState = "error"
)

type BuildStatus struct {
	State   BuildState `json:"state"`
	Message string     `json:"message"`
	Time    time.Time  `json:"time"`
}

type ControlAPI struct {
	fetcher    *config.ConfigFetcher
	deployer   *deployer.Deployer
	projectDir string
	app        *fiber.App
	startTime  time.Time
	version    string

	statusMu      sync.RWMutex
	currentStatus BuildStatus
}

func NewControlAPI(fetcher *config.ConfigFetcher, deployer *deployer.Deployer, projectDir string) *ControlAPI {
	return &ControlAPI{
		fetcher:    fetcher,
		deployer:   deployer,
		projectDir: projectDir,
		app:        fiber.New(fiber.Config{DisableStartupMessage: true}),
		startTime:  time.Now(),
		version:    "1.0.0",
		currentStatus: BuildStatus{
			State:   StateIdle,
			Message: "Ready",
			Time:    time.Now(),
		},
	}
}

func (api *ControlAPI) SetupRoutes() {
	api.app.Get("/health", api.HealthCheck)
	api.app.Get("/status", api.GetStatus)
	api.app.Get("/control/build-status", api.GetBuildStatus)
	api.app.Post("/control/restart", api.Restart)
	api.app.Post("/control/restart/:service", api.RestartService)
	api.app.Post("/control/stop", api.Stop)
	api.app.Post("/control/update-agent", api.UpdateAgent)
}

// GET /status
func (api *ControlAPI) GetStatus(c *fiber.Ctx) error {
	status, err := api.deployer.GetStatus()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(status)
}

// GET /control/build-status
func (api *ControlAPI) GetBuildStatus(c *fiber.Ctx) error {
	api.statusMu.RLock()
	defer api.statusMu.RUnlock()
	return c.JSON(api.currentStatus)
}

// POST /control/restart
// Async restart trigger
func (api *ControlAPI) Restart(c *fiber.Ctx) error {
	api.statusMu.Lock()
	if api.currentStatus.State != StateIdle &&
		api.currentStatus.State != StateSuccess &&
		api.currentStatus.State != StateError {
		api.statusMu.Unlock()
		return c.Status(409).JSON(fiber.Map{"error": "Build already in progress"})
	}
	api.statusMu.Unlock()

	go api.runAsyncRestart()

	return c.JSON(fiber.Map{"message": "Restart process started"})
}

func (api *ControlAPI) setStatus(state BuildState, message string) {
	api.statusMu.Lock()
	defer api.statusMu.Unlock()
	api.currentStatus = BuildStatus{
		State:   state,
		Message: message,
		Time:    time.Now(),
	}
	log.Printf("[Control] Status: %s - %s", state, message)
}

func (api *ControlAPI) runAsyncRestart() {
	log.Println("[Control] Starting async restart sequence")

	// 1. Pull latest config
	api.setStatus(StateConfiguring, "Fetching latest configuration...")
	err := api.updateConfig()
	if err != nil {
		log.Printf("[Control] Warning: Config update failed: %v", err)
		// Enable to fail hard if config is critical, for now we proceed with warning
	}

	// 1.5 Pull latest code from Git
	api.setStatus(StatePulling, "Pulling latest code...")
	if err := api.deployer.PullCode(); err != nil {
		log.Printf("[Control] Warning: Git pull failed: %v", err)
	}

	// 2. Stop service
	api.setStatus(StateStopping, "Stopping current services...")
	// Don't check error strictly here, as services might already be down
	api.deployer.Stop()

	// 3. Build service
	api.setStatus(StateBuilding, "Building services (this may take a while)...")
	if err := api.deployer.Build(); err != nil {
		api.setStatus(StateError, "Build failed: "+err.Error())
		return
	}

	// 4. Deploy service
	api.setStatus(StateDeploying, "Deploying services...")
	if err := api.deployer.Deploy(); err != nil {
		api.setStatus(StateError, "Deploy failed: "+err.Error())
		return
	}

	api.setStatus(StateSuccess, "Services restarted successfully")
}

// POST /control/restart/:service - Restart a single service (git pull → build → up)
func (api *ControlAPI) RestartService(c *fiber.Ctx) error {
	service := c.Params("service")

	// Whitelist allowed services
	allowed := map[string]bool{
		"yt-downloader":  true,
		"yt-extractor":   true,
		"tik-downloader": true,
		"tik-extractor":  true,
		"nginx":          true,
		"gost":           true,
	}
	if !allowed[service] {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Unknown service: %s", service)})
	}

	log.Printf("[Control] Restart requested for service: %s", service)

	go func() {
		api.setStatus(StateBuilding, fmt.Sprintf("Restarting service: %s", service))
		if err := api.deployer.RestartService(service); err != nil {
			api.setStatus(StateError, fmt.Sprintf("Restart %s failed: %v", service, err))
			return
		}
		api.setStatus(StateSuccess, fmt.Sprintf("Service %s restarted successfully", service))
	}()

	return c.JSON(fiber.Map{"message": fmt.Sprintf("Restart %s started", service)})
}

// POST /control/stop
func (api *ControlAPI) Stop(c *fiber.Ctx) error {
	log.Println("[Control] Received stop command")

	if err := api.deployer.Stop(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Service stopped"})
}

func (api *ControlAPI) Start(port int) error {
	return api.app.Listen(fmt.Sprintf(":%d", port))
}

// updateConfig fetches the latest config from Hub and updates .env
func (api *ControlAPI) updateConfig() error {
	log.Println("[Control] Fetching latest config from Hub...")
	config, err := api.fetcher.FetchConfig()
	if err != nil {
		return err
	}

	envPath := fmt.Sprintf("%s/.env", api.projectDir)
	if err := api.fetcher.GenerateEnvFile(config, envPath); err != nil {
		return err
	}
	return nil
}

// GET /health - Agent health check endpoint
func (api *ControlAPI) HealthCheck(c *fiber.Ctx) error {
	// Get disk usage
	var stat syscall.Statfs_t
	diskFreeGB := float64(0)
	diskTotalGB := float64(0)
	if err := syscall.Statfs("/", &stat); err == nil {
		diskFreeGB = float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
		diskTotalGB = float64(stat.Blocks*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	}

	// Get container status
	containersOK := false
	if status, err := api.deployer.GetStatus(); err == nil {
		if healthy, ok := status["healthy"].(bool); ok {
			containersOK = healthy
		}
	}

	// Calculate uptime
	uptime := time.Since(api.startTime)

	return c.JSON(fiber.Map{
		"status":         "healthy",
		"version":        api.version,
		"uptime_seconds": int(uptime.Seconds()),
		"uptime":         uptime.String(),
		"disk_free_gb":   fmt.Sprintf("%.2f", diskFreeGB),
		"disk_total_gb":  fmt.Sprintf("%.2f", diskTotalGB),
		"containers_ok":  containersOK,
	})
}

// POST /control/update-agent - Self-update the agent
func (api *ControlAPI) UpdateAgent(c *fiber.Ctx) error {
	log.Println("[Control] Agent update requested")

	// Check if already in a build process
	api.statusMu.Lock()
	if api.currentStatus.State != StateIdle &&
		api.currentStatus.State != StateSuccess &&
		api.currentStatus.State != StateError {
		api.statusMu.Unlock()
		return c.Status(409).JSON(fiber.Map{"error": "Build in progress, try again later"})
	}
	api.statusMu.Unlock()

	// Run update script in background
	go func() {
		log.Println("[Control] Starting agent self-update...")
		updateScript := fmt.Sprintf("%s/vps-agent/update.sh", api.projectDir)

		cmd := exec.Command("bash", updateScript)
		cmd.Dir = api.projectDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			log.Printf("[Control] Update script failed: %v, output: %s", err, output)
			return
		}

		log.Printf("[Control] Update script completed: %s", output)
		// The script will restart the agent, so this goroutine will be killed
	}()

	return c.JSON(fiber.Map{
		"message": "Agent update started. Agent will restart shortly.",
	})
}
