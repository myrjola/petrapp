package chatbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go"
)

// OpenAIErrorType represents different types of OpenAI API errors.
type OpenAIErrorType string

const (
	ErrorTypeRateLimit      OpenAIErrorType = "rate_limit"
	ErrorTypeQuotaExceeded  OpenAIErrorType = "quota_exceeded"
	ErrorTypeInvalidRequest OpenAIErrorType = "invalid_request"
	ErrorTypeAuthentication OpenAIErrorType = "authentication"
	ErrorTypePermission     OpenAIErrorType = "permission"
	ErrorTypeNotFound       OpenAIErrorType = "not_found"
	ErrorTypeServer         OpenAIErrorType = "server_error"
	ErrorTypeTimeout        OpenAIErrorType = "timeout"
	ErrorTypeUnknown        OpenAIErrorType = "unknown"
)

// OpenAIError wraps OpenAI API errors with additional context.
type OpenAIError struct {
	Type        OpenAIErrorType
	Message     string
	Code        string
	StatusCode  int
	RetryAfter  *time.Duration
	RequestID   string
	UserMessage string // User-friendly error message
	Retryable   bool
	OriginalErr error
}

func (e *OpenAIError) Error() string {
	return fmt.Sprintf("openai error (%s): %s", e.Type, e.Message)
}

func (e *OpenAIError) Unwrap() error {
	return e.OriginalErr
}

// ErrorHandler provides structured error handling for OpenAI API errors.
type ErrorHandler struct {
	logger *slog.Logger
}

// NewErrorHandler creates a new OpenAI error handler.
func NewErrorHandler(logger *slog.Logger) *ErrorHandler {
	return &ErrorHandler{
		logger: logger,
	}
}

// HandleOpenAIError processes OpenAI API errors and returns structured error information.
func (eh *ErrorHandler) HandleOpenAIError(ctx context.Context, err error, operation string) *OpenAIError {
	if err == nil {
		return nil
	}

	openaiErr := &OpenAIError{
		OriginalErr: err,
		Type:        ErrorTypeUnknown,
		Message:     err.Error(),
		UserMessage: "We're having trouble processing your request right now. Please try again.",
		Retryable:   false,
	}

	// Handle context timeouts
	if errors.Is(err, context.DeadlineExceeded) {
		openaiErr.Type = ErrorTypeTimeout
		openaiErr.Message = "Request timed out"
		openaiErr.UserMessage = "The request took too long to process. Please try again."
		openaiErr.Retryable = true
		eh.logError(ctx, openaiErr, operation)
		return openaiErr
	}

	if errors.Is(err, context.Canceled) {
		openaiErr.Type = ErrorTypeTimeout
		openaiErr.Message = "Request was canceled"
		openaiErr.UserMessage = "The request was canceled. Please try again."
		openaiErr.Retryable = true
		eh.logError(ctx, openaiErr, operation)
		return openaiErr
	}

	// Parse OpenAI API errors
	errMsg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "too many requests"):
		openaiErr.Type = ErrorTypeRateLimit
		openaiErr.Message = "API rate limit exceeded"
		openaiErr.UserMessage = "Too many requests. Please wait a moment before trying again."
		openaiErr.Retryable = true
		openaiErr.RetryAfter = eh.extractRetryAfter(err)

	case strings.Contains(errMsg, "quota") || strings.Contains(errMsg, "billing"):
		openaiErr.Type = ErrorTypeQuotaExceeded
		openaiErr.Message = "API quota exceeded"
		openaiErr.UserMessage = "Service temporarily unavailable. Please try again later."
		openaiErr.Retryable = false

	case strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "bad request"):
		openaiErr.Type = ErrorTypeInvalidRequest
		openaiErr.Message = "Invalid request format"
		openaiErr.UserMessage = "There was an issue with your request format. Please try rephrasing."
		openaiErr.Retryable = false

	case strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "api key"):
		openaiErr.Type = ErrorTypeAuthentication
		openaiErr.Message = "Authentication failed"
		openaiErr.UserMessage = "Authentication issue. Please contact support."
		openaiErr.Retryable = false

	case strings.Contains(errMsg, "forbidden") || strings.Contains(errMsg, "permission"):
		openaiErr.Type = ErrorTypePermission
		openaiErr.Message = "Permission denied"
		openaiErr.UserMessage = "Access denied. Please contact support."
		openaiErr.Retryable = false

	case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "model"):
		openaiErr.Type = ErrorTypeNotFound
		openaiErr.Message = "Model or resource not found"
		openaiErr.UserMessage = "The AI model is temporarily unavailable. Please try again."
		openaiErr.Retryable = true

	case strings.Contains(errMsg, "server") || strings.Contains(errMsg, "internal") ||
		strings.Contains(errMsg, "502") || strings.Contains(errMsg, "503") ||
		strings.Contains(errMsg, "504"):
		openaiErr.Type = ErrorTypeServer
		openaiErr.Message = "Server error"
		openaiErr.UserMessage = "The AI service is temporarily unavailable. Please try again."
		openaiErr.Retryable = true

	case strings.Contains(errMsg, "timeout"):
		openaiErr.Type = ErrorTypeTimeout
		openaiErr.Message = "Request timeout"
		openaiErr.UserMessage = "The request took too long. Please try again with a shorter message."
		openaiErr.Retryable = true

	default:
		// Keep default unknown error settings
		eh.logger.Warn("Unrecognized OpenAI error", "error", err.Error(), "operation", operation)
	}

	eh.logError(ctx, openaiErr, operation)
	return openaiErr
}

