package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tik-downloader/config"
	"tik-downloader/database"
	"tik-downloader/handlers"
	"tik-downloader/repository"
	"tik-downloader/services"
	"tik-downloader/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	log.Println("=== TikTok Downloader Starting ===")
	log.Printf("Port: %d", config.Port)
	log.Printf("Storage: %s", config.StorageDir)
	log.Printf("Extractor: %s", config.TikExtractorURL)
	log.Printf("Path Prefix: %s", config.PathPrefix)
	log.Printf("Domain: %s", config.Domain)

	// Ensure storage directory exists
	os.MkdirAll(config.StorageDir, 0755)

	// Initialize MongoDB
	log.Printf("Connecting to MongoDB: %s / %s", config.MongoURI, config.MongoDB)
	mongoDB, err := database.InitMongoDB(config.MongoURI, config.MongoDB)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoDB.Close()
	log.Println("✓ MongoDB connected")

	// Initialize cookie repository and provider
	cookieRepo := repository.NewCookieRepository(mongoDB.CookieCollection())
	services.InitCookieProvider(cookieRepo)
	defer services.StopCookieProvider()
	log.Println("✓ Cookie provider initialized")

	// Start cleanup scheduler
	cleanupCron := utils.StartCleanupScheduler()
	defer cleanupCron.Stop()
	log.Println("✓ Cleanup scheduler started")

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "TikTok-Downloader",
		ServerHeader: "TikTok-Downloader",
		BodyLimit:    10 * 1024 * 1024, // 10MB
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Hub-Token",
	}))

	// Routes
	app.Get("/health", handlers.HandleHealth)
	app.Post("/api/download", handlers.HandleDownload)
	app.Post("/api/info", handlers.HandleInfo)
	app.Get("/api/status/:id", handlers.HandleStatus)
	app.Get("/files/:id/:filename", handlers.HandleFiles)
	app.Get("/proxy/media", handlers.HandleProxyMedia)

	log.Println("=== Routes ===")
	log.Println("  POST /api/download     - Create download job")
	log.Println("  GET  /api/status/:id   - Check job status")
	log.Println("  GET  /files/:id/:file  - Download file")
	log.Println("  GET  /health           - Health check")
	log.Println("===============================")

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		app.Shutdown()
	}()

	// Start server
	addr := fmt.Sprintf(":%d", config.Port)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
