package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/attribution"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/stats"
)

type Report struct {
	RunID         string
	TimeWindow    string
	Baseline      string
	ProbeCount    int
	FailedProbes  int
	TokenCount    int
	EstimatedCost float64
	Changes       []stats.Change
	Attribution   attribution.Report
	Limitations   []string
}

func New(runID, baseline string, started, ended time.Time, probeCount, failed, tokens int, cost float64, changes []stats.Change, inference attribution.Report) Report {
	return Report{
		RunID: runID, TimeWindow: started.UTC().Format(time.RFC3339) + " to " + ended.UTC().Format(time.RFC3339),
		Baseline: baseline, ProbeCount: probeCount, FailedProbes: failed, TokenCount: tokens,
		EstimatedCost: cost, Changes: changes, Attribution: inference,
	}
}

func Markdown(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Model Monitor Report\n\n")
	fmt.Fprintf(&b, "**Run:** `%s`  \n**Time window:** %s  \n**Baseline:** `%s`  \n**Probes:** %d  \n**Failed probes:** %d  \n**Tokens:** %d  \n**Estimated cost:** %.6f\n\n",
		r.RunID, r.TimeWindow, r.Baseline, r.ProbeCount, r.FailedProbes, r.TokenCount, r.EstimatedCost)
	b.WriteString("## OBSERVED (facts)\n\n")
	b.WriteString("| Feature | Baseline | Current | Direction | Raw p | Adjusted p | Effect | Floor | Changed |\n")
	b.WriteString("|---|---:|---:|---|---:|---:|---:|---:|---|\n")
	for _, change := range r.Changes {
		fmt.Fprintf(&b, "| %s | %.4g | %.4g | %s | %.4g | %.4g | %.4g | %.4g | %t |\n",
			change.Feature, change.Baseline, change.Current, change.Direction,
			change.RawP, change.AdjustedP, change.Effect, change.EffectFloor, change.Changed)
	}
	b.WriteString("\n## LIKELY CAUSE (inference)\n\n")
	fmt.Fprintf(&b, "**Verdict:** `%s`\n\n", r.Attribution.Verdict)
	b.WriteString("| Candidate | Score | Coverage |\n|---|---:|---:|\n")
	for _, candidate := range r.Attribution.Candidates {
		fmt.Fprintf(&b, "| %s | %.4f | %.4f |\n", candidate.Layer, candidate.Score, candidate.Coverage)
	}
	if len(r.Attribution.FollowUps) > 0 {
		b.WriteString("\n### Follow-up probes\n\n")
		for _, followUp := range r.Attribution.FollowUps {
			fmt.Fprintf(&b, "- **%s:** %s\n", followUp.Kind, followUp.Reason)
		}
	}
	if len(r.Attribution.Outcomes) > 0 {
		b.WriteString("\n### Follow-up outcomes\n\n")
		for _, outcome := range r.Attribution.Outcomes {
			fmt.Fprintf(&b, "- **%s:** supported=%t — %s\n", outcome.FollowUp.Kind, outcome.Supported, outcome.Evidence)
		}
	}
	if len(r.Limitations) > 0 {
		b.WriteString("\n## Limitations\n\n")
		for _, limitation := range r.Limitations {
			fmt.Fprintf(&b, "- %s\n", limitation)
		}
	}
	return b.String()
}

func CSV(w io.Writer, r Report) error {
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"feature", "baseline", "current", "direction", "raw_p", "adjusted_p", "effect", "effect_floor", "changed"}); err != nil {
		return err
	}
	for _, change := range r.Changes {
		if err := writer.Write([]string{change.Feature, strconv.FormatFloat(change.Baseline, 'g', -1, 64),
			strconv.FormatFloat(change.Current, 'g', -1, 64), change.Direction,
			strconv.FormatFloat(change.RawP, 'g', -1, 64), strconv.FormatFloat(change.AdjustedP, 'g', -1, 64),
			strconv.FormatFloat(change.Effect, 'g', -1, 64), strconv.FormatFloat(change.EffectFloor, 'g', -1, 64),
			strconv.FormatBool(change.Changed)}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
