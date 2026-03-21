package casdk

import (
	"errors"
	"fmt"
)

var (
	// ErrCLINotFound is returned when the Claude Code CLI is not installed.
	ErrCLINotFound = errors.New("casdk: Claude Code CLI not found")

	// ErrCLIConnection is returned when the connection to the CLI fails.
	ErrCLIConnection = errors.New("casdk: connection to CLI failed")

	// ErrSessionClosed is returned when an operation is attempted on a closed session.
	ErrSessionClosed = errors.New("casdk: session is closed")
)

// ProcessError is returned when the Claude Code process exits with an error.
type ProcessError struct {
	ExitCode int
	Stderr   string
}

func (e *ProcessError) Error() string {
	return fmt.Sprintf("casdk: process exited with code %d: %s", e.ExitCode, e.Stderr)
}
