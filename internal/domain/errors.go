package domain

import "errors"

var (
	// ErrInvalidPassword is returned when a .depmap key container cannot be unlocked due to an invalid password.
	ErrInvalidPassword = errors.New("invalid password")

	// ErrUnsupportedFormat is returned when a file extension or format is not supported.
	ErrUnsupportedFormat = errors.New("unsupported file format")

	// ErrOperationCanceled is returned when an operation is cancelled via context.
	ErrOperationCanceled = errors.New("operation canceled")
)
