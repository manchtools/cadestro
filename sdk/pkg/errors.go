package pkg

import "errors"

var (
	// ErrNotFound reports that the requested package is absent.
	ErrNotFound = errors.New("package not found")
	// ErrUnsupported reports that the backend cannot perform an operation.
	ErrUnsupported = errors.New("package operation unsupported")
	// ErrInvalidArgument reports an invalid package-manager argument.
	ErrInvalidArgument = errors.New("invalid package argument")
)
