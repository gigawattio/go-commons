package errorlib

// Common errors.

import (
	"errors"
)

var (
	NotRunningError     = errors.New("not running")
	AlreadyRunningError = errors.New("already running")
	NotFoundError       = errors.New("not found")
	NotAuthorizedError  = errors.New("not authorized")
)
