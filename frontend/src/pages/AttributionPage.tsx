import React from 'react';
import { Cpu, Lock, Server, Sliders } from 'lucide-react';
import type { MonitoringStore } from '../hooks/useMonitoringData';
import type { LayerScore } from '../types/monitoring';
import { humanizeMetricName } from '../utils/formatters';
import { EmptyView, ErrorView, LoadingView } from '../components/StateViews';
import { StatusBadge } from '../components/StatusBadge';

interface AttributionPageProps {
  store: MonitoringStore;
}

interface LayerDefinition {
  id: 'model' | 'serving' | 'safety' | 'request_config';
  title: string;
  subtitle: string;
  hypothesis: string;
  signatureFeatures: string[];
  canaryDivisions: string[];
  icon: React.ReactNode;
}

const FOUR_LAYERS: LayerDefinition[] = [
  {
    id: 'model',
    title: 'Layer 1: Model / Alias',
    subtitle: 'Weights, alias routing target, tokenizer, or resolved model version',
    hypothesis:
      'The underlying model behind alias support-bot-v1 changed, shifting capability, output token length, or format adherence while infrastructure latency remained flat.',
    signatureFeatures: ['output_token_count', 'json_validity_rate', 'returned_model'],
    canaryDivisions: ['CAP-*', 'STY-*', 'TOK-*', 'FMT-*'],
    icon: <Cpu className="w-4 h-4 text-sky-400" />,
  },
  {
    id: 'serving',
    title: 'Layer 2: Serving Stack',
    subtitle: 'Inference runtime, quantization, batching, hardware, or routing',
    hypothesis:
      'Inference serving infrastructure or quantization changed, altering P50/P95 response latency or output token generation without altering safety refusal rates.',
    signatureFeatures: ['latency_p50', 'latency_p95', 'output_token_count'],
    canaryDivisions: ['LAT-*', 'DET-*'],
    icon: <Server className="w-4 h-4 text-sky-400" />,
  },
  {
    id: 'safety',
    title: 'Layer 3: Safety Configuration',
    subtitle: 'Provider-side moderation filters or safety policy thresholds',
    hypothesis:
      'Safety filtering became stricter or looser, moving refusal rates or content_filter finish reasons on borderline-but-safe probes.',
    signatureFeatures: ['refusal_rate', 'provider_filter_rate'],
    canaryDivisions: ['REF-*'],
    icon: <Lock className="w-4 h-4 text-sky-400" />,
  },
  {
    id: 'request_config',
    title: 'Layer 4: Request Configuration',
    subtitle: 'Temperature (0), Seed (42), Max Tokens (512), System Prompt & Format',
    hypothesis:
      'Client-side generation parameters (temperature, seed, max_tokens, system prompt, or response_format) shifted, causing truncation or instruction/format regressions.',
    signatureFeatures: ['finish_reason_length_rate', 'json_validity_rate', 'structure_compliance'],
    canaryDivisions: ['TRN-*', 'INS-*', 'FMT-*', 'DET-*'],
    icon: <Sliders className="w-4 h-4 text-sky-400" />,
  },
];

type AttributionOutcome =
  | 'Likely cause'
  | 'Multiple possible causes'
  | 'Inconclusive'
  | 'Insufficient evidence';

