package experience

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func normFingerprintField(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Fingerprint returns a stable dedup key for a candidate within an episode.
func Fingerprint(c Candidate) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s",
		string(c.Type),
		string(c.Scope),
		normFingerprintField(c.ScopeKey),
		normFingerprintField(c.Trigger),
		normFingerprintField(c.Content),
	)
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}
