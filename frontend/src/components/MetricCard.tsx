import React from 'react';

interface MetricCardProps {
  label: string;
  value: string | number;
  subtext?: string;
  accent?: 'neutral' | 'good' | 'warn' | 'danger';
  icon?: React.ReactNode;
}

export const MetricCard: React.FC<MetricCardProps> = ({
  label,
  value,
  subtext,
  accent = 'neutral',
  icon,
}) => {
  const borderClass = {
    neutral: 'border-slate-800',
    good: 'border-emerald-800/60',
    warn: 'border-amber-800/60',
    danger: 'border-rose-800/70',
  }[accent];

  return (
    <div className={`bg-slate-900/80 border ${borderClass} rounded-xl p-4 shadow-sm`}>
      <div className="flex items-center justify-between text-xs text-slate-400 mb-1.5">
        <span className="font-medium tracking-wide uppercase">{label}</span>
        {icon && <span className="text-slate-400">{icon}</span>}
      </div>
      <div className="text-2xl font-semibold text-slate-100 tabular-nums">{value}</div>
      {subtext && <p className="text-xs text-slate-400 mt-1">{subtext}</p>}
    </div>
  );
};
