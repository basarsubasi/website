package domain

import "errors"

var (
	ErrBusy        = errors.New("storage read concurrency limit reached")
	ErrUncertain   = errors.New("catalog write outcome is uncertain; reload before retrying")
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("record conflicts with an existing record or constraint")
	ErrUnavailable = errors.New("object storage is not configured")
	ErrTooLarge    = errors.New("photo must be between 1 byte and 25 MiB")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func Invalid(message string) error       { return &ValidationError{Message: message} }

type StorageError struct {
	Operation string
	Err       error
}

func (e *StorageError) Error() string { return "could not " + e.Operation + " in object storage" }
func (e *StorageError) Unwrap() error { return e.Err }
