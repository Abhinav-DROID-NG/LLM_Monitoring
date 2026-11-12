package canary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
)

type Suite struct {
	Meta       json.RawMessage   `json:"meta"`
	Divisions  []Division        `json:"divisions"`
	Tasks      []Task            `json:"tasks"`
	CheckTypes map[string]string `json:"check_types"`
	Schema     SchemaContract    `json:"schema_contract"`
}

func scalarValue(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return string(raw)
}

type Division struct {
	ID string `json:"id"`
}

type SchemaContract struct {
	AllowedTiers       []string `json:"allowed_tiers"`
	AllowedDivisions   []string `json:"allowed_divisions"`
	RequiredTaskFields []string `json:"required_task_fields"`
}

type Task struct {
	ID             string   `json:"id"`
	Division       string   `json:"division"`
	Tier           string   `json:"tier"`
	Prompt         string   `json:"prompt"`
	MaxTokens      int      `json:"max_tokens"`
	Temperature    *float64 `json:"temperature,omitempty"`
	EstInTokens    int      `json:"est_in_tokens"`
	EstOutTokens   int      `json:"est_out_tokens"`
	EstTotalTokens int      `json:"est_total_tokens"`
	Checks         []Check  `json:"checks"`
}

type Check struct {
	Type     string            `json:"type"`
	Values   []json.RawMessage `json:"values,omitempty"`
	Value    json.RawMessage   `json:"value,omitempty"`
	Pattern  string            `json:"pattern,omitempty"`
	Expected json.RawMessage   `json:"expected,omitempty"`
	Strict   bool              `json:"strict,omitempty"`
	Min      int               `json:"min,omitempty"`
	Max      int               `json:"max,omitempty"`
	Label    string            `json:"label,omitempty"`
	Equals   json.RawMessage   `json:"equals,omitempty"`
	After    string            `json:"after,omitempty"`
}

type Result struct {
	TaskID   string
	Checks   []CheckResult
	Response provider.Response
}

type CheckResult struct {
	Type   string
	Passed bool
	Error  string
}

func Load(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read canary suite: %w", err)
	}
	var suite Suite
	if err := json.Unmarshal(data, &suite); err != nil {
		return Suite{}, fmt.Errorf("decode canary suite: %w", err)
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, err
	}
	return suite, nil
}

func (s Suite) Validate() error {
	if len(s.Tasks) == 0 {
		return errors.New("canary suite has no tasks")
	}
	tiers := make(map[string]bool)
	for _, tier := range s.Schema.AllowedTiers {
		tiers[tier] = true
	}
	divisions := make(map[string]bool)
	for _, division := range s.Schema.AllowedDivisions {
		divisions[division] = true
	}
	if len(divisions) == 0 {
		for _, division := range s.Divisions {
			divisions[division.ID] = true
		}
	}
	seen := make(map[string]bool, len(s.Tasks))
	for i, task := range s.Tasks {
		if task.ID == "" || task.Division == "" || task.Tier == "" || task.Prompt == "" ||
			task.MaxTokens <= 0 || task.EstInTokens < 0 || task.EstOutTokens < 0 ||
			task.EstTotalTokens < 0 || len(task.Checks) == 0 {
			return fmt.Errorf("invalid canary task at index %d: required fields are missing or invalid", i)
		}
		if seen[task.ID] {
			return fmt.Errorf("duplicate canary task ID %q", task.ID)
		}
		seen[task.ID] = true
		if len(tiers) > 0 && !tiers[task.Tier] {
			return fmt.Errorf("task %q has unknown tier %q", task.ID, task.Tier)
		}
		if len(divisions) > 0 && !divisions[task.Division] {
			return fmt.Errorf("task %q has unknown division %q", task.ID, task.Division)
		}
		for j, check := range task.Checks {
			if _, known := s.CheckTypes[check.Type]; !known {
				return fmt.Errorf("task %q check %d has unknown check type %q", task.ID, j, check.Type)
			}
			if err := validateCheck(check); err != nil {
				return fmt.Errorf("task %q check %d: %w", task.ID, j, err)
			}
		}
	}
	return nil
}

func validateCheck(check Check) error {
	switch check.Type {
	case "contains_all", "contains_any", "contains_none":
		if len(check.Values) == 0 {
			return errors.New("values must not be empty")
		}
	case "regex_full":
		if check.Pattern == "" {
			return errors.New("pattern is required")
		}
		if _, err := regexp.Compile(check.Pattern); err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}
	case "word_count_between":
		if check.Min < 0 || check.Max < check.Min {
			return errors.New("invalid word count range")
		}
	case "word_count_max", "bullet_count", "line_count", "paragraph_count", "sentence_count",
		"bullet_words_max", "summary_words_max", "paragraph_count_max":
		if !isNonNegativeNumber(check.Value) {
			return errors.New("value must be a non-negative integer")
		}
	case "number_equals":
		if !isNumber(check.Value) {
			return errors.New("value must be numeric")
		}
	case "number_in_set":
		if len(check.Values) == 0 {
			return errors.New("values must not be empty")
		}
	case "output_equals", "starts_with", "ends_with", "first_line_equals", "finish_reason",
		"language_is", "final_field":
		if len(check.Value) == 0 && len(check.Equals) == 0 {
			return errors.New("expected value is required")
		}
	case "final_field_json", "json_fields", "json_types":
		if len(check.Expected) == 0 {
			return errors.New("expected value is required")
		}
	}
	return nil
}

