package stats

import (
	"math"
	"math/rand"
	"sort"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
)

type TestResult struct {
	PValue   float64
	Effect   float64
	Estimate float64
}

type Config struct {
	Q               float64
	RefusalFloor    float64
	JSONFloor       float64
	AccuracyFloor   float64
	TruncationFloor float64
	ContinuousFloor float64
	StyleFloor      float64
}

type Change struct {
	Feature     string  `json:"feature"`
	Baseline    float64 `json:"baseline"`
	Current     float64 `json:"current"`
	Direction   string  `json:"direction"`
	Absolute    float64 `json:"absolute"`
	Relative    float64 `json:"relative"`
	RawP        float64 `json:"raw_p"`
	AdjustedP   float64 `json:"adjusted_p"`
	Effect      float64 `json:"effect"`
	EffectFloor float64 `json:"effect_floor"`
	Changed     bool    `json:"changed"`
}

type Detector struct{ Config Config }

func DefaultConfig() Config {
	return Config{Q: 0.05, RefusalFloor: 0.15, JSONFloor: 0.15, AccuracyFloor: 0.15,
		TruncationFloor: 0.15, ContinuousFloor: 0.25, StyleFloor: 0.25}
}

func Detect(baseline, current []features.Features, config Config, seed int64) []Change {
	if config.Q <= 0 || config.Q >= 1 {
		config.Q = 0.05
	}
	tests := make([]Change, 0, 7)
	tests = append(tests,
		proportionChange("refusal", binary(baseline, func(f features.Features) bool { return f.Refused }),
			binary(current, func(f features.Features) bool { return f.Refused }), config.RefusalFloor),
		proportionChange("json_valid", binary(baseline, func(f features.Features) bool { return f.JSONValid }),
			binary(current, func(f features.Features) bool { return f.JSONValid }), config.JSONFloor),
		proportionChange("accuracy", probabilities(baseline, func(f features.Features) float64 { return f.Accuracy }),
			probabilities(current, func(f features.Features) float64 { return f.Accuracy }), config.AccuracyFloor),
		proportionChange("truncation", binary(baseline, func(f features.Features) bool { return f.Truncated }),
			binary(current, func(f features.Features) bool { return f.Truncated }), config.TruncationFloor),
		continuousChange("response_length", values(baseline, func(f features.Features) float64 { return float64(f.ResponseLength) }),
			values(current, func(f features.Features) float64 { return float64(f.ResponseLength) }), config.ContinuousFloor),
		continuousChange("latency", values(baseline, func(f features.Features) float64 { return float64(f.LatencyMS) }),
			values(current, func(f features.Features) float64 { return float64(f.LatencyMS) }), config.ContinuousFloor),
		styleChange(baseline, current, config.StyleFloor, seed),
	)
	adjusted := BenjaminiHochberg(pValues(tests), config.Q)
	for i := range tests {
		tests[i].AdjustedP = adjusted[i]
		tests[i].Changed = adjusted[i] <= config.Q && math.Abs(tests[i].Effect) >= tests[i].EffectFloor
	}
	return tests
}

func FisherExact(a, b, c, d int) float64 {
	if minInt(a, b, c, d) < 0 {
		return 1
	}
	row1, row2, col1, total := a+b, c+d, a+c, a+b+c+d
	observed := hypergeometric(a, row1, row2, col1, total)
	low, high := maxInt(0, col1-row2), minInt(row1, col1)
	p := 0.0
	for x := low; x <= high; x++ {
		prob := hypergeometric(x, row1, row2, col1, total)
		if prob <= observed+1e-12 {
			p += prob
		}
	}
	return clamp(p)
}

func TwoProportionZ(successA, totalA, successB, totalB int) float64 {
	if totalA <= 0 || totalB <= 0 {
		return 1
	}
	pA, pB := float64(successA)/float64(totalA), float64(successB)/float64(totalB)
	pooled := float64(successA+successB) / float64(totalA+totalB)
	se := math.Sqrt(pooled * (1 - pooled) * (1/float64(totalA) + 1/float64(totalB)))
	if se == 0 {
		if pA == pB {
			return 1
		}
		return 0
	}
	return 2 * normalSurvival(math.Abs((pA-pB)/se))
}

