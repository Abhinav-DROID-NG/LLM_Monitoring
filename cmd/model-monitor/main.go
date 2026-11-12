package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/groundtruth"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/logging"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/mockprovider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/probe"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/report"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/router"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/scheduler"
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
		return mockServe()
	case "start":
		return startMonitor()
	case "stop":
		return errors.New("stop requires a process supervisor or configured PID file")
	case "status":
		return status()
	case "baseline":
		return runCycle(true)
	case "run":
		return runCycle(false)
	case "report":
		return showReport()
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
	schedule := make([]groundtruth.Entry, 0, len(harness.Scenarios))
	for _, scenario := range harness.Scenarios {
		schedule = append(schedule, groundtruth.Entry{
			ScenarioID: scenario.ID, TrueLayers: evaluation.TrueLayers(scenario),
		})
	}
	sealedLog := groundtruth.New(schedule)
	commitment, err := harness.Commit()
	if err != nil {
		return err
	}
	if _, err := sealedLog.Commit(); err != nil {
		return err
	}
	mock := mockprovider.New(10000)
	aliasRouter, err := router.New(router.Config{Alias: "support-bot-v1", Upstream: "synthetic-mock"}, mock.Handler(), mock)
	if err != nil {
		return err
	}
	server := httptest.NewServer(aliasRouter.Handler())
	defer server.Close()
	httpProvider, err := provider.NewHTTPClient(server.URL, &http.Client{Timeout: 30 * time.Second}, 1)
	if err != nil {
		return err
	}
	suite, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	runner, err := probe.NewRunner(probe.Config{Concurrency: 8, Retries: 1, Alias: "support-bot-v1"})
	if err != nil {
		return err
	}
	baselineObservations, err := runner.Run(context.Background(), suite, httpProvider, "evaluation-baseline")
	if err != nil {
		return err
	}
	baselineFeatures := make([]features.Features, 0, len(baselineObservations))
	for _, observation := range baselineObservations {
		baselineFeatures = append(baselineFeatures, observation.Features)
	}
	trials := make([]evaluation.Trial, 0, len(harness.Scenarios))
	for _, scenario := range harness.Scenarios {
		layers := evaluation.TrueLayers(scenario)
		aliasRouter.SetLayers(layers)
		if err := sealedLog.Append(groundtruth.Entry{
			TimestampUTC: time.Now().UTC(), ScenarioID: scenario.ID, TrueLayers: layers,
		}); err != nil {
			return err
		}
		observations, err := runner.Run(context.Background(), suite, httpProvider,
			fmt.Sprintf("evaluation-%d", scenario.ID))
		if err != nil {
			return err
		}
		currentFeatures := make([]features.Features, 0, len(observations))
		trial := evaluation.Trial{Scenario: scenario}
		for _, observation := range observations {
			currentFeatures = append(currentFeatures, observation.Features)
			trial.Requests++
			trial.PromptTokens += observation.Store.InputTokens
			trial.CompletionTokens += observation.Store.OutputTokens
			trial.LatencyMS += observation.Store.LatencyMS
		}
		changes := stats.Detect(baselineFeatures, currentFeatures, stats.DefaultConfig(), scenario.Seed)
		inference := attribution.Analyze(changes, attribution.DefaultConfig())
		trial.Predicted = inference.Verdict
		trial.Detected = false
		for _, change := range changes {
			if change.Changed {
				trial.Detected = true
				break
			}
		}
		trials = append(trials, trial)
	}
	truth, err := harness.Unseal(commitment)
	if err != nil {
		return err
	}
	if _, err := sealedLog.Unseal(sealedLog.Commitment()); err != nil {
		return err
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
	suite, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	return runCycleWithSuite(saveBaseline, suite)
}

