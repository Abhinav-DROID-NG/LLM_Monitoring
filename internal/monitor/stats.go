package monitor

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

type Thresholds struct {
	FDRQ           float64            `json:"fdr_q"`
	MinEffect      map[string]float64 `json:"min_effect"`
	NoiseFloors    map[string]float64 `json:"noise_floors"`
	MinSamples     map[string]int     `json:"min_samples"`
	BaselineCycles int                `json:"baseline_cycles"`
}

func LoadThresholds(path string) (Thresholds, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Thresholds{}, err
	}
	var thresholds Thresholds
	if err := json.Unmarshal(data, &thresholds); err != nil {
		return Thresholds{}, err
	}
	return thresholds, nil
}

func TwoProportionZ(x1, n1, x2, n2 float64) (float64, float64) {
	if n1 <= 0 || n2 <= 0 {
		return 0, 1
	}
	p1, p2 := x1/n1, x2/n2
	smoothed := ((x1+0.5)/(n1+1) + (x2+0.5)/(n2+1)) / 2
	se := math.Sqrt(smoothed * (1 - smoothed) * (1/n1 + 1/n2))
	if se == 0 {
		return 0, 1
	}
	z := (p2 - p1) / se
	return z, Normal2SidedP(z)
}

func MannWhitneyU(a, b []float64) (u, z, p, rankBiserial float64) {
	if len(a) == 0 || len(b) == 0 {
		return 0, 0, 1, 0
	}
	type ranked struct {
		value float64
		group int
	}
	values := make([]ranked, 0, len(a)+len(b))
	for _, value := range a {
		values = append(values, ranked{value, 0})
	}
	for _, value := range b {
		values = append(values, ranked{value, 1})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].value < values[j].value })
	ranks := make([]float64, len(values))
	tieCounts := make([]int, 0)
	for i := 0; i < len(values); {
		j := i + 1
		for j < len(values) && values[j].value == values[i].value {
			j++
		}
		rank := float64(i+j+1) / 2
		for k := i; k < j; k++ {
			ranks[k] = rank
		}
		tieCounts = append(tieCounts, j-i)
		i = j
	}
	rankA := 0.0
	for i, value := range values {
		if value.group == 0 {
			rankA += ranks[i]
		}
	}
	n1, n2 := float64(len(a)), float64(len(b))
	u = rankA - n1*(n1+1)/2
	mean := n1 * n2 / 2
	total := n1 + n2
	tieCorrection := 0.0
	for _, count := range tieCounts {
		tieCorrection += float64(count*count*count - count)
	}
	variance := n1 * n2 / 12 * (total + 1 - tieCorrection/(total*(total-1)))
	if variance <= 0 {
		return u, 0, 1, 0
	}
	z = (u - mean) / math.Sqrt(variance)
	p = Normal2SidedP(z)
	rankBiserial = 1 - 2*u/(n1*n2)
	return
}

func BenjaminiHochberg(p []float64, q float64) ([]float64, []bool) {
	type item struct {
		value float64
		index int
	}
	items := make([]item, len(p))
	for i, value := range p {
		items[i] = item{value, i}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].value < items[j].value })
	adjusted := make([]float64, len(p))
	rejected := make([]bool, len(p))
	previous := 1.0
	for i := len(items) - 1; i >= 0; i-- {
		value := items[i].value * float64(len(p)) / float64(i+1)
		previous = math.Min(previous, value)
		adjusted[items[i].index] = math.Min(1, previous)
	}
	for i := range adjusted {
		rejected[i] = adjusted[i] <= q
	}
	return adjusted, rejected
}

func Normal2SidedP(z float64) float64 {
	return math.Erfc(math.Abs(z) / math.Sqrt2)
}

func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	p = math.Max(0, math.Min(1, p))
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	position := p * float64(len(sorted)-1)
	lower := int(position)
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	return sorted[lower] + (sorted[upper]-sorted[lower])*(position-float64(lower))
}

func MAD(values []float64) float64 {
	median := Median(values)
	deviations := make([]float64, len(values))
	for i, value := range values {
		deviations[i] = math.Abs(value - median)
	}
	return Median(deviations)
}

func Levenshtein(a, b string) int {
	left, right := []rune(a), []rune(b)
	if len(left) == 0 {
		return utf8.RuneCountInString(b)
	}
	if len(right) == 0 {
		return len(left)
	}
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, leftRune := range left {
		current[0] = i + 1
		for j, rightRune := range right {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[j+1] = minInt(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func normalizedEditDistance(a, b string) float64 {
	denominator := math.Max(1, float64(len([]rune(strings.TrimSpace(a)))))
	return float64(Levenshtein(strings.TrimSpace(a), strings.TrimSpace(b))) / denominator
}
