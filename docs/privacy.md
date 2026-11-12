# Privacy model

Only the checked-in synthetic canary suite enters the monitoring pipeline.
SQLite stores canary task IDs, provider-visible metadata, retry and status
fields, extracted aggregate features, baseline windows, detection vectors,
attribution results, and evaluation outputs.

The monitor discards prompt and response text after feature extraction, keeps
only short hashes for repeatability/style checks, and never stores
credentials, API keys, production traffic, or unrelated request context.
There is no production proxy endpoint. The automated privacy test injects a
sentinel into environment and context values, executes a run, scans stored
observations, and verifies that the sentinel is absent.

The mock provider's admin endpoint is separate from monitor APIs. Real
providers are optional and disabled by default; tests use only the local
mock and `httptest`.
