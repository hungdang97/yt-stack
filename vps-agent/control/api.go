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
	"vps-agent/metrics"

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

// AllServices is the whitelist of restartable services
var AllServices = []string{
	"yt-downloader", "yt-extractor",
	"tik-downloader", "tik-extractor",
	"insta-downloader", "insta-extractor",
	"nginx", "gost",
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

	// Per-service build states
	serviceStatusMu sync.RWMutex
	serviceStatuses map[string]BuildStatus
}

func NewControlAPI(fetcher *config.ConfigFetcher, deployer *deployer.Deployer, projectDir string) *ControlAPI {
	// Initialize per-service statuses
	serviceStatuses := make(map[string]BuildStatus)
	for _, svc := range AllServices {
		serviceStatuses[svc] = BuildStatus{
			State:   StateIdle,
			Message: "Ready",
			Time:    time.Now(),
		}
	}

	return &ControlAPI{
		fetcher:         fetcher,
		deployer:        deployer,
		projectDir:      projectDir,
		app:             fiber.New(fiber.Config{DisableStartupMessage: true}),
		startTime:       time.Now(),
		version:         "1.0.0",
		serviceStatuses: serviceStatuses,
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
	api.app.Get("/services/health", api.GetServicesHealth)
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
	stackStatus := api.currentStatus
	api.statusMu.RUnlock()

	// Only return services in AllServices whitelist
	allowed := make(map[string]bool, len(AllServices))
	for _, svc := range AllServices {
		allowed[svc] = true
	}

	api.serviceStatusMu.RLock()
	servicesCopy := make(map[string]BuildStatus, len(AllServices))
	for k, v := range api.serviceStatuses {
		if allowed[k] {
			servicesCopy[k] = v
		}
	}
	api.serviceStatusMu.RUnlock()

	return c.JSON(fiber.Map{
		"stack":    stackStatus,
		"services": servicesCopy,
	})
}

// GET /services/health — per-service health check (calls each service's /health endpoint)
func (api *ControlAPI) GetServicesHealth(c *fiber.Ctx) error {
	serviceHealth := metrics.CollectServiceHealth()

	api.serviceStatusMu.RLock()
	servicesCopy := make(map[string]BuildStatus, len(api.serviceStatuses))
	for k, v := range api.serviceStatuses {
		servicesCopy[k] = v
	}
	api.serviceStatusMu.RUnlock()

	type ServiceDetail struct {
		Health     metrics.ServiceInfo `json:"health"`
		BuildState BuildStatus         `json:"build"`
	}

	result := make(map[string]ServiceDetail)
	for name, health := range serviceHealth {
		result[name] = ServiceDetail{
			Health:     health,
			BuildState: servicesCopy[name],
		}
	}

	return c.JSON(result)
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

func (api *ControlAPI) setServiceStatus(service string, state BuildState, message string) {
	api.serviceStatusMu.Lock()
	defer api.serviceStatusMu.Unlock()
	api.serviceStatuses[service] = BuildStatus{
		State:   state,
		Message: message,
		Time:    time.Now(),
	}
	log.Printf("[Control] Service %s: %s - %s", service, state, message)
}

// setAllServicesStatus sets the same state for all services (used during full stack operations)
func (api *ControlAPI) setAllServicesStatus(state BuildState, message string) {
	api.serviceStatusMu.Lock()
	defer api.serviceStatusMu.Unlock()
	now := time.Now()
	for _, svc := range AllServices {
		api.serviceStatuses[svc] = BuildStatus{
			State:   state,
			Message: message,
			Time:    now,
		}
	}
}

func (api *ControlAPI) runAsyncRestart() {
	log.Println("[Control] Starting async restart sequence")

	// 1. Pull latest config
	api.setStatus(StateConfiguring, "Fetching latest configuration...")
	api.setAllServicesStatus(StateConfiguring, "Fetching latest configuration...")
	err := api.updateConfig()
	if err != nil {
		log.Printf("[Control] Warning: Config update failed: %v", err)
	}

	// 1.5 Pull latest code from Git
	api.setStatus(StatePulling, "Pulling latest code...")
	api.setAllServicesStatus(StatePulling, "Pulling latest code...")
	if err := api.deployer.PullCode(); err != nil {
		log.Printf("[Control] Warning: Git pull failed: %v", err)
	}

	// 2. Stop service
	api.setStatus(StateStopping, "Stopping current services...")
	api.setAllServicesStatus(StateStopping, "Stopping...")
	api.deployer.Stop()

	// 3. Build service
	api.setStatus(StateBuilding, "Building services (this may take a while)...")
	api.setAllServicesStatus(StateBuilding, "Building...")
	if err := api.deployer.Build(); err != nil {
		api.setStatus(StateError, "Build failed: "+err.Error())
		api.setAllServicesStatus(StateError, "Build failed: "+err.Error())
		return
	}

	// 4. Deploy service
	api.setStatus(StateDeploying, "Deploying services...")
	api.setAllServicesStatus(StateDeploying, "Deploying...")
	if err := api.deployer.Deploy(); err != nil {
		api.setStatus(StateError, "Deploy failed: "+err.Error())
		api.setAllServicesStatus(StateError, "Deploy failed: "+err.Error())
		return
	}

	api.setStatus(StateSuccess, "Services restarted successfully")
	api.setAllServicesStatus(StateSuccess, "Restarted successfully")
}

// POST /control/restart/:service - Restart a single service (git pull → build → up)
func (api *ControlAPI) RestartService(c *fiber.Ctx) error {
	service := c.Params("service")

	// Whitelist allowed services
	allowed := make(map[string]bool, len(AllServices))
	for _, svc := range AllServices {
		allowed[svc] = true
	}
	if !allowed[service] {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Unknown service: %s", service)})
	}

	// Check if this service is already being built
	api.serviceStatusMu.RLock()
	svcStatus := api.serviceStatuses[service]
	api.serviceStatusMu.RUnlock()
	if svcStatus.State == StateBuilding || svcStatus.State == StateDeploying || svcStatus.State == StatePulling {
		return c.Status(409).JSON(fiber.Map{"error": fmt.Sprintf("Service %s is already being restarted", service)})
	}

	log.Printf("[Control] Restart requested for service: %s", service)

	// Set status SYNCHRONOUSLY before launching goroutine
	// so that any immediate status poll will see "pulling" instead of "idle"
	api.setServiceStatus(service, StatePulling, "Pulling latest code...")

	go func() {
		if err := api.deployer.PullCode(); err != nil {
			log.Printf("[Control] Warning: Git pull failed for %s: %v", service, err)
		}

		api.setServiceStatus(service, StateBuilding, "Building...")
		if err := api.deployer.RestartService(service); err != nil {
			api.setServiceStatus(service, StateError, fmt.Sprintf("Restart failed: %v", err))
			return
		}
		api.setServiceStatus(service, StateSuccess, "Restarted successfully")
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