func CohensH(p1, p2 float64) float64 {
	p1, p2 = clamp(p1), clamp(p2)
	return 2*math.Asin(math.Sqrt(p2)) - 2*math.Asin(math.Sqrt(p1))
}

func MannWhitney(x, y []float64) TestResult {
	if len(x) == 0 || len(y) == 0 {
		return TestResult{PValue: 1}
	}
	type item struct {
		value float64
		group int
	}
	items := make([]item, 0, len(x)+len(y))
	for _, v := range x {
		items = append(items, item{v, 0})
	}
	for _, v := range y {
		items = append(items, item{v, 1})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].value < items[j].value })
	rankSum := 0.0
	tieCorrection := 0.0
	for i := 0; i < len(items); {
		j := i + 1
		for j < len(items) && items[j].value == items[i].value {
			j++
		}
		rank := float64(i+j+1) / 2
		for k := i; k < j; k++ {
			if items[k].group == 0 {
				rankSum += rank
			}
		}
		t := j - i
		if t > 1 {
			tieCorrection += float64(t*t*t - t)
		}
		i = j
	}
	n1, n2 := float64(len(x)), float64(len(y))
	u := rankSum - n1*(n1+1)/2
	delta := 2*u/(n1*n2) - 1
	n := n1 + n2
	variance := n1 * n2 / 12 * (n + 1 - tieCorrection/(n*(n-1)))
	if variance == 0 {
		return TestResult{PValue: 1, Effect: delta, Estimate: medianShift(x, y)}
	}
	z := (u - n1*n2/2) / math.Sqrt(variance)
	return TestResult{PValue: 2 * normalSurvival(math.Abs(z)), Effect: delta, Estimate: medianShift(x, y)}
}

func KolmogorovSmirnov(x, y []float64) float64 {
	if len(x) == 0 || len(y) == 0 {
		return 1
	}
	a, b := append([]float64(nil), x...), append([]float64(nil), y...)
	sort.Float64s(a)
	sort.Float64s(b)
	i, j, d := 0, 0, 0.0
	for i < len(a) || j < len(b) {
		value := 0.0
		if j == len(b) || (i < len(a) && a[i] <= b[j]) {
			value = a[i]
		} else {
			value = b[j]
		}
		for i < len(a) && a[i] <= value {
			i++
		}
		for j < len(b) && b[j] <= value {
			j++
		}
		d = math.Max(d, math.Abs(float64(i)/float64(len(a))-float64(j)/float64(len(b))))
	}
	en := math.Sqrt(float64(len(a)*len(b)) / float64(len(a)+len(b)))
	return clamp(ksSurvival((en + 0.12 + 0.11/en) * d))
}

func BenjaminiHochberg(p []float64, q float64) []float64 {
	result := make([]float64, len(p))
	type indexed struct {
		value float64
		index int
	}
	sorted := make([]indexed, len(p))
	for i, value := range p {
		sorted[i] = indexed{clamp(value), i}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].value < sorted[j].value })
	previous := 1.0
	for i := len(sorted) - 1; i >= 0; i-- {
		adjusted := sorted[i].value * float64(len(p)) / float64(i+1)
		previous = math.Min(previous, adjusted)
		result[sorted[i].index] = clamp(previous)
	}
	return result
}

func proportionChange(name string, baseline, current []bool, floor float64) Change {
	bTrue, cTrue := countTrue(baseline), countTrue(current)
	b, c := ratio(bTrue, len(baseline)), ratio(cTrue, len(current))
	p := FisherExact(bTrue, len(baseline)-bTrue, cTrue, len(current)-cTrue)
	if len(baseline) >= 20 && len(current) >= 20 {
		p = TwoProportionZ(bTrue, len(baseline), cTrue, len(current))
	}
	return change(name, b, c, p, CohensH(b, c), floor)
}

