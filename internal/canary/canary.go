package canary

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

var validTiers = map[string]bool{"easy": true, "general": true, "expensive": true}

type Check struct {
	Type   string   `json:"type"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}

func (c *Check) UnmarshalJSON(data []byte) error {
	type rawCheck struct {
		Type   string            `json:"type"`
		Value  json.RawMessage   `json:"value,omitempty"`
		Values []json.RawMessage `json:"values,omitempty"`
	}
	var raw rawCheck
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Type = raw.Type
	if len(raw.Value) > 0 && string(raw.Value) != "null" {
		var s string
		if err := json.Unmarshal(raw.Value, &s); err == nil {
			c.Value = s
		} else {
			var v any
			if err := json.Unmarshal(raw.Value, &v); err != nil {
				return err
			}
			c.Value = fmt.Sprintf("%v", v)
		}
	}
	if len(raw.Values) > 0 {
		c.Values = make([]string, 0, len(raw.Values))
		for _, item := range raw.Values {
			var s string
			if err := json.Unmarshal(item, &s); err == nil {
				c.Values = append(c.Values, s)
			} else {
				var v any
				if err := json.Unmarshal(item, &v); err != nil {
					return err
				}
				c.Values = append(c.Values, fmt.Sprintf("%v", v))
			}
		}
	}
	return nil
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
	ID          string  `json:"id"`
	Temperature float64 `json:"temperature,omitempty"`
}

type Library struct {
	Meta      map[string]any `json:"meta"`
	Divisions []Division     `json:"divisions"`
	Tasks     []Task         `json:"tasks"`
}

func Load(path string) (Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Library{}, fmt.Errorf("read canary library: %w", err)
	}
	var library Library
	if err := json.Unmarshal(data, &library); err != nil {
		return Library{}, fmt.Errorf("decode canary library: %w", err)
	}
	if err := Validate(library); err != nil {
		return Library{}, err
	}
	return library, nil
}

func Validate(library Library) error {
	if len(library.Tasks) != 81 {
		return fmt.Errorf("canary library contains %d tasks; want 81", len(library.Tasks))
	}
	if len(library.Divisions) != 9 {
		return fmt.Errorf("canary library contains %d divisions; want 9", len(library.Divisions))
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
		seen[task.ID] = true
	}
	return nil
}

func (l Library) Pinned() []Task {
	tasks := append([]Task(nil), l.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}
