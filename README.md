# LLM Monitoring

This repository runs synthetic canaries through the stable `support-bot-v1`
alias and measures changes without accepting real user prompts.

```bash
go test ./...
go run ./cmd/model-monitor validate
ollama pull qwen2.5:3b-instruct-fp16
ollama pull qwen2.5:3b-instruct-q8_0
ollama pull qwen2.5:3b-instruct-q4_K_M
go run ./cmd/model-monitor baseline --concurrency 4
go run ./cmd/model-monitor run --profile monitoring --data-dir data
go run ./cmd/model-monitor evaluate --trials 1 --concurrency 4
```

The default provider is Ollama at `http://127.0.0.1:11434/api/chat`. The
evaluation command runs live Ollama trials by default; use a small `--trials`
value while testing because each trial runs the full synthetic canary suite.
`--mock` is an explicit offline option for development only, and its numbers
are not claims about production model providers.

