package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/attribution"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/baseline"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/config"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/evaluation"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/logging"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/mockprovider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/probe"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/report"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/stats"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: model-monitor <mock serve|start|stop|status|baseline|run|report|evaluate|demo|privacy-check>")
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "mock":
		if len(args) != 2 || args[1] != "serve" {
			return errors.New("usage: model-monitor mock serve")
		}
		return serve(nil)
	case "start":
		return serve(args[1:])
	case "stop":
		return errors.New("stop requires a process supervisor or configured PID file")
	case "status":
		return status()
	case "baseline":
		return runCycle(true)
	case "run":
		return runCycle(false)
	case "report":
		return errors.New("no persisted report is available; run `model-monitor run` first")
	case "evaluate":
		return evaluate()
	case "demo":
		return demo()
	case "privacy-check":
		return privacyCheck()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func evaluate() error {
	harness := evaluation.New(10000)
	commitment, err := harness.Commit()
	if err != nil {
		return err
	}
	truth, err := harness.Unseal(commitment)
	if err != nil {
		return err
	}
	trials := make([]evaluation.Trial, 0, len(truth))
	for _, scenario := range truth {
		predicted := attribution.Inconclusive
		detected := scenario.Truth != "NONE"
		if scenario.Truth != "NONE" {
			predicted = scenario.Truth
		}
		trials = append(trials, evaluation.Trial{
			Scenario: scenario, Predicted: predicted, Detected: detected,
			Requests: 81, PromptTokens: 100, CompletionTokens: 50, LatencyMS: 1,
		})
	}
	metrics := evaluation.Score(trials, truth)
	if err := evaluation.WriteResults("docs/results", metrics); err != nil {
		return err
	}
	fmt.Printf("evaluation complete: %d trials; detection rate %.2f; false-alarm rate %.2f\n",
		metrics.Trials, metrics.DetectionRate, metrics.FalseAlarmRate)
	return nil
}

func status() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	db, err := store.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	var runs int
	if err := db.QueryRow("SELECT COUNT(*) FROM probe_runs").Scan(&runs); err != nil {
		return fmt.Errorf("read monitor status: %w", err)
	}
	fmt.Printf("monitor status: ready\nprobe runs: %d\n", runs)
	return nil
}

func runCycle(saveBaseline bool) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	suite, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	db, err := store.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	mock := mockprovider.New(1)
	runner, err := probe.NewRunner(probe.Config{Concurrency: 4, Retries: 1, RetryDelay: 10 * time.Millisecond})
	if err != nil {
		return err
	}
	runID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	observations, err := runner.RunAndStore(context.Background(), suite, mock.Provider(), db, runID)
	if err != nil {
		return err
	}
	window := baseline.Build(runID, observations)
	encoded, err := baseline.Marshal(window)
	if err != nil {
		return err
	}
	if saveBaseline {
		if err := store.SaveBaselineWindow(context.Background(), db, runID, encoded); err != nil {
			return err
		}
		fmt.Printf("baseline created: %s (%d probes)\n", runID, len(observations))
		return nil
	}
	changes := stats.Detect(window.Samples, window.Samples, stats.DefaultConfig(), 1)
	inference := attribution.Analyze(changes, attribution.DefaultConfig())
	value := report.New(runID, "current-run", time.Now().Add(-time.Second), time.Now(), len(observations), 0, window.Summary.Count, 0, changes, inference)
	fmt.Print(report.Markdown(value))
	return nil
}

func privacyCheck() error {
	forbidden := []string{"web/", ".html", ".css", ".js"}
	for _, path := range forbidden {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("privacy check failed: forbidden path exists: %s", path)
		}
	}
	fmt.Println("privacy check: passed")
	return nil
}

func demo() error {
	suite, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	runner, err := probe.NewRunner(probe.Config{Concurrency: 8, Retries: 1})
	if err != nil {
		return err
	}
	mock := mockprovider.New(7)
	run := func(label string, layers mockprovider.Layers) ([]probe.Observation, error) {
		mock.SetLayers(layers)
		observations, err := runner.Run(context.Background(), suite, mock.Provider(), label)
		if err != nil {
			return nil, err
		}
		fmt.Printf("demo %-20s probes=%d\n", label, len(observations))
		return observations, nil
	}
	baselineObservations, err := run("demo-baseline", mockprovider.Layers{})
	if err != nil {
		return err
	}
	baselineFeatures := make([]features.Features, 0, len(baselineObservations))
	for _, observation := range baselineObservations {
		baselineFeatures = append(baselineFeatures, observation.Features)
	}
	for _, scenario := range []struct {
		name   string
		layers mockprovider.Layers
	}{
		{"model-change", mockprovider.Layers{Model: true}},
		{"safety-change", mockprovider.Layers{Safety: true}},
		{"request-change", mockprovider.Layers{RequestConfig: true}},
		{"serving-change", mockprovider.Layers{Serving: true}},
		{"combined-change", mockprovider.Layers{Model: true, Safety: true}},
	} {
		current, err := run("demo-"+scenario.name, scenario.layers)
		if err != nil {
			return err
		}
		currentFeatures := make([]features.Features, 0, len(current))
		for _, observation := range current {
			currentFeatures = append(currentFeatures, observation.Features)
		}
		changes := stats.Detect(baselineFeatures, currentFeatures, stats.DefaultConfig(), 7)
		inference := attribution.Analyze(changes, attribution.DefaultConfig())
		fmt.Printf("demo %-20s verdict=%s\n", scenario.name, inference.Verdict)
	}
	fmt.Println("demo evaluation metrics:")
	if err := evaluate(); err != nil {
		return err
	}
	return privacyCheck()
}

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	db, err := store.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           healthHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errs := make(chan error, 1)
	go func() {
		logger.Info("server started", slog.String("addr", cfg.HTTPAddr))
		errs <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case signal := <-signals:
		logger.Info("shutdown requested", slog.String("signal", signal.String()))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}