func runCycleWithSuite(saveBaseline bool, suite canary.Suite) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	db, err := store.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	httpProvider, err := provider.NewHTTPClient(cfg.ProviderURL, &http.Client{Timeout: cfg.HTTPTimeout}, cfg.ProviderRetries)
	if err != nil {
		return err
	}
	runner, err := probe.NewRunner(probe.Config{Concurrency: 4, Retries: 1, RetryDelay: 10 * time.Millisecond})
	if err != nil {
		return err
	}
	runID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	observations, err := runner.RunAndStore(context.Background(), suite, httpProvider, db, runID)
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
	encodedBaseline, err := store.LatestBaselineWindow(context.Background(), db)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("no baseline exists; run `model-monitor baseline` first")
		}
		return err
	}
	var baselineWindow baseline.Window
	if err := json.Unmarshal([]byte(encodedBaseline), &baselineWindow); err != nil {
		return fmt.Errorf("decode saved baseline window: %w", err)
	}
	changes := stats.Detect(baselineWindow.Samples, window.Samples, stats.DefaultConfig(), 1)
	inference := attribution.Analyze(changes, attribution.DefaultConfig())
	failed, tokens := 0, 0
	for _, observation := range observations {
		if observation.Store.Error != "" {
			failed++
		}
		tokens += observation.Store.InputTokens + observation.Store.OutputTokens
	}
	value := report.New(runID, baselineWindow.ID, time.Now().Add(-time.Second), time.Now(), len(observations), failed, tokens, 0, changes, inference)
	markdown := report.Markdown(value)
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	attributionJSON, err := json.Marshal(inference)
	if err != nil {
		return err
	}
	if err := store.SaveReport(context.Background(), db, runID, markdown, string(changesJSON), string(attributionJSON)); err != nil {
		return err
	}
	fmt.Print(markdown)
	return nil
}

func showReport() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	db, err := store.Open(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	markdown, err := store.LatestReport(context.Background(), db)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("no persisted report is available; run `model-monitor run` first")
		}
		return err
	}
	fmt.Print(markdown)
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
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	httpProvider, err := provider.NewHTTPClient(cfg.ProviderURL, &http.Client{Timeout: cfg.HTTPTimeout}, cfg.ProviderRetries)
	if err != nil {
		return err
	}
	run := func(label string) ([]probe.Observation, error) {
		observations, err := runner.Run(context.Background(), suite, httpProvider, label)
		if err != nil {
			return nil, err
		}
		fmt.Printf("demo %-20s probes=%d\n", label, len(observations))
		return observations, nil
	}
	baselineObservations, err := run("demo-baseline")
	if err != nil {
		return err
	}
	baselineFeatures := make([]features.Features, 0, len(baselineObservations))
	for _, observation := range baselineObservations {
		baselineFeatures = append(baselineFeatures, observation.Features)
	}
	for _, scenario := range []struct {
		name string
	}{
		{"model-change"},
		{"safety-change"},
		{"request-change"},
		{"serving-change"},
		{"combined-change"},
	} {
		current, err := run("demo-" + scenario.name)
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

func startMonitor() error {
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
		Addr: cfg.HTTPAddr, Handler: healthHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	errs := make(chan error, 2)
	go func() {
		logger.Info("server started", slog.String("addr", cfg.HTTPAddr))
		errs <- server.ListenAndServe()
	}()
	go func() {
		errs <- scheduler.Run(ctx, scheduler.Config{
			Interval: cfg.Interval, Profile: scheduler.Profile(cfg.Profile),
		}, func(ctx context.Context, cycle int) error {
			if _, err := store.NextCycle(ctx, db); err != nil {
				return err
			}
			suite, err := canary.Load("canaries_fixed.json")
			if err != nil {
				return err
			}
			return runCycleWithSuite(false, scheduler.Expand(suite, cycle, scheduler.Profile(cfg.Profile)))
		})
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errs:
		if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("start: %w", err)
	}
}

func mockServe() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	mock := mockprovider.New(1)
	providerURL, err := url.Parse(cfg.ProviderURL)
	if err != nil || providerURL.Host == "" {
		return fmt.Errorf("invalid provider URL %q", cfg.ProviderURL)
	}
	server := &http.Server{
		Addr:              providerURL.Host,
		Handler:           mock.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("mock serve: %w", err)
	case signal := <-signals:
		_ = signal
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
