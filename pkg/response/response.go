package response

import (
	"github.com/gofiber/fiber/v2"
)

// Standard API response structure for consistency across all endpoints.

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type Meta struct {
	Page       int   `json:"page,omitempty"`
	PerPage    int   `json:"per_page,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
}

// Success sends a 200 OK response with data.
func OK(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(APIResponse{
		Success: true,
		Data:    data,
	})
}

// Created sends a 201 Created response.
func Created(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(APIResponse{
		Success: true,
		Data:    data,
	})
}

// Accepted sends a 202 Accepted response (for async operations like email send).
func Accepted(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusAccepted).JSON(APIResponse{
		Success: true,
		Data:    data,
	})
}

// Paginated sends a 200 OK response with data and pagination metadata.
func Paginated(c *fiber.Ctx, data interface{}, meta Meta) error {
	return c.Status(fiber.StatusOK).JSON(APIResponse{
		Success: true,
		Data:    data,
		Meta:    &meta,
	})
}

// BadRequest sends a 400 error response.
func BadRequest(c *fiber.Ctx, code, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: code, Message: message},
	})
}

// Unauthorized sends a 401 error response.
func Unauthorized(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: "UNAUTHORIZED", Message: message},
	})
}

// Forbidden sends a 403 error response.
func Forbidden(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusForbidden).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: "FORBIDDEN", Message: message},
	})
}

// NotFound sends a 404 error response.
func NotFound(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusNotFound).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: "NOT_FOUND", Message: message},
	})
}

// Conflict sends a 409 error response.
func Conflict(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusConflict).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: "CONFLICT", Message: message},
	})
}

// TooManyRequests sends a 429 error response.
func TooManyRequests(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusTooManyRequests).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: "RATE_LIMITED", Message: message},
	})
}

// InternalError sends a 500 error response.
func InternalError(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(APIResponse{
		Success: false,
		Error:   &APIError{Code: "INTERNAL_ERROR", Message: message},
	})
}

// ValidationError sends a 422 error response with field-level details.
func ValidationError(c *fiber.Ctx, details interface{}) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(APIResponse{
		Success: false,
		Error: &APIError{
			Code:    "VALIDATION_ERROR",
			Message: "Request validation failed",
			Details: details,
		},
	})
}
