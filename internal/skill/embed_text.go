package skill

import (
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// EmbeddingText builds the document embedded for SkillVersion semantic retrieval.
func EmbeddingText(sk Skill, ver Version) string {
	var tools []string
	var inputs []string
	for _, st := range ver.Spec.Steps {
		if st.Tool != "" {
			tools = append(tools, st.Tool)
		}
	}
	for name := range ver.Spec.Inputs {
		inputs = append(inputs, name)
	}
	parts := []string{
		sk.Name,
		sk.Description,
		ver.Spec.Name,
		ver.Spec.Description,
		strings.Join(tools, " "),
		strings.Join(inputs, " "),
	}
	return strings.Join(parts, "\n")
}

func cosineSim(a, b []float32) float64 {
	return experience.CosineSimilarity(a, b)
}
