package canary

import (
	"path/filepath"
	"testing"
)

func TestPlanProfiles(t *testing.T) {
	library, err := Load(filepath.Join("..", "..", "canaries_fixed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(library.Plan("monitoring", 1)); got != 27*10+27*6 {
		t.Fatalf("monitoring cycle 1 = %d, want 432", got)
	}
	if got := len(library.Plan("monitoring", 4)); got != 27*10+27*6+27*3 {
		t.Fatalf("monitoring cycle 4 = %d, want 513", got)
	}
	if got := len(library.Plan("development", 1)); got != 81 {
		t.Fatalf("development cycle = %d, want 81", got)
	}
}
