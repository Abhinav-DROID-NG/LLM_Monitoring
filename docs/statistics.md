# Statistical methods

Binary features use Fisher's exact test for small samples and a
two-proportion z-test for larger samples. Cohen's *h* is the proportion
effect size. Continuous features use tie-corrected Mann-Whitney U with a
normal approximation, Cliff's delta, median shift, and a two-sample
Kolmogorov-Smirnov check.

Style changes use a seeded permutation test over hashed character trigrams
and word unigrams. Baseline windows are compared with other unchanged
windows to estimate natural noise. Benjamini-Hochberg correction controls the
false-discovery rate at `q=0.05`; a feature changes only when its adjusted
p-value and configured effect-size floor are both satisfied.

These are customer-visible statistical signals, not proof of provider
internals. Small effects can remain below configured floors, and absent or
unreliable provider token metadata limits conclusions.
