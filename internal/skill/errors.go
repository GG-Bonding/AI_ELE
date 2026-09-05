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
	// ErrInvalidTransition is returned for illegal lifecycle state changes.
	ErrInvalidTransition = errors.New("invalid skill lifecycle transition")
	// ErrPromotionGate is returned when shadow/live promotion thresholds are unmet.
	ErrPromotionGate = errors.New("skill promotion gate not met")
	// ErrDuplicateLearning is returned when a learning event for feedback+version exists.
	ErrDuplicateLearning = errors.New("duplicate skill learning event")
	// ErrLearningNotFound is returned when a skill learning event is missing.
	ErrLearningNotFound = errors.New("skill learning event not found")
)
