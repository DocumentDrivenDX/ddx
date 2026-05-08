package cmd

import (
	"fmt"
	"os"
	"strings"
)

// Exit codes as per CLI contract
const (
	ExitCodeSuccess         = 0
	ExitCodeGeneralError    = 1
	ExitCodeMissingArg      = 2
	ExitCodeNoConfig        = 3
	ExitCodeInvalidConfig   = 4
	ExitCodeNetworkError    = 5
	ExitCodePersonaNotFound = 6
	ExitCodeBindingExists   = 7
	ExitCodeNoBindings      = 8
)

// ExitError represents an error with a specific exit code
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string {
	return e.Message
}

// NewExitError creates a new exit error
func NewExitError(code int, message string) *ExitError {
	return &ExitError{
		Code:    code,
		Message: message,
	}
}

// HandleError processes command errors and exits with appropriate code
func HandleError(err error) {
	if err == nil {
		os.Exit(ExitCodeSuccess)
		return
	}

	// Check if it's an ExitError
	if exitErr, ok := err.(*ExitError); ok {
		if exitErr.Message != "" {
			_, _ = fmt.Fprintln(os.Stderr, exitErr.Message)
		}
		os.Exit(exitErr.Code)
		return
	}

	// Default error handling
	_, _ = fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(ExitCodeGeneralError)
}

// CheckPersonaNotFound wraps persona not found errors
func CheckPersonaNotFound(err error, personaName string) error {
	if err != nil {
		// Check if it's a persona not found error
		if strings.Contains(err.Error(), fmt.Sprintf("persona '%s' not found", personaName)) ||
			strings.Contains(err.Error(), "not found at personas/") {
			return NewExitError(ExitCodePersonaNotFound,
				fmt.Sprintf("Persona '%s' not found", personaName))
		}
	}
	return err
}

// CheckNoConfig wraps no configuration errors
func CheckNoConfig(err error) error {
	if err != nil {
		// Check various no config scenarios
		if strings.Contains(err.Error(), "no .ddx.yml configuration found") ||
			strings.Contains(err.Error(), "No such file or directory") {
			return NewExitError(ExitCodeNoConfig,
				"No .ddx.yml configuration found")
		}
	}
	return err
}
