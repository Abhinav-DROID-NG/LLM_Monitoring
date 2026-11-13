# Team Backlog: Offline Development Issues

These 6 issues are designed for parallel development while Ollama models are downloading. They can be copied directly to GitHub Issues or tracked locally.

---

## Issue 1: Add Offline Mock Mode for Testing

**Labels:** `enhancement`, `priority: high`, `offline-development`, `testing`

### Description:
Currently, the monitor needs Ollama and downloaded models to run baseline and evaluation tests. This makes development slow when models are still downloading.

We need to add a mock mode that generates predefined responses without using a real AI model.

### Tasks:
- [ ] Create a `MockProvider` in `internal/monitor/mock.go`.
- [ ] Generate predictable responses for all 81 canary tests.
- [ ] Add a `--mock` flag to the CLI (`cmd/model-monitor/main.go`).
- [ ] Allow developers to run baseline and evaluation tests without Ollama.

### Expected Result:
Developers should be able to run:
```bash
go run ./cmd/model-monitor baseline --mock
```
The tests should complete in under 2 seconds without requiring Ollama.

---

## Issue 2: Add Missing Canary Check Evaluators

**Labels:** `bug`, `priority: high`, `offline-development`, `testing`

### Description:
Currently, the monitor supports only a few types of canary checks (`not_refused`, `contains`, `contains_all`, `json_valid`). Many other checks defined in `canaries_fixed.json` (39 other check types) are not properly evaluated and silently pass.

This means some tests may incorrectly pass even when the AI response does not meet the expected conditions.

### Tasks:
- [ ] Add count-based checks: `bullet_count`, `line_count`, `word_count_max`, `paragraph_count`, `sentence_count`.
- [ ] Add number-based checks: `number_equals`, `number_in_set`.
- [ ] Add formatting checks: `markdown_table`, `no_code_fence`, `starts_with`, `ends_with`.
- [ ] Create unit tests in `internal/canary/checks_test.go` covering all checks.

### Expected Result:
All supported canary check types should be evaluated correctly, and checks should fail when the expected conditions are not met.

---

## Issue 3: Add Real-Time Progress Updates in Terminal

**Labels:** `enhancement`, `priority: medium`, `offline-development`, `good first issue`

### Description:
Currently, when running a baseline or evaluation with multiple canary tests, the terminal does not show any updates until the entire process finishes.

This makes it difficult to know whether the monitor is still running or stuck.

### Tasks:
- [ ] Show the number of completed and total tests in real time.
- [ ] Display the canary ID, execution time, and status.
- [ ] Show how many checks passed or failed per task.
- [ ] Display a final summary with total execution time and pass/fail counts.

### Expected Result:
The terminal should display live updates like:
```text
[12/81] FMT-E2 completed in 180ms [2/2 PASS]
[13/81] REF-G1 completed in 240ms [1/1 PASS]

Execution completed!
Total tasks: 81
Failed tasks: 0
```

---

## Issue 4: Add Canary Tier Filtering

**Labels:** `enhancement`, `priority: medium`, `offline-development`, `good first issue`

### Description:
Currently, every baseline or evaluation run executes all 81 canary tests. This takes unnecessary time when developers only want to test a small part of the system.

We need to add an option to run canaries based on their difficulty tier (`easy`, `general`, `expensive`).

### Tasks:
- [ ] Add a `FilterTier()` function to the canary library in `internal/canary/canary.go`.
- [ ] Support `easy`, `general`, and `expensive` tiers.
- [ ] Add a `--tier` option to the CLI (`cmd/model-monitor/main.go`).
- [ ] Ensure only canaries belonging to the selected tier are executed.

### Expected Result:
Developers should be able to run:
```bash
go run ./cmd/model-monitor baseline --tier easy
```
This executes only the 27 easy-tier canaries instead of all 81 tests.

---

## Issue 5: Add Configuration File and Environment Variable Support

**Labels:** `enhancement`, `priority: medium`, `offline-development`

### Description:
Currently, model names and the Ollama endpoint are hardcoded directly in the Go source code.

This makes it difficult for developers to use different models or endpoints without modifying the source code.

### Tasks:
- [ ] Add support for loading settings from a `config.json` file.
- [ ] Allow model names to be changed through environment variables (`MONITOR_MODEL_A`, `MONITOR_MODEL_B`, `MONITOR_MODEL_C`).
- [ ] Allow developers to configure the Ollama endpoint via `OLLAMA_ENDPOINT`.
- [ ] Add a `--config` CLI option to specify a configuration file.

### Expected Result:
Developers can customize models and endpoints without editing Go code:
```bash
export OLLAMA_ENDPOINT="http://localhost:11434/api/chat"
export MONITOR_MODEL_A="qwen25-fp16-ctx8k:latest"
go run ./cmd/model-monitor baseline
```

---

## Issue 6: Add Style Feature Extraction for AI Responses

**Labels:** `enhancement`, `priority: low`, `offline-development`

### Description:
Currently, the monitor mainly records the AI response text and its execution time.

Changes in an AI model's writing style can also indicate a behavioural change, even when the actual meaning of the response remains similar.

We need to extract the 9 style features specified in `canaries_fixed.json`'s `style_feature_spec`.

### Tasks:
- [ ] Create `internal/features/features.go`.
- [ ] Calculate the 9 features:
  1. Word count
  2. Average sentence length
  3. Sentence length variation
  4. Unique word ratio
  5. Punctuation rate
  6. Uppercase ratio
  7. Paragraph count
  8. Bullet rate
  9. Contraction rate
- [ ] Attach the extracted features to each `Observation`.

### Expected Result:
Every AI response includes a feature vector containing the 9 writing-style measurements to assist layer attribution.
