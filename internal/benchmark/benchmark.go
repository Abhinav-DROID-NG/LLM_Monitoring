package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

type taskFile struct {
	ID            string  `json:"id"`
	Category      string  `json:"category"`
	Prompt        string  `json:"prompt"`
	EvalType      string  `json:"eval_type"`
	Expected      any     `json:"expected"`
	Tolerance     float64 `json:"tolerance"`
	CaseSensitive bool    `json:"case_sensitive"`
	StrictRaw     bool    `json:"strict_raw"`
}

type file struct {
	BenchmarkName string     `json:"benchmark_name"`
	Version       string     `json:"version"`
	Tasks         []taskFile `json:"tasks"`
}

type Suite struct {
	Name    string
	Version string
	Library canary.Library
}

func Load(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read benchmark suite: %w", err)
	}
	var source file
	if err := json.Unmarshal(data, &source); err != nil {
		return Suite{}, fmt.Errorf("decode benchmark suite: %w", err)
	}
	if source.BenchmarkName == "" || source.Version == "" || len(source.Tasks) == 0 {
		return Suite{}, fmt.Errorf("benchmark suite %s is missing metadata or tasks", path)
	}

	tasks := make([]canary.Task, 0, len(source.Tasks))
	divisions := map[string]bool{}
	seen := map[string]bool{}
	for _, item := range source.Tasks {
		if item.ID == "" || seen[item.ID] || item.Prompt == "" {
			return Suite{}, fmt.Errorf("benchmark suite %s has an invalid or duplicate task %q", path, item.ID)
		}
		seen[item.ID] = true
		divisions[item.Category] = true
		checks, err := checksFor(item)
		if err != nil {
			return Suite{}, fmt.Errorf("benchmark task %s: %w", item.ID, err)
		}
		tasks = append(tasks, canary.Task{ID: item.ID, Division: item.Category, Tier: "general", Prompt: item.Prompt, MaxTokens: 256, Checks: checks})
	}
	divisionList := make([]string, 0, len(divisions))
	for division := range divisions {
		divisionList = append(divisionList, division)
	}
	sort.Strings(divisionList)
	divisionValues := make([]canary.Division, 0, len(divisionList))
	for _, division := range divisionList {
		divisionValues = append(divisionValues, canary.Division{ID: division})
	}
	return Suite{Name: source.BenchmarkName, Version: source.Version, Library: canary.Library{Divisions: divisionValues, Tasks: tasks}}, nil
}

func checksFor(item taskFile) ([]canary.Check, error) {
	value, err := json.Marshal(item.Expected)
	if err != nil {
		return nil, err
	}
	switch item.EvalType {
	case "numerical":
		return []canary.Check{{Type: "number_equals", Params: map[string]json.RawMessage{"value": value}}}, nil
	case "exact_match":
		expected, ok := item.Expected.(string)
		if !ok {
			return nil, fmt.Errorf("exact_match expected value must be a string")
		}
		pattern := "^" + regexp.QuoteMeta(expected) + "$"
		if !item.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		patternJSON, _ := json.Marshal(pattern)
		return []canary.Check{{Type: "regex_full", Params: map[string]json.RawMessage{"value": patternJSON}}}, nil
	case "json_schema":
		expected, ok := item.Expected.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("json_schema expected value must be an object")
		}
		checks := []canary.Check{{Type: "json_valid"}}
		if keys, ok := expected["required_keys"].([]any); ok {
			values := make([]string, 0, len(keys))
			for _, key := range keys {
				values = append(values, fmt.Sprint(key))
			}
			raw, _ := json.Marshal(values)
			checks = append(checks, canary.Check{Type: "json_fields", Params: map[string]json.RawMessage{"values": raw}})
		}
		if types, ok := expected["key_types"].(map[string]any); ok {
			values := make([]string, 0, len(types))
			for key, kind := range types {
				values = append(values, key+":"+strings.ToLower(fmt.Sprint(kind)))
			}
			sort.Strings(values)
			raw, _ := json.Marshal(values)
			checks = append(checks, canary.Check{Type: "json_types", Params: map[string]json.RawMessage{"values": raw}})
		}
		return checks, nil
	case "json_exact":
		return []canary.Check{{Type: "json_exact", Params: map[string]json.RawMessage{"value": value}}}, nil
	case "safety_refusal":
		return []canary.Check{{Type: "not_refused"}}, nil
	default:
		return nil, fmt.Errorf("unsupported eval_type %q", item.EvalType)
	}
}
