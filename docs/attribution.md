# Attribution logic

The signature matrix maps changed features to four hypotheses: model/alias,
safety settings, request configuration, and serving stack. Scores weight
positive evidence, penalize features expected to remain stable, and record
primary-feature coverage. Scores are rankings, not calibrated probabilities.

The top candidate must clear `tau_min` and its margin over the runner-up must
clear `delta`. Otherwise the result is `INCONCLUSIVE` and the engine selects
existing-suite follow-ups, such as pinned request parameters, repeated
refusal canaries, or repeated probes across time.

Combined changes may return multiple layers only when each clears the
threshold and their changed primary features are distinct. Overlapping
signatures, effects too small to exceed a floor, missing metadata, and
conflicting evidence remain inconclusive rather than being forced into a
cause. The engine never reads mock hidden state or evaluation truth.
