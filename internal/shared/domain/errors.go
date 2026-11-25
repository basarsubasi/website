package domain

import (
	"errors"
)

const (
	busyMessage          = "storage read concurrency limit reached"
	uncertainMessage     = "catalog write outcome is uncertain; reload before retrying"
	notFoundMessage      = "not found"
	conflictMessage      = "record conflicts with an existing record or constraint"
	unavailableMessage   = "object storage is not configured"
	videoTooLargeMessage = "Video must be at most 25 MiB"
	tooLargeMessage      = "photo must be between 1 byte and 25 MiB"
	storageErrorPrefix   = "could not "
	storageErrorSuffix   = " in object storage"
)

var (
	ErrBusy          = errors.New(busyMessage)
	ErrUncertain     = errors.New(uncertainMessage)
	ErrNotFound      = errors.New(notFoundMessage)
	ErrConflict      = errors.New(conflictMessage)
	ErrUnavailable   = errors.New(unavailableMessage)
	ErrVideoTooLarge = errors.New(videoTooLargeMessage)
	ErrTooLarge      = errors.New(tooLargeMessage)
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func Invalid(message string) error       { return &ValidationError{Message: message} }

type StorageError struct {
	Operation string
	Err       error
}

func (e *StorageError) Error() string { return storageErrorPrefix + e.Operation + storageErrorSuffix }
func (e *StorageError) Unwrap() error { return e.Err }
