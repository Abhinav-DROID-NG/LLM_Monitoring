package main

import (
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
)

func TestBehavioralComparisonIsSharedByServerResults(t *testing.T) {
	srv := &apiServer{thresholds: monitor.Thresholds{MinEffect: map[string]float64{"latency_relative": .2}}}
	base := []monitor.Observation{{CanaryID: "C1", Checks: []monitor.CheckObservation{{CheckID: "check-a", Type: "contains", Passed: true}}}}
	current := []monitor.Observation{{CanaryID: "C1", Checks: []monitor.CheckObservation{{CheckID: "check-a", Type: "contains", Passed: false}}}}
	report := srv.behavioralComparison(base, current)
	if !report.BehavioralDriftDetected || report.RegressionCount != 1 || report.AttributedLayer != "unknown" {
		t.Fatalf("report=%+v", report)
	}
	if report.AttributionConfidence != "inconclusive" || len(report.Evidence) == 0 {
		t.Fatalf("report lacks consistent evidence=%+v", report)
	}
}

func TestBehavioralComparisonAttributesObservedModelChange(t *testing.T) {
	srv := &apiServer{}
	base := []monitor.Observation{{CanaryID: "C1", RequestedModel: "fp16", Checks: []monitor.CheckObservation{{CheckID: "a", Type: "contains", Passed: true}}}}
	current := []monitor.Observation{{CanaryID: "C1", RequestedModel: "q4", Checks: []monitor.CheckObservation{{CheckID: "a", Type: "contains", Passed: false}}}}
	report := srv.behavioralComparison(base, current)
	if report.AttributedLayer != "model" || report.AttributionConfidence != "high" {
		t.Fatalf("report=%+v", report)
	}
}
