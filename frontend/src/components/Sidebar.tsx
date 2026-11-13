import React from 'react';
import {
  Activity,
  FlaskConical,
  GitCompareArrows,
  Layers,
  ShieldCheck,
} from 'lucide-react';
import type { HealthResponse, NavigationTab } from '../types/monitoring';

interface SidebarProps {
  activeTab: NavigationTab;
  onSelectTab: (tab: NavigationTab) => void;
  health: HealthResponse | null;
  connected: boolean;
}

const NAV_ITEMS: { id: NavigationTab; label: string; icon: React.ReactNode }[] = [
  { id: 'dashboard', label: 'Dashboard', icon: <Activity className="w-4 h-4" /> },
  { id: 'canary-testing', label: 'Canary Testing', icon: <FlaskConical className="w-4 h-4" /> },
  { id: 'change-detection', label: 'Change Detection', icon: <GitCompareArrows className="w-4 h-4" /> },
  { id: 'attribution', label: 'Attribution', icon: <Layers className="w-4 h-4" /> },
];

export const Sidebar: React.FC<SidebarProps> = ({ activeTab, onSelectTab, health, connected }) => {
  return (
    <aside className="w-64 shrink-0 bg-slate-900 border-r border-slate-800 flex flex-col justify-between min-h-screen">
      <div>
        <div className="px-5 py-5 border-b border-slate-800 flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-sky-500/15 border border-sky-500/40 flex items-center justify-center text-sky-400">
            <ShieldCheck className="w-5 h-5" />
          </div>
          <div>
            <div className="text-sm font-semibold text-slate-100 tracking-tight">ModelWatch</div>
            <div className="text-[11px] text-slate-400">LLM Behavioural Monitor</div>
          </div>
        </div>

        <nav className="p-3 space-y-1" aria-label="Primary Navigation">
          {NAV_ITEMS.map((item) => {
            const active = activeTab === item.id;
            return (
              <button
                key={item.id}
                onClick={() => onSelectTab(item.id)}
                className={`w-full flex items-center gap-3 px-3.5 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                  active
                    ? 'bg-sky-500/15 text-sky-300 border border-sky-500/30'
                    : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/70 border border-transparent'
                }`}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
      </div>

      <div className="p-4 border-t border-slate-800 space-y-2.5 text-xs">
        <div className="flex items-center justify-between">
          <span className="text-slate-400">Go Backend</span>
          <span className="inline-flex items-center gap-1.5 font-medium text-slate-200">
            <span
              className={`w-2 h-2 rounded-full ${
                connected ? 'bg-emerald-400' : 'bg-rose-500'
              }`}
            />
            {connected ? 'Connected' : 'Offline'}
          </span>
        </div>
        <div className="bg-slate-950/80 border border-slate-800 rounded-lg p-2.5 space-y-1">
          <div className="text-[11px] text-slate-400">Monitored Alias</div>
          <div className="font-mono text-xs text-sky-300 truncate">
            {health?.alias || 'support-bot-v1'}
          </div>
          <div className="text-[11px] text-slate-400 pt-1">Target Model</div>
          <div className="font-mono text-xs text-slate-200 truncate">
            {health?.model_identifier || 'qwen2.5:3b-instruct-fp16'}
          </div>
        </div>
      </div>
    </aside>
  );
};
