# Layer-Attribution Model Monitor

`model-monitor` is a backend-only Go service that detects observable
behaviour changes behind a stable model alias and ranks likely causes. It
uses only synthetic canaries; it is not a production request proxy.

## Quick start

```text
go test ./...
go run ./cmd/model-monitor baseline
go run ./cmd/model-monitor run
go run ./cmd/model-monitor start
go run ./cmd/model-monitor demo
go run ./cmd/model-monitor privacy-check
go run ./cmd/model-monitor evaluate
```

The evaluation command writes `docs/results/evaluation.md` and
`docs/results/evaluation.csv`. Configure the SQLite path, server address,
provider URL, timeout, retries, and log level with the `MODEL_MONITOR_*`
environment variables.

For the default synthetic setup, start `go run ./cmd/model-monitor mock serve`
in another process first. Real OpenAI-compatible providers are optional and
off by default. `MODEL_MONITOR_ALIAS` defaults to `support-bot-v1`;
`MODEL_MONITOR_INTERVAL` defaults to `15m`, and
`MODEL_MONITOR_PROFILE` selects `development`, `monitoring`, or
`deep_analysis`. The monitor has no production-traffic route.

## Data and privacy

The monitor stores synthetic canary identifiers, provider metadata, hashed
or aggregate features, baseline summaries, detection vectors, attribution
results, and evaluation outputs. It discards real prompts, real responses,
credentials, API keys, and unrelated request context. No frontend or
production traffic ingestion route exists.

See [architecture](docs/architecture.md), [privacy](docs/privacy.md),
[statistics](docs/statistics.md), [attribution](docs/attribution.md),
[evaluation](docs/evaluation.md), [limitations](docs/limitations.md), and
the [CLI reference](docs/cli.md).
