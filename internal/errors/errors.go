// Package errors provides custom error types and error handling utilities for the MarkGo blog engine.
package errors

import (
	"errors"
	"fmt"
	"os"
)

// Domain-specific sentinel errors — only those actually used by the codebase.
var (
	ErrArticleNotFound    = errors.New("article not found")
	ErrInvalidFrontMatter = errors.New("invalid front matter")
	ErrEmailNotConfigured = errors.New("email credentials not configured")
	ErrSMTPAuthFailed     = errors.New("SMTP authentication failed")
	ErrTemplateNotFound   = errors.New("template not found")
	ErrTemplateParseError = errors.New("template parse error")
	ErrCLIValidation      = errors.New("CLI validation failed")
	ErrConfigValidation   = errors.New("configuration validation failed")
	ErrMissingConfig      = errors.New("missing required configuration")
	ErrPathEscape         = errors.New("path escapes containment base")
)

// ConfigError represents a configuration-related error
type ConfigError struct {
	Field   string
	Value   interface{}
	Message string
	Err     error
}

func (e *ConfigError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("config error for field %s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("config error: %s", e.Message)
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}

// NewConfigError creates a new ConfigError
func NewConfigError(field string, value interface{}, message string, err error) *ConfigError {
	return &ConfigError{
		Field:   field,
		Value:   value,
		Message: message,
		Err:     err,
	}
}

// ValidationError represents a validation error with field-specific details
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error for field %s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// NewValidationError creates a new ValidationError
func NewValidationError(field string, value interface{}, message string, err error) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
		Err:     err,
	}
}

// HTTPError represents an HTTP-specific error with status code
type HTTPError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

// NewHTTPError creates a new HTTPError
func NewHTTPError(statusCode int, message string, err error) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
		Err:        err,
	}
}

// CLIError represents a CLI-specific error with operation context
type CLIError struct {
	Operation string
	Message   string
	Err       error
	ExitCode  int
}

func (e *CLIError) Error() string {
	if e.Operation != "" {
		return fmt.Sprintf("CLI error during %s: %s", e.Operation, e.Message)
	}
	return fmt.Sprintf("CLI error: %s", e.Message)
}

func (e *CLIError) Unwrap() error {
	return e.Err
}

// NewCLIError creates a new CLIError
func NewCLIError(operation, message string, err error, exitCode int) *CLIError {
	return &CLIError{
		Operation: operation,
		Message:   message,
		Err:       err,
		ExitCode:  exitCode,
	}
}

// IsNotFound checks if an error indicates a "not found" condition
func IsNotFound(err error) bool {
	return errors.Is(err, ErrArticleNotFound) ||
		errors.Is(err, ErrTemplateNotFound)
}

// IsValidationError checks if an error is a validation-related error
func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidFrontMatter)
}

// IsConfigurationError checks if an error is configuration-related
func IsConfigurationError(err error) bool {
	return errors.Is(err, ErrConfigValidation) ||
		errors.Is(err, ErrMissingConfig)
}

// IsArticleNotFound checks if an error indicates an article was not found
func IsArticleNotFound(err error) bool {
	return errors.Is(err, ErrArticleNotFound)
}

// HandleCLIError handles CLI errors gracefully with optional cleanup
func HandleCLIError(err error, cleanup func()) {
	if err == nil {
		return
	}

	var exitCode = 1
	var message string

	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		exitCode = cliErr.ExitCode
		message = cliErr.Message
	} else {
		message = err.Error()
	}

	if cleanup != nil {
		cleanup()
	}
	if message != "" {
		fmt.Printf("Error: %s\n", message)
	}
	os.Exit(exitCode)
}
