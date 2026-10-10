package mcts

import (
	"testing"
)

func TestMCTSNodeAndUCB1(t *testing.T) {
	root := NewNode("root", nil, "", "", "root hypothesis")
	child1 := NewNode("c1", root, "diff1", "app.py", "hypothesis 1")
	child2 := NewNode("c2", root, "diff2", "app.py", "hypothesis 2")
	root.Children = append(root.Children, child1, child2)

	// Initially unvisited child has infinite UCB1
	if child1.UCB1(1.414) < 100000.0 {
		t.Fatalf("expected unvisited child to have infinite UCB1")
	}

	// Simulate visits
	child1.Backpropagate(0.8)
	child2.Backpropagate(0.2)

	if root.Visits != 2 {
		t.Fatalf("expected root to have 2 visits, got %d", root.Visits)
	}

	best := root.BestChild(1.414)
	if best == nil || best.ID != "c1" {
		t.Fatalf("expected child1 to be selected as best child, got %v", best)
	}
}

func TestRewardEvaluatorGrounded(t *testing.T) {
	eval := DefaultRewardEvaluator()

	// 1. Fail base tests -> heavy negative reward
	rFail := eval.ComputeReward(false, false, false, 0.5, "small diff")
	if rFail >= 0.0 {
		t.Fatalf("expected negative reward on failed test, got %f", rFail)
	}

	// 2. Pass base + pass adversarial + low blast radius -> high positive reward
	rSuccess := eval.ComputeReward(true, true, false, 0.1, "diff")
	if rSuccess <= 0.7 {
		t.Fatalf("expected high reward (>0.7), got %f", rSuccess)
	}

	// 3. Overfitted patch (base pass, adv fail) -> lower reward
	rOverfitted := eval.ComputeReward(true, false, false, 0.1, "diff")
	if rOverfitted >= rSuccess {
		t.Fatalf("expected overfitted patch reward < robust patch reward")
	}

	// 4. Skipped adversarial testing -> neutral (no +0.35 bonus, no -0.20 penalty)
	rSkipped := eval.ComputeReward(true, false, true, 0.1, "diff")
	if rSkipped >= 0.70 {
		t.Fatalf("skipped adversarial test must not trigger early convergence (>= 0.70), got %f", rSkipped)
	}
	if rSkipped <= rOverfitted {
		t.Fatalf("skipped test must not be punished worse than an overfitted failure, got %f vs %f", rSkipped, rOverfitted)
	}
}
