// Domain type definitions mirroring the Go backend (internal/domain).
// These interfaces describe the JSON shapes returned by /api/v1/*.

/** Common audit metadata embedded in most entities. */
export interface AuditMeta {
  created_by: string;
  created_at: string;
  updated_by: string;
  updated_at: string;
}

export type RiskLevel = 1 | 2 | 3;

export type DeviceCategory =
  | 'exoskeleton'
  | 'prosthesis'
  | 'orthosis'
  | 'rehabilitation_robot';

export type SubmissionStatus =
  | 'draft'
  | 'pending'
  | 'dispatched'
  | 'testing'
  | 'review'
  | 'completed'
  | 'rejected'
  | 'cancelled'
  | 'frozen';

/** Charge computation for a submission. Monetary fields arrive as
 *  JSON numbers (shopspring/decimal marshals to a raw number). */
export interface ChargeInfo {
  amount: number | string;
  currency: string;
  item_count: number;
  billed_at: string;
  is_duplicate: boolean;
}

export interface Submission {
  id: string;
  enterprise_id: string;
  enterprise_name: string;
  model_no: string;
  model_name: string;
  category: DeviceCategory;
  risk_level: RiskLevel;
  standard_codes: string[];
  batch_no: string;
  status: SubmissionStatus;
  charge: ChargeInfo;
  base_charge: number | string;
  notes?: string;
  audit: AuditMeta;
  idempotency_key: string;
  dispatch_round: number;
}

export type DispatchStatus =
  | 'created'
  | 'assigned'
  | 'in_progress'
  | 'completed'
  | 'failed'
  | 'timeout'
  | 'retrying'
  | 'failover'
  | 'cancelled'
  | 'dead_letter';

export interface DispatchTask {
  id: string;
  submission_id: string;
  model_no: string;
  batch_no: string;
  standard_code: string;
  agency_id: string;
  agency_name: string;
  status: DispatchStatus;
  priority: number;
  attempt: number;
  max_attempts: number;
  next_retry_at?: string;
  assigned_at: string;
  completed_at?: string;
  last_upstream_id?: string;
  failover_reason?: string;
  audit: AuditMeta;
}

export type RuleStatus =
  | 'draft'
  | 'trialing'
  | 'pending_review'
  | 'active'
  | 'superseded'
  | 'retired';

export interface Rule {
  id: string;
  standard_code: string;
  name: string;
  description?: string;
  expression: string;
  priority: number;
  status: RuleStatus;
  audit: AuditMeta;
}

export interface TrialEvidence {
  node_id: string;
  node_type: string;
  source: string;
  got_value: string;
  expected?: string;
  passed: boolean;
}

export type TrialStatus =
  | 'created'
  | 'completed'
  | 'failed'
  | 'approved'
  | 'rejected';

export interface RuleTrial {
  id: string;
  rule_id: string;
  expression: string;
  context_json: string;
  matched: boolean;
  evidence: TrialEvidence[];
  error_code?: string;
  error_message?: string;
  status: TrialStatus;
  reviewed_by?: string;
  reviewed_at?: string;
  audit: AuditMeta;
}

export type SummaryStatus =
  | 'pending'
  | 'computed'
  | 'stale'
  | 'frozen'
  | 'final';

export interface Summary {
  id: string;
  model_no: string;
  batch_no: string;
  submission_id: string;
  overall_passed: boolean;
  standard_count: number;
  passed_count: number;
  failed_count: number;
  status: SummaryStatus;
  computed_at: string;
  version: number;
  frozen_reason?: string;
  audit: AuditMeta;
}

export interface ConsistencyCheck {
  model_id: string;
  detail_count: number;
  summary_count: number;
  consistent: boolean;
  checked_at: string;
  divergence_note?: string;
}

export interface Agency {
  id: string;
  code: string;
  name: string;
  contact_email: string;
  is_active: boolean;
  priority: number;
  accredited_standards: string[];
  max_concurrent: number;
  base_url?: string;
  audit: AuditMeta;
}

export type CallbackStatus =
  | 'pending'
  | 'delivering'
  | 'delivered'
  | 'failed'
  | 'dead'
  | 'confirmed';

export interface Callback {
  id: string;
  submission_id: string;
  enterprise_id: string;
  webhook_url: string;
  payload: string;
  signature: string;
  status: CallbackStatus;
  attempt: number;
  max_attempts: number;
  next_retry_at?: string;
  last_error?: string;
  delivered_at?: string;
  confirmed_at?: string;
  audit: AuditMeta;
}

export interface AuditEntry {
  id: string;
  entity_type: string;
  entity_id: string;
  action: string;
  actor: string;
  detail?: string;
  occurred_at: string;
}

export interface UpstreamStatus {
  id: string;
  name: string;
  state: string;
  failures: number;
  base_url: string;
  priority: number;
}

export interface UpstreamTrace {
  id: string;
  upstream_id: string;
  dispatch_id: string;
  method: string;
  path: string;
  request_body?: string;
  response_body?: string;
  status_code: number;
  duration: string;
  error?: string;
  occurred_at: string;
}

export interface GatewayStatus {
  upstreams: UpstreamStatus[];
  recent_traces: UpstreamTrace[];
}

export interface BacklogStats {
  dispatch_backlog: number;
  callback_backlog: number;
  dead_letters: number;
}

/** Standard-list item shape returned by GET /standards. */
export interface Standard {
  code: string;
  name: string;
  description?: string;
  category: DeviceCategory;
  is_active: boolean;
  audit: AuditMeta;
}

/** Generic paginated list response: { items, total, page, page_size }. */
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

/** Generic count-wrapped response: { data/count or rules/count ... }. */
export interface CountResponse<T> {
  [key: string]: T[] | number;
}

/** Unified error format returned by the API. */
export interface ErrorResponse {
  error: {
    code: string;
    message: string;
  };
}

/** Create-submission request body. */
export interface CreateSubmissionRequest {
  enterprise_id: string;
  enterprise_name: string;
  model_no: string;
  model_name: string;
  category: DeviceCategory;
  risk_level: RiskLevel;
  standard_codes: string[];
  batch_no: string;
  notes?: string;
}

/** Response wrapper for a created submission (duplicate case adds fields). */
export interface CreateSubmissionResponse {
  submission: Submission;
  is_duplicate?: boolean;
  message?: string;
}

/** Dispatch result returned by POST /submissions/:id/dispatch. */
export interface DispatchResult {
  tasks: DispatchTask[];
  count: number;
}
