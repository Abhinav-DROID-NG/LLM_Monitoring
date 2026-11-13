import React, { useState } from 'react';
import { useMonitoringData } from './hooks/useMonitoringData';
import type { NavigationTab } from './types/monitoring';
import { Layout } from './components/Layout';
import { DashboardPage } from './pages/DashboardPage';
import { CanaryTestingPage } from './pages/CanaryTestingPage';
import { ChangeDetectionPage } from './pages/ChangeDetectionPage';
import { AttributionPage } from './pages/AttributionPage';

const PAGE_HEADERS: Record<NavigationTab, { title: string; subtitle: string }> = {
  dashboard: {
    title: 'Monitoring Overview',
    subtitle: 'Real-time health, canary pass rates, and baseline comparison for support-bot-v1',
  },
  'canary-testing': {
    title: 'Synthetic Canary Testing',
    subtitle: 'Inspect and execute the 51-item precision performance benchmark',
  },
  'change-detection': {
    title: 'Behaviour Change Detection',
    subtitle: 'Benjamini-Hochberg FDR statistical drift detection against historical baselines',
  },
  attribution: {
    title: 'Four-Layer Attribution',
    subtitle: 'Investigate Model/Alias, Serving Stack, Safety, and Request Configuration evidence',
  },
};

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<NavigationTab>('dashboard');
  const store = useMonitoringData();
  const connected = Boolean(store.health && store.health.status === 'connected' && !store.error);

  return (
    <Layout
      activeTab={activeTab}
      onSelectTab={setActiveTab}
      title={PAGE_HEADERS[activeTab].title}
      subtitle={PAGE_HEADERS[activeTab].subtitle}
      health={store.health}
      connected={connected}
      loading={store.loading}
      onRefresh={store.refreshAll}
    >
      {activeTab === 'dashboard' && <DashboardPage store={store} onNavigate={setActiveTab} />}
      {activeTab === 'canary-testing' && <CanaryTestingPage store={store} />}
      {activeTab === 'change-detection' && <ChangeDetectionPage store={store} />}
      {activeTab === 'attribution' && <AttributionPage store={store} />}
    </Layout>
  );
};

export default App;
