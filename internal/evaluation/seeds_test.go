package evaluation

import "testing"

func TestBaselineAndEvaluationSeedsAreDisjoint(t *testing.T) {
	if BaselineSeed == EvaluationSeed {
		t.Fatalf("baseline and evaluation seeds must be disjoint: %d", BaselineSeed)
	}
}
