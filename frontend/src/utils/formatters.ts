import type { DetectionStatusLabel, FeatureChange } from '../types/monitoring';

export function formatTimestamp(ts: string | null | undefined): string {
  if (!ts) return 'Never executed';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export function formatPercent(ratio: number | undefined | null, decimals = 1): string {
  if (ratio === undefined || ratio === null || Number.isNaN(ratio)) return '—';
  return `${(ratio * 100).toFixed(decimals)}%`;
}

export function formatNumber(val: number | undefined | null, decimals = 2): string {
  if (val === undefined || val === null || Number.isNaN(val)) return '—';
  return val.toFixed(decimals);
}

export function humanizeMetricName(key: string): string {
  const labels: Record<string, string> = {
    refusal_rate: 'Refusal Rate',
    json_validity_rate: 'JSON Format Adherence',
    structure_compliance: 'Structure Compliance',
    structure_compliance_rate: 'Structure Compliance Rate',
    rule_compliance_rate: 'Rule Compliance Rate',
    output_token_count: 'Mean Output Length (Tokens)',
    output_tokens: 'Mean Output Length (Tokens)',
    output_words: 'Mean Output Length (Words)',
    input_token_count: 'Input Token Count',
    latency_p50: 'Latency P50 (ms)',
    latency_p95: 'Latency P95 (ms)',
    latency_mean: 'Mean Latency (ms)',
    finish_reason_length_rate: 'Truncation Rate (length)',
    truncation_rate: 'Truncation Rate',
    sentinel_present_rate: 'Sentinel Adherence Rate',
    returned_model: 'Returned Model Fingerprint',
    returned_model_consistency: 'Returned Model Consistency',
    error_rate: 'Provider Error Rate',
    ambiguous_rate: 'Ambiguous Refusal Rate',
    provider_filter_rate: 'Provider Filter Rate',
  };
  return labels[key] || key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

export function classifyDetectionStatus(
  breach: FeatureChange | undefined,
  zScore: number,
  previouslyBreached = false
): DetectionStatusLabel {
  if (breach) {
    return 'Confirmed threshold breach';
  }
  if (previouslyBreached) {
    return 'Recovered behaviour';
  }
  if (Math.abs(zScore) >= 2.0) {
    return 'Potential deviation';
  }
  return 'Normal variation';
}
