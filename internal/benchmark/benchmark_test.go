package benchmark

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

func TestLoadBenchmarkSuites(t *testing.T) {
	for _, path := range []string{"canary_suite.json", "smoke_benchmark.json"} {
		suite, err := Load(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		if suite.Name == "" || len(suite.Library.Tasks) == 0 {
			t.Fatalf("invalid suite loaded from %s", path)
		}
	}
}

func TestJSONExactCheck(t *testing.T) {
	check := canary.Check{Type: "json_exact", Params: map[string]json.RawMessage{
		"value": json.RawMessage(`{"status":"active","count":42}`),
	}}
	if !canary.EvaluateCheck(check, `{"count":42,"status":"active"}`) {
		t.Fatal("expected equivalent JSON objects to pass")
	}
}
