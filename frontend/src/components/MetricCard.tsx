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
    neutral: 'metric-neutral',
    good: 'metric-good',
    warn: 'metric-warn',
    danger: 'metric-danger',
  }[accent];

  return (
    <div className={`metric-card ${borderClass}`}>
      <div className="metric-label">
        <span>{label}</span>
        {icon && <span>{icon}</span>}
      </div>
      <div className="metric-value">{value}</div>
      {subtext && <p className="metric-subtext">{subtext}</p>}
    </div>
  );
};
