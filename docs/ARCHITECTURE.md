# Architecture

Performance probes are loaded from `precision_reasoning_benchmark.json`, sent
only through the stable alias, and evaluated by exact-answer checks and
statistical detectors.

```text
benchmark items -> alias proxy -> provider -> upstream
                    \-> observations -> detector -> pinned rerun -> attribution -> report
```

The proxy allowlist rejects content that is not one of the benchmark prompts.
Ground-truth mutations are sealed separately from detector input.

