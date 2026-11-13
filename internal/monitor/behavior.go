package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

// BehaviorThresholds controls the independent latency comparison. Behavioral
// regressions are directional and are never offset by improvements.
type BehaviorThresholds struct {
	LatencyRelative float64
}

type CheckTransition struct {
	CanaryID   string `json:"canary_id"`
	CheckID    string `json:"check_id"`
	CheckType  string `json:"check_type"`
	Transition string `json:"transition"`
}

// BehavioralReport is the shared, check-level result used by API consumers.
// Statistical significance describes evidence, not practical severity: a
// single deterministic regression can be operationally important while still
// being statistically insufficient for a p-value claim.
type BehavioralReport struct {
	BehavioralDriftDetected bool              `json:"behavioral_drift_detected"`
	LatencyDriftDetected    bool              `json:"latency_drift_detected"`
	RegressionCount         int               `json:"regression_count"`
	ImprovementCount        int               `json:"improvement_count"`
	ComparableCheckCount    int               `json:"comparable_check_count"`
	UnavailableCheckCount   int               `json:"unavailable_check_count"`
	RegressionRate          float64           `json:"regression_rate"`
	EffectSize              float64           `json:"effect_size"`
	StatisticalPValue       float64           `json:"statistical_p_value"`
	StatisticalSignificance string            `json:"statistical_significance"`
	AttributedLayer         string            `json:"attributed_layer"`
	AttributionConfidence   string            `json:"attribution_confidence"`
	Evidence                []string          `json:"evidence"`
	Limitations             []string          `json:"limitations"`
	Transitions             []CheckTransition `json:"transitions,omitempty"`
	StableFailureCount      int               `json:"stable_failure_count"`
	MissingCheckCount       int               `json:"missing_check_count"`
	MissingCanaryCount      int               `json:"missing_canary_count"`
	InvalidCheckCount       int               `json:"invalid_check_count"`
}

type indexedCheck struct {
	check CheckObservation
	key   string
}

// CompareBehavioral compares paired canaries and checks by stable identity.
// Legacy records without check_id use type+expected only when that key is
// unique within the canary; duplicate legacy checks are explicitly invalid.
func CompareBehavioral(reference, current []Observation, thresholds *BehaviorThresholds) BehavioralReport {
	report := BehavioralReport{StatisticalSignificance: "insufficient_data", StatisticalPValue: 1, AttributedLayer: "unknown", AttributionConfidence: "inconclusive", Evidence: []string{}, Limitations: []string{}}
	base := make(map[string]Observation, len(reference))
	for _, observation := range reference {
		if observation.CanaryID != "" {
			if _, exists := base[observation.CanaryID]; !exists {
				base[observation.CanaryID] = observation
			}
		}
	}
	seenCanaries := make(map[string]bool, len(current))
	var baselinePasses, discordant int
	var latencyBase, latencyCurrent []float64
	for _, candidate := range current {
		previous, ok := base[candidate.CanaryID]
		seenCanaries[candidate.CanaryID] = true
		if !ok {
			report.MissingCanaryCount++
			continue
		}
		baseChecks, baseInvalid := indexChecks(previous.Checks)
		currentChecks, currentInvalid := indexChecks(candidate.Checks)
		report.InvalidCheckCount += baseInvalid + currentInvalid
		keys := make(map[string]bool, len(baseChecks)+len(currentChecks))
		for key := range baseChecks {
			keys[key] = true
		}
		for key := range currentChecks {
			keys[key] = true
		}
		for key := range keys {
			before, hasBefore := baseChecks[key]
			after, hasAfter := currentChecks[key]
			if !hasBefore || !hasAfter {
				report.MissingCheckCount++
				continue
			}
			if before.check.Type != after.check.Type {
				report.InvalidCheckCount++
				continue
			}
			if before.check.Unavailable || after.check.Unavailable {
				report.UnavailableCheckCount++
				report.Transitions = append(report.Transitions, CheckTransition{candidate.CanaryID, key, after.check.Type, "unavailable"})
				continue
			}
			report.ComparableCheckCount++
			if before.check.Passed {
				baselinePasses++
			}
			transition := "stable_failure"
			switch {
			case before.check.Passed && !after.check.Passed:
				report.RegressionCount++
				discordant++
				transition = "regression"
				report.Evidence = append(report.Evidence, fmt.Sprintf("%s/%s: PASS -> FAIL", candidate.CanaryID, key))
			case !before.check.Passed && after.check.Passed:
				report.ImprovementCount++
				discordant++
				transition = "improvement"
			case !before.check.Passed:
				report.StableFailureCount++
			case before.check.Passed:
				transition = "stable_pass"
			}
			report.Transitions = append(report.Transitions, CheckTransition{candidate.CanaryID, key, after.check.Type, transition})
		}
		if previous.Error == "" && candidate.Error == "" && previous.LatencyMS > 0 && candidate.LatencyMS > 0 {
			latencyBase = append(latencyBase, float64(previous.LatencyMS))
			latencyCurrent = append(latencyCurrent, float64(candidate.LatencyMS))
		}
	}
	for id := range base {
		if !seenCanaries[id] {
			report.MissingCanaryCount++
		}
	}
	if baselinePasses > 0 {
		report.RegressionRate = float64(report.RegressionCount) / float64(baselinePasses)
	}
	// This is a one-sided practical effect: improvements cannot cancel regressions.
	report.EffectSize = report.RegressionRate
	report.BehavioralDriftDetected = report.RegressionCount > 0
	if discordant >= 10 {
		report.StatisticalPValue = mcnemarExactP(report.RegressionCount, report.ImprovementCount)
		if report.StatisticalPValue <= .05 {
			report.StatisticalSignificance = "significant"
		} else {
			report.StatisticalSignificance = "not_significant"
		}
	} else {
		report.Limitations = append(report.Limitations, "fewer than 10 discordant paired checks; McNemar exact test is reported as insufficient")
	}
	if len(latencyBase) > 0 {
		if thresholds == nil || thresholds.LatencyRelative <= 0 {
			thresholds = &BehaviorThresholds{LatencyRelative: .2}
		}
		report.LatencyDriftDetected = Median(latencyCurrent) > Median(latencyBase)*(1+thresholds.LatencyRelative)
		if report.LatencyDriftDetected {
			report.Evidence = append(report.Evidence, "matched-canary median latency increased beyond threshold")
		}
	}
	if report.ComparableCheckCount == 0 {
		report.Limitations = append(report.Limitations, "no comparable check pairs")
	}
	return report
}

