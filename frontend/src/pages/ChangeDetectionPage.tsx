import React, { useMemo, useState } from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import type { MonitoringStore } from '../hooks/useMonitoringData';
import type { FeatureChange } from '../types/monitoring';
import {
  classifyDetectionStatus,
  formatNumber,
  formatTimestamp,
  humanizeMetricName,
} from '../utils/formatters';
import { EmptyView, ErrorView, LoadingView } from '../components/StateViews';
import { StatusBadge } from '../components/StatusBadge';

interface ChangeDetectionPageProps {
  store: MonitoringStore;
}

export const ChangeDetectionPage: React.FC<ChangeDetectionPageProps> = ({ store }) => {
  const { detection, baseline, runs, loading, error, refreshAll } = store;
  const [selectedFeature, setSelectedFeature] = useState<string | null>(null);

  if (loading && !detection) {
    return <LoadingView label="Evaluating statistical deviations against baseline..." />;
  }

  if (error && !detection) {
    return <ErrorView message={error} onRetry={refreshAll} />;
  }

  if (!detection?.available) {
    return (
      <EmptyView
        title="Baseline Required for Change Detection"
        description={
          detection?.message ||
          'Collect at least one baseline run on the Canary Testing page before evaluating behavioural drift.'
        }
      />
    );
  }

  const baseStats = detection.baseline_stats || baseline?.features || {};
  const baseMetrics = detection.baseline_metrics || {};
  const curMetrics = detection.current_metrics || {};
  const breaches = detection.detection?.changes || [];
  const breachMap = new Map<string, FeatureChange>(breaches.map((c) => [c.feature, c]));

  const allMetricKeys = useMemo(() => {
    const keys = new Set<string>([
      ...Object.keys(baseStats),
      ...Object.keys(baseMetrics),
      ...Object.keys(curMetrics),
    ]);
    return Array.from(keys).sort();
  }, [baseStats, baseMetrics, curMetrics]);

  const rows = useMemo(() => {
    return allMetricKeys.map((key) => {
      const stat = baseStats[key];
      const baseVal = stat ? stat.mean : baseMetrics[key] ?? 0;
      const curVal = breachMap.get(key)?.current ?? curMetrics[key] ?? baseVal;
      const absDev = curVal - baseVal;
      const relDev = Math.abs(baseVal) > 1e-6 ? (absDev / baseVal) * 100 : null;
      const breach = breachMap.get(key);
      const sd = stat?.sd ?? 0;
      const zScore = breach ? breach.effect_size : sd > 0 ? absDev / sd : 0;

      const isLatency = key.includes('latency');
      const minEff = isLatency
        ? detection.thresholds?.min_effect?.latency_relative ?? 0.2
        : detection.thresholds?.min_effect?.proportion_abs ?? 0.1;

      const status = classifyDetectionStatus(breach, zScore);

      return {
        key,
        label: humanizeMetricName(key),
        baseline: baseVal,
        current: curVal,
        absDev,
        relDev,
        sd,
        zScore,
        thresholdText: `FDR q≤${detection.thresholds?.fdr_q ?? 0.05}, Δ≥${minEff}`,
        status,
        breach,
      };
    });
  }, [allMetricKeys, baseStats, baseMetrics, curMetrics, breachMap, detection.thresholds]);

  const activeRow = rows.find((r) => r.key === selectedFeature) || rows[0];

  const trendData = useMemo(() => {
    const history = [...(runs?.runs || [])].reverse();
    return history.map((r, idx) => ({
      cycle: `#${idx + 1}`,
      timestamp: formatTimestamp(r.timestamp),
      passRate: Number(r.pass_rate.toFixed(1)),
      kind: r.kind,
    }));
  }, [runs]);

  const zChartData = rows
    .filter((r) => r.key !== 'returned_model')
    .map((r) => ({
      name: r.key,
      zScore: Number(r.zScore.toFixed(2)),
    }));

  return (
    <div className="space-y-6">
      <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h2 className="text-base font-semibold text-slate-100">
              Statistical Behaviour Change Detection
            </h2>
            <StatusBadge
              status={
                detection.detection?.detected
                  ? 'Confirmed threshold breach'
                  : 'Normal variation'
              }
            />
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Comparing <code className="text-sky-300">{detection.run_file}</code> against{' '}
            {baseline?.baseline_batches ?? 1} baseline batch(es) using Benjamini-Hochberg FDR correction (q ={' '}
            {detection.thresholds?.fdr_q ?? 0.05})
          </p>
        </div>

        <div className="text-xs font-mono text-slate-300 bg-slate-950 px-3 py-2 rounded-lg border border-slate-800">
          Breached Features: <span className="text-sky-300 font-semibold">{breaches.length}</span> /{' '}
          {rows.length}
        </div>
      </section>

      <section className="bg-slate-900/80 border border-slate-800 rounded-xl overflow-hidden">
        <div className="px-5 py-3.5 border-b border-slate-800">
          <h3 className="text-sm font-semibold text-slate-100">
            Baseline vs Current Feature Comparison
          </h3>
          <p className="text-xs text-slate-400">
            Click any metric row to inspect statistical test details and drift evidence
          </p>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs">
            <thead className="bg-slate-950 text-slate-400 border-b border-slate-800">
              <tr>
                <th className="py-3 px-4 font-medium">Metric</th>
                <th className="py-3 px-4 font-medium">Baseline</th>
                <th className="py-3 px-4 font-medium">Current</th>
                <th className="py-3 px-4 font-medium">Abs. Deviation</th>
                <th className="py-3 px-4 font-medium">Rel. Deviation</th>
                <th className="py-3 px-4 font-medium">Configured Threshold</th>
                <th className="py-3 px-4 font-medium">Detection Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/80">
              {rows.map((row) => {
                const selected = activeRow?.key === row.key;
                return (
                  <tr
                    key={row.key}
                    onClick={() => setSelectedFeature(row.key)}
                    className={`cursor-pointer transition-colors ${
                      selected ? 'bg-sky-950/35' : 'hover:bg-slate-800/40'
                    }`}
                  >
                    <td className="py-3 px-4 font-medium text-slate-200">
                      <div>{row.label}</div>
                      <div className="text-[11px] font-mono text-slate-500">{row.key}</div>
                    </td>
                    <td className="py-3 px-4 font-mono text-slate-300">
                      {formatNumber(row.baseline, 4)}
                    </td>
                    <td className="py-3 px-4 font-mono text-slate-100 font-semibold">
                      {formatNumber(row.current, 4)}
                    </td>
                    <td className="py-3 px-4 font-mono">
                      <span
                        className={
                          row.absDev > 0
                            ? 'text-amber-300'
                            : row.absDev < 0
                            ? 'text-sky-300'
                            : 'text-slate-400'
                        }
                      >
                        {row.absDev >= 0 ? '+' : ''}
                        {formatNumber(row.absDev, 4)}
                      </span>
                    </td>
                    <td className="py-3 px-4 font-mono text-slate-300">
                      {row.relDev !== null
                        ? `${row.relDev >= 0 ? '+' : ''}${row.relDev.toFixed(1)}%`
                        : 'N/A (0 baseline)'}
                    </td>
                    <td className="py-3 px-4 font-mono text-slate-400">{row.thresholdText}</td>
                    <td className="py-3 px-4">
                      <StatusBadge status={row.status} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-3">
          <h3 className="text-sm font-semibold text-slate-100">Deviation Detail Inspector</h3>
          {activeRow ? (
            <div className="space-y-3 text-xs">
              <div className="p-3 rounded-lg bg-slate-950 border border-slate-800 space-y-1.5">
                <div className="font-semibold text-sky-300">{activeRow.label}</div>
                <div className="font-mono text-slate-400">Feature key: {activeRow.key}</div>
                <div className="pt-1">
                  <StatusBadge status={activeRow.status} />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2 font-mono">
                <div className="p-2.5 rounded bg-slate-950 border border-slate-800">
                  <div className="text-[11px] text-slate-400">Effect Size (Z)</div>
                  <div className="text-sm font-semibold text-slate-100 mt-0.5">
                    {activeRow.zScore.toFixed(3)}σ
                  </div>
                </div>
                <div className="p-2.5 rounded bg-slate-950 border border-slate-800">
                  <div className="text-[11px] text-slate-400">Adjusted p-value</div>
                  <div className="text-sm font-semibold text-slate-100 mt-0.5">
                    {activeRow.breach ? activeRow.breach.adjusted_p_value.toExponential(2) : 'Not rejected'}
                  </div>
                </div>
              </div>

              <div>
                <div className="text-[11px] uppercase tracking-wider text-slate-400 mb-1">
                  Backend Drift Evidence
                </div>
                {detection.analysis?.evidence && detection.analysis.evidence.length > 0 ? (
                  <ul className="space-y-1 max-h-40 overflow-y-auto font-mono text-[11px] text-slate-300 bg-slate-950 p-2.5 rounded-lg border border-slate-800">
                    {detection.analysis.evidence.map((ev, i) => (
                      <li key={i}>• {ev}</li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-slate-400">No task-level drift evidence logged for this run.</p>
                )}
              </div>
            </div>
          ) : null}
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
          <h3 className="text-sm font-semibold text-slate-100 mb-1">
            Standardized Deviation (Z-Score)
          </h3>
          <p className="text-xs text-slate-400 mb-4">Threshold reference lines at ±4.0σ</p>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={zChartData} layout="vertical" margin={{ left: 20, right: 16 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
                <XAxis type="number" tick={{ fill: '#94a3b8', fontSize: 11 }} />
                <YAxis
                  dataKey="name"
                  type="category"
                  width={125}
                  tick={{ fill: '#94a3b8', fontSize: 10 }}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#0f172a',
                    borderColor: '#334155',
                    color: '#f8fafc',
                    fontSize: 12,
                  }}
                />
                <ReferenceLine x={4} stroke="#f43f5e" strokeDasharray="4 4" />
                <ReferenceLine x={-4} stroke="#f43f5e" strokeDasharray="4 4" />
                <Bar dataKey="zScore" fill="#38bdf8" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
          <h3 className="text-sm font-semibold text-slate-100 mb-1">
            Canary Pass Rate Trend Timeline
          </h3>
          <p className="text-xs text-slate-400 mb-4">Sequential pass rate across persisted cycles</p>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={trendData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
                <XAxis dataKey="cycle" tick={{ fill: '#94a3b8', fontSize: 11 }} />
                <YAxis domain={[0, 100]} unit="%" tick={{ fill: '#94a3b8', fontSize: 11 }} />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#0f172a',
                    borderColor: '#334155',
                    color: '#f8fafc',
                    fontSize: 12,
                  }}
                />
                <Legend wrapperStyle={{ fontSize: 12 }} />
                <Line
                  type="monotone"
                  dataKey="passRate"
                  name="Pass Rate (%)"
                  stroke="#38bdf8"
                  strokeWidth={2}
                  dot={{ r: 3 }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>
    </div>
  );
};
