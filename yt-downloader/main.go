package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"yt-downloader-go/config"
	_ "yt-downloader-go/docs"
	"yt-downloader-go/handlers"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"
)

// @title YT Downloader API
// @version 2.0.0
// @description API for downloading YouTube videos and audio
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@ytconvert.org

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host api.ytconvert.org
// @BasePath /
// @schemes https http

func main() {
	if err := os.MkdirAll(config.StorageDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create storage directory: %v", err))
	}

	// Start cleanup scheduler
	cleanupCron := utils.StartCleanupScheduler()
	defer cleanupCron.Stop()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:       "YouTube Downloader Go",
		ServerHeader:  "yt-downloader-go",
		CaseSensitive: true,
		StrictRouting: false,
		// Disable body limit for file streaming
		BodyLimit: 0,
		// Enable IPv6 (dual-stack)
		Network: "tcp",
	})

	// Middleware
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,DELETE,OPTIONS",
		AllowHeaders: "Content-Type,Accept,X-Hub-Token",
	}))

	// Swagger docs
	app.Get("/swagger/*", swagger.HandlerDefault)

	// API routes
	api := app.Group("/api")
	api.Post("/download", handlers.HandleDownload)
	api.Post("/info", handlers.HandleInfo)
	api.Get("/status/:id", handlers.HandleStatus)
	api.Delete("/jobs/:id", handlers.HandleDeleteJob)

	// File serving
	app.Get("/files/:id/:filename", handlers.HandleFiles)

	// Stream serving (FFmpeg pipe)
	app.Get("/stream/:id", handlers.HandleStream)

	// Health check
	app.Get("/health", handlers.HandleHealth)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		app.Shutdown()
	}()

	addr := fmt.Sprintf(":%d", config.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("Failed to create listener: %v", err))
	}

	if err := app.Listener(ln); err != nil {
		panic(fmt.Sprintf("Failed to start server: %v", err))
	}
}