func indexChecks(checks []CheckObservation) (map[string]indexedCheck, int) {
	indexed := make(map[string]indexedCheck, len(checks))
	invalidKeys := make(map[string]bool)
	invalid := 0
	for _, check := range checks {
		key := check.CheckID
		if key == "" {
			key = "legacy:" + check.Type + ":" + check.Expected
		}
		if invalidKeys[key] {
			invalid++
			continue
		}
		if _, exists := indexed[key]; exists {
			invalid++
			delete(indexed, key)
			invalidKeys[key] = true
			continue
		}
		indexed[key] = indexedCheck{check: check, key: key}
	}
	return indexed, invalid
}

func stableCheckID(check canary.Check) string {
	keys := make([]string, 0, len(check.Params))
	for key := range check.Params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := []any{check.Type, check.Value, check.Values}
	for _, key := range keys {
		fields = append(fields, key, json.RawMessage(check.Params[key]))
	}
	raw, _ := json.Marshal(fields)
	sum := sha256.Sum256(raw)
	return "check-" + hex.EncodeToString(sum[:8])
}

func mcnemarExactP(regressions, improvements int) float64 {
	d := regressions + improvements
	if d == 0 {
		return 1
	}
	smaller := regressions
	if improvements < smaller {
		smaller = improvements
	}
	prob := math.Pow(.5, float64(d))
	sum := 0.0
	for k := 0; k <= smaller; k++ {
		sum += binomial(d, k) * prob
	}
	return math.Min(1, 2*sum)
}

func binomial(n, k int) float64 {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	result := 1.0
	for i := 1; i <= k; i++ {
		result *= float64(n-k+i) / float64(i)
	}
	return result
}

// AttributeBehavioral uses observed metadata only. Empty hashes and absent
// model/runtime fields are not evidence for request configuration changes.
func AttributeBehavioral(reference, current []Observation, report BehavioralReport) (string, string, []string) {
	if !report.BehavioralDriftDetected {
		return "unknown", "n/a", nil
	}
	base, candidate := firstComparable(reference), firstComparable(current)
	if base.RequestConfigHash != "" && candidate.RequestConfigHash != "" && base.RequestConfigHash != candidate.RequestConfigHash {
		return "request_config", "high", []string{"non-empty request configuration hashes differ"}
	}
	if (base.RequestedModel != "" && candidate.RequestedModel != "" && base.RequestedModel != candidate.RequestedModel) ||
		(base.ReturnedModel != "" && candidate.ReturnedModel != "" && base.ReturnedModel != candidate.ReturnedModel) {
		return "model", "high", []string{"observed requested or returned model changed"}
	}
	if base.SystemFingerprint != nil && candidate.SystemFingerprint != nil && *base.SystemFingerprint != *candidate.SystemFingerprint {
		return "serving", "medium", []string{"observed serving system fingerprint changed"}
	}
	for _, transition := range report.Transitions {
		if transition.Transition == "regression" && strings.Contains(transition.CheckType, "refus") {
			return "safety", "low", []string{"regression is refusal-related but no causal metadata is present"}
		}
	}
	return "unknown", "inconclusive", []string{"experiment metadata is absent or insufficient for causal attribution"}
}

func firstComparable(observations []Observation) Observation {
	for _, observation := range observations {
		if observation.Error == "" {
			return observation
		}
	}
	return Observation{}
}
