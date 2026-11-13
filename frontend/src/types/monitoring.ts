export type NavigationTab =
  | 'overview'
  | 'evaluations'
  | 'comparison'
  | 'drift'
  | 'canary-testing'
  | 'history'
  | 'settings';

export interface ExperimentConfig {
  temperature: number;
  seed: number;
  max_tokens: number;
}

export interface HealthResponse {
  status: 'connected' | 'disconnected';
  alias: string;
  model_identifier: string;
  candidate_models: string[];
  provider: string;
  endpoint: string;
  mock_mode: boolean;
  has_baseline: boolean;
  baseline_file: string;
  latest_run_file: string;
  last_execution_time: string | null;
  experiment_config: ExperimentConfig;
}

export interface CanaryCheckSpec {
  type: string;
  value?: string;
  values?: string[];
  [key: string]: unknown;
}

export interface CanaryTask {
  id: string;
  division: string;
  tier: 'easy' | 'general' | 'expensive';
  prompt: string;
  system?: string;
  filler?: unknown;
  temperature?: number;
  response_format?: string;
  max_tokens: number;
  checks: CanaryCheckSpec[];
  est_in_tokens: number;
  est_out_tokens: number;
  est_total_tokens: number;
}

export interface CanaryDivision {
  id: string;
  temperature?: number;
  targets_layer?: string;
  primary_metrics?: string[];
}

export interface CanariesResponse {
  meta: Record<string, unknown>;
  divisions: CanaryDivision[];
  tasks: CanaryTask[];
  check_types: Record<string, string>;
  profiles: Record<string, unknown>;
}

export interface CheckObservation {
  type: string;
  expected: string;
  passed: boolean;
  unavailable?: boolean;
  detail?: string;
}

export interface Observation {
  canary_id: string;
  run_id: string;
  probe_execution_id: string;
  request_config_hash: string;
  requested_model?: string;
  returned_model?: string;
  system_fingerprint?: string | null;
  timestamp: string;
  response: string;
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  finish_reason: string;
  http_status: number;
  retry_count: number;
  error?: string;
  checks: CheckObservation[];
  style?: number[];
}

export interface RunSummary {
  file_name: string;
  kind: 'run' | 'baseline';
  timestamp: string;
  total_tasks: number;
  passed_tasks: number;
  failed_tasks: number;
  pass_rate: number;
  observations?: Observation[];
}

export interface RunsResponse {
  runs: RunSummary[];
}

export interface ExecuteRunRequest {
  profile?: string;
  tiers?: ('easy' | 'general' | 'expensive')[];
  mode?: 'run' | 'baseline';
  temperature: number;
  seed: number;
  max_tokens: number;
}

export interface ExecuteRunResponse {
  file_name: string;
  mode: string;
  total_tasks: number;
  passed_tasks: number;
  failed_tasks: number;
  pass_rate: number;
  observations: Observation[];
}

export interface FeatureStats {
  mean: number;
  sd: number;
  n: number;
}

export interface BaselineResponse {
  available: boolean;
  message?: string;
  baseline_batches?: number;
  features?: Record<string, FeatureStats>;
  extracted_metrics?: Record<string, number>;
  observations?: Observation[];
}

export interface FeatureChange {
  feature: string;
  direction: 'increased' | 'decreased';
  effect_size: number;
  adjusted_p_value: number;
  baseline: number;
  current: number;
}

export interface DetectionResult {
  detected: boolean;
  changes: FeatureChange[];
}

export interface DriftAnalysis {
  detected: boolean;
  confidence: number;
  predicted_layer: string;
  evidence: string[];
}

export interface BehavioralReport {
  behavioral_drift_detected: boolean;
  latency_drift_detected: boolean;
  regression_count: number;
  improvement_count: number;
  comparable_check_count: number;
  unavailable_check_count: number;
  regression_rate: number;
  effect_size: number;
  statistical_p_value: number;
  statistical_significance: string;
  attributed_layer: string;
  attribution_confidence: string;
  evidence: string[];
  limitations: string[];
  stable_failure_count: number;
  missing_check_count: number;
  missing_canary_count: number;
  invalid_check_count: number;
}

export interface ThresholdsConfig {
  fdr_q: number;
  min_effect: Record<string, number>;
  [key: string]: unknown;
}

export interface DetectionResponse {
  available: boolean;
  message?: string;
  run_file?: string;
  thresholds?: ThresholdsConfig;
  detection?: DetectionResult;
  analysis?: DriftAnalysis;
  behavioral?: BehavioralReport;
  baseline_stats?: Record<string, FeatureStats>;
  baseline_metrics?: Record<string, number>;
  current_metrics?: Record<string, number>;
}

export interface LayerScore {
  layer: 'model' | 'serving' | 'safety' | 'request_config' | string;
  score: number;
  core_hit: number;
  precision: number;
}

export interface AttributionResult {
  ranked: LayerScore[];
  margin: number;
  confidence: 'high' | 'medium' | 'inconclusive' | 'n/a' | string;
  layers?: string[];
  scores?: LayerScore[];
  config_explained?: string[];
  provider_side?: string[];
  inconclusive: boolean;
  reasons?: string[];
}

export interface CheckReport {
  status: string;
  observed_evidence: DetectionResult;
  likely_cause: AttributionResult;
  suggested_next_check: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  seconds: number;
  run_id?: string;
  profile?: string;
  baseline_id?: string;
  unchanged?: string[];
  config_hash_baseline?: string;
  config_hash_current?: string;
  unavailable_metadata?: number;
}

export interface AttributionResponse {
  available: boolean;
  message?: string;
  run_file?: string;
  detection?: DetectionResult;
  attribution?: AttributionResult;
  analysis?: DriftAnalysis;
  behavioral?: BehavioralReport;
  report?: CheckReport;
}

export type DetectionStatusLabel =
  | 'Normal variation'
  | 'Potential deviation'
  | 'Confirmed threshold breach'
  | 'Recovered behaviour';
