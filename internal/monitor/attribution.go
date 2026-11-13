package monitor

import (
	"sort"
	"strings"
)

type LayerScore struct {
	Layer     string  `json:"layer"`
	Score     float64 `json:"score"`
	CoreHit   float64 `json:"core_hit"`
	Precision float64 `json:"precision"`
}

type Attribution struct {
	Ranked          []LayerScore `json:"ranked"`
	Margin          float64      `json:"margin"`
	Confidence      string       `json:"confidence"`
	Layers          []string     `json:"layers,omitempty"`
	Scores          []LayerScore `json:"scores,omitempty"`
	ConfigExplained []string     `json:"config_explained,omitempty"`
	ProviderSide    []string     `json:"provider_side,omitempty"`
	Inconclusive    bool         `json:"inconclusive"`
	Reasons         []string     `json:"reasons,omitempty"`
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

func Attribute(input any, rerun ...*ChangeVector) Attribution {
	switch value := input.(type) {
	case Detection:
		return attributeDetection(value)
	case ChangeVector:
		var reference *ChangeVector
		if len(rerun) > 0 {
			reference = rerun[0]
		}
		return attributeChangeVector(value, reference)
	case *ChangeVector:
		if value == nil {
			return Attribution{Confidence: "n/a", Inconclusive: true}
		}
		var reference *ChangeVector
		if len(rerun) > 0 {
			reference = rerun[0]
		}
		return attributeChangeVector(*value, reference)
	default:
		return Attribution{Confidence: "n/a", Inconclusive: true}
	}
}

func attributeDetection(detection Detection) Attribution {
	if !detection.Detected {
		return Attribution{Confidence: "n/a"}
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
	return Attribution{Ranked: ranked, Scores: ranked, Margin: margin, Confidence: confidence,
		Inconclusive: confidence == "inconclusive"}
}

type ChangeVector struct {
	Detected  bool            `json:"detected"`
	Changes   []FeatureChange `json:"changes"`
	Unchanged []string        `json:"unchanged,omitempty"`
}

func attributeChangeVector(current ChangeVector, rerun *ChangeVector) Attribution {
	if !current.Detected {
		return Attribution{Confidence: "n/a"}
	}
	changed := featureSet(current.Changes)
	configured := map[string]bool{}
	provider := make([]string, 0)
	if rerun != nil {
		remaining := featureSet(rerun.Changes)
		for feature := range changed {
			if !remaining[feature] {
				configured[feature] = true
			} else {
				provider = append(provider, feature)
			}
		}
	} else {
		for feature := range changed {
			provider = append(provider, feature)
		}
	}
	ranked := scoreFeatures(provider)
	if len(provider) == 0 && len(configured) > 0 {
		return Attribution{Layers: []string{"request_config"}, Ranked: ranked, Scores: ranked,
			ConfigExplained: sortedKeys(configured), Confidence: "high", Reasons: []string{"pinned rerun cleared all changed features"}}
	}
	layers := make([]string, 0, 2)
	confidence := "inconclusive"
	if len(ranked) > 0 {
		layers = append(layers, ranked[0].Layer)
		margin := ranked[0].Score
		if len(ranked) > 1 {
			margin -= ranked[1].Score
		}
		if ranked[0].Score >= 0.8 && margin >= 0.3 {
			confidence = "high"
		} else if ranked[0].Score >= 0.4 && margin >= 0.15 {
			confidence = "medium"
		} else if len(ranked) > 1 && ranked[1].Score >= 0.4 {
			layers = append(layers, ranked[1].Layer)
		}
	}
	if len(configured) > 0 {
		layers = append([]string{"request_config"}, layers...)
	}
	return Attribution{Layers: layers, Ranked: ranked, Scores: ranked, Margin: marginOf(ranked),
		ConfigExplained: sortedKeys(configured), ProviderSide: provider, Confidence: confidence,
		Inconclusive: confidence == "inconclusive", Reasons: []string{"changed features: " + strings.Join(provider, ", ")}}
}

func featureSet(changes []FeatureChange) map[string]bool {
	result := make(map[string]bool, len(changes))
	for _, change := range changes {
		result[change.Feature] = true
	}
	return result
}

func scoreFeatures(features []string) []LayerScore {
	changed := make(map[string]bool, len(features))
	for _, feature := range features {
		changed[feature] = true
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
		precision := safeDivide(hits, float64(len(features)))
		coreHit := safeDivide(core, float64(len(signature.Core)))
		ranked = append(ranked, LayerScore{Layer: layer, Precision: precision, CoreHit: coreHit,
			Score: precision * (0.5 + 0.5*coreHit)})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
	return ranked
}

func marginOf(scores []LayerScore) float64 {
	if len(scores) < 2 {
		if len(scores) == 1 {
			return scores[0].Score
		}
		return 0
	}
	return scores[0].Score - scores[1].Score
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
