package utils

import "github.com/gofiber/fiber/v2"

// Error codes
const (
	ErrInvalidRequest  = "INVALID_REQUEST"
	ErrValidationError = "VALIDATION_ERROR"
	ErrInvalidURL      = "INVALID_URL"
	ErrInvalidJobID    = "INVALID_JOB_ID"
	ErrInvalidFilename = "INVALID_FILENAME"
	ErrInvalidExpires  = "INVALID_EXPIRES"
	ErrJobNotReady     = "JOB_NOT_READY"
	ErrUnauthorized    = "UNAUTHORIZED"
	ErrForbidden       = "FORBIDDEN"
	ErrJobNotFound     = "JOB_NOT_FOUND"
	ErrVideoNotFound   = "VIDEO_NOT_FOUND"
	ErrAudioNotFound   = "AUDIO_NOT_FOUND"
	ErrFileNotFound    = "FILE_NOT_FOUND"
	ErrInternalError   = "INTERNAL_ERROR"
	ErrExtractFailed   = "EXTRACT_FAILED"
)

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error returns a JSON error response
func Error(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// BadRequest returns 400 error
func BadRequest(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusBadRequest, code, message)
}

// Unauthorized returns 401 error
func Unauthorized(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusUnauthorized, ErrUnauthorized, message)
}

// Forbidden returns 403 error
func Forbidden(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusForbidden, ErrForbidden, message)
}

// NotFound returns 404 error
func NotFound(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusNotFound, code, message)
}

// InternalError returns 500 error
func InternalError(c *fiber.Ctx, message string) error {
	return Error(c, fiber.StatusInternalServerError, ErrInternalError, message)
}
