import React, { useMemo, useState } from 'react';
import {
  CheckCircle2,
  Database,
  Loader2,
  Play,
  Search,
  XCircle,
} from 'lucide-react';
import { monitoringApi } from '../services/api';
import type { MonitoringStore } from '../hooks/useMonitoringData';
import type { CanaryTask, Observation } from '../types/monitoring';
import { formatTimestamp } from '../utils/formatters';
import { EmptyView, ErrorView, LoadingView } from '../components/StateViews';
import { StatusBadge } from '../components/StatusBadge';

interface CanaryTestingPageProps {
  store: MonitoringStore;
}

export const CanaryTestingPage: React.FC<CanaryTestingPageProps> = ({ store }) => {
  const { canaries, runs, loading, error, refreshAll } = store;

  const [divisionFilter, setDivisionFilter] = useState<string>('all');
  const [tierFilter, setTierFilter] = useState<string>('all');
  const [checkTypeFilter, setCheckTypeFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState<string>('');

  const [isExecuting, setIsExecuting] = useState<boolean>(false);
  const [execMode, setExecMode] = useState<'run' | 'baseline'>('run');
  const [execError, setExecError] = useState<string | null>(null);
  const [latestObservations, setLatestObservations] = useState<Observation[] | null>(null);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);

  const tasks = canaries?.tasks || [];
  const checkDescriptions = canaries?.check_types || {};

  const activeObservations = useMemo(() => {
    if (latestObservations) return latestObservations;
    const latestWithObs = runs?.runs?.find((r) => r.observations && r.observations.length > 0);
    return latestWithObs?.observations || [];
  }, [latestObservations, runs]);

  const obsByCanaryId = useMemo(() => {
    const map = new Map<string, Observation>();
    for (const obs of activeObservations) {
      map.set(obs.canary_id, obs);
    }
    return map;
  }, [activeObservations]);

  const availableCheckTypes = useMemo(() => {
    const set = new Set<string>();
    tasks.forEach((t) => t.checks.forEach((c) => set.add(c.type)));
    return Array.from(set).sort();
  }, [tasks]);

  const filteredTasks = useMemo(() => {
    return tasks.filter((t) => {
      if (divisionFilter !== 'all' && t.division !== divisionFilter) return false;
      if (tierFilter !== 'all' && t.tier !== tierFilter) return false;
      if (checkTypeFilter !== 'all' && !t.checks.some((c) => c.type === checkTypeFilter)) {
        return false;
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        return (
          t.id.toLowerCase().includes(q) ||
          t.prompt.toLowerCase().includes(q) ||
          t.division.toLowerCase().includes(q)
        );
      }
      return true;
    });
  }, [tasks, divisionFilter, tierFilter, checkTypeFilter, searchQuery]);

  const handleRunTests = async (mode: 'run' | 'baseline') => {
    if (isExecuting) return;
    setIsExecuting(true);
    setExecMode(mode);
    setExecError(null);
    try {
      const tiers =
        tierFilter !== 'all'
          ? [tierFilter as 'easy' | 'general' | 'expensive']
          : undefined;

      const result = await monitoringApi.executeRun({
        mode,
        tiers,
        temperature: 0,
        seed: 42,
        max_tokens: 512,
      });
      setLatestObservations(result.observations);
      await refreshAll();
    } catch (err) {
      setExecError(err instanceof Error ? err.message : 'Canary execution failed');
    } finally {
      setIsExecuting(false);
    }
  };

  if (loading && !canaries) {
    return <LoadingView label="Loading synthetic canary library..." />;
  }

  if (error && !canaries) {
    return <ErrorView message={error} onRetry={refreshAll} />;
  }

  const selectedTask: CanaryTask | undefined = tasks.find((t) => t.id === selectedTaskId);
  const selectedObs: Observation | undefined = selectedTaskId
    ? obsByCanaryId.get(selectedTaskId)
    : undefined;

  return (
    <div className="space-y-6">
      <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-slate-100">
            Synthetic Canary Execution Pipeline
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Executes {tierFilter === 'all' ? 'all 81' : filteredTasks.length} synthetic probes via{' '}
            <code className="text-sky-300">support-bot-v1</code> with fixed parameters (temp=0, seed=42, max_tokens=512)
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <button
            onClick={() => handleRunTests('baseline')}
            disabled={isExecuting}
            className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 disabled:opacity-50 border border-slate-700 text-xs font-medium text-slate-200 transition-colors"
          >
            {isExecuting && execMode === 'baseline' ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin text-sky-400" />
            ) : (
              <Database className="w-3.5 h-3.5 text-sky-400" />
            )}
            Record Baseline Run
          </button>

          <button
            onClick={() => handleRunTests('run')}
            disabled={isExecuting}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-xs font-semibold text-white transition-colors"
          >
            {isExecuting && execMode === 'run' ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : (
              <Play className="w-3.5 h-3.5" />
            )}
            {isExecuting ? 'Executing Canaries...' : 'Run Tests'}
          </button>
        </div>
      </section>

      {isExecuting && (
        <div className="p-4 rounded-xl bg-sky-950/30 border border-sky-800/60 flex items-center gap-3 text-xs text-sky-200">
          <Loader2 className="w-4 h-4 animate-spin text-sky-400 shrink-0" />
          <span>
            Running live synthetic probes against the Go backend ({execMode} mode). This waits for actual model completions—please do not reload.
          </span>
        </div>
      )}

      {execError && <ErrorView message={execError} />}

      <section className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        <div className="relative">
          <Search className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search ID or prompt..."
            className="w-full pl-9 pr-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-xs text-slate-200 placeholder:text-slate-500 focus:outline-none focus:border-sky-500"
          />
        </div>

        <select
          value={divisionFilter}
          onChange={(e) => setDivisionFilter(e.target.value)}
          className="px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-xs text-slate-200 focus:outline-none focus:border-sky-500"
        >
          <option value="all">All Divisions (9)</option>
          {(canaries?.divisions || []).map((d) => (
            <option key={d.id} value={d.id}>
              Division: {d.id}
            </option>
          ))}
        </select>

        <select
          value={tierFilter}
          onChange={(e) => setTierFilter(e.target.value)}
          className="px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-xs text-slate-200 focus:outline-none focus:border-sky-500"
        >
          <option value="all">All Tiers (easy / general / expensive)</option>
          <option value="easy">Tier: easy (27)</option>
          <option value="general">Tier: general (27)</option>
          <option value="expensive">Tier: expensive (27)</option>
        </select>

        <select
          value={checkTypeFilter}
          onChange={(e) => setCheckTypeFilter(e.target.value)}
          className="px-3 py-2 rounded-lg bg-slate-900 border border-slate-800 text-xs text-slate-200 focus:outline-none focus:border-sky-500"
        >
          <option value="all">All Check Categories ({availableCheckTypes.length})</option>
          {availableCheckTypes.map((ct) => (
            <option key={ct} value={ct}>
              {ct}
            </option>
          ))}
        </select>
      </section>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 bg-slate-900/80 border border-slate-800 rounded-xl overflow-hidden">
          {filteredTasks.length === 0 ? (
            <EmptyView
              title="No Canaries Match Your Filter"
              description="Reset division, tier, or check category filters to inspect the 81 synthetic probes."
            />
          ) : (
            <div className="overflow-x-auto max-h-[640px]">
              <table className="w-full text-left border-collapse text-xs">
                <thead className="bg-slate-950 text-slate-400 border-b border-slate-800 sticky top-0">
                  <tr>
                    <th className="py-3 px-4 font-medium">Canary ID</th>
                    <th className="py-3 px-4 font-medium">Division / Tier</th>
                    <th className="py-3 px-4 font-medium">Check Categories</th>
                    <th className="py-3 px-4 font-medium">Last Result</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/80">
                  {filteredTasks.map((task) => {
                    const obs = obsByCanaryId.get(task.id);
                    const hasError = Boolean(obs?.error);
                    const evalChecks = obs?.checks.filter((c) => !c.unavailable) || [];
                    const passedAll =
                      obs && !hasError && evalChecks.every((c) => c.passed);
                    const isSelected = selectedTaskId === task.id;

                    return (
                      <tr
                        key={task.id}
                        onClick={() => setSelectedTaskId(task.id)}
                        className={`cursor-pointer transition-colors ${
                          isSelected ? 'bg-sky-950/35' : 'hover:bg-slate-800/40'
                        }`}
                      >
                        <td className="py-3 px-4 font-mono font-semibold text-sky-300 whitespace-nowrap">
                          {task.id}
                        </td>
                        <td className="py-3 px-4">
                          <div className="text-slate-200 font-medium capitalize">{task.division}</div>
                          <div className="text-[11px] text-slate-400 font-mono">{task.tier}</div>
                        </td>
                        <td className="py-3 px-4">
                          <div className="flex flex-wrap gap-1">
                            {task.checks.map((c, idx) => (
                              <span
                                key={`${task.id}-${c.type}-${idx}`}
                                className="px-2 py-0.5 rounded bg-slate-800 text-slate-300 font-mono text-[11px]"
                              >
                                {c.type}
                              </span>
                            ))}
                          </div>
                        </td>
                        <td className="py-3 px-4 whitespace-nowrap">
                          {!obs ? (
                            <span className="text-slate-500">Not run</span>
                          ) : hasError ? (
                            <StatusBadge status="FAIL" />
                          ) : passedAll ? (
                            <StatusBadge status="PASS" />
                          ) : (
                            <StatusBadge status="FAIL" />
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-4">
          {!selectedTask ? (
            <div className="text-xs text-slate-400 py-10 text-center">
              Select any canary row to inspect its synthetic prompt, check assertions, expected value, and actual model output.
            </div>
          ) : (
            <>
              <div className="flex items-center justify-between border-b border-slate-800 pb-3">
                <div>
                  <span className="font-mono text-sm font-semibold text-sky-300">
                    {selectedTask.id}
                  </span>
                  <span className="ml-2 text-xs text-slate-400 capitalize">
                    ({selectedTask.division} · {selectedTask.tier})
                  </span>
                </div>
                {selectedObs && (
                  <span className="text-xs font-mono text-slate-400">
                    {selectedObs.latency_ms} ms
                  </span>
                )}
              </div>

              {selectedTask.system && (
                <div>
                  <div className="text-[11px] uppercase tracking-wider text-slate-400 mb-1">
                    System Instruction
                  </div>
                  <pre className="p-2.5 rounded-lg bg-slate-950 border border-slate-800 text-xs text-slate-300 whitespace-pre-wrap font-mono">
                    {selectedTask.system}
                  </pre>
                </div>
              )}

              <div>
                <div className="text-[11px] uppercase tracking-wider text-slate-400 mb-1">
                  Synthetic Prompt
                </div>
                <pre className="p-2.5 rounded-lg bg-slate-950 border border-slate-800 text-xs text-slate-200 whitespace-pre-wrap font-mono">
                  {selectedTask.prompt}
                </pre>
              </div>

              <div>
                <div className="text-[11px] uppercase tracking-wider text-slate-400 mb-1.5">
                  Expected Assertions & Check Results
                </div>
                <div className="space-y-2">
                  {selectedTask.checks.map((chk, idx) => {
                    const obsChk = selectedObs?.checks[idx];
                    return (
                      <div
                        key={idx}
                        className="p-2.5 rounded-lg bg-slate-950/90 border border-slate-800 text-xs space-y-1"
                      >
                        <div className="flex items-center justify-between">
                          <span className="font-mono font-semibold text-slate-200">{chk.type}</span>
                          {obsChk ? (
                            obsChk.unavailable ? (
                              <StatusBadge status="UNAVAILABLE" />
                            ) : obsChk.passed ? (
                              <span className="inline-flex items-center gap-1 text-emerald-400 font-medium">
                                <CheckCircle2 className="w-3.5 h-3.5" /> Passed
                              </span>
                            ) : (
                              <span className="inline-flex items-center gap-1 text-rose-400 font-medium">
                                <XCircle className="w-3.5 h-3.5" /> Failed
                              </span>
                            )
                          ) : null}
                        </div>
                        {checkDescriptions[chk.type] && (
                          <p className="text-[11px] text-slate-400">{checkDescriptions[chk.type]}</p>
                        )}
                        {obsChk?.expected && (
                          <div className="text-[11px] font-mono text-sky-300">
                            Expected: {obsChk.expected}
                          </div>
                        )}
                        {obsChk?.detail && (
                          <div className="text-[11px] font-mono text-slate-400">
                            Detail: {obsChk.detail}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>

              {selectedObs && (
                <div>
                  <div className="text-[11px] uppercase tracking-wider text-slate-400 mb-1">
                    Actual Model Response
                  </div>
                  {selectedObs.error ? (
                    <div className="p-2.5 rounded-lg bg-rose-950/40 border border-rose-800 text-xs text-rose-200 font-mono">
                      Error: {selectedObs.error}
                    </div>
                  ) : (
                    <pre className="p-2.5 rounded-lg bg-slate-950 border border-slate-800 text-xs text-slate-200 whitespace-pre-wrap font-mono max-h-48 overflow-y-auto">
                      {selectedObs.response || '(empty response)'}
                    </pre>
                  )}
                  <div className="flex items-center justify-between text-[11px] text-slate-400 font-mono mt-1.5">
                    <span>finish_reason: {selectedObs.finish_reason || 'unknown'}</span>
                    <span>
                      tokens: {selectedObs.input_tokens} in / {selectedObs.output_tokens} out
                    </span>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>

      <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5">
        <h3 className="text-sm font-semibold text-slate-100 mb-3">Execution History</h3>
        {!runs?.runs || runs.runs.length === 0 ? (
          <p className="text-xs text-slate-400">No previous runs persisted in data directory.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs border-collapse">
              <thead className="text-slate-400 border-b border-slate-800">
                <tr>
                  <th className="py-2 px-3">Artifact File</th>
                  <th className="py-2 px-3">Mode</th>
                  <th className="py-2 px-3">Timestamp</th>
                  <th className="py-2 px-3">Passed / Total</th>
                  <th className="py-2 px-3">Pass Rate</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/70">
                {runs.runs.map((r) => (
                  <tr key={r.file_name}>
                    <td className="py-2.5 px-3 font-mono text-slate-200">{r.file_name}</td>
                    <td className="py-2.5 px-3 uppercase text-sky-400 font-medium">{r.kind}</td>
                    <td className="py-2.5 px-3 text-slate-400">{formatTimestamp(r.timestamp)}</td>
                    <td className="py-2.5 px-3 font-mono">
                      {r.passed_tasks}/{r.total_tasks}
                    </td>
                    <td className="py-2.5 px-3 font-mono font-semibold text-slate-100">
                      {r.pass_rate.toFixed(1)}%
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
};
