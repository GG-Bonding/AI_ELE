package skill

import (
	"math"
	"math/rand"
)

// SelectThompson picks one ranked skill via Thompson Sampling on Beta(α,β) (V3.2).
// Among candidates with Score>0, samples utility posterior and returns the max draw.
// If none are eligible, returns the top RankedSkill when present.
func SelectThompson(ranked []RankedSkill, rng *rand.Rand) (RankedSkill, bool) {
	if len(ranked) == 0 {
		return RankedSkill{}, false
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	type cand struct {
		r    RankedSkill
		draw float64
	}
	var best cand
	found := false
	for _, r := range ranked {
		if r.Score <= 0 || r.Availability <= 0 || r.Validity <= 0 {
			continue
		}
		alpha := r.Version.Alpha
		beta := r.Version.Beta
		if alpha <= 0 {
			alpha = 1
		}
		if beta <= 0 {
			beta = 1
		}
		draw := sampleBeta(rng, alpha, beta)
		if !found || draw > best.draw {
			best = cand{r: r, draw: draw}
			found = true
		}
	}
	if found {
		return best.r, true
	}
	return ranked[0], ranked[0].Score > 0
}

// sampleBeta uses Gamma(alpha)/ (Gamma(alpha)+Gamma(beta)).
func sampleBeta(rng *rand.Rand, alpha, beta float64) float64 {
	x := sampleGamma(rng, alpha)
	y := sampleGamma(rng, beta)
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

// Marsaglia-Tsang / simple Gamma(shape) for shape>=1; for shape<1 uses boosting.
func sampleGamma(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		return sampleGamma(rng, shape+1) * math.Pow(rng.Float64(), 1/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)
	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*(x*x)*(x*x) {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}
