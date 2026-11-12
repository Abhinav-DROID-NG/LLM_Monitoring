package evaluation

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/attribution"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/groundtruth"
)

type Scenario struct {
	ID    int
	Truth string
	Seed  int64
}

type Trial struct {
	Scenario         Scenario
	Predicted        string
	Detected         bool
	Requests         int
	PromptTokens     int
	CompletionTokens int
	LatencyMS        int64
}

type Metrics struct {
	Trials                         int                       `json:"trials"`
	InconclusiveTrials             int                       `json:"inconclusive_trials"`
	DetectionRate                  float64                   `json:"detection_rate"`
	DetectionCI                    [2]float64                `json:"detection_ci"`
	MissedChangeRate               float64                   `json:"missed_change_rate"`
	FalseAlarmRate                 float64                   `json:"false_alarm_rate"`
	FalseAlarmCI                   [2]float64                `json:"false_alarm_ci"`
	SingleLayerAttributionAccuracy float64                   `json:"single_layer_attribution_accuracy"`
	CombinedAttributionRate        float64                   `json:"combined_attribution_rate"`
	RequestsPerCycle               float64                   `json:"requests_per_cycle"`
	PromptTokensPerCycle           float64                   `json:"prompt_tokens_per_cycle"`
	CompletionTokensPerCycle       float64                   `json:"completion_tokens_per_cycle"`
	EstimatedCostAvailable         bool                      `json:"estimated_cost_available"`
	EstimatedCost                  float64                   `json:"estimated_cost"`
	LatencyPerCycleMS              float64                   `json:"latency_per_cycle_ms"`
	Confusion                      map[string]map[string]int `json:"confusion_matrix"`
}

type Harness struct {
	Scenarios   []Scenario
	Commitment  string
	TruthSealed bool
}

func New(seed int64) Harness {
	counts := []struct {
		truth string
		count int
	}{
		{"NONE", 60}, {attribution.Model, 30}, {attribution.Safety, 30},
		{attribution.RequestConfig, 30}, {attribution.Serving, 30}, {"model+safety", 40},
	}

	scenarios := make([]Scenario, 0, 220)
	id := 0
	for _, group := range counts {
		for i := 0; i < group.count; i++ {
			scenarios = append(scenarios, Scenario{ID: id, Truth: group.truth, Seed: seed + int64(id)})
			id++
		}
	}
	return Harness{Scenarios: scenarios, TruthSealed: true}
}

func TrueLayers(scenario Scenario) groundtruth.Layers {
	switch scenario.Truth {
	case attribution.Model:
		return groundtruth.Layers{Model: true}
	case attribution.Safety:
		return groundtruth.Layers{Safety: true}
	case attribution.RequestConfig:
		return groundtruth.Layers{RequestConfig: true}
	case attribution.Serving:
		return groundtruth.Layers{Serving: true}
	case "model+safety":
		return groundtruth.Layers{Model: true, Safety: true}
	default:
		return groundtruth.Layers{}
	}
}

func (h *Harness) Commit() (string, error) {
	if !h.TruthSealed {
		return "", fmt.Errorf("truth has already been unsealed")
	}
	data, err := json.Marshal(h.Scenarios)
	if err != nil {
		return "", fmt.Errorf("marshal truth commitment: %w", err)
	}
	hash := sha256.Sum256(data)
	h.Commitment = hex.EncodeToString(hash[:])
	return h.Commitment, nil
}

func (h *Harness) Unseal(commitment string) ([]Scenario, error) {
	if h.Commitment == "" || commitment != h.Commitment {
		return nil, fmt.Errorf("truth commitment verification failed")
	}
	h.TruthSealed = false
	return append([]Scenario(nil), h.Scenarios...), nil
}

func (h Harness) Truth() ([]Scenario, error) {
	if h.TruthSealed {
		return nil, fmt.Errorf("truth is sealed until commitment verification")
	}
	return append([]Scenario(nil), h.Scenarios...), nil
}

