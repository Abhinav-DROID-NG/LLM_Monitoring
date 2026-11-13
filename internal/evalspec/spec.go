package evalspec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Scenario struct {
	ID                  string   `json:"id"`
	Layer               string   `json:"layer"`
	Inject              any      `json:"inject"`
	ExpectedMove        []string `json:"expected_move"`
	ExpectedFlat        []string `json:"expected_flat"`
	ExpectedAttribution any      `json:"expected_attribution"`
	PinnedRerun         string   `json:"pinned_rerun"`
}

type Spec struct {
	Meta            map[string]any   `json:"meta"`
	Thresholds      map[string]any   `json:"thresholds"`
	SingleLayer     []Scenario       `json:"-"`
	Combined        []Scenario       `json:"-"`
	FailureCases    []Scenario       `json:"-"`
	PrivacyTests    []map[string]any `json:"privacy_tests"`
	EvaluationTests []map[string]any `json:"evaluation_tests"`
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("read evaluation spec: %w", err)
	}
	var raw struct {
		Meta       map[string]any `json:"meta"`
		Thresholds map[string]any `json:"thresholds"`
		Scenarios  struct {
			SingleLayer  []Scenario `json:"single_layer"`
			Combined     []Scenario `json:"combined"`
			FailureCases []Scenario `json:"failure_cases"`
		} `json:"scenarios"`
		PrivacyTests    []map[string]any `json:"privacy_tests"`
		EvaluationTests []map[string]any `json:"evaluation_tests"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Spec{}, fmt.Errorf("decode evaluation spec: %w", err)
	}
	spec := Spec{Meta: raw.Meta, Thresholds: raw.Thresholds, SingleLayer: raw.Scenarios.SingleLayer,
		Combined: raw.Scenarios.Combined, FailureCases: raw.Scenarios.FailureCases,
		PrivacyTests: raw.PrivacyTests, EvaluationTests: raw.EvaluationTests}
	spec.NormalizeLayers()
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func (s Spec) Validate() error {
	if len(s.SingleLayer) == 0 {
		return errors.New("evaluation spec has no single-layer scenarios")
	}
	seen := map[string]bool{}
	for _, group := range [][]Scenario{s.SingleLayer, s.Combined, s.FailureCases} {
		for i, scenario := range group {
			if scenario.ID == "" || seen[scenario.ID] {
				return fmt.Errorf("scenario %d has a missing or duplicate id", i)
			}
			seen[scenario.ID] = true
			if scenario.Layer == "config" {
				scenario.Layer = "request_config"
			}
		}
	}
	return nil
}

func (s *Spec) NormalizeLayers() {
	for i := range s.SingleLayer {
		if s.SingleLayer[i].Layer == "config" {
			s.SingleLayer[i].Layer = "request_config"
		}
	}
	for i := range s.Combined {
		if s.Combined[i].Layer == "config" {
			s.Combined[i].Layer = "request_config"
		}
	}
}
