import React from 'react';
import { AlertTriangle, Inbox, Loader2, RefreshCw } from 'lucide-react';

export const LoadingView: React.FC<{ label?: string }> = ({ label = 'Loading monitoring telemetry...' }) => (
  <div className="state-view">
    <Loader2 className="state-icon spin" />
    <p>{label}</p>
  </div>
);

export const ErrorView: React.FC<{ message: string; onRetry?: () => void }> = ({ message, onRetry }) => (
  <div className="state-view state-error">
    <div className="state-title">
      <AlertTriangle className="state-icon" />
      <span>Backend Integration Error</span>
    </div>
    <p>{message}</p>
    {onRetry && (
      <button
        onClick={onRetry}
      className="button button-secondary"
      >
        <RefreshCw className="w-3.5 h-3.5" />
        Retry Connection
      </button>
    )}
  </div>
);

export const EmptyView: React.FC<{ title: string; description: string; action?: React.ReactNode }> = ({
  title,
  description,
  action,
}) => (
  <div className="state-view">
    <Inbox className="state-icon" />
    <h3>{title}</h3>
    <p>{description}</p>
    {action}
  </div>
);
