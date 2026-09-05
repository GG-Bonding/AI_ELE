package skill

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// RetrieveQuery selects candidate Skills for a task.
type RetrieveQuery struct {
	TenantID string
	Task     string
	Tools    []string // agent available tools; empty means ignore agent-side filter
	TopK     int
}

// RankedSkill is one scored SkillVersion for retrieval.
type RankedSkill struct {
	Skill   Skill
	Version Version
	Score   float64
	Sim     float64
	Util    float64
	Conf    float64
	Validity    float64
	Validation  float64
	Availability float64
}

// Retrieve ranks ACTIVE skills by sim×util×conf×validity×validation×availability.
func Retrieve(ctx context.Context, repo Repository, tools *toolregistry.Registry, q RetrieveQuery) ([]RankedSkill, error) {
	if repo == nil {
		return nil, fmt.Errorf("%w: repo is required", ErrInvalidInput)
	}
	if tools == nil {
		tools = toolregistry.Default()
	}
	tenantID := strings.TrimSpace(q.TenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	skills, err := repo.ListSkills(ctx, tenantID, nil)
	if err != nil {
		return nil, err
	}
	agentTools := map[string]struct{}{}
	for _, t := range q.Tools {
		t = strings.TrimSpace(t)
		if t != "" {
			agentTools[t] = struct{}{}
		}
	}

	var ranked []RankedSkill
	for _, sk := range skills {
		if sk.ActiveVersionID == nil {
			continue
		}
		ver, err := repo.GetVersion(ctx, tenantID, *sk.ActiveVersionID)
		if err != nil {
			continue
		}

		sim := lexicalSkillSim(q.Task, sk, ver)
		util := clamp01(ver.Utility)
		conf := clamp01(ver.Confidence)

		validity := 0.0
		if sk.Status == StatusActive {
			validity = 1
		}
		validation := 0.0
		if ver.ValidationStatus == ValidationPassed {
			validation = 1
		}
		availability := skillAvailability(ver.Spec, tools, agentTools, len(q.Tools) == 0)

		score := sim * util * conf * validity * validation * availability
		ranked = append(ranked, RankedSkill{
			Skill:        sk,
			Version:      ver,
			Score:        score,
			Sim:          sim,
			Util:         util,
			Conf:         conf,
			Validity:     validity,
			Validation:   validation,
			Availability: availability,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].Skill.Name < ranked[j].Skill.Name
		}
		return ranked[i].Score > ranked[j].Score
	})
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	return ranked, nil
}

func lexicalSkillSim(task string, sk Skill, ver Version) float64 {
	var tools []string
	for _, st := range ver.Spec.Steps {
		if st.Tool != "" {
			tools = append(tools, st.Tool)
		}
	}
	haystack := strings.Join([]string{
		sk.Name,
		sk.Description,
		ver.Spec.Name,
		ver.Spec.Description,
		strings.Join(tools, " "),
	}, " ")
	return retrieval.LexicalOverlap(task, haystack)
}

func skillAvailability(spec Spec, tools *toolregistry.Registry, agentTools map[string]struct{}, ignoreAgentFilter bool) float64 {
	for _, st := range spec.Steps {
		if !tools.Has(st.Tool) {
			return 0
		}
		if !ignoreAgentFilter {
			if _, ok := agentTools[st.Tool]; !ok {
				return 0
			}
		}
	}
	return 1
}
