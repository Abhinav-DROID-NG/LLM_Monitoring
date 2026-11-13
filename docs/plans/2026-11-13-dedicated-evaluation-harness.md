# Dedicated Evaluation Harness Implementation Plan

**Goal:** Add independent behavioural and performance evaluation runs, baselines, metrics, drift results, storage, APIs, and dashboard views while preserving the existing detector meta-evaluation command.

**Architecture:** Keep `cmd/model-monitor/evaluation_harness.go` as the existing injected-scenario meta-evaluation path. Add a reusable `internal/evaluation` package with versioned test definitions, bounded execution, evaluator results, raw observations, metric aggregation, baseline compatibility, and independent drift pipelines. The existing provider/router and JSON persistence utilities are reused; legacy canary/run files remain readable.

**Tech Stack:** Go standard library, existing `internal/monitor` provider/router and statistics, JSON files, React/TypeScript/Recharts.

## Audit findings

- `evaluate` currently measures detector/attribution behavior from `problem3_test_cases.json`; it is not a model-quality harness and must remain unchanged.
- `internal/canary` currently loads the 51-item precision benchmark into task/check objects; checks are mostly exact output checks.
- `internal/monitor` provides provider adapters, alias routing, concurrent canary execution, observations, JSON persistence, basic feature statistics, BH correction, and drift detection, but no TTFT/resource telemetry or independent evaluation schemas.
- Existing baseline/run files are raw observation arrays; the new schema must be additive and versioned rather than replacing them.
- The API exposes canaries, runs, one baseline, detection, and attribution only. The dashboard is built around those endpoints and has no evaluation-specific sections.
- Current verification: focused Go packages pass; full Go tests are blocked by the already-deleted `scenarios_fixed.json` and sandbox socket restrictions; frontend build is blocked because `frontend/node_modules`/`tsc` is unavailable.

## Implementation phases

1. Add `internal/evaluation` schemas and registry validation for behavioural categories/evaluators and performance scenarios, including dataset/version/config metadata, repetition policies, explicit pass/fail/inconclusive/error outcomes, and versioned JSON envelopes.
2. Add evaluator implementations and tests: exact, regex, JSON/structure, rule-based, optional semantic interface, and optional model-judge interface. Unsupported evaluators return inconclusive/error and never pass.
3. Add bounded, cancellable execution with per-trial records, timeout/retry separation, progress, deterministic run IDs, resume de-duplication, and mock-provider adapters. Preserve raw responses and errors.
4. Add separate behavioural and performance metric engines. Include category scores/coverage and consistency for behaviour; TTFT/latency percentiles, throughput, errors/timeouts, and explicitly unavailable resource telemetry for performance.
5. Add independent baseline files and compatibility checks, plus direction-aware drift statuses (`NORMAL`, `POTENTIAL_DRIFT`, `CONFIRMED_DRIFT`, `INCONCLUSIVE`, `RECOVERED`) with sample-size/effect/noise controls. Keep `monitor.Detect` and the existing meta-harness intact.
6. Add CLI commands/options for behavioural, performance, and combined evaluations, baseline creation, comparison, profile/repetition/concurrency/timeout/resume controls, and progress output. Keep existing commands and file readers working.
7. Add API routes for execution, metrics, drift, history, details, and baseline selection/creation with request validation and conflict handling.
8. Extend TypeScript API/types/store and add separate Behavioural Monitoring, Performance Monitoring, and Evaluation History dashboard sections. Mark unavailable telemetry distinctly and reuse existing cards/charts where appropriate.
9. Add unit/integration tests for validation, evaluators, repetitions, concurrency/cancellation/timeouts/retries/resume, metrics/percentiles, baseline compatibility, drift false positives, missing telemetry, API behavior, and legacy file readability. Update docs and run Go tests, frontend typecheck/build, and mock end-to-end checks.

## Main files expected to change

- Create: `internal/evaluation/*.go`, `internal/evaluation/*_test.go`, behavioural/performance dataset JSON, and evaluation-specific docs.
- Modify: `cmd/model-monitor/main.go`, `cmd/model-monitor/server.go`, `internal/monitor/monitor.go` only where shared telemetry/context hooks are required, and existing frontend API/types/store/app/pages.
- Preserve: `cmd/model-monitor/evaluation_harness.go` and its scenario semantics; existing raw run/baseline JSON readers.
