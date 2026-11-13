package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/evaluation"
)

type evaluationRequest struct {
	Objective string `json:"objective"`
}

func (s *apiServer) handleEvaluations(w http.ResponseWriter, _ *http.Request) {
	files, _ := filepath.Glob(filepath.Join(s.cfg.DataDir, "evaluations", "*.json"))
	sort.Strings(files)
	items := make([]evaluation.Evaluation, 0, len(files))
	for _, file := range files {
		item, err := evaluation.Load(file)
		if err == nil {
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": items})
}

func (s *apiServer) handleEvaluation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request evaluationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return
	}
	var objective evaluation.Objective
	switch request.Objective {
	case string(evaluation.Behavioural):
		objective = evaluation.Behavioural
	case string(evaluation.Performance):
		objective = evaluation.Performance
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "objective must be behavioural or performance"})
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an evaluation is already in progress"})
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	result, err := runEvaluation(s.cfg, objective)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	path, err := evaluation.Save(s.cfg.DataDir, result)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file_name": filepath.Base(path), "evaluation": result})
}