func continuousChange(name string, baseline, current []float64, floor float64) Change {
	b, c := mean(baseline), mean(current)
	result := MannWhitney(baseline, current)
	ks := KolmogorovSmirnov(baseline, current)
	return change(name, b, c, math.Max(result.PValue, ks), result.Effect, floor)
}

func styleChange(baseline, current []features.Features, floor float64, seed int64) Change {
	b, c := styleDistances(baseline), styleDistances(current)
	baselineEstimate, currentEstimate := mean(b), mean(c)
	p := permutationPValue(b, c, 1000, seed)
	return change("style", baselineEstimate, currentEstimate, p, currentEstimate-baselineEstimate, floor)
}

func change(name string, baseline, current, p, effect, floor float64) Change {
	direction := "unchanged"
	if current > baseline {
		direction = "increase"
	} else if current < baseline {
		direction = "decrease"
	}
	relative := 0.0
	if baseline != 0 {
		relative = math.Abs(current-baseline) / math.Abs(baseline)
	}
	return Change{Feature: name, Baseline: baseline, Current: current, Direction: direction,
		Absolute: math.Abs(current - baseline), Relative: relative, RawP: clamp(p), Effect: effect, EffectFloor: floor}
}

func permutationPValue(a, b []float64, permutations int, seed int64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 1
	}
	observed := math.Abs(mean(a) - mean(b))
	combined := append(append([]float64(nil), a...), b...)
	rng := rand.New(rand.NewSource(seed))
	extreme := 0
	for i := 0; i < permutations; i++ {
		rng.Shuffle(len(combined), func(i, j int) { combined[i], combined[j] = combined[j], combined[i] })
		if math.Abs(mean(combined[:len(a)])-mean(combined[len(a):])) >= observed {
			extreme++
		}
	}
	return float64(extreme+1) / float64(permutations+1)
}

func binary(values []features.Features, fn func(features.Features) bool) []bool {
	result := make([]bool, len(values))
	for i, value := range values {
		result[i] = fn(value)
	}
	return result
}
func probabilities(values []features.Features, fn func(features.Features) float64) []bool {
	result := make([]bool, len(values))
	for i, value := range values {
		result[i] = fn(value) >= 0.5
	}
	return result
}
func values(input []features.Features, fn func(features.Features) float64) []float64 {
	result := make([]float64, len(input))
	for i, value := range input {
		result[i] = fn(value)
	}
	return result
}
func styleDistances(values []features.Features) []float64 {
	empty := features.StyleVector("")
	result := make([]float64, len(values))
	for i, value := range values {
		result[i] = features.CosineDistance(value.Vector, empty)
	}
	return result
}
func pValues(changes []Change) []float64 {
	result := make([]float64, len(changes))
	for i, c := range changes {
		result[i] = c.RawP
	}
	return result
}
func countTrue(values []bool) int {
	result := 0
	for _, value := range values {
		if value {
			result++
		}
	}
	return result
}
func ratio(value, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total)
}
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}
func medianShift(a, b []float64) float64 { return median(b) - median(a) }
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copy := append([]float64(nil), values...)
	sort.Float64s(copy)
	return copy[len(copy)/2]
}
func hypergeometric(x, row1, row2, col1, total int) float64 {
	return math.Exp(logChoose(row1, x) + logChoose(row2, col1-x) - logChoose(total, col1))
}
func logChoose(n, k int) float64 {
	if k < 0 || k > n {
		return math.Inf(-1)
	}
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}
func normalSurvival(z float64) float64 { return 0.5 * math.Erfc(z/math.Sqrt2) }
func ksSurvival(lambda float64) float64 {
	sum := 0.0
	for k := 1; k < 100; k++ {
		term := math.Exp(-2 * float64(k*k) * lambda * lambda)
		if k%2 == 1 {
			sum += term
		} else {
			sum -= term
		}
		if term < 1e-12 {
			break
		}
	}
	return 2 * sum
}
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func minInt(values ...int) int {
	result := values[0]
	for _, v := range values[1:] {
		if v < result {
			result = v
		}
	}
	return result
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
