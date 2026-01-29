package control

import (
	"fmt"
	"log"
	"sync"
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

	statusMu      sync.RWMutex
	currentStatus BuildStatus
}

func NewControlAPI(fetcher *config.ConfigFetcher, deployer *deployer.Deployer, projectDir string) *ControlAPI {
	return &ControlAPI{
		fetcher:    fetcher,
		deployer:   deployer,
		projectDir: projectDir,
		app:        fiber.New(fiber.Config{DisableStartupMessage: true}),
		currentStatus: BuildStatus{
			State:   StateIdle,
			Message: "Ready",
			Time:    time.Now(),
		},
	}
}

func (api *ControlAPI) SetupRoutes() {
	api.app.Get("/status", api.GetStatus)
	api.app.Get("/control/build-status", api.GetBuildStatus)
	api.app.Post("/control/restart", api.Restart)
	api.app.Post("/control/stop", api.Stop)
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
