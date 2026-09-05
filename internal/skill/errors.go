package skill

import "errors"

var (
	// ErrInvalidSpec is returned when YAML/Spec cannot be normalized.
	ErrInvalidSpec = errors.New("invalid skill spec")
	// ErrNotFound is returned when a skill or version is missing.
	ErrNotFound = errors.New("skill not found")
	// ErrConflict is returned on unique constraint violations.
	ErrConflict = errors.New("skill conflict")
	// ErrInvalidInput is returned for bad repository inputs.
	ErrInvalidInput = errors.New("invalid skill input")
)
