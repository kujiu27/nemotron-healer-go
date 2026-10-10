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
// Honest verification semantics:
// - Primary base tests must pass (+0.50); failing them receives a severe -0.80 penalty.
// - Adversarially repelled counter-examples award the full +0.35 robustness bonus.
// - Broken/overfitted patches that fail adversarial falsification are penalized with -0.20.
// - Skipped adversarial tests (infrastructure error, malformed generator output) are neutral (+0.00):
//   they do not punish the patch, but critically CANNOT award the +0.35 bonus to falsely trigger
//   early convergence (>= 0.70) on untested candidates.
func (e *RewardEvaluator) ComputeReward(
	baseTestsPassed bool,
	advTestsPassed bool,
	advTestsSkipped bool,
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

	// 2. Adversarial Falsification Tests (+0.35 if passed, 0.0 if skipped, -0.20 if failed)
	if advTestsSkipped {
		// Honest neutrality: generator was unavailable or test was malformed;
		// no bonus awarded, no overfitting penalty applied.
	} else if advTestsPassed {
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
