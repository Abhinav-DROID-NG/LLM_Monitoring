package evaluation

import (
	"encoding/json"
	"regexp"
	"strings"
)

type EvaluationResult struct {
	Status TrialStatus
	Score  float64
	Detail string
}

func Evaluate(spec EvaluatorSpec, expected, output string) EvaluationResult {
	switch spec.Type {
	case "exact_match":
		if strings.TrimSpace(output) == strings.TrimSpace(expected) {
			return EvaluationResult{Status: Pass, Score: 1}
		}
		return EvaluationResult{Status: Fail, Score: 0, Detail: "exact output mismatch"}
	case "regex":
		raw, _ := spec.Params["pattern"].(string)
		r, err := regexp.Compile(raw)
		if err != nil {
			return EvaluationResult{Status: EvaluationError, Detail: err.Error()}
		}
		if r.MatchString(output) {
			return EvaluationResult{Status: Pass, Score: 1}
		}
		return EvaluationResult{Status: Fail, Score: 0, Detail: "pattern did not match"}
	case "json_valid":
		var value any
		if json.Unmarshal([]byte(output), &value) == nil {
			return EvaluationResult{Status: Pass, Score: 1}
		}
		return EvaluationResult{Status: Fail, Score: 0, Detail: "invalid JSON"}
	case "rule":
		return evaluateRule(spec.Params, output)
	case "semantic_similarity", "model_judge":
		return EvaluationResult{Status: Inconclusive, Detail: "optional evaluator is not configured"}
	default:
		return EvaluationResult{Status: EvaluationError, Detail: "unknown evaluator"}
	}
}

func evaluateRule(params map[string]any, output string) EvaluationResult {
	if value, ok := params["contains"].(string); ok && !strings.Contains(strings.ToLower(output), strings.ToLower(value)) {
		return EvaluationResult{Status: Fail, Detail: "required text missing"}
	}
	if value, ok := params["forbid"].(string); ok && strings.Contains(strings.ToLower(output), strings.ToLower(value)) {
		return EvaluationResult{Status: Fail, Detail: "forbidden text present"}
	}
	return EvaluationResult{Status: Pass, Score: 1}
}
