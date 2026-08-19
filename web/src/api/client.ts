import axios, { AxiosError } from 'axios';
import type {
  Agency,
  AuditEntry,
  BacklogStats,
  ConsistencyCheck,
  CreateSubmissionRequest,
  CreateSubmissionResponse,
  DispatchResult,
  DispatchTask,
  ErrorResponse,
  GatewayStatus,
  PaginatedResponse,
  Rule,
  RuleTrial,
  Standard,
  Submission,
  Summary,
} from '../types';

/**
 * Single axios instance for all backend calls. The base URL is relative
 * (`/api/v1`) so that in production the Go server serves the frontend and
 * the API at the same origin. In development, Vite proxies `/api` to the
 * Go backend on port 52661 (see vite.config.ts).
 */
export const api = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
});

/** Extract a human-readable message from any axios error. */
export function getApiErrorMessage(err: unknown, fallback = 'Request failed'): string {
  if (axios.isAxiosError(err)) {
    const axErr = err as AxiosError<ErrorResponse>;
    if (axErr.response?.data?.error?.message) {
      return axErr.response.data.error.message;
    }
    if (axErr.response) {
      return `${fallback} (HTTP ${axErr.response.status})`;
    }
    if (axErr.request) {
      return 'Network error: no response from server';
    }
    return axErr.message || fallback;
  }
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

/** Extract the backend error code (e.g. VALIDATION_ERROR) if present. */
export function getApiErrorCode(err: unknown): string | undefined {
  if (axios.isAxiosError(err)) {
    const axErr = err as AxiosError<ErrorResponse>;
    return axErr.response?.data?.error?.code;
  }
  return undefined;
}

// ---------------------------------------------------------------------------
// Submissions
// ---------------------------------------------------------------------------

export interface ListParams {
  page?: number;
  page_size?: number;
  status?: string;
  enterprise?: string;
  agency?: string;
  entity_type?: string;
  entity_id?: string;
  actor?: string;
}

export const submissionsApi = {
  list(params: ListParams = {}): Promise<PaginatedResponse<Submission>> {
    return api.get<PaginatedResponse<Submission>>('/submissions', { params }).then((r) => r.data);
  },
  get(id: string): Promise<{ submission: Submission }> {
    return api.get<{ submission: Submission }>(`/submissions/${id}`).then((r) => r.data);
  },
  create(body: CreateSubmissionRequest): Promise<CreateSubmissionResponse> {
    return api.post<CreateSubmissionResponse>('/submissions', body).then((r) => r.data);
  },
  cancel(id: string, cancelledBy = 'operator'): Promise<{ submission: Submission }> {
    return api
      .delete<{ submission: Submission }>(`/submissions/${id}/cancel`, {
        data: { cancelled_by: cancelledBy },
      })
      .then((r) => r.data);
  },
  dispatch(id: string): Promise<DispatchResult> {
    return api.post<DispatchResult>(`/submissions/${id}/dispatch`).then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Dispatch tasks
// ---------------------------------------------------------------------------

export const dispatchesApi = {
  list(params: ListParams = {}): Promise<PaginatedResponse<DispatchTask>> {
    return api.get<PaginatedResponse<DispatchTask>>('/dispatches', { params }).then((r) => r.data);
  },
  get(id: string): Promise<{ task: DispatchTask }> {
    return api.get<{ task: DispatchTask }>(`/dispatches/${id}`).then((r) => r.data);
  },
  retry(id: string): Promise<{ message: string; task_id: string }> {
    return api.post<{ message: string; task_id: string }>(`/dispatches/${id}/retry`).then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Summaries & consistency
// ---------------------------------------------------------------------------

export const summariesApi = {
  get(submissionID: string): Promise<{ summary: Summary }> {
    return api.get<{ summary: Summary }>(`/summaries/${submissionID}`).then((r) => r.data);
  },
  check(submissionID: string): Promise<{ check: ConsistencyCheck }> {
    return api.post<{ check: ConsistencyCheck }>(`/summaries/${submissionID}/check`).then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Rules & trials
// ---------------------------------------------------------------------------

export interface CreateRuleRequest {
  standard_code: string;
  name: string;
  expression: string;
  priority: number;
}

export const rulesApi = {
  list(standardCode = ''): Promise<{ rules: Rule[]; count: number }> {
    return api
      .get<{ rules: Rule[]; count: number }>('/rules', { params: { standard: standardCode } })
      .then((r) => r.data);
  },
  get(id: string): Promise<{ rule: Rule }> {
    return api.get<{ rule: Rule }>(`/rules/${id}`).then((r) => r.data);
  },
  create(body: CreateRuleRequest): Promise<{ rule: Rule }> {
    return api.post<{ rule: Rule }>('/rules', body).then((r) => r.data);
  },
  startTrial(id: string, contextJson: string): Promise<{ trial: RuleTrial }> {
    return api
      .post<{ trial: RuleTrial }>(`/rules/${id}/trial`, { context_json: contextJson })
      .then((r) => r.data);
  },
  approveTrial(id: string, trialID: string, reviewedBy: string): Promise<{ rule: Rule }> {
    return api
      .post<{ rule: Rule }>(`/rules/${id}/trials/${trialID}/approve`, { reviewed_by: reviewedBy })
      .then((r) => r.data);
  },
  rejectTrial(
    id: string,
    trialID: string,
    reviewedBy: string,
    reason: string,
  ): Promise<{ trial: RuleTrial }> {
    return api
      .post<{ trial: RuleTrial }>(`/rules/${id}/trials/${trialID}/reject`, {
        reviewed_by: reviewedBy,
        reason,
      })
      .then((r) => r.data);
  },
  listTrials(id: string): Promise<{ trials: RuleTrial[]; count: number }> {
    return api.get<{ trials: RuleTrial[]; count: number }>(`/rules/${id}/trials`).then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Agencies
// ---------------------------------------------------------------------------

export const agenciesApi = {
  list(): Promise<{ agencies: Agency[]; count: number }> {
    return api.get<{ agencies: Agency[]; count: number }>('/agencies').then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Standards
// ---------------------------------------------------------------------------

export const standardsApi = {
  list(): Promise<{ standards: Standard[]; count: number }> {
    return api.get<{ standards: Standard[]; count: number }>('/standards').then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Audit logs
// ---------------------------------------------------------------------------

export interface AuditListResult {
  /** When filtered by entity/actor the API returns { logs, count } (no pagination). */
  logs?: AuditEntry[];
  count?: number;
  /** Unfiltered paginated response fields. */
  items?: AuditEntry[];
  total?: number;
  page?: number;
  page_size?: number;
}

export const auditApi = {
  list(params: ListParams = {}): Promise<AuditListResult> {
    return api.get<AuditListResult>('/audit', { params }).then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Gateway & backlog (admin)
// ---------------------------------------------------------------------------

export const adminApi = {
  gatewayStatus(): Promise<GatewayStatus> {
    return api.get<GatewayStatus>('/gateway/status').then((r) => r.data);
  },
  backlog(): Promise<BacklogStats> {
    return api.get<BacklogStats>('/backlog').then((r) => r.data);
  },
  resetCircuit(id: string): Promise<{ message: string; upstream_id: string }> {
    return api.post<{ message: string; upstream_id: string }>(`/gateway/${id}/reset`).then((r) => r.data);
  },
};

// ---------------------------------------------------------------------------
// Health checks (served at root, not under /api/v1)
// ---------------------------------------------------------------------------

export const healthApi = {
  healthz(): Promise<{ status: string }> {
    return axios.get<{ status: string }>('/healthz').then((r) => r.data);
  },
  readyz(): Promise<{ status: string; reason?: string; problems?: string[] }> {
    return axios.get<{ status: string; reason?: string; problems?: string[] }>('/readyz').then((r) => r.data);
  },
};
