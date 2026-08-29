package eval

import "time"

// Arm is one evaluation experimental condition.
type Arm string

const (
	// ArmBaseline uses no retrieval / no experience context.
	ArmBaseline Arm = "baseline"
	// ArmRawRetrieval ranks candidates by Similarity only.
	ArmRawRetrieval Arm = "raw_retrieval"
	// ArmUtilityRetrieval uses two-phase utility-aware ranking.
	ArmUtilityRetrieval Arm = "utility_retrieval"
	// ArmUtilityLearning is utility retrieval plus feedback → utility updates.
	ArmUtilityLearning Arm = "utility_learning"
)

// AllArms returns the four V1 evaluation arms in report order.
func AllArms() []Arm {
	return []Arm{ArmBaseline, ArmRawRetrieval, ArmUtilityRetrieval, ArmUtilityLearning}
}

// TaskResult is one simulated agent task under an arm.
type TaskResult struct {
	Arm               Arm
	TaskIndex         int
	Task              string
	Success           bool
	RetrievedIDs      []string
	RelevantIDs       []string
	UsedExperience    bool
	NegativeTransfer  bool
	Tokens            int
	Latency           time.Duration
	HelpfulUtility    float64
	HarmfulUtility    float64
	HelpfulFinalScore float64
	KeptContents      []string
}

// Metrics aggregates TaskResults for one arm.
type Metrics struct {
	Arm                    Arm     `json:"arm"`
	Tasks                  int     `json:"tasks"`
	Successes              int     `json:"successes"`
	TaskSuccessRate        float64 `json:"task_success_rate"`
	RetrievalPrecision     float64 `json:"retrieval_precision"`
	ExperienceUtilization  float64 `json:"experience_utilization"`
	AverageUtility         float64 `json:"average_utility"`
	NegativeTransferRate   float64 `json:"negative_transfer_rate"`
	TokenCost              float64 `json:"token_cost"`
	LatencyMs              float64 `json:"latency_ms"`
	HelpfulUtilityFinal    float64 `json:"helpful_utility_final"`
	HelpfulFinalScoreFinal float64 `json:"helpful_final_score_final"`
}

// Aggregate computes V1 metrics from per-task results.
func Aggregate(arm Arm, results []TaskResult) Metrics {
	m := Metrics{Arm: arm, Tasks: len(results)}
	if len(results) == 0 {
		return m
	}

	var (
		precisionNum, precisionDen float64
		utilHits, utilDen          float64
		utilitySum                 float64
		utilityN                   int
		negHits                    float64
		tokenSum                   float64
		latencySum                 float64
	)

	for _, r := range results {
		if r.Success {
			m.Successes++
		}
		if len(r.RetrievedIDs) > 0 {
			utilDen++
			if r.UsedExperience {
				utilHits++
			}
			hits := overlapCount(r.RetrievedIDs, r.RelevantIDs)
			precisionNum += float64(hits)
			precisionDen += float64(len(r.RetrievedIDs))
		}
		if r.HelpfulUtility > 0 || r.HarmfulUtility > 0 {
			utilitySum += r.HelpfulUtility
			utilityN++
		}
		if r.NegativeTransfer {
			negHits++
		}
		tokenSum += float64(r.Tokens)
		latencySum += float64(r.Latency.Milliseconds())
		m.HelpfulUtilityFinal = r.HelpfulUtility
		m.HelpfulFinalScoreFinal = r.HelpfulFinalScore
	}

	m.TaskSuccessRate = float64(m.Successes) / float64(m.Tasks)
	if precisionDen > 0 {
		m.RetrievalPrecision = precisionNum / precisionDen
	}
	if utilDen > 0 {
		m.ExperienceUtilization = utilHits / utilDen
	}
	if utilityN > 0 {
		m.AverageUtility = utilitySum / float64(utilityN)
	}
	m.NegativeTransferRate = negHits / float64(m.Tasks)
	m.TokenCost = tokenSum
	m.LatencyMs = latencySum / float64(m.Tasks)
	return m
}

func overlapCount(got, want []string) int {
	set := map[string]struct{}{}
	for _, id := range want {
		set[id] = struct{}{}
	}
	n := 0
	for _, id := range got {
		if _, ok := set[id]; ok {
			n++
		}
	}
	return n
}
