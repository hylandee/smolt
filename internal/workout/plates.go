package workout

// PlateCount represents a quantity of one plate size.
type PlateCount struct {
	Weight float64
	Count  int
}

// PlatesPerSide returns the plates to load on one side of the barbell,
// largest first, using a greedy algorithm.
// unit is "lb" or "kg". Bar weight is always 45 lb / 20 kg.
// Returns nil if totalWeight <= bar weight.
func PlatesPerSide(totalWeight float64, unit string) []PlateCount {
	barWeight := 45.0
	if unit == "kg" {
		barWeight = 20.0
	}

	maxWeight := 500.0
	if unit == "kg" {
		maxWeight = 220.0
	}
	if totalWeight > maxWeight {
		return nil
	}

	perSide := (totalWeight - barWeight) / 2
	if perSide <= 0 {
		return nil
	}

	var available []float64
	if unit == "kg" {
		available = []float64{25, 20, 15, 10, 5, 2.5, 2, 1.5, 1, 0.5}
	} else {
		available = []float64{45, 35, 25, 10, 5, 2.5}
	}

	remaining := perSide
	var result []PlateCount
	for _, plate := range available {
		if remaining < 0.01 {
			break
		}
		count := int(remaining/plate + 0.001) // tolerance for float noise
		if count > 0 {
			result = append(result, PlateCount{Weight: plate, Count: count})
			remaining -= float64(count) * plate
			if remaining < 0.01 {
				remaining = 0
			}
		}
	}
	return result
}
