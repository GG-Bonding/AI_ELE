package experience

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Skill proposal gates (V2-9).
const (
	MinSkillPatternUtility = 0.75
	MinSkillPatternSupport = 3
)

// SkillDraft is a proposed SkillCandidate body before persistence.
type SkillDraft struct {
	Name        string
	Description string
	SpecYAML    string
	Confidence  float64
	Utility     float64
}

// SkillBuilder turns a Pattern into a Skill Candidate draft.
type SkillBuilder interface {
	Build(p Pattern) (SkillDraft, error)
}

// HeuristicSkillBuilder produces a deterministic YAML skill blueprint without an LLM.
type HeuristicSkillBuilder struct{}

// Build implements SkillBuilder.
func (HeuristicSkillBuilder) Build(p Pattern) (SkillDraft, error) {
	name := skillNameFromPattern(p)
	desc := strings.TrimSpace(p.Content)
	if desc == "" {
		desc = strings.TrimSpace(p.Trigger)
	}
	if desc == "" {
		desc = "Apply the generalized pattern when the trigger matches."
	}

	yaml := buildSkillYAML(name, p)
	return SkillDraft{
		Name:        name,
		Description: desc,
		SpecYAML:    yaml,
		Confidence:  p.Confidence,
		Utility:     p.Utility,
	}, nil
}

func skillProposeGate(p Pattern) string {
	if !p.Status.Retrievable() {
		return fmt.Sprintf("pattern status %s is not eligible", p.Status)
	}
	// Prefer ACTIVE patterns; allow strong CANDIDATE as a soft path.
	if p.Status != PatternStatusActive {
		if p.Utility < MinSkillPatternUtility {
			return fmt.Sprintf("non-ACTIVE pattern utility %.3f below %.2f", p.Utility, MinSkillPatternUtility)
		}
		if p.SupportCount < MinSkillPatternSupport {
			return fmt.Sprintf("non-ACTIVE pattern support %d below %d", p.SupportCount, MinSkillPatternSupport)
		}
	}
	if p.Utility < MinSkillPatternUtility && p.Status != PatternStatusActive {
		return fmt.Sprintf("pattern utility %.3f below %.2f", p.Utility, MinSkillPatternUtility)
	}
	return ""
}

func skillNameFromPattern(p Pattern) string {
	parts := make([]string, 0, 4)
	if p.ScopeKey != "" {
		parts = append(parts, slugToken(p.ScopeKey))
	} else {
		parts = append(parts, strings.ToLower(string(p.Scope)))
	}
	parts = append(parts, strings.ToLower(string(p.Type)))
	trig := slugToken(firstWords(p.Trigger, 4))
	if trig != "" {
		parts = append(parts, trig)
	}
	name := strings.Join(filterEmpty(parts), "_")
	if name == "" {
		name = "pattern_skill"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func buildSkillYAML(name string, p Pattern) string {
	var b strings.Builder
	b.WriteString("skill:\n")
	b.WriteString("  name: " + yamlScalar(name) + "\n")
	b.WriteString("  auto_execute: false\n")
	b.WriteString("  source_pattern_id: " + yamlScalar(p.ID) + "\n")
	b.WriteString("  when: " + yamlScalar(strings.TrimSpace(p.Trigger)) + "\n")
	b.WriteString("  input:\n")
	b.WriteString("    task_context: string\n")
	if p.ScopeKey != "" {
		b.WriteString("    scope_key: string\n")
	}
	b.WriteString("  steps:\n")
	b.WriteString("    - intent: apply_pattern\n")
	b.WriteString("      guidance: " + yamlScalar(strings.TrimSpace(p.Content)) + "\n")
	b.WriteString("    - validate:\n")
	b.WriteString("        outcome: success\n")
	b.WriteString("    - return:\n")
	b.WriteString("        applied: true\n")
	return b.String()
}

func yamlScalar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	needsQuote := false
	for _, r := range s {
		if r == ':' || r == '#' || r == '"' || r == '\'' || r == '\n' || r == '{' || r == '[' || unicode.IsSpace(r) {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `"` + escaped + `"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	return s
}

func firstWords(s string, n int) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > n {
		fields = fields[:n]
	}
	return strings.Join(fields, " ")
}

func filterEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
