package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
)

func LoadDataset(path string) (Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, fmt.Errorf("read evaluation dataset: %w", err)
	}
	var dataset Dataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return Dataset{}, fmt.Errorf("decode evaluation dataset: %w", err)
	}
	if err := dataset.Validate(); err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}
