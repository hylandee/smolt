package workout

import (
	"testing"
)

func TestWarmupSetsReturnNilAtOrBelowBarWeight(t *testing.T) {
	if got := WarmupSets(45, "lb"); got != nil {
		t.Errorf("expected nil at bar weight, got %v", got)
	}
	if got := WarmupSets(44, "lb"); got != nil {
		t.Errorf("expected nil below bar weight, got %v", got)
	}
	if got := WarmupSets(20, "kg"); got != nil {
		t.Errorf("expected nil at bar weight (kg), got %v", got)
	}
}

func TestWarmupSetsAlwaysStartsWithTwoBarSets(t *testing.T) {
	sets := WarmupSets(135, "lb")
	if len(sets) < 2 {
		t.Fatalf("expected at least 2 sets, got %d", len(sets))
	}
	if sets[0].Weight != 45 || sets[0].Reps != 5 {
		t.Errorf("first set should be bar (45lb) x5, got %v", sets[0])
	}
	if sets[1].Weight != 45 || sets[1].Reps != 5 {
		t.Errorf("second set should be bar (45lb) x5, got %v", sets[1])
	}
}

func TestWarmupSetsRampReps(t *testing.T) {
	// With enough ramp sets they should decrease in reps: 5, 3, 2
	sets := WarmupSets(225, "lb") // heavy enough for all 3 ramp steps
	if len(sets) < 3 {
		t.Fatalf("expected at least 3 sets for 225lb, got %d: %v", len(sets), sets)
	}
	// After the two bar sets, check ramp decreases
	rampSets := sets[2:]
	for i := 1; i < len(rampSets); i++ {
		if rampSets[i].Reps > rampSets[i-1].Reps {
			t.Errorf("ramp reps should decrease, but sets[%d].Reps=%d > sets[%d].Reps=%d",
				i, rampSets[i].Reps, i-1, rampSets[i-1].Reps)
		}
	}
}

func TestWarmupSetsRampWeightsIncreasing(t *testing.T) {
	sets := WarmupSets(225, "lb")
	for i := 1; i < len(sets); i++ {
		if sets[i].Weight < sets[i-1].Weight {
			t.Errorf("warmup weights should be non-decreasing, but sets[%d].Weight=%.1f < sets[%d].Weight=%.1f",
				i, sets[i].Weight, i-1, sets[i-1].Weight)
		}
	}
}

func TestWarmupSetsNeverExceedWorkWeight(t *testing.T) {
	workWeight := 135.0
	sets := WarmupSets(workWeight, "lb")
	for _, s := range sets {
		if s.Weight >= workWeight {
			t.Errorf("warmup set weight %.1f should be below work weight %.1f", s.Weight, workWeight)
		}
	}
}

func TestWarmupSetsMetricBarWeight(t *testing.T) {
	sets := WarmupSets(60, "kg")
	if len(sets) < 2 {
		t.Fatalf("expected at least 2 sets, got %d", len(sets))
	}
	if sets[0].Weight != 20 {
		t.Errorf("first metric warmup set should be 20kg bar, got %.1f", sets[0].Weight)
	}
	if sets[1].Weight != 20 {
		t.Errorf("second metric warmup set should be 20kg bar, got %.1f", sets[1].Weight)
	}
}

func TestWarmupSetsRoundedToPlateStep(t *testing.T) {
	// lb rounds to 5lb steps
	sets := WarmupSets(135, "lb")
	for _, s := range sets {
		if int(s.Weight)%5 != 0 {
			t.Errorf("lb warmup weight %.1f is not a multiple of 5", s.Weight)
		}
	}
	// kg rounds to 2.5kg steps
	sets = WarmupSets(100, "kg")
	for _, s := range sets {
		rem := s.Weight - float64(int(s.Weight/2.5))*2.5
		if rem > 0.01 {
			t.Errorf("kg warmup weight %.1f is not a multiple of 2.5", s.Weight)
		}
	}
}
