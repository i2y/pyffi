package oasdk

import "errors"

var (
	// ErrMaxTurnsExceeded is returned when the agent exceeds the maximum number of turns.
	ErrMaxTurnsExceeded = errors.New("oasdk: max turns exceeded")
)
