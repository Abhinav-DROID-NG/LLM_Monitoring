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

type Task struct {
	ID        string  `json:"id"`
	Division  string  `json:"division"`
	Tier      string  `json:"tier"`
	Prompt    string  `json:"prompt"`
	MaxTokens int     `json:"max_tokens"`
	Checks    []Check `json:"checks"`
	EstIn     int     `json:"est_in_tokens"`
	EstOut    int     `json:"est_out_tokens"`
	EstTotal  int     `json:"est_total_tokens"`
}

type Division struct {
	ID string `json:"id"`
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
		seen[task.ID] = true
	}
	return nil
}

func (l Library) Pinned() []Task {
	tasks := append([]Task(nil), l.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}
