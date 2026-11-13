package monitor

import (
	"math"
	"testing"
)

func TestStatisticalHelpers(t *testing.T) {
	if got := Median([]float64{1, 4, 2}); got != 2 {
		t.Fatalf("median = %v", got)
	}
	if got := Percentile([]float64{0, 10}, 0.25); got != 2.5 {
		t.Fatalf("percentile = %v", got)
	}
	if got := Levenshtein("kitten", "sitting"); got != 3 {
		t.Fatalf("levenshtein = %d", got)
	}
	adjusted, rejected := BenjaminiHochberg([]float64{0.001, 0.04, 0.8}, 0.05)
	if len(adjusted) != 3 || !rejected[0] || rejected[2] || math.IsNaN(adjusted[1]) {
		t.Fatalf("BH result = %v, %v", adjusted, rejected)
	}
}

func TestTwoProportionAndMannWhitney(t *testing.T) {
	z, p := TwoProportionZ(1, 10, 9, 10)
	if z <= 0 || p >= 0.05 {
		t.Fatalf("unexpected proportion test: z=%v p=%v", z, p)
	}
	_, _, p, rank := MannWhitneyU([]float64{1, 2, 3, 4}, []float64{5, 6, 7, 8})
	if p <= 0 || rank <= 0 {
		t.Fatalf("unexpected Mann-Whitney result: p=%v rank=%v", p, rank)
	}
}
