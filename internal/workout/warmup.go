package workout

import "math"

// WarmupSet represents one warm-up set to display before work sets.
type WarmupSet struct {
	Weight float64
	Reps   int
}

// WarmupSets calculates the warm-up ramp for a given work weight.
// Returns nil if workWeight is at or below bar weight.
// unit should be "lb" or "kg".
func WarmupSets(workWeight float64, unit string) []WarmupSet {
	barWeight := 45.0
	if unit == "kg" {
		barWeight = 20.0
	}

	if workWeight <= barWeight {
		return nil
	}

	// Round to nearest plate step
	step := 5.0
	if unit == "kg" {
		step = 2.5
	}

	// Always start with two empty-bar sets
	sets := []WarmupSet{
		{Weight: barWeight, Reps: 5},
		{Weight: barWeight, Reps: 5},
	}

	// Ramp at ~40%, ~60%, ~80% of work weight
	percentages := []struct {
		pct  float64
		reps int
	}{
		{0.4, 5},
		{0.6, 3},
		{0.8, 2},
	}

	prev := barWeight
	for _, p := range percentages {
		target := workWeight * p.pct
		rounded := math.Round(target/step) * step
		if rounded <= prev || rounded >= workWeight {
			continue
		}
		sets = append(sets, WarmupSet{Weight: rounded, Reps: p.reps})
		prev = rounded
	}

	return sets
}
