# Evaluation methodology

The harness creates 220 reproducible scenarios: 60 unchanged, 30 each for
model, safety, request configuration, and serving changes, plus 40 combined
changes. It writes a SHA-256 commitment before execution, keeps truth sealed
while monitor outputs are produced, verifies the commitment, and only then
unseals truth for scoring.

Results include detection, missed-change, false-alarm, single-layer
attribution, combined attribution, confusion-matrix, request, token, cost,
and latency metrics. Wilson intervals are used for rates. Monetary cost is
unavailable when provider pricing is not configured; token usage is still
reported. Development and evaluation seeds must remain disjoint.

The checked-in result files are generated under `docs/results/`. Actual
measurements, including missed targets or limitations, must be reported
without changing thresholds after held-out evaluation.
