import React from 'react';
import { AlertTriangle, Inbox, Loader2, RefreshCw } from 'lucide-react';

export const LoadingView: React.FC<{ label?: string }> = ({ label = 'Loading monitoring telemetry...' }) => (
  <div className="flex flex-col items-center justify-center py-16 px-4 bg-slate-900/60 border border-slate-800 rounded-xl">
    <Loader2 className="w-7 h-7 text-sky-400 animate-spin mb-3" />
    <p className="text-sm text-slate-300 font-medium">{label}</p>
  </div>
);

export const ErrorView: React.FC<{ message: string; onRetry?: () => void }> = ({ message, onRetry }) => (
  <div className="flex flex-col items-start gap-3 p-5 bg-rose-950/30 border border-rose-800/60 rounded-xl">
    <div className="flex items-center gap-2 text-rose-300 font-semibold text-sm">
      <AlertTriangle className="w-4 h-4 shrink-0" />
      <span>Backend Integration Error</span>
    </div>
    <p className="text-sm text-slate-300 leading-relaxed">{message}</p>
    {onRetry && (
      <button
        onClick={onRetry}
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-100 border border-slate-700 transition-colors"
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
  <div className="flex flex-col items-center justify-center text-center py-14 px-6 bg-slate-900/50 border border-slate-800 rounded-xl">
    <Inbox className="w-8 h-8 text-slate-500 mb-3" />
    <h3 className="text-sm font-semibold text-slate-200">{title}</h3>
    <p className="text-xs text-slate-400 max-w-md mt-1 mb-4 leading-relaxed">{description}</p>
    {action}
  </div>
);
