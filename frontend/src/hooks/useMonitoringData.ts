import { useCallback, useEffect, useState } from 'react';
import { monitoringApi } from '../services/api';
import type {
  AttributionResponse,
  BaselineResponse,
  CanariesResponse,
  DetectionResponse,
  HealthResponse,
  RunsResponse,
} from '../types/monitoring';

export interface MonitoringStore {
  health: HealthResponse | null;
  canaries: CanariesResponse | null;
  runs: RunsResponse | null;
  baseline: BaselineResponse | null;
  detection: DetectionResponse | null;
  attribution: AttributionResponse | null;
  loading: boolean;
  error: string | null;
  refreshAll: () => Promise<void>;
}

export function useMonitoringData(): MonitoringStore {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [canaries, setCanaries] = useState<CanariesResponse | null>(null);
  const [runs, setRuns] = useState<RunsResponse | null>(null);
  const [baseline, setBaseline] = useState<BaselineResponse | null>(null);
  const [detection, setDetection] = useState<DetectionResponse | null>(null);
  const [attribution, setAttribution] = useState<AttributionResponse | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  const refreshAll = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [h, c, r, b, d, a] = await Promise.all([
        monitoringApi.getHealth(),
        monitoringApi.getCanaries(),
        monitoringApi.getRuns(),
        monitoringApi.getBaseline(),
        monitoringApi.getDetection(),
        monitoringApi.getAttribution(),
      ]);
      setHealth(h);
      setCanaries(c);
      setRuns(r);
      setBaseline(b);
      setDetection(d);
      setAttribution(a);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect to ModelWatch backend');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refreshAll();
  }, [refreshAll]);

  return {
    health,
    canaries,
    runs,
    baseline,
    detection,
    attribution,
    loading,
    error,
    refreshAll,
  };
}
