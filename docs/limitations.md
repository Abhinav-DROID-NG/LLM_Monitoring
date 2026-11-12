# Limitations

Customer-visible evidence cannot prove a provider's internal cause. Model,
safety, request-default, and serving signatures overlap, so ambiguous cases
return `INCONCLUSIVE`. Combined changes can mask one another, and effects
below configured floors do not alert.

Provider fingerprints, finish reasons, token counts, and latency may be
absent, approximate, or provider-specific. Hashed n-gram style vectors are
fixed-dimensional and collision-prone; they measure structural similarity,
not meaning. The mock provider is deterministic and synthetic, so it cannot
represent every hosted-provider behavior.

Rate limits and transient errors can reduce sample size despite bounded
retries. Baselines describe only the observed canary suite and time window;
they are not production traffic baselines. No frontend or live traffic
dashboard is provided.

The checked-in blind evaluation is a synthetic HTTP exercise, not evidence
about a real hosted model. Its current generated result includes misses and
`INCONCLUSIVE` attributions; see `docs/results/evaluation.md` and `.csv`
for the measured values rather than a guaranteed detection or attribution
rate.
