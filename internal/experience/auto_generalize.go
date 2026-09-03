package experience

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Auto-generalization defaults (V2.1-5).
const (
	MinClusterSimilarity = 0.85
	MaxScanExperiences   = 200
)

// AutoGeneralizeOptions configures an evolution scan for a tenant.
type AutoGeneralizeOptions struct {
	MinUtility     float64 // default MinGeneralizeAvgUtility
	MinSimilarity  float64 // default MinClusterSimilarity
	MaxCandidates  int     // default MaxScanExperiences
	Statuses       []Status
}

// AutoGeneralizeResult summarizes one scan pass.
type AutoGeneralizeResult struct {
	Scanned   int                `json:"scanned"`
	Clusters  int                `json:"clusters"`
	Created   []Pattern          `json:"created,omitempty"`
	Skipped   []ClusterSkip      `json:"skipped,omitempty"`
}

// ClusterSkip records why a candidate cluster did not create a Pattern.
type ClusterSkip struct {
	ExperienceIDs []string `json:"experience_ids"`
	Reason        string   `json:"reason"`
}

// ClusterFingerprint returns a stable SHA-256 of sorted experience IDs.
func ClusterFingerprint(ids []string) string {
	cleaned := uniqueNonEmpty(ids)
	if len(cleaned) == 0 {
		return ""
	}
	sort.Strings(cleaned)
	sum := sha256.Sum256([]byte(strings.Join(cleaned, "\n")))
	return hex.EncodeToString(sum[:])
}

// AutoGeneralize discovers high-utility experience neighborhoods and generalizes them (V2.1-5).
func (s *Service) AutoGeneralize(ctx context.Context, tenantID string, opts AutoGeneralizeOptions) (AutoGeneralizeResult, error) {
	if s.patterns == nil {
		return AutoGeneralizeResult{}, fmt.Errorf("%w: pattern repository not configured", ErrInvalidInput)
	}
	if err := requireNonEmpty("tenant_id", tenantID); err != nil {
		return AutoGeneralizeResult{}, err
	}
	if opts.MinUtility <= 0 {
		opts.MinUtility = MinGeneralizeAvgUtility
	}
	if opts.MinSimilarity <= 0 {
		opts.MinSimilarity = MinClusterSimilarity
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = MaxScanExperiences
	}
	statuses := opts.Statuses
	if len(statuses) == 0 {
		statuses = []Status{StatusActive}
	}

	candidates, err := s.repo.List(ctx, ListFilter{
		TenantID:   tenantID,
		Statuses:   statuses,
		MinUtility: opts.MinUtility,
		Limit:      opts.MaxCandidates,
	})
	if err != nil {
		return AutoGeneralizeResult{}, fmt.Errorf("list experiences for auto-generalize: %w", err)
	}

	out := AutoGeneralizeResult{Scanned: len(candidates)}
	groups := groupByFamily(candidates)
	for _, group := range groups {
		clusters := clusterBySimilarity(group, opts.MinSimilarity)
		for _, cluster := range clusters {
			out.Clusters++
			ids := make([]string, 0, len(cluster))
			for _, e := range cluster {
				ids = append(ids, e.ID)
			}
			res, err := s.Generalize(ctx, tenantID, GeneralizeInput{ExperienceIDs: ids})
			if err != nil {
				return out, err
			}
			if res.Created {
				out.Created = append(out.Created, res.Pattern)
				continue
			}
			out.Skipped = append(out.Skipped, ClusterSkip{ExperienceIDs: ids, Reason: res.Skipped})
		}
	}
	return out, nil
}

type familyKey struct {
	Type     Type
	Scope    Scope
	ScopeKey string
}

func groupByFamily(exps []Experience) [][]Experience {
	buckets := map[familyKey][]Experience{}
	order := make([]familyKey, 0)
	for _, e := range exps {
		k := familyKey{Type: e.Type, Scope: e.Scope, ScopeKey: e.ScopeKey}
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], e)
	}
	out := make([][]Experience, 0, len(order))
	for _, k := range order {
		out = append(out, buckets[k])
	}
	return out
}

// clusterBySimilarity builds connected components where edges require cosine >= minSim.
func clusterBySimilarity(exps []Experience, minSim float64) [][]Experience {
	n := len(exps)
	if n == 0 {
		return nil
	}
	adj := make([][]int, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			sim := CosineSimilarity(exps[i].Embedding, exps[j].Embedding)
			if sim < minSim {
				// Fall back to lexical overlap when embeddings are missing/weak.
				if len(exps[i].Embedding) == 0 || len(exps[j].Embedding) == 0 {
					lex := lexicalJaccard(exps[i].Trigger+" "+exps[i].Content, exps[j].Trigger+" "+exps[j].Content)
					if lex < minSim {
						continue
					}
				} else {
					continue
				}
			}
			adj[i] = append(adj[i], j)
			adj[j] = append(adj[j], i)
		}
	}

	seen := make([]bool, n)
	var clusters [][]Experience
	for i := 0; i < n; i++ {
		if seen[i] {
			continue
		}
		queue := []int{i}
		seen[i] = true
		var members []Experience
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			members = append(members, exps[cur])
			for _, nb := range adj[cur] {
				if seen[nb] {
					continue
				}
				seen[nb] = true
				queue = append(queue, nb)
			}
		}
		if len(members) >= MinGeneralizeExperiences {
			clusters = append(clusters, members)
		}
	}
	return clusters
}

func lexicalJaccard(a, b string) float64 {
	ta := tokenize(a)
	tb := tokenize(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if _, ok := tb[t]; ok {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
