# Implementation Roadmap

The rebuild proceeds in independently testable phases. A phase is complete
only after its focused tests and the full Go test suite pass, its
documentation is updated, and its changes are committed and pushed.

| Phase | Scope | Exit evidence |
|---|---|---|
| 0 | Repository inspection, prerequisite checks, architecture assessment | Assessment, architecture, and roadmap are present; tests and vet pass |
| 1 | Configuration, logging, SQLite migrations, provider abstraction, Ollama provider, health checks | Provider and persistence tests pass; Ollama health/inference path is explicit |
| 2 | Canary loading, validation, versioning, tier scheduling, experiment execution | All 81 canaries load and execute through the scheduler |
| 3 | Stable alias, hidden routing, configuration epochs, sealed ground truth | Alias remains stable while controlled layers switch; commitments verify |
| 4 | Response persistence and feature extraction | Raw observations become structured, privacy-safe JSON features |
| 5 | Baselines, variability-aware drift detection, thresholds | Controlled changes are detected without treating every output difference as drift |
| 6 | Layer attribution and discriminating follow-up probes | Model/quantization changes produce evidence-backed probabilistic attribution |
| 7 | CLI commands and human/JSON reports | The complete workflow is operable from the terminal |
| 8 | Blind evaluation and stress testing | At least 200 reproducible trials produce detection and attribution metrics |
| 9 | Recovery, shutdown, consistency, documentation, and final demonstration | Full suite and end-to-end demonstration pass with limitations documented |

## Architectural invariants

- Monitoring code depends on `provider.Provider`, not hidden router or mock
  state.
- Ground truth is committed before detection and unsealed only for evaluation.
- Only the checked-in synthetic canaries enter the monitoring pipeline.
- SQLite is the sole persistent store and JSON is the interchange format.
- Model identity and hidden layer configuration are never supplied as
  monitoring input.
