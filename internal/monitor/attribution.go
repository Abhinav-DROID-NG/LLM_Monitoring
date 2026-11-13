package monitor

import "sort"

type LayerScore struct {
	Layer     string  `json:"layer"`
	Score     float64 `json:"score"`
	CoreHit   float64 `json:"core_hit"`
	Precision float64 `json:"precision"`
}

type Attribution struct {
	Ranked     []LayerScore `json:"ranked"`
	Margin     float64      `json:"margin"`
	Confidence string       `json:"confidence"`
}

var layerSignatures = map[string]struct {
	CanChange []string
	Core      []string
}{
	"safety":         {CanChange: []string{"refusal_rate"}, Core: []string{"refusal_rate"}},
	"serving":        {[]string{"latency_p50", "latency_p95", "output_token_count"}, []string{"latency_p50"}},
	"model":          {[]string{"output_token_count", "json_validity_rate"}, []string{"output_token_count"}},
	"request_config": {[]string{"finish_reason_length_rate", "json_validity_rate", "structure_compliance"}, []string{"finish_reason_length_rate"}},
}

func Attribute(detection Detection) Attribution {
	if !detection.Detected {
		return Attribution{Confidence: "inconclusive"}
	}
	changed := make(map[string]bool, len(detection.Changes))
	for _, change := range detection.Changes {
		changed[change.Feature] = true
	}
	ranked := make([]LayerScore, 0, len(layerSignatures))
	for layer, signature := range layerSignatures {
		var hits, core float64
		for _, feature := range signature.CanChange {
			if changed[feature] {
				hits++
			}
		}
		for _, feature := range signature.Core {
			if changed[feature] {
				core++
			}
		}
		precision := safeDivide(hits, float64(len(signature.CanChange)))
		coreHit := safeDivide(core, float64(len(signature.Core)))
		ranked = append(ranked, LayerScore{Layer: layer, Precision: precision, CoreHit: coreHit,
			Score: precision * (0.5 + 0.5*coreHit)})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
	margin := ranked[0].Score
	if len(ranked) > 1 {
		margin -= ranked[1].Score
	}
	confidence := "inconclusive"
	if ranked[0].Score >= 0.8 && margin >= 0.3 {
		confidence = "high"
	} else if ranked[0].Score >= 0.4 && margin >= 0.15 {
		confidence = "medium"
	}
	return Attribution{Ranked: ranked, Margin: margin, Confidence: confidence}
}
