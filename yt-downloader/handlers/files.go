package handlers

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"yt-downloader-go/models"
	"yt-downloader-go/utils"

	"github.com/gofiber/fiber/v2"
)

// HandleFiles handles GET /files/:id/:filename
// @Summary Download file
// @Description Download the merged output file
// @Tags files
// @Produce octet-stream
// @Param id path string true "Job ID"
// @Param filename path string true "Output filename"
// @Param token query string true "Signed URL token"
// @Param expires query integer true "Expiration timestamp"
// @Success 200 {file} binary "Output file"
// @Failure 400 {object} utils.ErrorResponse "Invalid parameters"
// @Failure 401 {object} utils.ErrorResponse "Missing auth"
// @Failure 403 {object} utils.ErrorResponse "Invalid token"
// @Failure 404 {object} utils.ErrorResponse "Not found"
// @Router /files/{id}/{filename} [get]
func HandleFiles(c *fiber.Ctx) error {
	jobID := c.Params("id")
	filename := c.Params("filename")
	token := c.Query("token")
	expiresStr := c.Query("expires")

	// Validate job ID
	if !utils.ValidateJobID(jobID) {
		return utils.BadRequest(c, utils.ErrInvalidJobID, "Invalid job ID format")
	}

	// Validate filename (prevent path traversal)
	if !utils.ValidateFilename(filename) {
		return utils.BadRequest(c, utils.ErrInvalidFilename, "Invalid filename")
	}

	// Validate signed URL
	if token == "" || expiresStr == "" {
		return utils.Unauthorized(c, "Missing token or expires parameter")
	}

	expires, err := utils.ParseExpires(expiresStr)
	if err != nil {
		return utils.BadRequest(c, utils.ErrInvalidExpires, "Invalid expires parameter")
	}

	if !utils.ValidateSignedURL(jobID, filename, token, expires) {
		return utils.Forbidden(c, "Invalid or expired download link")
	}

	// Check if job exists
	if !utils.JobExists(jobID) {
		return utils.NotFound(c, utils.ErrJobNotFound, "Job not found")
	}

	// Read metadata to get actual output filename
	meta, err := utils.ReadMeta(jobID)
	if err != nil {
		return utils.InternalError(c, "Failed to read job metadata")
	}

	// Check if job is completed
	if meta.Status != models.StatusCompleted {
		return utils.BadRequest(c, utils.ErrJobNotReady, "Job is not completed yet")
	}

	// Build file path
	filePath := filepath.Join(utils.GetJobDir(jobID), filename)

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		return utils.NotFound(c, utils.ErrFileNotFound, "File not found")
	}

	// Get content type
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	contentType := utils.ContentTypeFromExt(ext)

	// Generate download filename
	downloadFilename := utils.GenerateOutputFilename(meta)

	// RFC 5987 encoding for non-ASCII characters
	encodedFilename := url.PathEscape(downloadFilename)

	// Set headers
	c.Set("Content-Type", contentType)
	c.Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, downloadFilename, encodedFilename))

	// Stream file
	return c.SendFile(filePath)
}
