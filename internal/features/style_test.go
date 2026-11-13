package features

import (
	"math"
	"testing"
)

func TestStyleVectorAndDistance(t *testing.T) {
	vector := StyleVectorV1("One short sentence. Two words.")
	if vector[0] <= 0 || vector[1] != 2.5 || vector[6] != 1 {
		t.Fatalf("unexpected style vector: %#v", vector)
	}
	if Distance(vector, vector) != 0 {
		t.Fatal("identical vectors must have zero distance")
	}
	if math.IsNaN(vector[4]) || math.IsInf(vector[4], 0) {
		t.Fatal("punctuation feature is not finite")
	}
}