// ShouldRetry determines if an operation should be retried based on the error.
func (eh *ErrorHandler) ShouldRetry(err *OpenAIError, attemptCount int) (bool, time.Duration) {
	if err == nil || !err.Retryable || attemptCount >= 3 {
		return false, 0
	}

	// Use exponential backoff with jitter
	baseDelay := time.Second * time.Duration(1<<uint(attemptCount)) // 1s, 2s, 4s

	switch err.Type {
	case ErrorTypeRateLimit:
		if err.RetryAfter != nil {
			return true, *err.RetryAfter
		}
		return true, baseDelay + time.Second*5 // Extra delay for rate limits

	case ErrorTypeServer, ErrorTypeTimeout:
		return true, baseDelay

	case ErrorTypeNotFound:
		// Only retry once for model not found
		if attemptCount == 0 {
			return true, time.Second * 2
		}
		return false, 0

	default:
		return false, 0
	}
}

// FormatUserError returns a user-friendly error message.
func (eh *ErrorHandler) FormatUserError(err *OpenAIError, includeRetryInfo bool) string {
	message := err.UserMessage

	if includeRetryInfo && err.Retryable {
		if err.RetryAfter != nil {
			message += fmt.Sprintf(" Please wait %v before trying again.", err.RetryAfter.Round(time.Second))
		} else {
			message += " Please try again in a few moments."
		}
	}

	return message
}

// GetErrorMetrics returns error metrics for monitoring.
func (eh *ErrorHandler) GetErrorMetrics(err *OpenAIError) map[string]interface{} {
	return map[string]interface{}{
		"error_type":      string(err.Type),
		"retryable":       err.Retryable,
		"status_code":     err.StatusCode,
		"has_retry_after": err.RetryAfter != nil,
	}
}

func (eh *ErrorHandler) extractRetryAfter(err error) *time.Duration {
	// Try to extract Retry-After header from error message
	errMsg := err.Error()

	// Look for common patterns in OpenAI rate limit errors
	if strings.Contains(errMsg, "retry after") {
		// This is a simplified extraction - in a real implementation,
		// you'd parse the actual HTTP headers if available
		duration := time.Minute // Default retry after 1 minute
		return &duration
	}

	return nil
}

func (eh *ErrorHandler) logError(ctx context.Context, err *OpenAIError, operation string) {
	fields := []slog.Attr{
		slog.String("operation", operation),
		slog.String("error_type", string(err.Type)),
		slog.String("error_message", err.Message),
		slog.Bool("retryable", err.Retryable),
	}

	if err.StatusCode > 0 {
		fields = append(fields, slog.Int("status_code", err.StatusCode))
	}

	if err.Code != "" {
		fields = append(fields, slog.String("error_code", err.Code))
	}

	if err.RequestID != "" {
		fields = append(fields, slog.String("request_id", err.RequestID))
	}

	if err.RetryAfter != nil {
		fields = append(fields, slog.Duration("retry_after", *err.RetryAfter))
	}

	// Log level based on error severity
	switch err.Type {
	case ErrorTypeAuthentication, ErrorTypePermission, ErrorTypeQuotaExceeded:
		eh.logger.ErrorContext(ctx, "Critical OpenAI API error", fields...)
	case ErrorTypeRateLimit, ErrorTypeServer, ErrorTypeTimeout:
		eh.logger.WarnContext(ctx, "Retryable OpenAI API error", fields...)
	default:
		eh.logger.InfoContext(ctx, "OpenAI API error", fields...)
	}
}

// RetryableOperation wraps an operation with automatic retry logic.
func (eh *ErrorHandler) RetryableOperation(ctx context.Context, operation string, fn func() error) error {
	var lastErr *OpenAIError

	for attempt := 0; attempt < 3; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		openaiErr := eh.HandleOpenAIError(ctx, err, operation)
		lastErr = openaiErr

		shouldRetry, delay := eh.ShouldRetry(openaiErr, attempt)
		if !shouldRetry {
			return openaiErr
		}

		eh.logger.InfoContext(ctx, "Retrying OpenAI operation",
			"operation", operation,
			"attempt", attempt+1,
			"delay", delay,
			"error_type", string(openaiErr.Type))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return lastErr
}

// Circuit breaker states
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker provides circuit breaker functionality for OpenAI API calls.
type CircuitBreaker struct {
	state        CircuitState
	failureCount int
	lastFailure  time.Time
	threshold    int
	timeout      time.Duration
	logger       *slog.Logger
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(threshold int, timeout time.Duration, logger *slog.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		state:     CircuitClosed,
		threshold: threshold,
		timeout:   timeout,
		logger:    logger,
	}
}

// Execute runs a function through the circuit breaker.
func (cb *CircuitBreaker) Execute(ctx context.Context, operation string, fn func() error) error {
	if cb.state == CircuitOpen {
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = CircuitHalfOpen
			cb.logger.InfoContext(ctx, "Circuit breaker half-open", "operation", operation)
		} else {
			return &OpenAIError{
				Type:        ErrorTypeServer,
				Message:     "Circuit breaker is open",
				UserMessage: "Service temporarily unavailable. Please try again later.",
				Retryable:   true,
			}
		}
	}

	err := fn()
	if err != nil {
		cb.failureCount++
		cb.lastFailure = time.Now()

		if cb.failureCount >= cb.threshold {
			cb.state = CircuitOpen
			cb.logger.WarnContext(ctx, "Circuit breaker opened",
				"operation", operation,
				"failure_count", cb.failureCount)
		}

		return err
	}

	// Success - reset failure count and close circuit
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.logger.InfoContext(ctx, "Circuit breaker closed", "operation", operation)
	}
	cb.failureCount = 0

	return nil
}
