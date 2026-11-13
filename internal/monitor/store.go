package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Cycle struct {
	RunID        string        `json:"run_id"`
	Profile      string        `json:"profile"`
	StartedAt    time.Time     `json:"started_at"`
	Config       RequestConfig `json:"config"`
	Layers       Layers        `json:"layers"`
	Observations []Observation `json:"observations"`
	Cost         Cost          `json:"cost"`
}

type Cost struct {
	Calls        int      `json:"calls"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	Seconds      float64  `json:"seconds"`
	USD          *float64 `json:"usd,omitempty"`
}

func NewCycle(profile string, config RequestConfig, layers Layers, observations []Observation) Cycle {
	var cost Cost
	for _, observation := range observations {
		cost.Calls++
		cost.InputTokens += observation.InputTokens
		cost.OutputTokens += observation.OutputTokens
		cost.Seconds += float64(observation.LatencyMS) / 1000
	}
	return Cycle{RunID: firstRunID(observations), Profile: profile, StartedAt: time.Now().UTC(),
		Config: config, Layers: layers, Observations: observations, Cost: cost}
}

func SaveCycle(root string, cycle Cycle) error {
	if strings.TrimSpace(cycle.RunID) == "" {
		return errors.New("cycle run_id is required")
	}
	dir := filepath.Join(root, "cycles")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cycle store: %w", err)
	}
	data, err := json.MarshalIndent(cycle, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cycle: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, cycle.RunID+".json"), append(data, '\n'), 0600)
}

func LoadCycle(path string) (Cycle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Cycle{}, fmt.Errorf("read cycle: %w", err)
	}
	var cycle Cycle
	if err := json.Unmarshal(data, &cycle); err != nil {
		return Cycle{}, fmt.Errorf("decode cycle: %w", err)
	}
	if cycle.RunID == "" {
		return Cycle{}, errors.New("cycle run_id is missing")
	}
	return cycle, nil
}

func firstRunID(observations []Observation) string {
	if len(observations) > 0 && observations[0].RunID != "" {
		return observations[0].RunID
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