func Score(trials []Trial, truth []Scenario) Metrics {
	metrics := Metrics{Trials: len(trials), Confusion: map[string]map[string]int{}}
	truthByID := make(map[int]Scenario, len(truth))
	for _, scenario := range truth {
		truthByID[scenario.ID] = scenario
	}
	var changed, detected, falseAlarms, correct, singleLayer, combined int
	for _, trial := range trials {
		scenario := truthByID[trial.Scenario.ID]
		isChanged := scenario.Truth != "NONE"
		if isChanged {
			changed++
		}
		if trial.Detected {
			detected++
			if !isChanged {
				falseAlarms++
			}
		}
		predicted := trial.Predicted
		if predicted == "" {
			predicted = attribution.Inconclusive
		}
		if predicted == attribution.Inconclusive {
			metrics.InconclusiveTrials++
		}
		if metrics.Confusion[scenario.Truth] == nil {
			metrics.Confusion[scenario.Truth] = map[string]int{}
		}
		metrics.Confusion[scenario.Truth][predicted]++
		if isChanged && scenario.Truth != "model+safety" {
			singleLayer++
			if predicted == scenario.Truth {
				correct++
			}
		}
		if scenario.Truth == "model+safety" && predicted != attribution.Inconclusive {
			combined++
		}
		metrics.RequestsPerCycle += float64(trial.Requests)
		metrics.PromptTokensPerCycle += float64(trial.PromptTokens)
		metrics.CompletionTokensPerCycle += float64(trial.CompletionTokens)
		metrics.LatencyPerCycleMS += float64(trial.LatencyMS)
	}
	metrics.DetectionRate = ratio(detected-falseAlarms, changed)
	metrics.MissedChangeRate = ratio(changed-(detected-falseAlarms), changed)
	metrics.FalseAlarmRate = ratio(falseAlarms, len(trials)-changed)
	metrics.DetectionCI = Wilson(detected-falseAlarms, changed)
	metrics.FalseAlarmCI = Wilson(falseAlarms, len(trials)-changed)
	metrics.SingleLayerAttributionAccuracy = ratio(correct, singleLayer)
	metrics.CombinedAttributionRate = ratio(combined, 40)
	if len(trials) > 0 {
		n := float64(len(trials))
		metrics.RequestsPerCycle /= n
		metrics.PromptTokensPerCycle /= n
		metrics.CompletionTokensPerCycle /= n
		metrics.LatencyPerCycleMS /= n
	}
	return metrics
}

func Wilson(success, total int) [2]float64 {
	if total == 0 {
		return [2]float64{0, 0}
	}
	z := 1.959963984540054
	p := float64(success) / float64(total)
	denominator := 1 + z*z/float64(total)
	center := (p + z*z/(2*float64(total))) / denominator
	margin := z / denominator * sqrt(p*(1-p)/float64(total)+z*z/(4*float64(total)*float64(total)))
	return [2]float64{max(0, center-margin), min(1, center+margin)}
}

func WriteResults(dir string, metrics Metrics) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	markdown := fmt.Sprintf("# Blind Evaluation Results\n\nTrials: %d\n\nDetection rate: %.4f (95%% CI %.4f-%.4f)\n\nMissed-change rate: %.4f\n\nFalse-alarm rate: %.4f (95%% CI %.4f-%.4f)\n\nINCONCLUSIVE predictions: %d\n\nSingle-layer attribution accuracy: %.4f\n\nCombined attribution rate: %.4f\n\nRequests/cycle: %.2f\n\nPrompt tokens/cycle: %.2f\n\nCompletion tokens/cycle: %.2f\n\nLatency/cycle: %.2f ms\n\n## Confusion matrix\n\n", metrics.Trials, metrics.DetectionRate, metrics.DetectionCI[0], metrics.DetectionCI[1], metrics.MissedChangeRate, metrics.FalseAlarmRate, metrics.FalseAlarmCI[0], metrics.FalseAlarmCI[1], metrics.InconclusiveTrials, metrics.SingleLayerAttributionAccuracy, metrics.CombinedAttributionRate, metrics.RequestsPerCycle, metrics.PromptTokensPerCycle, metrics.CompletionTokensPerCycle, metrics.LatencyPerCycleMS)
	keys := make([]string, 0, len(metrics.Confusion))
	for key := range metrics.Confusion {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var matrix strings.Builder
	matrix.WriteString(markdown)
	for _, truth := range keys {
		fmt.Fprintf(&matrix, "- %s: %v\n", truth, metrics.Confusion[truth])
	}
	if err := os.WriteFile(dir+"/evaluation.md", []byte(matrix.String()), 0o644); err != nil {
		return err
	}
	file, err := os.Create(dir + "/evaluation.csv")
	if err != nil {
		return err
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"metric", "value"}); err != nil {
		return err
	}
	rows := [][]string{{"trials", fmt.Sprint(metrics.Trials)}, {"detection_rate", fmt.Sprint(metrics.DetectionRate)}, {"missed_change_rate", fmt.Sprint(metrics.MissedChangeRate)}, {"false_alarm_rate", fmt.Sprint(metrics.FalseAlarmRate)}, {"inconclusive_trials", fmt.Sprint(metrics.InconclusiveTrials)}, {"single_layer_attribution_accuracy", fmt.Sprint(metrics.SingleLayerAttributionAccuracy)}, {"latency_per_cycle_ms", fmt.Sprint(metrics.LatencyPerCycleMS)}}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func ratio(value, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) / float64(total)
}
func sqrt(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Sqrt(value)
}
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
