package canary

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/mockprovider"
)

func TestLoadExistingCanarySuite(t *testing.T) {
	suite, err := Load(filepath.Join("..", "..", "canaries_fixed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Tasks) != 81 {
		t.Fatalf("loaded %d tasks, want 81", len(suite.Tasks))
	}
	provider := mockprovider.New(3).Provider()
	result, err := suite.Execute(context.Background(), provider, "REF-E1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checks) != 2 || !result.Checks[0].Passed || !result.Checks[1].Passed {
		t.Fatalf("unexpected canary result: %+v", result.Checks)
	}
}

func TestLoadRejectsMalformedDefinitions(t *testing.T) {
	base := `{
		"check_types":{"output_equals":"check"},
		"schema_contract":{"allowed_tiers":["easy"],"allowed_divisions":["refusal"]},
		"tasks":[{"id":"%s","division":"refusal","tier":"easy","prompt":"synthetic","max_tokens":20,"checks":[{"type":"output_equals","value":"ok"}]}]
	}`
	tests := []struct {
		name string
		data string
		want string
	}{
		{"duplicate ID", `{"check_types":{"output_equals":"check"},"schema_contract":{"allowed_tiers":["easy"],"allowed_divisions":["refusal"]},"tasks":[{"id":"same","division":"refusal","tier":"easy","prompt":"a","max_tokens":1,"checks":[{"type":"output_equals","value":"ok"}]},{"id":"same","division":"refusal","tier":"easy","prompt":"b","max_tokens":1,"checks":[{"type":"output_equals","value":"ok"}]}]}`, "duplicate"},
		{"unknown check", `{"check_types":{"known":"check"},"schema_contract":{"allowed_tiers":["easy"],"allowed_divisions":["refusal"]},"tasks":[{"id":"one","division":"refusal","tier":"easy","prompt":"a","max_tokens":1,"checks":[{"type":"unknown"}]}]}`, "unknown check"},
		{"missing prompt", strings.Replace(fmt.Sprintf(base, "one"), `"prompt":"synthetic",`, "", 1), "required"},
		{"invalid regex", `{"check_types":{"regex_full":"check"},"schema_contract":{"allowed_tiers":["easy"],"allowed_divisions":["refusal"]},"tasks":[{"id":"one","division":"refusal","tier":"easy","prompt":"a","max_tokens":1,"checks":[{"type":"regex_full","pattern":"(?P"}]}]}`, "invalid pattern"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "canaries.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsInvalidNumericExpectedValue(t *testing.T) {
	var suite Suite
	if err := json.Unmarshal([]byte(`{"check_types":{"number_equals":"check"},"schema_contract":{"allowed_tiers":["easy"],"allowed_divisions":["refusal"]},"tasks":[{"id":"one","division":"refusal","tier":"easy","prompt":"a","max_tokens":1,"checks":[{"type":"number_equals","value":"not a number"}]}]}`), &suite); err != nil {
		t.Fatal(err)
	}
	if err := suite.Validate(); err == nil {
		t.Fatal("expected invalid numeric value to fail")
	}
}
