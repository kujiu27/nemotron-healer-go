package cli

import (
	"math"
	"testing"
)

func TestAggregateByCaseMeanStd(t *testing.T) {
	results := []EvalRunResult{
		{CaseID: "AHB-01", Name: "n1", Archetype: "a", BaselinePass: false, FullPass: true, TurnsTaken: 2, DurationS: 10, CostUSD: 0.001},
		{CaseID: "AHB-01", Name: "n1", Archetype: "a", BaselinePass: false, FullPass: true, TurnsTaken: 4, DurationS: 20, CostUSD: 0.003},
		{CaseID: "AHB-01", Name: "n1", Archetype: "a", BaselinePass: false, FullPass: false, TurnsTaken: 6, DurationS: 30, CostUSD: 0.005},
		{CaseID: "AHB-02", Name: "n2", Archetype: "b", BaselinePass: true, FullPass: true, TurnsTaken: 1, DurationS: 5, CostUSD: 0.01},
	}
	stats := aggregateByCase(results)
	if len(stats) != 2 || stats[0].CaseID != "AHB-01" || stats[1].CaseID != "AHB-02" {
		t.Fatalf("want 2 cases in first-seen order, got %+v", stats)
	}
	s := stats[0]
	if s.Repeats != 3 || s.BaselineWins != 0 || s.FullWins != 2 {
		t.Fatalf("wins wrong: %+v", s)
	}
	if s.TurnsMean != 4 || s.DurationMean != 20 || math.Abs(s.CostMean-0.003) > 1e-12 {
		t.Fatalf("means wrong: %+v", s)
	}
	if math.Abs(s.TurnsStd-math.Sqrt(8.0/3.0)) > 1e-12 {
		t.Fatalf("turns std wrong: %v (population std over 2,4,6)", s.TurnsStd)
	}
	if stats[1].TurnsStd != 0 || stats[1].Repeats != 1 {
		t.Fatalf("single-run case must have zero std: %+v", stats[1])
	}
}

func TestMeanStdEmptyAndSingle(t *testing.T) {
	if m, sd := meanStd(nil); m != 0 || sd != 0 {
		t.Fatalf("empty: %v %v", m, sd)
	}
	if m, sd := meanStd([]float64{7}); m != 7 || sd != 0 {
		t.Fatalf("single: %v %v", m, sd)
	}
}
