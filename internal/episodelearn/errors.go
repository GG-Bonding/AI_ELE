package episodelearn

import "errors"

// ErrNotFound is returned when no learning job exists for an episode.
var ErrNotFound = errors.New("episode learning job not found")
