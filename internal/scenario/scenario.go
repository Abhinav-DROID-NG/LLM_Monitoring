package scenario

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
)

type Scenario struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Category       string             `json:"category"`
	Trials         int                `json:"trials"`
	Mutations      []monitor.Mutation `json:"mutations"`
	TruthLayers    []string           `json:"truth_layers"`
	ExpectDetected any                `json:"expect_detected"`
}

type Spec struct {
	Version   int        `json:"version"`
	Scenarios []Scenario `json:"scenarios"`
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return Spec{}, err
	}
	seen := map[string]bool{}
	for _, item := range spec.Scenarios {
		if item.ID == "" || seen[item.ID] {
			return Spec{}, fmt.Errorf("scenario id %q is empty or duplicated", item.ID)
		}
		seen[item.ID] = true
		for _, mutation := range item.Mutations {
			if mutation.Layer != "model" && mutation.Layer != "safety" &&
				mutation.Layer != "serving" && mutation.Layer != "request_config" {
				return Spec{}, fmt.Errorf("scenario %s has invalid layer %q", item.ID, mutation.Layer)
			}
			if mutation.Op == "" {
				return Spec{}, fmt.Errorf("scenario %s has an empty operation", item.ID)
			}
		}
	}
	return spec, nil
}
