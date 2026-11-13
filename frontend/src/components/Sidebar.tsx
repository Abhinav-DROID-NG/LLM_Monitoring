import React from 'react';
import { Activity, FlaskConical, GitCompareArrows, History, LayoutDashboard, Settings2, ShieldCheck, Waves } from 'lucide-react';
import type { HealthResponse, NavigationTab } from '../types/monitoring';

interface SidebarProps { activeTab: NavigationTab; onSelectTab: (tab: NavigationTab) => void; health: HealthResponse | null; connected: boolean; }
const NAV_ITEMS: { id: NavigationTab; label: string; icon: React.ReactNode; group?: string }[] = [
  { id: 'overview', label: 'Overview', icon: <LayoutDashboard /> }, { id: 'evaluations', label: 'Evaluations', icon: <Activity /> },
  { id: 'comparison', label: 'Model comparison', icon: <GitCompareArrows /> }, { id: 'drift', label: 'Drift analysis', icon: <Waves /> },
  { id: 'canary-testing', label: 'Canary tests', icon: <FlaskConical />, group: 'OPERATE' }, { id: 'history', label: 'Experiment history', icon: <History /> },
  { id: 'settings', label: 'Settings', icon: <Settings2 />, group: 'WORKSPACE' },
];
export const Sidebar: React.FC<SidebarProps> = ({ activeTab, onSelectTab, health, connected }) => (
  <aside className="sidebar"><div><div className="brand"><div className="brand-mark"><ShieldCheck /></div><div><div className="brand-name">ModelWatch</div><div className="brand-caption">AI behaviour observability</div></div></div>
    <nav className="nav-list" aria-label="Primary navigation">{NAV_ITEMS.map((item, index) => <React.Fragment key={item.id}>{item.group && index > 0 && <div className="nav-group-label">{item.group}</div>}<button className={`nav-item ${activeTab === item.id ? 'active' : ''}`} onClick={() => onSelectTab(item.id)}>{React.cloneElement(item.icon as React.ReactElement, { size: 16, strokeWidth: 1.8 })}<span>{item.label}</span></button></React.Fragment>)}</nav>
  </div><div className="sidebar-footer"><div className="connection-row"><span className={`connection-dot ${connected ? 'online' : ''}`} /><span>{connected ? 'Backend connected' : 'Backend offline'}</span></div><div className="workspace-card"><div className="eyebrow">MONITORED ALIAS</div><div className="mono truncate">{health?.alias || 'support-bot-v1'}</div><div className="workspace-meta">{health?.provider || 'ollama'} · {health?.mock_mode ? 'mock mode' : 'live provider'}</div></div><div className="version-row"><span>ModelWatch</span><span>v1.0.0</span></div></div></aside>
);
