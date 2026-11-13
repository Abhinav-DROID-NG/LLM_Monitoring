package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
)

type CheckReport struct {
	Status              string              `json:"status"`
	Evidence            monitor.Detection   `json:"observed_evidence"`
	Cause               monitor.Attribution `json:"likely_cause"`
	SuggestedNextCheck  string              `json:"suggested_next_check"`
	Calls               int                 `json:"calls"`
	InputTokens         int                 `json:"input_tokens"`
	OutputTokens        int                 `json:"output_tokens"`
	Seconds             float64             `json:"seconds"`
	RunID               string              `json:"run_id,omitempty"`
	Profile             string              `json:"profile,omitempty"`
	BaselineID          string              `json:"baseline_id,omitempty"`
	Unchanged           []string            `json:"unchanged,omitempty"`
	ConfigHashBaseline  string              `json:"config_hash_baseline,omitempty"`
	ConfigHashCurrent   string              `json:"config_hash_current,omitempty"`
	UnavailableMetadata int                 `json:"unavailable_metadata,omitempty"`
}

func New(check monitor.Detection, cause monitor.Attribution, calls, input, output int, seconds float64) CheckReport {
	status := "STABLE"
	if check.Detected {
		status = "CHANGE DETECTED"
	}
	return CheckReport{Status: status, Evidence: check, Cause: cause,
		SuggestedNextCheck: nextCheck(cause), Calls: calls, InputTokens: input,
		OutputTokens: output, Seconds: seconds}
}

func Markdown(report CheckReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Model monitor\n\n## STATUS\n%s\n\n## OBSERVED EVIDENCE\n", report.Status)
	if len(report.Evidence.Changes) == 0 {
		b.WriteString("No feature crossed the detection threshold.\n")
	} else {
		for _, change := range report.Evidence.Changes {
			fmt.Fprintf(&b, "- **%s** %s: baseline %.4g, now %.4g, deviation %.2f sigma (p=%.4g)\n",
				change.Feature, change.Direction, change.Baseline, change.Current, change.EffectSize, change.PValue)
		}
	}
	b.WriteString("\n## UNCHANGED FEATURES\n")
	if len(report.Unchanged) == 0 {
		b.WriteString("None recorded.\n")
	} else {
		b.WriteString(strings.Join(report.Unchanged, ", ") + "\n")
	}
	b.WriteString("\n## LIKELY CAUSE (INFERENCE, NOT PROOF)\n")
	if len(report.Cause.Ranked) == 0 {
		b.WriteString("Inconclusive.\n")
	} else {
		for _, layer := range report.Cause.Ranked {
			fmt.Fprintf(&b, "- %s: score %.2f\n", layer.Layer, layer.Score)
		}
		fmt.Fprintf(&b, "Confidence: **%s** (margin %.2f)\n", report.Cause.Confidence, report.Cause.Margin)
		for _, reason := range report.Cause.Reasons {
			fmt.Fprintf(&b, "- %s\n", reason)
		}
	}
	fmt.Fprintf(&b, "\n## SUGGESTED NEXT CHECK\n%s\n\n## PROBE COST\n%d calls, %d input tokens, %d output tokens, %.3f seconds\n",
		report.SuggestedNextCheck, report.Calls, report.InputTokens, report.OutputTokens, report.Seconds)
	if report.ConfigHashBaseline != "" || report.ConfigHashCurrent != "" {
		fmt.Fprintf(&b, "Request config hash: baseline `%s`, current `%s`.\n", report.ConfigHashBaseline, report.ConfigHashCurrent)
	}
	fmt.Fprintf(&b, "\nScores are ranked evidence, not calibrated probabilities.\n")
	return b.String()
}

func JSON(report CheckReport) ([]byte, error) { return json.MarshalIndent(report, "", "  ") }

func nextCheck(cause monitor.Attribution) string {
	if cause.Confidence == "inconclusive" || len(cause.Ranked) == 0 {
		return "Run a pinned-config rerun with the expensive tier."
	}
	return "Pin the request configuration and rerun the affected divisions."
}
