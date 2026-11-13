package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/report"
)

type apiServer struct {
	mu         sync.Mutex
	running    bool
	cfg        monitor.Config
	library    canary.Library
	thresholds monitor.Thresholds
}

type runRequestPayload struct {
	Profile     string   `json:"profile,omitempty"`
	Tiers       []string `json:"tiers,omitempty"`
	Mode        string   `json:"mode,omitempty"` // "run" | "baseline"
	Temperature float64  `json:"temperature"`
	Seed        int64    `json:"seed"`
	MaxTokens   int      `json:"max_tokens"`
}

func serve(cfg monitor.Config) error {
	library, err := canary.Load("precision_reasoning_benchmark.json")
	if err != nil {
		return err
	}
	thresholds, err := monitor.LoadThresholds("thresholds.json")
	if err != nil {
		thresholds = monitor.Thresholds{
			FDRQ: 0.05,
			MinEffect: map[string]float64{
				"proportion_abs":   0.1,
				"latency_relative": 0.2,
			},
		}
	}
	srv := &apiServer{
		cfg:        cfg,
		library:    library,
		thresholds: thresholds,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", srv.handleHealth)
	mux.HandleFunc("/api/canaries", srv.handleCanaries)
	mux.HandleFunc("/api/runs", srv.handleRuns)
	mux.HandleFunc("/api/run", srv.handleExecuteRun)
	mux.HandleFunc("/api/baseline", srv.handleBaseline)
	mux.HandleFunc("/api/detection", srv.handleDetection)
	mux.HandleFunc("/api/attribution", srv.handleAttribution)

	addr := ":8080"
	fmt.Printf("ModelWatch API server listening on http://localhost%s (alias=%s, model=%s, mock=%v)\n",
		addr, monitor.Alias, cfg.ModelA, cfg.Mock)
	return http.ListenAndServe(addr, withCORS(mux))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *apiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	baselineObs, baseFile, baseErr := loadLatestBaseline(s.cfg.DataDir)
	latestObs, runFile, runErr := loadLatestRun(s.cfg.DataDir)

	var lastExec *time.Time
	if runErr == nil && len(latestObs) > 0 {
		t := latestObs[len(latestObs)-1].Timestamp
		lastExec = &t
	} else if baseErr == nil && len(baselineObs) > 0 {
		t := baselineObs[len(baselineObs)-1].Timestamp
		lastExec = &t
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":              "connected",
		"alias":               monitor.Alias,
		"model_identifier":    s.cfg.ModelA,
		"candidate_models":    []string{s.cfg.ModelA, s.cfg.ModelB, s.cfg.ModelC},
		"provider":            map[bool]string{true: "offline-mock", false: "ollama"}[s.cfg.Mock],
		"endpoint":            s.cfg.LocalEndpoint,
		"mock_mode":           s.cfg.Mock,
		"has_baseline":        baseErr == nil,
		"baseline_file":       filepath.Base(baseFile),
		"latest_run_file":     filepath.Base(runFile),
		"last_execution_time": lastExec,
		"experiment_config": map[string]any{
			"temperature": 0,
			"seed":        42,
			"max_tokens":  512,
		},
	})
}

func (s *apiServer) handleCanaries(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"meta":        s.library.Meta,
		"divisions":   s.library.Divisions,
		"tasks":       s.library.Pinned(),
		"check_types": s.library.CheckTypes,
		"profiles":    s.library.Profiles,
	})
}