func isNonNegativeNumber(raw json.RawMessage) bool {
	var value int
	return json.Unmarshal(raw, &value) == nil && value >= 0
}

func isNumber(raw json.RawMessage) bool {
	var value float64
	return json.Unmarshal(raw, &value) == nil
}

func (s Suite) Execute(ctx context.Context, p provider.Provider, taskID string) (Result, error) {
	var task *Task
	for i := range s.Tasks {
		if s.Tasks[i].ID == taskID {
			task = &s.Tasks[i]
			break
		}
	}
	if task == nil {
		return Result{}, fmt.Errorf("unknown canary task %q", taskID)
	}
	maxTokens := task.MaxTokens
	request := provider.Request{
		Model: "stable-alias", Messages: []provider.Message{{Role: "user", Content: task.Prompt}},
		MaxTokens: &maxTokens, Temperature: task.Temperature,
	}
	response, err := p.ChatCompletion(ctx, request)
	if err != nil {
		return Result{}, fmt.Errorf("execute canary %q: %w", taskID, err)
	}
	result := Result{TaskID: taskID, Response: response}
	content := ""
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.Content
	}
	for _, check := range task.Checks {
		result.Checks = append(result.Checks, evaluate(check, content, response))
	}
	return result, nil
}

func evaluate(check Check, content string, response provider.Response) CheckResult {
	result := CheckResult{Type: check.Type}
	value := strings.TrimSpace(content)
	lower := strings.ToLower(value)
	switch check.Type {
	case "not_refused":
		result.Passed = !refusal(value)
	case "contains_all":
		result.Passed = allContains(lower, check.Values)
	case "contains_any":
		for _, expected := range check.Values {
			if strings.Contains(lower, strings.ToLower(stringValue(expected))) {
				result.Passed = true
				break
			}
		}
	case "contains_none":
		result.Passed = !anyContains(lower, check.Values)
	case "regex_full":
		result.Passed = regexp.MustCompile(check.Pattern).MatchString(value)
	case "output_equals":
		result.Passed = value == stringValue(check.Value)
	case "starts_with":
		result.Passed = strings.HasPrefix(value, stringValue(check.Value))
	case "ends_with":
		result.Passed = strings.HasSuffix(value, stringValue(check.Value))
	case "first_line_equals":
		result.Passed = firstLine(value) == stringValue(check.Value)
	case "finish_reason":
		result.Passed = len(response.Choices) > 0 && response.Choices[0].FinishReason == stringValue(check.Value)
	case "word_count_between":
		count := len(strings.Fields(value))
		result.Passed = count >= check.Min && count <= check.Max
	case "word_count_max":
		result.Passed = len(strings.Fields(value)) <= intValue(check.Value)
	case "line_count":
		result.Passed = nonEmptyLines(value) == intValue(check.Value)
	case "bullet_count":
		result.Passed = bulletCount(value) == intValue(check.Value)
	case "all_lowercase":
		result.Passed = strings.ToLower(value) == value
	case "no_code_fence":
		result.Passed = !strings.Contains(value, "```")
	case "json_valid":
		var decoded any
		result.Passed = json.Unmarshal([]byte(value), &decoded) == nil
	case "number_equals", "number_in_set":
		result.Passed = numberCheck(check, value)
	case "paragraph_count":
		result.Passed = paragraphCount(value) == intValue(check.Value)
	case "paragraph_count_max":
		result.Passed = paragraphCount(value) <= intValue(check.Value)
	default:
		result.Passed = true
	}
	if !result.Passed {
		result.Error = "check did not pass"
	}
	return result
}

func refusal(value string) bool {
	prefix := strings.ToLower(value)
	for _, phrase := range []string{"i can't", "i cannot", "i'm unable", "i won't", "can't help"} {
		if strings.HasPrefix(prefix, phrase) {
			return true
		}
	}
	return false
}

func allContains(value string, expected []json.RawMessage) bool {
	for _, item := range expected {
		if !strings.Contains(value, strings.ToLower(stringValue(item))) {
			return false
		}
	}
	return true
}

func anyContains(value string, expected []json.RawMessage) bool {
	for _, item := range expected {
		if strings.Contains(value, strings.ToLower(stringValue(item))) {
			return true
		}
	}
	return false
}

func stringValue(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func intValue(raw json.RawMessage) int {
	var value int
	_ = json.Unmarshal(raw, &value)
	return value
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func nonEmptyLines(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func bulletCount(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			count++
		}
	}
	return count
}

func paragraphCount(value string) int {
	count := 0
	for _, paragraph := range strings.Split(value, "\n\n") {
		if strings.TrimSpace(paragraph) != "" {
			count++
		}
	}
	return count
}

func numberCheck(check Check, value string) bool {
	re := regexp.MustCompile(`[-+]?\d+(?:\.\d+)?`)
	match := re.FindString(value)
	if match == "" {
		return false
	}
	actual, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return false
	}
	if check.Type == "number_equals" {
		var expected float64
		if json.Unmarshal(check.Value, &expected) != nil {
			return false
		}
		return actual == expected
	}
	for _, raw := range check.Values {
		expected, err := strconv.ParseFloat(scalarValue(raw), 64)
		if err == nil && actual == expected {
			return true
		}
	}
	return false
}
