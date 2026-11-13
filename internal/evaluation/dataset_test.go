package evaluation

import (
	"path/filepath"
	"testing"
)

func TestLoadDataset(t *testing.T) {
	dataset, err := LoadDataset(filepath.Join("..", "..", "behavioral_evaluation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Behavioural) < 7 {
		t.Fatalf("behavioural tests=%d", len(dataset.Behavioural))
	}
}
