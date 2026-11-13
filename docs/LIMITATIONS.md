# Limitations

The lexical refusal classifier can misclassify quoted or contextual refusal
language. Sentence counting is punctuation-based and has known abbreviation
limitations. The language detector and mock-provider probabilities are
demonstrations, not calibrated provider measurements.

Rare inputs outside the fixed canary set are intentionally not covered.
Quantization can resemble a minor model change, hidden metadata can make model
attribution inconclusive, and slow drift requires periodic re-baselining.
Subtle changes and combined changes may fall below statistical thresholds; the
evaluation output reports their measured detection and exact-set rates.