func (s *apiServer) handleRuns(w http.ResponseWriter, r *http.Request) {
	files, _ := filepath.Glob(filepath.Join(s.cfg.DataDir, "*.json"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	type runSummary struct {
		FileName     string                `json:"file_name"`
		Kind         string                `json:"kind"`
		Timestamp    time.Time             `json:"timestamp"`
		TotalTasks   int                   `json:"total_tasks"`
		PassedTasks  int                   `json:"passed_tasks"`
		FailedTasks  int                   `json:"failed_tasks"`
		PassRate     float64               `json:"pass_rate"`
		Observations []monitor.Observation `json:"observations,omitempty"`
	}
	var history []runSummary
	for i, f := range files {
		base := filepath.Base(f)
		if !strings.HasPrefix(base, "run-") && !strings.HasPrefix(base, "baseline-") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var obs []monitor.Observation
		if json.Unmarshal(raw, &obs) != nil || len(obs) == 0 {
			continue
		}
		passed := 0
		for _, o := range obs {
			if o.Error == "" && allChecksPassed(o.Checks) {
				passed++
			}
		}
		kind := "run"
		if strings.HasPrefix(base, "baseline-") {
			kind = "baseline"
		}
		item := runSummary{
			FileName:    base,
			Kind:        kind,
			Timestamp:   obs[0].Timestamp,
			TotalTasks:  len(obs),
			PassedTasks: passed,
			FailedTasks: len(obs) - passed,
			PassRate:    float64(passed) * 100.0 / float64(len(obs)),
		}
		if i == 0 {
			item.Observations = obs
		}
		history = append(history, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": history})
}

func (s *apiServer) handleExecuteRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a canary execution is already in progress"})
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	var req runRequestPayload
	_ = json.NewDecoder(r.Body).Decode(&req)

	cfg := s.cfg
	if req.MaxTokens > 0 {
		cfg.MaxTokens = req.MaxTokens
	}
	cfg.Temperature = req.Temperature
	if req.Seed != 0 {
		cfg.Seed = req.Seed
	}

	router, err := newRouter(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	aliasServer := httptest.NewServer(router.Handler())
	defer aliasServer.Close()
	router.SetCanaryAllowlist(s.library)

	lib := s.library
	if len(req.Tiers) > 0 {
		lib.Tasks = s.library.FilterTier(req.Tiers...)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	observations := monitor.RunViaAlias(ctx, lib, aliasServer.URL+"/v1/chat/completions", nil, cfg.Concurrency)

	prefix := "run"
	if req.Mode == "baseline" {
		prefix = "baseline"
	}
	outPath := filepath.Join(cfg.DataDir, prefix+"-"+time.Now().UTC().Format("20060102T150405Z")+".json")
	if err := monitor.SaveJSON(outPath, observations); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	passed := 0
	for _, o := range observations {
		if o.Error == "" && allChecksPassed(o.Checks) {
			passed++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file_name":    filepath.Base(outPath),
		"mode":         prefix,
		"total_tasks":  len(observations),
		"passed_tasks": passed,
		"failed_tasks": len(observations) - passed,
		"pass_rate":    float64(passed) * 100.0 / float64(maxInt(1, len(observations))),
		"observations": observations,
	})
}

func (s *apiServer) handleBaseline(w http.ResponseWriter, r *http.Request) {
	runs := loadAllRunsByPrefix(s.cfg.DataDir, "baseline-")
	if len(runs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "message": "No baseline runs found. Run baseline collection first."})
		return
	}
	baseline := monitor.BuildBaseline(runs)
	latest := runs[len(runs)-1]
	features := monitor.ExtractMetrics(s.library, latest)
	writeJSON(w, http.StatusOK, map[string]any{
		"available":         true,
		"baseline_batches":  len(runs),
		"features":          baseline.Features,
		"extracted_metrics": features,
		"observations":      latest,
	})
}

func (s *apiServer) handleDetection(w http.ResponseWriter, r *http.Request) {
	baseRuns := loadAllRunsByPrefix(s.cfg.DataDir, "baseline-")
	if len(baseRuns) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "message": "No baseline available for comparison."})
		return
	}
	latestObs, runFile, err := loadLatestRun(s.cfg.DataDir)
	if err != nil {
		latestObs = baseRuns[len(baseRuns)-1]
		runFile = "baseline"
	}
	baseline := monitor.BuildBaseline(baseRuns)
	detection := monitor.DetectWithThresholds(baseline, latestObs, s.thresholds)
	behavior := s.behavioralComparison(baseRuns[len(baseRuns)-1], latestObs)
	analysis := monitor.Analysis{Detected: behavior.BehavioralDriftDetected,
		Confidence: behavior.EffectSize, PredictedLayer: behavior.AttributedLayer, Evidence: behavior.Evidence}
	currentMetrics := monitor.ExtractMetrics(s.library, latestObs)
	baselineMetrics := monitor.ExtractMetrics(s.library, baseRuns[len(baseRuns)-1])

	writeJSON(w, http.StatusOK, map[string]any{
		"available":                 true,
		"run_file":                  filepath.Base(runFile),
		"thresholds":                s.thresholds,
		"detection":                 detection,
		"behavioral":                behavior,
		"behavioral_drift_detected": behavior.BehavioralDriftDetected,
		"latency_drift_detected":    behavior.LatencyDriftDetected,
		"regression_count":          behavior.RegressionCount,
		"improvement_count":         behavior.ImprovementCount,
		"comparable_check_count":    behavior.ComparableCheckCount,
		"unavailable_check_count":   behavior.UnavailableCheckCount,
		"regression_rate":           behavior.RegressionRate,
		"effect_size":               behavior.EffectSize,
		"statistical_significance":  behavior.StatisticalSignificance,
		"attributed_layer":          behavior.AttributedLayer,
		"attribution_confidence":    behavior.AttributionConfidence,
		"evidence":                  behavior.Evidence,
		"limitations":               behavior.Limitations,
		"analysis":                  analysis,
		"baseline_stats":            baseline.Features,
		"baseline_metrics":          baselineMetrics,
		"current_metrics":           currentMetrics,
	})
}

