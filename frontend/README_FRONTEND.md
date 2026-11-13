# ModelWatch Frontend & Go API Bridge

## Contents
- `frontend/`: Complete React + TypeScript + Vite + Tailwind CSS + Recharts + Lucide React application (`Dashboard`, `Canary Testing`, `Change Detection`, `Attribution`).
- `backend_patch/cmd/model-monitor/server.go`: Additive HTTP REST + CORS server exposing your existing Go engine over `/api/*`.
- `backend_patch/BACKEND_NOTES.md`: Documented findings and minor bug notes discovered during Go repository inspection.

## Quick Start

1. **Wire the Go HTTP server (`serve` command):**
   - Copy `backend_patch/cmd/model-monitor/server.go` into your repository at `cmd/model-monitor/server.go`.
   - In `cmd/model-monitor/main.go`, add `case "serve": return serve(cfg)` inside `switch command`.
   - Start the backend (use `--mock` for instant offline testing, or omit `--mock` for live Ollama):
     ```bash
     go run ./cmd/model-monitor serve --mock --concurrency 4 --data-dir data
     ```

2. **Start the Frontend:**
   ```bash
   cd frontend
   cp .env.example .env
   npm install
   npm run build
   npm run dev
   ```
   Open `http://localhost:5173`.
