package outcome

import (
	"encoding/json"
	"time"
)

// Outcome is the terminal result of an Episode.
type Outcome struct {
	ID        string `json:"id"`
	EpisodeID string `json:"episode_id"`
	TenantID  string `json:"tenant_id"`

	Status string `json:"status"`

	Result json.RawMessage `json:"result,omitempty"`

	Verified bool   `json:"verified"`
	Verifier string `json:"verifier,omitempty"`

	Metrics map[string]float64 `json:"metrics,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
