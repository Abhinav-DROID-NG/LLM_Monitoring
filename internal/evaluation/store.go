package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func Save(root string, evaluation Evaluation) (string, error) {
	if evaluation.SchemaVersion == "" {
		evaluation.SchemaVersion = SchemaVersion
	}
	if evaluation.Metadata.RunID == "" {
		return "", fmt.Errorf("evaluation run_id is required")
	}
	dir := filepath.Join(root, "evaluations")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, evaluation.Metadata.RunID+".json")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("evaluation already exists: %s", evaluation.Metadata.RunID)
	}
	data, err := json.MarshalIndent(evaluation, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func Load(path string) (Evaluation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Evaluation{}, err
	}
	var evaluation Evaluation
	if err := json.Unmarshal(data, &evaluation); err != nil {
		return Evaluation{}, err
	}
	if evaluation.SchemaVersion == "" {
		return Evaluation{}, fmt.Errorf("evaluation schema version is missing")
	}
	if evaluation.Metadata.RunID == "" {
		return Evaluation{}, fmt.Errorf("evaluation run_id is missing")
	}
	return evaluation, nil
}
