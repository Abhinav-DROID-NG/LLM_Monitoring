import React from 'react';
import type { DetectionStatusLabel } from '../types/monitoring';

interface StatusBadgeProps {
  status: DetectionStatusLabel | 'PASS' | 'FAIL' | 'UNAVAILABLE' | string;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status }) => {
  const styles: Record<string, string> = {
    'Normal variation': 'bg-slate-800 text-slate-300 border-slate-700',
    'Potential deviation': 'bg-amber-950/70 text-amber-300 border-amber-700/60',
    'Confirmed threshold breach': 'bg-rose-950/80 text-rose-300 border-rose-700/70',
    'Recovered behaviour': 'bg-sky-950/70 text-sky-300 border-sky-700/60',
    PASS: 'bg-emerald-950/70 text-emerald-300 border-emerald-700/60',
    FAIL: 'bg-rose-950/80 text-rose-300 border-rose-700/70',
    UNAVAILABLE: 'bg-slate-800 text-slate-400 border-slate-700',
    high: 'bg-rose-950/80 text-rose-300 border-rose-700/70',
    medium: 'bg-amber-950/70 text-amber-300 border-amber-700/60',
    inconclusive: 'bg-slate-800 text-slate-300 border-slate-700',
  };

  const cls = styles[status] || 'bg-slate-800 text-slate-300 border-slate-700';

  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium border ${cls}`}>
      {status}
    </span>
  );
};
