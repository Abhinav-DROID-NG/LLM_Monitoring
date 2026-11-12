# Architecture

The CLI (`cmd/model-monitor`) dispatches operational commands. The provider
boundary in `internal/provider` accepts OpenAI-compatible requests and
responses. `internal/mockprovider` supplies a deterministic local provider
with independent model, serving, safety, and request-default switches.

`internal/canary` loads and validates the existing `canaries_fixed.json`
suite. `internal/probe` runs those tasks with bounded concurrency,
cancellation, retries, and metadata persistence. `internal/features` derives
privacy-safe structural metrics and hashed style vectors. `internal/baseline`
builds aggregate windows, while `internal/stats` detects changes.

`internal/attribution` ranks hidden-layer hypotheses without accessing mock
state. `internal/report` renders observed facts separately from inferred
causes. `internal/store` owns SQLite migrations and persistence. The
evaluation package is the only component allowed to unseal evaluation truth.

Dependencies flow inward through interfaces: monitor logic uses
`provider.Provider`, never mock internals. SQLite is the only persistence
technology and `net/http` is the only HTTP implementation.
