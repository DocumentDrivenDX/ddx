package cmd

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
