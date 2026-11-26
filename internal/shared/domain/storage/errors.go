package storage

import "errors"

var (
	ErrBusy        = errors.New("storage read concurrency limit reached")
	ErrUncertain   = errors.New("catalog write outcome is uncertain; reload before retrying")
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("record conflicts with an existing record or constraint")
	ErrUnavailable = errors.New("object storage is not configured")
)

type Error struct {
	Operation string
	Err       error
}

func (e *Error) Error() string { return "could not " + e.Operation + " in object storage" }
func (e *Error) Unwrap() error { return e.Err }
