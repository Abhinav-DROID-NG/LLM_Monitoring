import React, { useMemo, useState } from 'react';
import type { MonitoringStore } from '../hooks/useMonitoringData';
import { MetricCard } from '../components/MetricCard';
import { formatNumber, formatTimestamp } from '../utils/formatters';

export const EvaluationsPage: React.FC<{ store: MonitoringStore }> = ({ store }) => {
  const [running, setRunning] = useState<string | null>(null);
  const evaluations = store.evaluations?.evaluations || [];
  const latest = useMemo(() => ({ behavioural: evaluations.find((e) => e.objective === 'behavioural'), performance: evaluations.find((e) => e.objective === 'performance') }), [evaluations]);
  const run = async (objective: 'behavioural' | 'performance') => { setRunning(objective); try { await fetch('/api/evaluation', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ objective }) }); await store.refreshAll(); } finally { setRunning(null); } };
  return <div className="space-y-6">
    <section className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {(['behavioural', 'performance'] as const).map((objective) => { const item = latest[objective]; const values = item?.metrics.values || {}; return <div key={objective} className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
        <div className="flex justify-between items-center mb-4"><div><h2 className="font-semibold capitalize">{objective} monitoring</h2><p className="text-xs text-slate-400">{item ? `Run ${item.metadata.run_id}` : 'No evaluation recorded'}</p></div><button onClick={() => void run(objective)} disabled={running !== null} className="px-3 py-1.5 rounded-lg bg-sky-600 text-xs font-semibold disabled:opacity-50">{running === objective ? 'Running…' : 'Run'}</button></div>
        <div className="grid grid-cols-2 gap-3"><MetricCard label="Coverage" value={item ? `${(item.metrics.coverage * 100).toFixed(1)}%` : '—'} /><MetricCard label={objective === 'performance' ? 'Latency P95' : 'Pass Rate'} value={objective === 'performance' ? formatNumber(values.latency_p95) : item ? `${((values.pass_rate || 0) * 100).toFixed(1)}%` : '—'} /></div>
        {item && <p className="text-xs text-slate-500 mt-4">Started {formatTimestamp(item.metadata.started_at)} · {item.trials.length} raw trials preserved</p>}
      </div>; })}
    </section>
    <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5"><h2 className="font-semibold mb-3">Evaluation history</h2><div className="space-y-2">{evaluations.map((item) => <div key={item.metadata.run_id} className="flex justify-between text-xs border-b border-slate-800 py-2"><span>{item.metadata.run_id} · {item.objective}</span><span className="text-slate-400">{item.status} · {formatTimestamp(item.metadata.started_at)}</span></div>)}</div></section>
  </div>;
};
