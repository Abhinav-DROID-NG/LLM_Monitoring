import React from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  Cpu,
  Play,
  Server,
  XCircle,
} from 'lucide-react';
import type { MonitoringStore } from '../hooks/useMonitoringData';
import type { NavigationTab } from '../types/monitoring';
import { formatNumber, formatPercent, formatTimestamp, humanizeMetricName } from '../utils/formatters';
import { MetricCard } from '../components/MetricCard';
import { EmptyView, ErrorView, LoadingView } from '../components/StateViews';
import { StatusBadge } from '../components/StatusBadge';

interface DashboardPageProps {
  store: MonitoringStore;
  onNavigate: (tab: NavigationTab) => void;
}

export const DashboardPage: React.FC<DashboardPageProps> = ({ store, onNavigate }) => {
  const { health, runs, baseline, detection, loading, error, refreshAll } = store;

  if (loading && !health) {
    return <LoadingView label="Fetching model health and baseline telemetry..." />;
  }

  if (error && !health) {
    return <ErrorView message={error} onRetry={refreshAll} />;
  }

  const runHistory = runs?.runs || [];
  const latestRun = runHistory[0];
  const totalExecutions = runHistory.reduce((acc, r) => acc + r.total_tasks, 0);
  const latestPassed = latestRun?.passed_tasks ?? 0;
  const latestFailed = latestRun?.failed_tasks ?? 0;
  const latestPassRate = latestRun ? latestRun.pass_rate : null;
  const detectedChangesCount = detection?.detection?.changes?.length ?? 0;

  const baseMetrics = detection?.baseline_metrics || baseline?.extracted_metrics || {};
  const curMetrics = detection?.current_metrics || baseline?.extracted_metrics || {};

  const baselineRun = runHistory.find((r) => r.kind === 'baseline');
  const baselinePassRate = baselineRun ? baselineRun.pass_rate / 100 : undefined;
  const currentPassRateRatio = latestRun ? latestRun.pass_rate / 100 : undefined;

  const rateComparisonData = [
    {
      metric: 'Canary Pass Rate',
      Baseline: baselinePassRate !== undefined ? Number((baselinePassRate * 100).toFixed(1)) : 0,
      Current: currentPassRateRatio !== undefined ? Number((currentPassRateRatio * 100).toFixed(1)) : 0,
    },
    {
      metric: 'Format Adherence (JSON)',
      Baseline: Number(((baseMetrics.json_validity_rate ?? 0) * 100).toFixed(1)),
      Current: Number(((curMetrics.json_validity_rate ?? 0) * 100).toFixed(1)),
    },
    {
      metric: 'Structure Compliance',
      Baseline: Number(((baseMetrics.structure_compliance_rate ?? 0) * 100).toFixed(1)),
      Current: Number(((curMetrics.structure_compliance_rate ?? 0) * 100).toFixed(1)),
    },
    {
      metric: 'Refusal Rate',
      Baseline: Number(((baseMetrics.refusal_rate ?? 0) * 100).toFixed(1)),
      Current: Number(((curMetrics.refusal_rate ?? 0) * 100).toFixed(1)),
    },
    {
      metric: 'Semantic/Style Drift Proxy',
      Baseline: Number(((baseMetrics.ambiguous_rate ?? 0) * 100).toFixed(1)),
      Current: Number(((curMetrics.ambiguous_rate ?? 0) * 100).toFixed(1)),
    },
  ];

  return (
    <div className="space-y-6">
      <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
        <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-800">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-400">
              <Cpu className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center gap-2.5">
                <h2 className="text-base font-semibold text-slate-100">
                  {health?.model_identifier || 'qwen2.5:3b-instruct-fp16'}
                </h2>
                <span className="px-2 py-0.5 text-xs font-mono rounded bg-slate-800 text-sky-300 border border-slate-700">
                  alias: {health?.alias || 'support-bot-v1'}
                </span>
              </div>
              <p className="text-xs text-slate-400 mt-0.5">
                Synthetic canary monitoring without storing production user prompts
              </p>
            </div>
          </div>

          <button
            onClick={() => onNavigate('canary-testing')}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold transition-colors"
          >
            <Play className="w-3.5 h-3.5" />
            Run Synthetic Canaries
          </button>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 pt-4 text-xs">
          <div className="flex items-start gap-2.5">
            <Server className="w-4 h-4 text-slate-400 mt-0.5 shrink-0" />
            <div>
              <div className="text-slate-400">Provider & Endpoint</div>
              <div className="font-mono text-slate-200 mt-0.5 break-all">
                {health?.provider || 'ollama'} — {health?.endpoint || 'http://127.0.0.1:11434/api/chat'}
              </div>
            </div>
          </div>

          <div className="flex items-start gap-2.5">
            <CheckCircle2 className="w-4 h-4 text-emerald-400 mt-0.5 shrink-0" />
            <div>
              <div className="text-slate-400">Backend Connection</div>
              <div className="font-medium text-emerald-300 mt-0.5">
                {health?.status === 'connected' ? 'Connected (Go Monitor API)' : 'Disconnected'}
              </div>
            </div>
          </div>

          <div className="flex items-start gap-2.5">
            <Clock className="w-4 h-4 text-slate-400 mt-0.5 shrink-0" />
            <div>
              <div className="text-slate-400">Latest Execution Time</div>
              <div className="text-slate-200 mt-0.5">
                {formatTimestamp(health?.last_execution_time)}
              </div>
            </div>
          </div>

          <div className="flex items-start gap-2.5">
            <AlertCircle className="w-4 h-4 text-slate-400 mt-0.5 shrink-0" />
            <div>
              <div className="text-slate-400">Reference Baseline</div>
              <div className="font-mono text-slate-200 mt-0.5">
                {health?.has_baseline ? health.baseline_file : 'Not yet collected'}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <MetricCard
          label="Total Canary Executions"
          value={totalExecutions}
          subtext={`Across ${runHistory.length} recorded cycle(s)`}
        />
        <MetricCard
          label="Passed Canary Tests"
          value={latestRun ? latestPassed : '—'}
          subtext={latestRun ? `Latest: ${latestRun.file_name}` : 'No runs yet'}
          accent="good"
          icon={<CheckCircle2 className="w-4 h-4 text-emerald-400" />}
        />
        <MetricCard
          label="Failed Canary Tests"
          value={latestRun ? latestFailed : '—'}
          subtext={latestRun ? `Out of ${latestRun.total_tasks} probes` : 'No runs yet'}
          accent={latestFailed > 0 ? 'warn' : 'neutral'}
          icon={<XCircle className="w-4 h-4 text-rose-400" />}
        />
        <MetricCard
          label="Canary Pass Rate"
          value={latestPassRate !== null ? `${latestPassRate.toFixed(1)}%` : '—'}
          subtext={
            baselinePassRate !== undefined
              ? `Baseline: ${(baselinePassRate * 100).toFixed(1)}%`
              : 'Collect baseline for reference'
          }
        />
        <MetricCard
          label="Behavioural Changes"
          value={detectedChangesCount}
          subtext={
            detectedChangesCount > 0
              ? 'Threshold breach flagged'
              : 'Within statistical baseline'
          }
          accent={detectedChangesCount > 0 ? 'danger' : 'good'}
        />
      </section>

      {!baseline?.available && runHistory.length === 0 ? (
        <EmptyView
          title="No Baseline or Monitoring Runs Recorded"
          description="Execute a baseline run first so ModelWatch can establish statistical distributions across all 81 synthetic canaries."
          action={
            <button
              onClick={() => onNavigate('canary-testing')}
              className="px-4 py-2 rounded-lg bg-sky-600 hover:bg-sky-500 text-xs font-semibold text-white"
            >
              Go to Canary Testing
            </button>
          }
        />
      ) : (
        <section className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="lg:col-span-2 bg-slate-900/80 border border-slate-800 rounded-xl p-5">
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-slate-100">
                Behavioural Rates: Baseline vs Current (%)
              </h3>
              <p className="text-xs text-slate-400 mt-0.5">
                Direct comparison of format adherence, refusal rate, pass rate, and style/ambiguity drift
              </p>
            </div>
            <div className="h-72">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={rateComparisonData} margin={{ top: 10, right: 16, left: 0, bottom: 10 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
                  <XAxis dataKey="metric" tick={{ fill: '#94a3b8', fontSize: 11 }} />
                  <YAxis domain={[0, 100]} tick={{ fill: '#94a3b8', fontSize: 11 }} unit="%" />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: '#0f172a',
                      borderColor: '#334155',
                      color: '#f8fafc',
                      fontSize: 12,
                    }}
                  />
                  <Legend wrapperStyle={{ fontSize: 12 }} />
                  <Bar dataKey="Baseline" fill="#475569" radius={[4, 4, 0, 0]} />
                  <Bar dataKey="Current" fill="#38bdf8" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>

          <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-3">
            <h3 className="text-sm font-semibold text-slate-100">Key Behavioural Indicators</h3>
            <p className="text-xs text-slate-400">Baseline vs current measurements from Go detector</p>

            <div className="divide-y divide-slate-800 text-xs">
              {[
                {
                  name: 'Format Adherence (JSON)',
                  base: formatPercent(baseMetrics.json_validity_rate),
                  cur: formatPercent(curMetrics.json_validity_rate),
                },
                {
                  name: 'Refusal Rate',
                  base: formatPercent(baseMetrics.refusal_rate),
                  cur: formatPercent(curMetrics.refusal_rate),
                },
                {
                  name: 'Output Length (Tokens)',
                  base: formatNumber(baseMetrics.output_tokens, 1),
                  cur: formatNumber(curMetrics.output_tokens, 1),
                },
                {
                  name: 'Semantic / Style Drift (Ambiguous)',
                  base: formatPercent(baseMetrics.ambiguous_rate),
                  cur: formatPercent(curMetrics.ambiguous_rate),
                },
                {
                  name: 'Canary Pass Rate',
                  base: formatPercent(baselinePassRate),
                  cur: formatPercent(currentPassRateRatio),
                },
              ].map((row) => (
                <div key={row.name} className="py-2.5 flex items-center justify-between gap-2">
                  <span className="text-slate-300 font-medium">{row.name}</span>
                  <div className="text-right font-mono">
                    <span className="text-slate-400">{row.base}</span>
                    <span className="text-slate-600 mx-1.5">→</span>
                    <span className="text-sky-300 font-semibold">{row.cur}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
      )}

      <section className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
          <h3 className="text-sm font-semibold text-slate-100 mb-3">Recent Canary Executions</h3>
          {runHistory.length === 0 ? (
            <p className="text-xs text-slate-400">No execution history available yet.</p>
          ) : (
            <div className="space-y-2.5">
              {runHistory.slice(0, 6).map((item) => (
                <div
                  key={item.file_name}
                  className="flex items-center justify-between p-3 rounded-lg bg-slate-950/70 border border-slate-800 text-xs"
                >
                  <div>
                    <div className="font-mono text-slate-200">{item.file_name}</div>
                    <div className="text-slate-400 mt-0.5">{formatTimestamp(item.timestamp)}</div>
                  </div>
                  <div className="text-right">
                    <div className="font-semibold text-slate-100">
                      {item.passed_tasks}/{item.total_tasks} passed ({item.pass_rate.toFixed(1)}%)
                    </div>
                    <div className="text-[11px] uppercase tracking-wider text-sky-400 mt-0.5">
                      {item.kind}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm font-semibold text-slate-100">Detected Behavioural Deviations</h3>
            {detectedChangesCount > 0 && (
              <button
                onClick={() => onNavigate('change-detection')}
                className="text-xs text-sky-400 hover:text-sky-300 font-medium"
              >
                Inspect Details →
              </button>
            )}
          </div>
          {!detection?.detection?.changes || detection.detection.changes.length === 0 ? (
            <p className="text-xs text-slate-400">
              No statistically significant deviations detected in the latest monitoring cycle.
            </p>
          ) : (
            <div className="space-y-2.5">
              {detection.detection.changes.map((chg) => (
                <div
                  key={chg.feature}
                  onClick={() => onNavigate('change-detection')}
                  className="cursor-pointer flex items-center justify-between p-3 rounded-lg bg-rose-950/20 hover:bg-rose-950/35 border border-rose-800/50 text-xs transition-colors"
                >
                  <div>
                    <div className="font-semibold text-rose-200">
                      {humanizeMetricName(chg.feature)} ({chg.direction})
                    </div>
                    <div className="text-slate-400 font-mono mt-0.5">
                      Baseline: {chg.baseline.toFixed(3)} → Current: {chg.current.toFixed(3)} (z=
                      {chg.effect_size.toFixed(2)}σ)
                    </div>
                  </div>
                  <StatusBadge status="Confirmed threshold breach" />
                </div>
              ))}
            </div>
          )}
        </div>
      </section>
    </div>
  );
};
