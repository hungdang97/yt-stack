package control

import (
	"fmt"
	"log"

	"vps-agent/config"
	"vps-agent/deployer"

	"github.com/gofiber/fiber/v2"
)

type ControlAPI struct {
	fetcher    *config.ConfigFetcher
	deployer   *deployer.Deployer
	projectDir string
	app        *fiber.App
}

func NewControlAPI(fetcher *config.ConfigFetcher, deployer *deployer.Deployer, projectDir string) *ControlAPI {
	return &ControlAPI{
		fetcher:    fetcher,
		deployer:   deployer,
		projectDir: projectDir,
		app:        fiber.New(fiber.Config{DisableStartupMessage: true}),
	}
}

func (api *ControlAPI) SetupRoutes() {
	api.app.Get("/status", api.GetStatus)
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

// POST /control/restart
// LUÔN pull config mới từ Hub trước khi restart
func (api *ControlAPI) Restart(c *fiber.Ctx) error {
	log.Println("[Control] Received restart command")

	// 1. Pull latest config
	api.updateConfig()

	// 1.5 Pull latest code from Git
	log.Println("[Control] Pulling latest code...")
	if err := api.deployer.PullCode(); err != nil {
		log.Printf("[Control] Warning: Git pull failed: %v", err)
		// We continue even if pull fails, to at least try to rebuild with what we have
	}

	// 2. Stop service
	log.Println("[Control] Stopping service...")
	api.deployer.Stop()

	// 3. Build service
	log.Println("[Control] Building service...")
	if err := api.deployer.Build(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Build failed: " + err.Error()})
	}

	// 4. Deploy service
	log.Println("[Control] Deploying service...")
	if err := api.deployer.Deploy(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Deploy failed: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Config pulled and service rebuilt successfully"})
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
// It logs warnings on failure but does not return error to allow operations to proceed with existing config
func (api *ControlAPI) updateConfig() {
	log.Println("[Control] Fetching latest config from Hub...")
	config, err := api.fetcher.FetchConfig()
	if err != nil {
		log.Printf("[Control] Warning: Failed to fetch config, using existing .env: %v", err)
		return
	}

	envPath := fmt.Sprintf("%s/.env", api.projectDir)
	if err := api.fetcher.GenerateEnvFile(config, envPath); err != nil {
		log.Printf("[Control] Warning: Failed to update .env: %v", err)
	} else {
		log.Println("[Control] Config updated successfully")
	}
}
