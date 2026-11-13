import React from 'react';
import type { MonitoringStore } from '../hooks/useMonitoringData';
import { EmptyView } from '../components/StateViews';
export const EvaluationsPage: React.FC<{ store: MonitoringStore }> = ({ store: _store }) => <EmptyView title="Evaluation API not registered" description="The repository contains evaluation handlers, but the current Go serve route does not register them. This surface stays unavailable until a GET /api/evaluations endpoint is exposed." />;