func (s *apiServer) handleAttribution(w http.ResponseWriter, r *http.Request) {
	baseRuns := loadAllRunsByPrefix(s.cfg.DataDir, "baseline-")
	if len(baseRuns) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "message": "No baseline available for attribution."})
		return
	}
	latestObs, runFile, err := loadLatestRun(s.cfg.DataDir)
	if err != nil {
		latestObs = baseRuns[len(baseRuns)-1]
		runFile = "baseline"
	}
	baseline := monitor.BuildBaseline(baseRuns)
	detection := monitor.DetectWithThresholds(baseline, latestObs, s.thresholds)
	behavior := s.behavioralComparison(baseRuns[len(baseRuns)-1], latestObs)
	attr := monitor.Attribution{Layers: []string{behavior.AttributedLayer}, Confidence: behavior.AttributionConfidence,
		Inconclusive: behavior.AttributedLayer == "unknown", Reasons: behavior.Evidence}
	analysis := monitor.Analysis{Detected: behavior.BehavioralDriftDetected,
		Confidence: behavior.EffectSize, PredictedLayer: behavior.AttributedLayer, Evidence: behavior.Evidence}

	var inTok, outTok int
	var totalSec float64
	for _, o := range latestObs {
		inTok += o.InputTokens
		outTok += o.OutputTokens
		totalSec += float64(o.LatencyMS) / 1000.0
	}
	checkReport := report.New(detection, attr, len(latestObs), inTok, outTok, totalSec)

	writeJSON(w, http.StatusOK, map[string]any{
		"available":   true,
		"run_file":    filepath.Base(runFile),
		"detection":   detection,
		"behavioral":  behavior,
		"attribution": attr,
		"analysis":    analysis,
		"report":      checkReport,
	})
}

func (s *apiServer) behavioralComparison(reference, current []monitor.Observation) monitor.BehavioralReport {
	report := monitor.CompareBehavioral(reference, current, &monitor.BehaviorThresholds{LatencyRelative: s.thresholds.MinEffect["latency_relative"]})
	layer, confidence, evidence := monitor.AttributeBehavioral(reference, current, report)
	report.AttributedLayer, report.AttributionConfidence = layer, confidence
	report.Evidence = append(report.Evidence, evidence...)
	return report
}

func loadLatestRun(dataDir string) ([]monitor.Observation, string, error) {
	files, err := filepath.Glob(filepath.Join(dataDir, "run-*.json"))
	if err != nil || len(files) == 0 {
		return nil, "", fmt.Errorf("no run files found")
	}
	sort.Strings(files)
	latest := files[len(files)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		return nil, "", err
	}
	var obs []monitor.Observation
	return obs, latest, json.Unmarshal(data, &obs)
}

func loadAllRunsByPrefix(dataDir, prefix string) [][]monitor.Observation {
	files, _ := filepath.Glob(filepath.Join(dataDir, prefix+"*.json"))
	sort.Strings(files)
	var all [][]monitor.Observation
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var obs []monitor.Observation
		if json.Unmarshal(raw, &obs) == nil && len(obs) > 0 {
			all = append(all, obs)
		}
	}
	return all
}

func allChecksPassed(checks []monitor.CheckObservation) bool {
	for _, c := range checks {
		if !c.Unavailable && !c.Passed {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
