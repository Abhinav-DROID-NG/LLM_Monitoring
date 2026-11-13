# LLM Monitoring

This repository runs synthetic canaries through the stable `support-bot-v1`
alias and measures changes without accepting real user prompts.

```bash
go test ./...
go run ./cmd/model-monitor validate
go run ./cmd/model-monitor baseline --mock --concurrency 4
go run ./cmd/model-monitor run --mock --profile monitoring --data-dir data
go run ./cmd/model-monitor evaluate --mock --trials 20
```

Useful flags include `--mock`, `--profile`, `--tier`, `--data-dir`, `--seed`,
`--trials`, `--concurrency`, and `--quiet`. The mock provider is an offline
demonstration; its numbers are not claims about production model providers.

