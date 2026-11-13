import React, { useState } from 'react';
import { useMonitoringData } from './hooks/useMonitoringData';
import type { NavigationTab } from './types/monitoring';
import { Layout } from './components/Layout';
import { DashboardPage } from './pages/DashboardPage';
import { CanaryTestingPage } from './pages/CanaryTestingPage';
import { ChangeDetectionPage } from './pages/ChangeDetectionPage';
import { AttributionPage } from './pages/AttributionPage';
import { EvaluationsPage } from './pages/EvaluationsPage';
import { HistoryPage } from './pages/HistoryPage';
import { SettingsPage } from './pages/SettingsPage';

const HEADERS: Record<NavigationTab, { title: string; subtitle: string }> = {
  overview: { title: 'Overview', subtitle: 'A concise view of behavioural health across your monitored model.' },
  evaluations: { title: 'Evaluations', subtitle: 'Evaluation runs are intentionally shown only when the backend exposes them.' },
  comparison: { title: 'Model comparison', subtitle: 'Compare baseline and candidate observations at metric and check level.' },
  drift: { title: 'Drift analysis', subtitle: 'Understand what changed, how strong the evidence is, and what it may mean.' },
  'canary-testing': { title: 'Canary tests', subtitle: 'Inspect and execute the pinned synthetic canary suite.' },
  history: { title: 'Experiment history', subtitle: 'Review prior baseline and candidate runs recorded by the monitor.' },
  settings: { title: 'Settings', subtitle: 'Inspect connected workspace configuration and API capabilities.' },
};

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<NavigationTab>('overview'); const store = useMonitoringData(); const connected = Boolean(store.health?.status === 'connected' && !store.error); const header = HEADERS[activeTab];
  return <Layout activeTab={activeTab} onSelectTab={setActiveTab} title={header.title} subtitle={header.subtitle} health={store.health} connected={connected} loading={store.loading} onRefresh={store.refreshAll}>
    {activeTab === 'overview' && <DashboardPage store={store} onNavigate={setActiveTab} />}
    {activeTab === 'evaluations' && <EvaluationsPage store={store} />}
    {activeTab === 'comparison' && <AttributionPage store={store} />}
    {activeTab === 'drift' && <ChangeDetectionPage store={store} />}
    {activeTab === 'canary-testing' && <CanaryTestingPage store={store} />}
    {activeTab === 'history' && <HistoryPage store={store} />}
    {activeTab === 'settings' && <SettingsPage store={store} />}
  </Layout>;
};
export default App;