export const AttributionPage: React.FC<AttributionPageProps> = ({ store }) => {
  const { attribution, runs, loading, error, refreshAll } = store;

  if (loading && !attribution) {
    return <LoadingView label="Computing four-layer causal attribution..." />;
  }

  if (error && !attribution) {
    return <ErrorView message={error} onRetry={refreshAll} />;
  }

  if (!attribution?.available) {
    return (
      <EmptyView
        title="No Baseline Data for Attribution"
        description={
          attribution?.message ||
          'Collect baseline and monitoring runs first so the four-layer attribution engine can evaluate observable deviations.'
        }
      />
    );
  }

  const attr = attribution.attribution;
  const detection = attribution.detection;
  const detectedChanges = detection?.changes || [];
  const rankedScores = attr?.ranked || [];
  const scoreMap = new Map<string, LayerScore>(rankedScores.map((r) => [r.layer, r]));
  const latestObs = runs?.runs?.[0]?.observations || [];

  const getLayerConclusion = (layerId: LayerDefinition['id']): AttributionOutcome => {
    if (!detection?.detected || detectedChanges.length === 0) {
      return 'Insufficient evidence';
    }
    const layerScore = scoreMap.get(layerId);
    if (!layerScore || layerScore.score === 0) {
      return 'Insufficient evidence';
    }
    const topScore = rankedScores[0]?.score ?? 0;
    const plausibleCount = rankedScores.filter((r) => r.score >= 0.4).length;

    if (attr?.inconclusive || attr?.confidence === 'inconclusive') {
      return plausibleCount > 1 ? 'Multiple possible causes' : 'Inconclusive';
    }
    if (layerScore.score === topScore && plausibleCount === 1) {
      return 'Likely cause';
    }
    if (layerScore.score >= 0.4 && plausibleCount > 1) {
      return 'Multiple possible causes';
    }
    return 'Inconclusive';
  };

  return (
    <div className="space-y-6">
      <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 className="text-base font-semibold text-slate-100">
              Four-Layer Causal Attribution Analysis
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Evaluated strictly from customer-observable canary telemetry (no provider-internal telemetry assumed)
            </p>
          </div>
          <div className="flex items-center gap-2 text-xs">
            <span className="text-slate-400">Backend Confidence:</span>
            <StatusBadge status={attr?.confidence || 'n/a'} />
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-2 text-xs font-mono">
          <div className="p-3 rounded-lg bg-slate-950 border border-slate-800">
            <div className="text-slate-400">Detected Feature Breaches</div>
            <div className="text-slate-100 font-semibold mt-1">{detectedChanges.length}</div>
          </div>
          <div className="p-3 rounded-lg bg-slate-950 border border-slate-800">
            <div className="text-slate-400">Top Layer Score Margin</div>
            <div className="text-slate-100 font-semibold mt-1">
              {attr ? attr.margin.toFixed(3) : '0.000'}
            </div>
          </div>
          <div className="p-3 rounded-lg bg-slate-950 border border-slate-800">
            <div className="text-slate-400">Fixed Request Controls</div>
            <div className="text-sky-300 font-semibold mt-1">temp=0 · seed=42 · max_tokens=512</div>
          </div>
        </div>

        {attribution.report?.suggested_next_check && (
          <div className="p-3 rounded-lg bg-slate-950/90 border border-slate-800 text-xs text-slate-300">
            <span className="text-sky-400 font-semibold">Recommended Verification Step: </span>
            {attribution.report.suggested_next_check}
          </div>
        )}
      </section>

      <section className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {FOUR_LAYERS.map((layer) => {
          const scoreObj = scoreMap.get(layer.id);
          const conclusion = getLayerConclusion(layer.id);

          const supporting = detectedChanges.filter((c) =>
            layer.signatureFeatures.includes(c.feature)
          );
          const contradicting = detectedChanges.filter(
            (c) => !layer.signatureFeatures.includes(c.feature)
          );

          const relevantObs = latestObs
            .filter((o) =>
              layer.canaryDivisions.some((prefix) =>
                o.canary_id.startsWith(prefix.replace('*', ''))
              )
            )
            .slice(0, 4);

          return (
            <div
              key={layer.id}
              className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 flex flex-col justify-between space-y-4"
            >
              <div className="space-y-4">
                <div className="flex items-start justify-between gap-3 border-b border-slate-800 pb-3">
                  <div className="flex items-start gap-2.5">
                    <div className="p-2 rounded-lg bg-slate-950 border border-slate-800 mt-0.5">
                      {layer.icon}
                    </div>
                    <div>
                      <h3 className="text-sm font-semibold text-slate-100">{layer.title}</h3>
                      <p className="text-xs text-slate-400">{layer.subtitle}</p>
                    </div>
                  </div>
                  <div className="text-right shrink-0">
                    <StatusBadge status={conclusion} />
                    {scoreObj && (
                      <div className="text-[11px] font-mono text-slate-400 mt-1">
                        signature score: {scoreObj.score.toFixed(2)}
                      </div>
                    )}
                  </div>
                </div>

                <div className="text-xs space-y-3">
                  <div>
                    <div className="text-[11px] uppercase tracking-wider text-slate-400 font-medium mb-1">
                      Investigation Status & Hypothesis
                    </div>
                    <p className="text-slate-300 leading-relaxed">{layer.hypothesis}</p>
                  </div>

                  {layer.id === 'request_config' && (
                    <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800 font-mono text-[11px] text-slate-300">
                      Observed Client Controls: Temperature = 0 | Seed = 42 | Max Tokens = 512 | System Instructions = Active
                    </div>
                  )}

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div className="p-3 rounded-lg bg-slate-950/90 border border-slate-800">
                      <div className="text-[11px] font-semibold text-emerald-400 mb-1">
                        Supporting Evidence
                      </div>
                      {supporting.length === 0 ? (
                        <p className="text-[11px] text-slate-500">
                          No breached signature metrics observed for this layer.
                        </p>
                      ) : (
                        <ul className="space-y-1 text-[11px] font-mono text-slate-200">
                          {supporting.map((s) => (
                            <li key={s.feature}>
                              • {humanizeMetricName(s.feature)} {s.direction} ({s.baseline.toFixed(2)} →{' '}
                              {s.current.toFixed(2)})
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>

                    <div className="p-3 rounded-lg bg-slate-950/90 border border-slate-800">
                      <div className="text-[11px] font-semibold text-amber-400 mb-1">
                        Contradicting / Other Evidence
                      </div>
                      {contradicting.length === 0 ? (
                        <p className="text-[11px] text-slate-500">
                          No conflicting deviations outside this layer’s signature.
                        </p>
                      ) : (
                        <ul className="space-y-1 text-[11px] font-mono text-slate-300">
                          {contradicting.map((c) => (
                            <li key={c.feature}>
                              • {humanizeMetricName(c.feature)} also shifted ({c.effect_size.toFixed(1)}σ)
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                  </div>

                  <div>
                    <div className="text-[11px] uppercase tracking-wider text-slate-400 font-medium mb-1">
                      Relevant Canary Probes ({layer.canaryDivisions.join(', ')})
                    </div>
                    {relevantObs.length === 0 ? (
                      <p className="text-[11px] text-slate-500">
                        Run synthetic canaries to populate probe-level observations for this layer.
                      </p>
                    ) : (
                      <div className="flex flex-wrap gap-1.5">
                        {relevantObs.map((o) => {
                          const ok =
                            !o.error &&
                            o.checks.filter((c) => !c.unavailable).every((c) => c.passed);
                          return (
                            <span
                              key={o.canary_id}
                              className={`px-2 py-0.5 rounded font-mono text-[11px] border ${
                                ok
                                  ? 'bg-emerald-950/40 text-emerald-300 border-emerald-800/50'
                                  : 'bg-rose-950/50 text-rose-300 border-rose-800/60'
                              }`}
                            >
                              {o.canary_id}: {ok ? 'PASS' : 'FAIL'} ({o.latency_ms}ms)
                            </span>
                          );
                        })}
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <div className="pt-3 border-t border-slate-800 flex items-center justify-between text-xs">
                <span className="text-slate-400">Layer Conclusion</span>
                <span className="font-semibold text-slate-100">{conclusion}</span>
              </div>
            </div>
          );
        })}
      </section>
    </div>
  );
};
