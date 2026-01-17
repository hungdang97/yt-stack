package handlers

import (
	"yt-downloader-go/models"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleDeleteJob handles DELETE /api/jobs/:id
// @Summary Delete job
// @Description Delete a job and its associated files
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.DeleteResponse
// @Failure 400 {object} utils.ErrorResponse "Invalid job ID"
// @Failure 404 {object} utils.ErrorResponse "Job not found"
// @Failure 500 {object} utils.ErrorResponse "Delete failed"
// @Router /api/jobs/{id} [delete]
func HandleDeleteJob(c *fiber.Ctx) error {
	jobID := c.Params("id")

	// Validate job ID
	if !utils.ValidateJobID(jobID) {
		return utils.BadRequest(c, utils.ErrInvalidJobID, "Invalid job ID format")
	}

	// Check if job exists
	if !utils.JobExists(jobID) {
		return utils.NotFound(c, utils.ErrJobNotFound, "Job not found")
	}

	// Delete job directory
	if err := utils.DeleteJobDir(jobID); err != nil {
		return utils.InternalError(c, "Failed to delete job")
	}

	return c.JSON(models.DeleteResponse{
		Deleted: true,
	})
}
