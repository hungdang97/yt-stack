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
	api.app.Post("/control/rebuild", api.Rebuild)
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

	// 1. Fetch latest config from Hub
	log.Println("[Control] Fetching latest config from Hub...")
	config, err := api.fetcher.FetchConfig()
	if err != nil {
		log.Printf("[Control] Warning: Failed to fetch config, using existing .env: %v", err)
	} else {
		// 2. Generate new .env
		envPath := fmt.Sprintf("%s/.env", api.projectDir)
		if err := api.fetcher.GenerateEnvFile(config, envPath); err != nil {
			log.Printf("[Control] Warning: Failed to update .env: %v", err)
		} else {
			log.Println("[Control] Config updated successfully")
		}
	}

	// 3. Restart service
	if err := api.deployer.Restart(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Restart failed: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Config pulled and service restarted successfully"})
}

// POST /control/rebuild
func (api *ControlAPI) Rebuild(c *fiber.Ctx) error {
	log.Println("[Control] Received rebuild command")

	api.deployer.Stop()

	if err := api.deployer.Build(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Build failed: " + err.Error()})
	}

	if err := api.deployer.Deploy(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Deploy failed: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Service rebuilt and deployed successfully"})
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
