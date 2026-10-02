package cli

import "math"

// EvalRunResult is one measured benchmark run (one arm, one case, one repeat).
type EvalRunResult struct {
	CaseID       string
	Name         string
	Archetype    string
	BaselinePass bool
	FullPass     bool
	TurnsTaken   int
	DurationS    float64
	CostUSD      float64
}

// CaseStats aggregates EvalRunResult across --repeat runs of one case.
type CaseStats struct {
	CaseID       string
	Name         string
	Archetype    string
	Repeats      int
	BaselineWins int
	FullWins     int
	TurnsMean    float64
	TurnsStd     float64
	DurationMean float64
	DurationStd  float64
	CostMean     float64
	CostStd      float64
}

func meanStd(vs []float64) (float64, float64) {
	if len(vs) == 0 {
		return 0, 0
	}
	m := 0.0
	for _, v := range vs {
		m += v
	}
	m /= float64(len(vs))
	if len(vs) == 1 {
		return m, 0
	}
	variance := 0.0
	for _, v := range vs {
		variance += (v - m) * (v - m)
	}
	variance /= float64(len(vs)) // population std over the measured runs
	return m, math.Sqrt(variance)
}

// aggregateByCase folds raw run rows into one CaseStats per case, preserving
// first-seen order.
func aggregateByCase(results []EvalRunResult) []CaseStats {
	var order []string
	byCase := make(map[string]*CaseStats)
	collect := make(map[string][]EvalRunResult)

	for _, r := range results {
		if _, ok := byCase[r.CaseID]; !ok {
			byCase[r.CaseID] = &CaseStats{CaseID: r.CaseID, Name: r.Name, Archetype: r.Archetype}
			order = append(order, r.CaseID)
		}
		collect[r.CaseID] = append(collect[r.CaseID], r)
	}

	for _, id := range order {
		rows := collect[id]
		s := byCase[id]
		s.Repeats = len(rows)
		turns := make([]float64, 0, len(rows))
		durs := make([]float64, 0, len(rows))
		costs := make([]float64, 0, len(rows))
		for _, r := range rows {
			if r.BaselinePass {
				s.BaselineWins++
			}
			if r.FullPass {
				s.FullWins++
			}
			turns = append(turns, float64(r.TurnsTaken))
			durs = append(durs, r.DurationS)
			costs = append(costs, r.CostUSD)
		}
		s.TurnsMean, s.TurnsStd = meanStd(turns)
		s.DurationMean, s.DurationStd = meanStd(durs)
		s.CostMean, s.CostStd = meanStd(costs)
	}

	stats := make([]CaseStats, 0, len(order))
	for _, id := range order {
		stats = append(stats, *byCase[id])
	}
	return stats
}
