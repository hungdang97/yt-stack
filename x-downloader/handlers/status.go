package handlers

import (
	"x-downloader/models"
	"x-downloader/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleStatus handles GET /api/status/:id
func HandleStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")

	if !utils.ValidateJobID(jobID) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid job ID",
		})
	}

	// Validate signed URL token
	token := c.Query("token")
	expiresStr := c.Query("expires")
	if token != "" && expiresStr != "" {
		expires, err := utils.ParseExpires(expiresStr)
		if err != nil || !utils.ValidateStatusURL(jobID, token, expires) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}
	}

	// Read meta
	meta, err := utils.ReadMeta(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Job not found",
		})
	}

	// Build response
	response := models.StatusResponse{
		Status:    meta.Status,
		Progress:  utils.CalculateProgress(meta),
		Title:     meta.Title,
		Duration:  meta.Duration,
		Thumbnail: meta.ThumbnailURL,
	}

	if meta.Status == models.StatusCompleted {
		response.Progress = 100
		response.DownloadURL = utils.GenerateSignedURL(meta.ID, meta.Output)
	}

	if meta.Status == models.StatusError {
		response.Error = meta.Error
	}

	return c.JSON(response)
}
