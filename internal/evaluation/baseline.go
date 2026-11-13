package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func SaveBaseline(root string, baseline Baseline) (string, error) {
	if baseline.BaselineID == "" {
		return "", fmt.Errorf("baseline_id is required")
	}
	dir := filepath.Join(root, "evaluation-baselines")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, baseline.BaselineID+".json")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("baseline already exists: %s", baseline.BaselineID)
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func LoadBaseline(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, err
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, err
	}
	if baseline.SchemaVersion == "" || baseline.BaselineID == "" {
		return Baseline{}, fmt.Errorf("invalid evaluation baseline")
	}
	return baseline, nil
}
