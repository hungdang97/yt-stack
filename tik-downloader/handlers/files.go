package handlers

import (
	"path/filepath"
	"tik-downloader/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleFiles handles GET /files/:id/:filename
func HandleFiles(c *fiber.Ctx) error {
	jobID := c.Params("id")
	filename := c.Params("filename")

	if !utils.ValidateJobID(jobID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid job ID",
		})
	}

	// Validate signed URL
	token := c.Query("token")
	expiresStr := c.Query("expires")

	if token == "" || expiresStr == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Token and expires are required",
		})
	}

	expires, err := utils.ParseExpires(expiresStr)
	if err != nil || !utils.ValidateSignedURL(jobID, filename, token, expires) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Check job exists
	meta, err := utils.ReadMeta(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Job not found",
		})
	}

	// Build file path
	filePath := filepath.Join(utils.GetJobDir(jobID), filename)

	// Verify file exists
	if utils.GetFileSize(filePath) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	// Set content type based on extension
	ext := filepath.Ext(filename)
	switch ext {
	case ".mp4":
		c.Set("Content-Type", "video/mp4")
	case ".mp3":
		c.Set("Content-Type", "audio/mpeg")
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	// Set download headers
	downloadName := meta.Title + ext
	c.Set("Content-Disposition", "attachment; filename=\""+downloadName+"\"")

	return c.SendFile(filePath)
}
