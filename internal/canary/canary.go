package canary

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

var validTiers = map[string]bool{"easy": true, "general": true, "expensive": true}

type Check struct {
	Type   string                     `json:"type"`
	Params map[string]json.RawMessage `json:"-"`
	Value  string                     `json:"value,omitempty"`
	Values []string                   `json:"values,omitempty"`
}

func (c *Check) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	typeField, ok := fields["type"]
	if !ok {
		return errors.New("check type is missing")
	}
	if err := json.Unmarshal(typeField, &c.Type); err != nil {
		return err
	}
	c.Params = make(map[string]json.RawMessage, len(fields)-1)
	for key, value := range fields {
		if key != "type" {
			c.Params[key] = append(json.RawMessage(nil), value...)
		}
	}
	c.Value, _ = c.String("value")
	c.Values = c.Strings("values")
	return nil
}

func (c Check) Raw(key string) json.RawMessage {
	parts := strings.Split(key, ".")
	raw, ok := c.Params[parts[0]]
	if !ok {
		return nil
	}
	for _, part := range parts[1:] {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return nil
		}
		raw, ok = object[part]
		if !ok {
			return nil
		}
	}
	return append(json.RawMessage(nil), raw...)
}

func (c Check) String(key string) (string, bool) {
	raw, ok := c.Params[key]
	if !ok || string(raw) == "null" {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	var valueAny any
	if json.Unmarshal(raw, &valueAny) != nil {
		return "", false
	}
	return fmt.Sprint(valueAny), true
}

func (c Check) Int(key string) (int, bool) {
	raw, ok := c.Params[key]
	if !ok {
		return 0, false
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}

func (c Check) Float(key string) (float64, bool) {
	raw, ok := c.Params[key]
	if !ok {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}

func (c Check) Bool(key string) bool {
	raw, ok := c.Params[key]
	if !ok {
		return false
	}
	var value bool
	return json.Unmarshal(raw, &value) == nil && value
}

func (c Check) Strings(key string) []string {
	raw, ok := c.Params[key]
	if !ok {
		return nil
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		if value, ok := c.String(key); ok {
			return []string{value}
		}
		return nil
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		var value string
		if json.Unmarshal(item, &value) == nil {
			result = append(result, value)
			continue
		}
		var valueAny any
		if json.Unmarshal(item, &valueAny) == nil {
			result = append(result, fmt.Sprint(valueAny))
		}
	}
	return result
}

func (c Check) IntPair(key string) ([2]int, bool) {
	var result [2]int
	raw := c.Raw(key)
	if len(raw) == 0 || json.Unmarshal(raw, &result) != nil {
		return result, false
	}
	return result, true
}

func (c Check) ValueString() string {
	if value, ok := c.String("value"); ok {
		return value
	}
	if value, ok := c.String("equals"); ok {
		return value
	}
	return c.Value
}
func (c Check) ValuesList() []string {
	values := c.Strings("values")
	if len(values) > 0 {
		return values
	}
	return c.Values
}

type Task struct {
	ID             string  `json:"id"`
	Division       string  `json:"division"`
	Tier           string  `json:"tier"`
	Prompt         string  `json:"prompt"`
	System         string  `json:"system,omitempty"`
	Filler         any     `json:"filler,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	MaxTokens      int     `json:"max_tokens"`
	Checks         []Check `json:"checks"`
	EstIn          int     `json:"est_in_tokens"`
	EstOut         int     `json:"est_out_tokens"`
	EstTotal       int     `json:"est_total_tokens"`
}

type Division struct {
	ID             string   `json:"id"`
	Temperature    float64  `json:"temperature,omitempty"`
	TargetsLayer   string   `json:"targets_layer,omitempty"`
	PrimaryMetrics []string `json:"primary_metrics,omitempty"`
}

type Library struct {
	Meta       map[string]any    `json:"meta"`
	Divisions  []Division        `json:"divisions"`
	Tasks      []Task            `json:"tasks"`
	Tiers      map[string]any    `json:"tiers"`
	Profiles   map[string]any    `json:"execution_profiles"`
	CheckTypes map[string]string `json:"check_types"`
}

type performanceBenchmark struct {
	Dataset struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	} `json:"dataset"`
	Items []performanceItem `json:"items"`
}

type performanceItem struct {
	ID              string `json:"id"`
	Category        string `json:"category"`
	Difficulty      string `json:"difficulty"`
	Prompt          string `json:"prompt"`
	Answer          string `json:"answer"`
	EstimatedTokens int    `json:"estimated_prompt_tokens"`
}

func Load(path string) (Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Library{}, fmt.Errorf("read canary library: %w", err)
	}
	var shape struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &shape); err != nil {
		return Library{}, fmt.Errorf("decode canary library: %w", err)
	}
	var library Library
	if len(shape.Items) > 0 {
		var benchmark performanceBenchmark
		if err := json.Unmarshal(data, &benchmark); err != nil {
			return Library{}, fmt.Errorf("decode performance benchmark: %w", err)
		}
		library = benchmarkLibrary(benchmark)
	} else if err := json.Unmarshal(data, &library); err != nil {
		return Library{}, fmt.Errorf("decode canary library: %w", err)
	}
	if err := Validate(library); err != nil {
		return Library{}, err
	}
	return library, nil
}

func Validate(library Library) error {
	if len(library.Tasks) == 0 {
		return errors.New("canary library contains no tasks")
	}
	if len(library.Divisions) == 0 {
		return errors.New("canary library contains no divisions")
	}
	seen := make(map[string]bool, len(library.Tasks))
	divisions := make(map[string]bool, len(library.Divisions))
	for _, division := range library.Divisions {
		if division.ID == "" || divisions[division.ID] {
			return errors.New("canary divisions must have unique non-empty IDs")
		}
		divisions[division.ID] = true
	}
	for _, task := range library.Tasks {
		if task.ID == "" || seen[task.ID] {
			return fmt.Errorf("canary task IDs must be unique and non-empty: %q", task.ID)
		}
		if !divisions[task.Division] {
			return fmt.Errorf("canary %s references unknown division %q", task.ID, task.Division)
		}
		if !validTiers[task.Tier] {
			return fmt.Errorf("canary %s has invalid tier %q", task.ID, task.Tier)
		}
		if task.Prompt == "" || task.MaxTokens <= 0 {
			return fmt.Errorf("canary %s has invalid prompt or max_tokens", task.ID)
		}
		if err := ValidateChecks(task.Checks); err != nil {
			return fmt.Errorf("canary %s: %w", task.ID, err)
		}
		for _, check := range task.Checks {
			if len(library.CheckTypes) > 0 {
				if _, ok := library.CheckTypes[check.Type]; !ok {
					return fmt.Errorf("canary %s uses check type %q missing from check_types", task.ID, check.Type)
				}
			}
		}
		seen[task.ID] = true
	}
	return nil
}

func benchmarkLibrary(benchmark performanceBenchmark) Library {
	divisions := make(map[string]Division)
	tasks := make([]Task, 0, len(benchmark.Items))
	for _, item := range benchmark.Items {
		if _, ok := divisions[item.Category]; !ok {
			divisions[item.Category] = Division{ID: item.Category, TargetsLayer: "performance", PrimaryMetrics: []string{"accuracy"}}
		}
		tier := "general"
		if item.Difficulty == "easy" {
			tier = "easy"
		} else if item.Difficulty == "hard" || item.Difficulty == "expert" {
			tier = "expensive"
		}
		maxTokens := item.EstimatedTokens + 256
		if maxTokens < 64 {
			maxTokens = 64
		}
		tasks = append(tasks, Task{
			ID: item.ID, Division: item.Category, Tier: tier, Prompt: item.Prompt,
			MaxTokens: maxTokens, Checks: []Check{{Type: "output_equals", Value: item.Answer}},
		})
	}
	divisionList := make([]Division, 0, len(divisions))
	for _, division := range divisions {
		divisionList = append(divisionList, division)
	}
	sort.Slice(divisionList, func(i, j int) bool { return divisionList[i].ID < divisionList[j].ID })
	return Library{
		Meta:      map[string]any{"name": benchmark.Dataset.Name, "version": benchmark.Dataset.Version, "description": benchmark.Dataset.Description, "mode": "performance"},
		Divisions: divisionList, Tasks: tasks,
		CheckTypes: map[string]string{"output_equals": "Exact trimmed output match against the benchmark answer."},
	}
}

func (l Library) FilterTier(tiers ...string) []Task {
	allowed := make(map[string]bool, len(tiers))
	for _, tier := range tiers {
		allowed[tier] = true
	}
	tasks := make([]Task, 0)
	for _, task := range l.Tasks {
		if allowed[task.Tier] {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

func (l Library) ByDivision(id string) []Task {
	tasks := make([]Task, 0)
	for _, task := range l.Tasks {
		if task.Division == id {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

type PlannedProbe struct {
	Task        Task `json:"task"`
	RepeatIndex int  `json:"repeat_index"`
}

func (l Library) Plan(profile string, cycleIndex int) []PlannedProbe {
	repeats := map[string]int{"easy": 10, "general": 6, "expensive": 3}
	expensiveEvery := 4
	if profile == "development" {
		repeats["easy"], repeats["general"], repeats["expensive"] = 1, 1, 1
		expensiveEvery = 1
	} else if profile == "deep_analysis" {
		expensiveEvery = 1
	}
	if configured, ok := l.Profiles[profile].(map[string]any); ok {
		if overrides, ok := configured["repeat_overrides"].(map[string]any); ok {
			for tier, value := range overrides {
				if number, ok := value.(float64); ok {
					repeats[tier] = int(number)
				}
			}
		}
		if value, ok := configured["expensive_every_n_cycles"].(float64); ok && value > 0 {
			expensiveEvery = int(value)
		}
	}
	planned := make([]PlannedProbe, 0)
	for _, task := range l.Pinned() {
		if task.Tier == "expensive" && (cycleIndex <= 0 || cycleIndex%expensiveEvery != 0) {
			continue
		}
		for repeat := 0; repeat < repeats[task.Tier]; repeat++ {
			planned = append(planned, PlannedProbe{Task: task, RepeatIndex: repeat + 1})
		}
	}
	return planned
}

func RequestIdentity(task Task, model string, temperature float64, maxTokens int, system string, responseFormat string) string {
	canonical := struct {
		MaxTokens      int     `json:"max_tokens"`
		Model          string  `json:"model"`
		ResponseFormat string  `json:"response_format"`
		System         string  `json:"system"`
		TaskID         string  `json:"task_id"`
		Temperature    float64 `json:"temperature"`
	}{
		MaxTokens: maxTokens, Model: model, ResponseFormat: responseFormat,
		System: system, TaskID: task.ID, Temperature: temperature,
	}
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func (l Library) Pinned() []Task {
	tasks := append([]Task(nil), l.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}
