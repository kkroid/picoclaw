package runs

import "errors"

var (
	ErrRunNotFound    = errors.New("run not found")
	ErrInvalidRequest = errors.New("invalid request")
	ErrTerminalRun    = errors.New("run already in terminal state")
)
