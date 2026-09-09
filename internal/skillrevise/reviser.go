package skillrevise

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/sanitize"
	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

const reviserSystem = `You revise a failed Skill Spec into a safer next candidate.
Return ONLY JSON with keys:
reason (string), changes (string array), new_spec_yaml (string), confidence (0-1), evidence_ids (string array).
Rules:
- Keep tools from the provided tool allow-list only.
- Prefer inserting a resolve/search step before create when project key failures occur.
- Do not invent credentials or secrets.
- new_spec_yaml must be valid Skill YAML with name, inputs, steps, risk, max_steps.`

// Proposal is an evidence-grounded revision suggestion (never auto-ACTIVATED).
type Proposal struct {
	Reason      string   `json:"reason"`
	Changes     []string `json:"changes"`
	NewSpecYAML string   `json:"new_spec_yaml"`
	Confidence  float64  `json:"confidence"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// Input gathers failure traces for revision.
type Input struct {
	Current         skill.Spec
	FailureCodes    []string
	FailureMessages []string
	StepTrace       string
	PatternContent  string
	ExperienceText  string
	ToolAllowList   []string
	EvidenceIDs     []string
}

// LLMReviser proposes Skill revisions via LLM, falling back to rule-based AutoRevise hints.
type LLMReviser struct {
	LLM provider.LLMProvider
}

func NewLLMReviser(llm provider.LLMProvider) (*LLMReviser, error) {
	if llm == nil {
		return nil, fmt.Errorf("skillrevise: llm is required")
	}
	return &LLMReviser{LLM: llm}, nil
}

// Propose returns a candidate revision proposal.
func (r *LLMReviser) Propose(ctx context.Context, in Input) (Proposal, error) {
	user := fmt.Sprintf(`Current Spec JSON:
%s

Failure codes: %v
Failure messages: %v
Step trace:
%s
Pattern:
%s
Experience:
%s
Allowed tools: %v
Evidence IDs: %v
`, mustJSON(in.Current), in.FailureCodes, in.FailureMessages, in.StepTrace, in.PatternContent, in.ExperienceText, in.ToolAllowList, in.EvidenceIDs)

	resp, err := r.LLM.Complete(ctx, provider.CompletionRequest{
		System:      sanitize.AppendUntrustedBoundary(reviserSystem),
		User:        user,
		Temperature: 0.1,
		MaxTokens:   2000,
	})
	if err != nil {
		return Proposal{}, err
	}
	raw := strings.TrimSpace(resp.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var p Proposal
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Proposal{}, fmt.Errorf("skillrevise: invalid llm json: %w", err)
	}
	p.NewSpecYAML = strings.TrimSpace(p.NewSpecYAML)
	if p.NewSpecYAML == "" {
		return Proposal{}, fmt.Errorf("skillrevise: empty new_spec_yaml")
	}
	if _, err := skill.ParseYAML(p.NewSpecYAML); err != nil {
		return Proposal{}, fmt.Errorf("skillrevise: invalid new_spec_yaml: %w", err)
	}
	if p.Confidence <= 0 {
		p.Confidence = 0.5
	}
	if len(p.EvidenceIDs) == 0 {
		p.EvidenceIDs = in.EvidenceIDs
	}
	return p, nil
}

// ProposeOrFallback tries LLM then rule-based SuggestRevisionYAML.
func ProposeOrFallback(ctx context.Context, llm *LLMReviser, in Input) (Proposal, bool, error) {
	if llm != nil {
		p, err := llm.Propose(ctx, in)
		if err == nil {
			return p, true, nil
		}
	}
	yamlDoc, ok := skill.SuggestRevisionYAML(in.Current, skill.RevisionHint{
		FailureCodes: in.FailureCodes, FailureMessages: in.FailureMessages, PatternContent: in.PatternContent,
	})
	if !ok {
		return Proposal{}, false, nil
	}
	return Proposal{
		Reason:      "rule-based fallback",
		Changes:     []string{"injected jira.search_projects before create"},
		NewSpecYAML: yamlDoc,
		Confidence:  0.7,
		EvidenceIDs: in.EvidenceIDs,
	}, true, nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
