package mcts

import (
	"math"
	"strings"
)

type RewardEvaluator struct {
	WeightBasePass   float64
	WeightAdvPass    float64
	WeightBlastRisk  float64
	WeightPatchScale float64
}

func DefaultRewardEvaluator() *RewardEvaluator {
	return &RewardEvaluator{
		WeightBasePass:   0.50,
		WeightAdvPass:    0.35,
		WeightBlastRisk:  0.10,
		WeightPatchScale: 0.05,
	}
}

// ComputeReward calculates a physically grounded verifiable reward score between -1.0 and 1.0.
func (e *RewardEvaluator) ComputeReward(
	baseTestsPassed bool,
	advTestsPassed bool,
	blastRiskScore float64,
	diffPatch string,
) float64 {
	if !baseTestsPassed {
		// Severe penalty for failing primary tests
		return -0.8
	}

	reward := 0.0

	// 1. Base Tests Passed (+0.50)
	reward += e.WeightBasePass

	// 2. Adversarial Falsification Tests Passed (+0.35)
	if advTestsPassed {
		reward += e.WeightAdvPass
	} else {
		reward -= 0.20 // penalty for overfitting
	}

	// 3. Blast Radius Penalty (-0.10 * Risk)
	clampedRisk := math.Max(0.0, math.Min(1.0, blastRiskScore))
	reward -= e.WeightBlastRisk * clampedRisk

	// 4. Minimal Patch Size Occam's Razor Penalty (-0.05 if bloated)
	patchLines := len(strings.Split(diffPatch, "\n"))
	if patchLines > 50 {
		scalePenalty := math.Min(1.0, float64(patchLines-50)/100.0)
		reward -= e.WeightPatchScale * scalePenalty
	}

	// Clamp to [-1.0, 1.0]
	return math.Max(-1.0, math.Min(1.0, math.Round(reward*1000)/1000))
}
