import React from 'react';
import type { DetectionStatusLabel } from '../types/monitoring';

interface StatusBadgeProps {
  status: DetectionStatusLabel | 'PASS' | 'FAIL' | 'UNAVAILABLE' | string;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status }) => {
  const styles: Record<string, string> = {
    'Normal variation': 'badge-neutral',
    'Potential deviation': 'badge-warning',
    'Confirmed threshold breach': 'badge-danger',
    'Recovered behaviour': 'badge-good',
    PASS: 'badge-good',
    FAIL: 'badge-danger',
    UNAVAILABLE: 'badge-neutral',
    high: 'badge-danger',
    medium: 'badge-warning',
    inconclusive: 'badge-neutral',
    stable: 'badge-neutral',
    improvement: 'badge-good',
    regression: 'badge-danger',
  };

  const cls = styles[status] || 'bg-slate-800 text-slate-300 border-slate-700';

  return (
    <span className={`status-badge ${cls}`}>
      {status}
    </span>
  );
};
