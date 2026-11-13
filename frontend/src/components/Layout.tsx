import React from 'react';
import { RefreshCw, SlidersHorizontal } from 'lucide-react';
import type { HealthResponse, NavigationTab } from '../types/monitoring';
import { Sidebar } from './Sidebar';

interface LayoutProps {
  activeTab: NavigationTab;
  onSelectTab: (tab: NavigationTab) => void;
  title: string;
  subtitle: string;
  health: HealthResponse | null;
  connected: boolean;
  loading: boolean;
  onRefresh: () => void;
  children: React.ReactNode;
}

export const Layout: React.FC<LayoutProps> = ({
  activeTab,
  onSelectTab,
  title,
  subtitle,
  health,
  connected,
  loading,
  onRefresh,
  children,
}) => {
  return (
    <div className="min-h-screen flex bg-slate-950 text-slate-100">
      <Sidebar
        activeTab={activeTab}
        onSelectTab={onSelectTab}
        health={health}
        connected={connected}
      />

      <div className="flex-1 flex flex-col min-w-0">
        <header className="bg-slate-900/70 border-b border-slate-800 px-6 py-4 flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 className="text-lg font-semibold text-slate-100 tracking-tight">{title}</h1>
            <p className="text-xs text-slate-400 mt-0.5">{subtitle}</p>
          </div>

          <div className="flex items-center gap-3">
            <div
              className="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-800 text-xs text-slate-300 font-mono"
              title="Fixed Qwen2.5-3B-Instruct Ollama experiment parameters"
            >
              <SlidersHorizontal className="w-3.5 h-3.5 text-sky-400" />
              <span>temp=0</span>
              <span className="text-slate-600">|</span>
              <span>seed=42</span>
              <span className="text-slate-600">|</span>
              <span>max_tokens=512</span>
            </div>

            <button
              onClick={onRefresh}
              disabled={loading}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 disabled:opacity-50 border border-slate-700 text-xs font-medium text-slate-200 transition-colors"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
        </header>

        <main className="flex-1 p-6 max-w-7xl w-full mx-auto space-y-6">{children}</main>
      </div>
    </div>
  );
};
