package workout

import (
	"testing"
)

func TestPlatesPerSideImperial(t *testing.T) {
	cases := []struct {
		name   string
		weight float64
		want   []PlateCount
	}{
		{
			name:   "45lb bar only returns nil",
			weight: 45,
			want:   nil,
		},
		{
			name:   "below bar returns nil",
			weight: 44,
			want:   nil,
		},
		{
			name:   "135lb = one 45 per side",
			weight: 135,
			want:   []PlateCount{{Weight: 45, Count: 1}},
		},
		{
			name:   "225lb = two 45s per side",
			weight: 225,
			want:   []PlateCount{{Weight: 45, Count: 2}},
		},
		{
			name:   "315lb = three 45s per side",
			weight: 315,
			want:   []PlateCount{{Weight: 45, Count: 3}},
		},
		{
			name:   "95lb = one 25 per side",
			weight: 95,
			want:   []PlateCount{{Weight: 25, Count: 1}},
		},
		{
			name:   "115lb = one 35 per side",
			weight: 115,
			want:   []PlateCount{{Weight: 35, Count: 1}},
		},
		{
			name:   "185lb = two 45s minus... 70lb per side: 45+25",
			weight: 185,
			want:   []PlateCount{{Weight: 45, Count: 1}, {Weight: 25, Count: 1}},
		},
		{
			// 30lb per side = 25+5, NOT 3x10
			name:   "30lb per side is 25+5",
			weight: 105,
			want:   []PlateCount{{Weight: 25, Count: 1}, {Weight: 5, Count: 1}},
		},
		{
			// (500-45)/2 = 227.5 per side: 5x45=225, remainder 2.5 → 1x2.5
			name:   "500lb is at cap (should compute)",
			weight: 500,
			want:   []PlateCount{{Weight: 45, Count: 5}, {Weight: 2.5, Count: 1}},
		},
		{
			name:   "over 500lb returns nil",
			weight: 500.01,
			want:   nil,
		},
		{
			name:   "50lb = 2.5 per side",
			weight: 50,
			want:   []PlateCount{{Weight: 2.5, Count: 1}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PlatesPerSide(tc.weight, "lb")
			if len(got) != len(tc.want) {
				t.Fatalf("PlatesPerSide(%.1f, lb) = %v, want %v", tc.weight, got, tc.want)
			}
			for i, pc := range tc.want {
				if got[i].Weight != pc.Weight || got[i].Count != pc.Count {
					t.Errorf("PlatesPerSide(%.1f, lb)[%d] = %v, want %v", tc.weight, i, got[i], pc)
				}
			}
		})
	}
}

func TestPlatesPerSideMetric(t *testing.T) {
	cases := []struct {
		name   string
		weight float64
		want   []PlateCount
	}{
		{
			name:   "20kg bar only returns nil",
			weight: 20,
			want:   nil,
		},
		{
			name:   "70kg = one 25 per side",
			weight: 70,
			want:   []PlateCount{{Weight: 25, Count: 1}},
		},
		{
			// (100-20)/2 = 40kg per side: 1x25=25, remainder 15 → 1x15
			name:   "100kg = one 25 and one 15 per side",
			weight: 100,
			want:   []PlateCount{{Weight: 25, Count: 1}, {Weight: 15, Count: 1}},
		},
		{
			name:   "60kg = one 20 per side",
			weight: 60,
			want:   []PlateCount{{Weight: 20, Count: 1}},
		},
		{
			name:   "over 220kg returns nil",
			weight: 220.01,
			want:   nil,
		},
		{
			// (220-20)/2 = 100kg per side: 4x25=100, remainder 0
			name:   "220kg is at cap (should compute)",
			weight: 220,
			want:   []PlateCount{{Weight: 25, Count: 4}},
		},
		{
			// duplicate of 100kg test to verify greedy correctness
			name:   "greedy prefers big plates: 100kg = 25+15 per side",
			weight: 100,
			want:   []PlateCount{{Weight: 25, Count: 1}, {Weight: 15, Count: 1}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PlatesPerSide(tc.weight, "kg")
			if len(got) != len(tc.want) {
				t.Fatalf("PlatesPerSide(%.1f, kg) = %v, want %v", tc.weight, got, tc.want)
			}
			for i, pc := range tc.want {
				if got[i].Weight != pc.Weight || got[i].Count != pc.Count {
					t.Errorf("PlatesPerSide(%.1f, kg)[%d] = %v, want %v", tc.weight, i, got[i], pc)
				}
			}
		})
	}
}

func TestPlatesGreedyNeverUsesSmallPlatesUnnecessarily(t *testing.T) {
	// 105lb = 45 bar + 30lb each side = 25+5, never 3x10
	plates := PlatesPerSide(105, "lb")
	for _, pc := range plates {
		if pc.Weight == 10 && pc.Count >= 3 {
			t.Errorf("30lb per side should not use 3x10lb, got: %v", plates)
		}
	}
	// verify it's 25+5
	if len(plates) < 2 || plates[0].Weight != 25 || plates[1].Weight != 5 {
		t.Errorf("105lb per side should be 25+5, got: %v", plates)
	}
}
