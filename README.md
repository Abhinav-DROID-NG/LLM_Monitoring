# LLM Monitoring

LLM Monitoring runs the precision-reasoning performance benchmark through the
stable `support-bot-v1` alias. It measures exact-answer performance without
accepting real user prompts.

## Requirements

- Go 1.23 or newer
- Ollama running locally at `http://127.0.0.1:11434`
- Node.js and npm for the frontend

The default Ollama models are:

```text
qwen2.5:3b-instruct-fp16
qwen2.5:3b-instruct-q8_0
qwen2.5:3b-instruct-q4_K_M
```

Pull them before running live checks:

```bash
ollama pull qwen2.5:3b-instruct-fp16
ollama pull qwen2.5:3b-instruct-q8_0
ollama pull qwen2.5:3b-instruct-q4_K_M
```

## Validate and test

Run these commands from the repository root:

```bash
go test ./...
go run ./cmd/model-monitor validate
go run ./cmd/model-monitor privacy-check
```

`validate` checks the canary library. `privacy-check` verifies that the
synthetic-only input policy is satisfied.

## CLI usage

Create a baseline, run monitoring cycles, compare models, or evaluate changes:

```bash
go run ./cmd/model-monitor baseline --concurrency 4
go run ./cmd/model-monitor run --profile monitoring --data-dir data
go run ./cmd/model-monitor start --profile monitoring --data-dir data
go run ./cmd/model-monitor compare --concurrency 4
go run ./cmd/model-monitor evaluate --trials 1 --concurrency 4
```

The `start` command runs continuously at the configured interval. The
`compare` command loads the latest baseline for Model A and evaluates the
configured models side by side. The `evaluate` command runs scenario trials;
use a small `--trials` value during development because each trial executes
the synthetic canary suite.

Useful options include:

```text
--mock              Use the offline mock provider
--profile <name>    Select the execution profile
--tier <tiers>      Run selected comma-separated tiers
--data-dir <path>   Store generated results in this directory
--concurrency <n>   Set probe concurrency
--trials <n>        Set evaluation trial count
--quiet             Suppress progress output
```

`--mock` is intended for offline development; its results are not claims about
production model providers.

## Frontend

The frontend is a React, TypeScript, Vite, Tailwind CSS, and Recharts
application. It communicates with the Go API server on port `8080`.

Start the API server:

```bash
go run ./cmd/model-monitor serve
```

For offline development, use the mock provider:

```bash
go run ./cmd/model-monitor serve --mock
```

In another terminal, install and start the frontend:

```bash
cd frontend
npm install
copy .env.example .env
npm run dev
```

Open `http://localhost:5173`. To create a production build:

```bash
npm run build
```

The frontend API URL can be changed with `VITE_API_BASE_URL` in
`frontend/.env`. Do not commit `.env` files or generated `data/`,
`frontend/node_modules/`, or `frontend/dist/` directories.

