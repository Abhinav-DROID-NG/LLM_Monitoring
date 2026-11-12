# CLI reference

The binary is run with `go run ./cmd/model-monitor <command>`.

| Command | Purpose |
|---|---|
| `mock serve` | Start the local OpenAI-compatible mock provider. |
| `start` | Run the health endpoint and scheduled monitor in the foreground. |
| `stop` | Report that an external process supervisor is required. |
| `status` | Read SQLite monitor status. |
| `baseline` | Execute the existing synthetic suite and save a baseline window. |
| `run` | Execute one synthetic monitoring cycle and print a report. |
| `report` | Read the latest report when report persistence is configured. |
| `evaluate` | Generate the blind-evaluation Markdown and CSV outputs. |
| `demo` | Run a repeatable synthetic demonstration cycle. |
| `privacy-check` | Check for forbidden frontend paths and privacy artifacts. |

Configuration uses `MODEL_MONITOR_ADDR`, `MODEL_MONITOR_PROVIDER_URL`,
`MODEL_MONITOR_ALIAS` (default `support-bot-v1`), `MODEL_MONITOR_DATABASE`,
`MODEL_MONITOR_HTTP_TIMEOUT`, `MODEL_MONITOR_PROVIDER_RETRIES`,
`MODEL_MONITOR_INTERVAL` (default `15m`), `MODEL_MONITOR_PROFILE`
(`development`, `monitoring`, or `deep_analysis`), and
`MODEL_MONITOR_LOG_LEVEL`.

Run `model-monitor mock serve` in a separate process before `baseline`, `run`,
or `start`. It serves only synthetic OpenAI-compatible responses and an
admin-only layer switch endpoint; the monitor sends only the alias and
synthetic prompt to the provider URL.
