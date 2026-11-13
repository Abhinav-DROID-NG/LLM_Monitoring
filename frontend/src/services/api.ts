import type {
  AttributionResponse,
  BaselineResponse,
  CanariesResponse,
  DetectionResponse,
  ExecuteRunRequest,
  ExecuteRunResponse,
  HealthResponse,
  RunsResponse,
} from '../types/monitoring';

const API_BASE = (import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '');

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${API_BASE}${path}`;
  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(init?.headers || {}),
      },
    });
  } catch {
    throw new ApiError(
      `Backend unreachable at ${API_BASE || 'http://localhost:8080'}. Ensure 'go run ./cmd/model-monitor serve' is running.`,
      0
    );
  }

  const text = await response.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = { error: text };
    }
  }

  if (!response.ok) {
    const errMsg =
      typeof parsed === 'object' && parsed !== null && 'error' in parsed
        ? String((parsed as { error: unknown }).error)
        : `HTTP ${response.status} ${response.statusText}`;
    throw new ApiError(errMsg, response.status);
  }

  return parsed as T;
}

export const monitoringApi = {
  getHealth: () => requestJSON<HealthResponse>('/api/health'),
  getCanaries: () => requestJSON<CanariesResponse>('/api/canaries'),
  getRuns: () => requestJSON<RunsResponse>('/api/runs'),
  getBaseline: () => requestJSON<BaselineResponse>('/api/baseline'),
  getDetection: () => requestJSON<DetectionResponse>('/api/detection'),
  getAttribution: () => requestJSON<AttributionResponse>('/api/attribution'),
  executeRun: (payload: ExecuteRunRequest) =>
    requestJSON<ExecuteRunResponse>('/api/run', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
};
