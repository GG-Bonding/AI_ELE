package extractor

import (
	_ "embed"
	"fmt"
)

//go:embed schema/experience_candidates.json
var experienceCandidatesSchemaJSON []byte

// SchemaJSON returns the embedded JSON Schema used for extractor validation.
func SchemaJSON() []byte {
	out := make([]byte, len(experienceCandidatesSchemaJSON))
	copy(out, experienceCandidatesSchemaJSON)
	return out
}

// EnsureSchemaPresent fails fast if the embedded schema is missing/empty.
func EnsureSchemaPresent() error {
	if len(experienceCandidatesSchemaJSON) == 0 {
		return fmt.Errorf("embedded experience candidates schema is empty")
	}
	return nil
}
