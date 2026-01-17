package handlers

import (
	"yt-downloader-go/models"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleStatus handles GET /api/status/:id
// @Summary Get job status
// @Description Check the status and progress of a download job
// @Tags status
// @Produce json
// @Param id path string true "Job ID"
// @Param token query string true "Signed URL token"
// @Param expires query string true "Expiration timestamp"
// @Success 200 {object} models.StatusResponse
// @Failure 400 {object} utils.ErrorResponse "Invalid job ID"
// @Failure 401 {object} utils.ErrorResponse "Missing token or expires"
// @Failure 403 {object} utils.ErrorResponse "Invalid or expired token"
// @Failure 404 {object} utils.ErrorResponse "Job not found"
// @Failure 500 {object} utils.ErrorResponse "Server error"
// @Router /api/status/{id} [get]
func HandleStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")
	token := c.Query("token")
	expiresStr := c.Query("expires")

	// Validate job ID
	if !utils.ValidateJobID(jobID) {
		return utils.BadRequest(c, utils.ErrInvalidJobID, "Invalid job ID format")
	}

	// Check token and expires
	if token == "" || expiresStr == "" {
		return utils.Unauthorized(c, "Missing token or expires parameter")
	}

	// Parse expires
	expires, err := utils.ParseExpires(expiresStr)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidRequest, "Invalid expires format")
	}

	// Validate token
	if !utils.ValidateStatusURL(jobID, token, expires) {
		return utils.Forbidden(c, "Invalid or expired token")
	}

	// Check if job exists
	if !utils.JobExists(jobID) {
		return utils.NotFound(c, utils.ErrJobNotFound, "Job not found")
	}

	// Read metadata
	meta, err := utils.ReadMeta(jobID)
	if err != nil {
		return utils.InternalError(c, "Failed to read job metadata")
	}

	// Calculate progress
	progress := utils.CalculateProgress(meta)

	response := models.StatusResponse{
		Status:   meta.Status,
		Progress: progress,
		Title:    meta.Title,
		Duration: meta.Duration,
	}

	// Set downloadUrl when completed
	if meta.Status == models.StatusCompleted {
		response.Progress = 100
		if meta.Output != "" {
			// Merged file available - use static file URL
			response.DownloadURL = utils.GenerateSignedURL(jobID, meta.Output)
		} else if meta.StreamOnly {
			// Stream only - use stream URL
			response.DownloadURL = utils.GenerateStreamURL(jobID)
		}
	}

	// Set jobError when error
	if meta.Status == models.StatusError {
		response.JobError = meta.Error
	}

	return c.JSON(response)
}
