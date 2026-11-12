# Phase 0 Repository Assessment

## Current state

The repository is a Go module using SQLite through `modernc.org/sqlite`. It
already contains a working synthetic monitoring path:

- `internal/canary` loads and validates the checked-in 81-task suite.
- `internal/probe` executes canaries with bounded concurrency, cancellation,
  retries, and optional SQLite persistence.
- `internal/provider` defines the provider boundary and an
  OpenAI-compatible HTTP implementation.
- `internal/router`, `internal/mockprovider`, and `internal/groundtruth`
  provide a stable alias, hidden synthetic layers, and sealed commitments.
- `internal/features`, `internal/baseline`, `internal/stats`, and
  `internal/attribution` implement observation analysis.
- `internal/store` owns versioned SQLite migrations.
- `cmd/model-monitor` exposes monitoring, demonstration, and evaluation
  commands.

The canary file contains 81 tasks across 9 divisions and the easy, general,
and expensive tiers. The suite is synthetic-only; production prompts are not
accepted by the monitoring path.

## Verified prerequisites

| Requirement | Result |
|---|---|
| Go | `go1.26.8 linux/amd64` |
| SQLite CLI | `3.46.1` |
| Ollama CLI | `0.40.11` |
| Ollama API | Reachable at `127.0.0.1:11434` |
| Go tests | `go test ./...` passes |
| Static checks | `go vet ./...` passes |
| Git remote | `origin` is configured |

## Gaps for the requested rebuild

1. The real provider boundary currently speaks an OpenAI-compatible API;
   Ollama-native request/response behavior and health checks must be added
   without exposing hidden model configuration to monitoring code.
2. The existing hidden router and mock provider are process-local synthetic
   components. Their separation and commitment guarantees need to be retained
   as later phases add persistent experiment metadata.
3. The current CLI has the core workflow but not every requested command
   spelling or machine-readable report mode.
4. The evaluation harness is reproducible and blind, but its controlled
   scenarios need to be extended and validated against the requested stress
   cases.

These gaps are implementation targets for later phases, not reasons to
replace the working canary, analysis, or persistence packages.
