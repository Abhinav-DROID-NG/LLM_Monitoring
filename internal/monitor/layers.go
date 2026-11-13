package monitor

import (
	"fmt"
	"strconv"
	"time"
)

type Mutation struct {
	Layer  string         `json:"layer"`
	Op     string         `json:"op"`
	Params map[string]any `json:"params"`
}

func (l Layers) Apply(m Mutation) (Layers, error) {
	next := l
	switch m.Layer {
	case "model":
		if m.Op != "set_alias_target" {
			return Layers{}, fmt.Errorf("unsupported model operation %q", m.Op)
		}
		next.Model = stringParam(m.Params, "model", next.Model)
	case "safety":
		if m.Op != "set_level" {
			return Layers{}, fmt.Errorf("unsupported safety operation %q", m.Op)
		}
		level := stringParam(m.Params, "level", "")
		if level != "loose" && level != "normal" && level != "strict" {
			return Layers{}, fmt.Errorf("invalid safety level %q", level)
		}
		next.Safety = level
	case "serving":
		switch m.Op {
		case "set_serving":
			next.Serving = stringParam(m.Params, "route", next.Serving)
			next.LatencyMS = intParam(m.Params, "latency_ms_add", next.LatencyMS)
			next.JitterMS = intParam(m.Params, "jitter_pct", next.JitterMS)
			next.Noise = floatParam(m.Params, "nondeterminism", next.Noise)
		case "reset":
			next.Serving, next.LatencyMS, next.JitterMS, next.Noise = l.Serving, 0, 0, 0
		default:
			return Layers{}, fmt.Errorf("unsupported serving operation %q", m.Op)
		}
	case "request_config":
		switch m.Op {
		case "set":
			if value, ok := m.Params["temperature"]; ok {
				v := floatValue(value)
				next.App.Temperature = &v
			}
			if value, ok := m.Params["max_tokens"]; ok {
				v := intValue(value)
				next.App.MaxTokens = &v
			}
			if value, ok := m.Params["system_prompt"]; ok {
				v := fmt.Sprint(value)
				next.App.SystemPrompt = &v
			}
			if value, ok := m.Params["response_format"]; ok {
				next.App.ResponseFormat = fmt.Sprint(value)
			}
		case "reset":
			next.App = RequestConfig{}
		default:
			return Layers{}, fmt.Errorf("unsupported request_config operation %q", m.Op)
		}
	default:
		return Layers{}, fmt.Errorf("unsupported mutation layer %q", m.Layer)
	}
	return next, nil
}

func (l Layers) Reset(layer string) (Layers, error) {
	return l.Apply(Mutation{Layer: layer, Op: "reset"})
}

func stringParam(params map[string]any, key, fallback string) string {
	if value, ok := params[key]; ok {
		return fmt.Sprint(value)
	}
	return fallback
}

func intParam(params map[string]any, key string, fallback int) int {
	if value, ok := params[key]; ok {
		return intValue(value)
	}
	return fallback
}

func floatParam(params map[string]any, key string, fallback float64) float64 {
	if value, ok := params[key]; ok {
		return floatValue(value)
	}
	return fallback
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case string:
		result, _ := strconv.Atoi(typed)
		return result
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case string:
		result, _ := strconv.ParseFloat(typed, 64)
		return result
	default:
		return 0
	}
}

func mutationEntry(previous, next Layers, mutation Mutation) GroundTruthEntry {
	return GroundTruthEntry{
		Timestamp:  time.Now().UTC(),
		Previous:   previous,
		New:        next,
		Layer:      mutation.Layer,
		Experiment: mutation.Op,
	}
}
