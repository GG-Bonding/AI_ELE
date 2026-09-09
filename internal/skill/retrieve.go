package skill

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/provider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/retrieval"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// RetrieveQuery selects candidate Skills for a task.
type RetrieveQuery struct {
	TenantID string
	Task     string
	Tools    []string // agent available tools; empty means ignore agent-side filter
	TopK     int
	// QueryEmbedding optional precomputed task embedding (skips Embedder call).
	QueryEmbedding []float32
}

// RankedSkill is one scored SkillVersion for retrieval.
type RankedSkill struct {
	Skill        Skill
	Version      Version
	Score        float64
	Sim          float64
	Util         float64
	Conf         float64
	Validity     float64
	Validation   float64
	Availability float64
	Semantic     bool // true when Sim came from embeddings
}

// Retriever ranks skills; Embedder enables semantic TopK (V3.2).
type Retriever struct {
	Repo     Repository
	Tools    *toolregistry.Registry
	Embedder provider.EmbeddingProvider // optional
}

// Retrieve ranks ACTIVE skills by sim×util×conf×validity×validation×availability.
// When Embedder (or QueryEmbedding) is set, similarity is cosine over SkillVersion embeddings
// with lexical fallback for rows missing embeddings.
func Retrieve(ctx context.Context, repo Repository, tools *toolregistry.Registry, q RetrieveQuery) ([]RankedSkill, error) {
	return (&Retriever{Repo: repo, Tools: tools}).Retrieve(ctx, q)
}

// Retrieve implements semantic-aware Skill retrieval.
func (r *Retriever) Retrieve(ctx context.Context, q RetrieveQuery) ([]RankedSkill, error) {
	if r == nil || r.Repo == nil {
		return nil, fmt.Errorf("%w: repo is required", ErrInvalidInput)
	}
	tools := r.Tools
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

	agentTools := map[string]struct{}{}
	for _, t := range q.Tools {
		t = strings.TrimSpace(t)
		if t != "" {
			agentTools[t] = struct{}{}
		}
	}

	queryEmb := q.QueryEmbedding
	if len(queryEmb) == 0 && r.Embedder != nil && strings.TrimSpace(q.Task) != "" {
		vecs, err := r.Embedder.Embed(ctx, []string{q.Task})
		if err != nil {
			return nil, fmt.Errorf("embed task: %w", err)
		}
		if len(vecs) > 0 {
			queryEmb = vecs[0]
		}
	}

	var candidates []ScoredVersion
	if len(queryEmb) > 0 {
		scored, err := r.Repo.SearchActiveByEmbedding(ctx, tenantID, queryEmb, max(topK*3, 20))
		if err != nil && !errors.Is(err, ErrNotSupported) {
			return nil, err
		}
		if err == nil {
			candidates = scored
		}
	}
	if len(candidates) == 0 {
		// Lexical / full scan path.
		skills, err := r.Repo.ListSkills(ctx, tenantID, []Status{StatusActive})
		if err != nil {
			return nil, err
		}
		for _, sk := range skills {
			if sk.ActiveVersionID == nil {
				continue
			}
			ver, err := r.Repo.GetVersion(ctx, tenantID, *sk.ActiveVersionID)
			if err != nil {
				continue
			}
			sim := lexicalSkillSim(q.Task, sk, ver)
			if len(queryEmb) > 0 && len(ver.Embedding) > 0 {
				sim = cosineSim(queryEmb, ver.Embedding)
			}
			candidates = append(candidates, ScoredVersion{Skill: sk, Version: ver, Similarity: sim})
		}
	}

	var ranked []RankedSkill
	for _, c := range candidates {
		sk, ver := c.Skill, c.Version
		sim := c.Similarity
		semantic := len(queryEmb) > 0 && len(ver.Embedding) > 0 && sim > 0
		if sim <= 0 {
			sim = lexicalSkillSim(q.Task, sk, ver)
			semantic = false
		}
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
			Skill: sk, Version: ver, Score: score, Sim: sim, Util: util, Conf: conf,
			Validity: validity, Validation: validation, Availability: availability, Semantic: semantic,
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
		sk.Name, sk.Description, ver.Spec.Name, ver.Spec.Description, strings.Join(tools, " "),
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
