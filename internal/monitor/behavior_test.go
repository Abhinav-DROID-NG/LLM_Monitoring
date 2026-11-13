package monitor

import "testing"

func check(id, typ string, passed bool) CheckObservation {
	return CheckObservation{CheckID: id, Type: typ, Expected: typ, Passed: passed}
}

func TestCompareBehavioralMatchesChecksByIDWhenReordered(t *testing.T) {
	base := []Observation{{CanaryID: "C1", Checks: []CheckObservation{
		check("a", "contains", true), check("b", "json_valid", true),
	}}}
	current := []Observation{{CanaryID: "C1", Checks: []CheckObservation{
		check("b", "json_valid", true), check("a", "contains", false),
	}}}

	report := CompareBehavioral(base, current, nil)
	if !report.BehavioralDriftDetected || report.RegressionCount != 1 || report.ImprovementCount != 0 {
		t.Fatalf("report=%+v", report)
	}
	if report.ComparableCheckCount != 2 || report.RegressionRate != .5 {
		t.Fatalf("comparison accounting=%+v", report)
	}
}

func TestCompareBehavioralDoesNotCancelRegressionsWithImprovements(t *testing.T) {
	base := []Observation{{CanaryID: "C1", Checks: []CheckObservation{
		check("regress", "contains", true), check("improve", "contains", false), check("stable", "contains", true),
	}}}
	current := []Observation{{CanaryID: "C1", Checks: []CheckObservation{
		check("stable", "contains", true), check("improve", "contains", true), check("regress", "contains", false),
	}}}

	report := CompareBehavioral(base, current, nil)
	if report.RegressionCount != 1 || report.ImprovementCount != 1 || !report.BehavioralDriftDetected {
		t.Fatalf("report=%+v", report)
	}
}

func TestCompareBehavioralExcludesUnavailableAndMissingChecks(t *testing.T) {
	base := []Observation{{CanaryID: "C1", Checks: []CheckObservation{
		check("pass", "contains", true), check("missing", "contains", true), check("unavailable", "finish_reason", true),
	}}}
	base[0].Checks[2].Unavailable = true
	current := []Observation{{CanaryID: "C1", Checks: []CheckObservation{
		check("pass", "contains", true), {CheckID: "unavailable", Type: "finish_reason", Unavailable: true},
	}}}

	report := CompareBehavioral(base, current, nil)
	if report.ComparableCheckCount != 1 || report.UnavailableCheckCount != 1 || report.MissingCheckCount != 1 {
		t.Fatalf("report=%+v", report)
	}
}

func TestCompareBehavioralFlagsInsufficientStatisticalEvidence(t *testing.T) {
	report := CompareBehavioral(
		[]Observation{{CanaryID: "C1", Checks: []CheckObservation{check("a", "contains", true)}}},
		[]Observation{{CanaryID: "C1", Checks: []CheckObservation{check("a", "contains", false)}}}, nil)
	if report.StatisticalSignificance != "insufficient_data" || report.StatisticalPValue != 1 {
		t.Fatalf("report=%+v", report)
	}
	if !report.BehavioralDriftDetected {
		t.Fatal("a newly failing comparable check is a practical behavioral regression")
	}
}

func TestCompareBehavioralKeepsLatencySeparate(t *testing.T) {
	base := []Observation{{CanaryID: "C1", LatencyMS: 100, Checks: []CheckObservation{check("a", "contains", true)}}}
	current := []Observation{{CanaryID: "C1", LatencyMS: 300, Checks: []CheckObservation{check("a", "contains", true)}}}
	report := CompareBehavioral(base, current, &BehaviorThresholds{LatencyRelative: .2})
	if report.BehavioralDriftDetected || !report.LatencyDriftDetected {
		t.Fatalf("report=%+v", report)
	}
}

func TestCompareBehavioralRejectsDuplicateCheckIDs(t *testing.T) {
	base := []Observation{{CanaryID: "C1", Checks: []CheckObservation{check("a", "contains", true), check("a", "contains", true)}}}
	current := []Observation{{CanaryID: "C1", Checks: []CheckObservation{check("a", "contains", false)}}}
	report := CompareBehavioral(base, current, nil)
	if report.InvalidCheckCount != 1 || report.RegressionCount != 0 {
		t.Fatalf("report=%+v", report)
	}
}
