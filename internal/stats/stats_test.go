package stats

import (
	"math"
	"math/rand"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
)

func TestFisherExactLadyTastingTea(t *testing.T) {
	// Four cups were correctly identified and one incorrectly identified in
	// each direction: the classic 2x2 table has a two-sided p ~= 0.4857.
	p := FisherExact(3, 1, 1, 3)
	if math.Abs(p-0.4857142857) > 1e-9 {
		t.Fatalf("p = %.12f, want approximately 0.4857142857", p)
	}
}

func TestProportionAndEffectSize(t *testing.T) {
	if p := TwoProportionZ(50, 100, 50, 100); p < 0.99 {
		t.Fatalf("identical proportions p = %v", p)
	}
	if effect := CohensH(0.1, 0.9); effect < 1 {
		t.Fatalf("Cohen's h = %v", effect)
	}
	if p := FisherExact(0, 10, 10, 0); p >= 0.01 {
		t.Fatalf("extreme Fisher table p = %v", p)
	}
}

func TestContinuousTestsHandleTiesAndIdenticalValues(t *testing.T) {
	identical := []float64{1, 1, 1, 1}
	result := MannWhitney(identical, identical)
	if result.PValue < 0.99 || math.Abs(result.Effect) > 1e-12 {
		t.Fatalf("identical Mann-Whitney result: %+v", result)
	}
	if p := KolmogorovSmirnov(identical, identical); p < 0.99 {
		t.Fatalf("identical KS p = %v", p)
	}
	shifted := MannWhitney([]float64{1, 2, 3}, []float64{10, 11, 12})
	if shifted.PValue >= 0.1 || shifted.Effect >= 0 {
		t.Fatalf("shifted Mann-Whitney result: %+v", shifted)
	}
}

func TestBenjaminiHochberg(t *testing.T) {
	adjusted := BenjaminiHochberg([]float64{0.001, 0.02, 0.5, 0.01}, 0.05)
	if adjusted[0] > adjusted[3] || adjusted[3] > adjusted[1] || adjusted[1] >= 0.5 {
		t.Fatalf("unexpected adjusted p-values: %v", adjusted)
	}
}

func TestDetectorKnownChangeAndNullReproducibility(t *testing.T) {
	vector := features.StyleVector("stable")
	baseline := make([]features.Features, 40)
	current := make([]features.Features, 40)
	for i := range baseline {
		baseline[i] = features.Features{Accuracy: 1, Vector: vector, ResponseLength: 10, LatencyMS: 5}
		current[i] = features.Features{Accuracy: 0, Refused: true, Vector: features.StyleVector("changed output"), ResponseLength: 40, LatencyMS: 20}
	}
	config := DefaultConfig()
	changes := Detect(baseline, current, config, 9)
	found := false
	for _, change := range changes {
		if change.Feature == "refusal" && change.Changed {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected refusal change: %+v", changes)
	}
	first := Detect(baseline, baseline, config, 9)
	second := Detect(baseline, baseline, config, 9)
	for i := range first {
		if first[i].RawP != second[i].RawP || first[i].AdjustedP != second[i].AdjustedP {
			t.Fatalf("detector is not reproducible: %v != %v", first, second)
		}
	}
}

func TestNullSimulationIsNotSystematicallySignificant(t *testing.T) {
	rng := rand.New(rand.NewSource(22))
	significant := 0
	for i := 0; i < 100; i++ {
		a, b := make([]float64, 20), make([]float64, 20)
		for j := range a {
			a[j] = rng.NormFloat64()
			b[j] = rng.NormFloat64()
		}
		if MannWhitney(a, b).PValue < 0.05 {
			significant++
		}
	}
	if significant > 20 {
		t.Fatalf("null simulation produced too many significant results: %d/100", significant)
	}
}
