# Architecture

Canaries are loaded from `canaries_fixed.json`, sent only through the stable
alias, routed through model/safety/serving/request-configuration layers, and
then evaluated by checks and statistical detectors.

```text
canaries -> alias proxy -> provider layers -> upstream
                    \-> observations -> detector -> pinned rerun -> attribution -> report
```

The proxy allowlist rejects content that is not one of the synthetic canary
prompts. Ground-truth mutations are sealed separately from detector input.

