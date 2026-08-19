import { Tag } from 'antd';

export type StatusKind =
  | 'submission'
  | 'dispatch'
  | 'rule'
  | 'trial'
  | 'summary'
  | 'callback'
  | 'upstream';

const COLOR_MAPS: Record<StatusKind, Record<string, string>> = {
  submission: {
    draft: 'default',
    pending: 'processing',
    dispatched: 'blue',
    testing: 'cyan',
    review: 'gold',
    completed: 'success',
    rejected: 'error',
    cancelled: 'default',
    frozen: 'warning',
  },
  dispatch: {
    created: 'default',
    assigned: 'blue',
    in_progress: 'cyan',
    completed: 'success',
    failed: 'error',
    timeout: 'warning',
    retrying: 'gold',
    failover: 'magenta',
    cancelled: 'default',
    dead_letter: 'error',
  },
  rule: {
    draft: 'default',
    trialing: 'gold',
    pending_review: 'warning',
    active: 'success',
    superseded: 'blue',
    retired: 'default',
  },
  trial: {
    created: 'default',
    completed: 'blue',
    failed: 'error',
    approved: 'success',
    rejected: 'warning',
  },
  summary: {
    pending: 'default',
    computed: 'blue',
    stale: 'warning',
    frozen: 'magenta',
    final: 'success',
  },
  callback: {
    pending: 'default',
    delivering: 'processing',
    delivered: 'cyan',
    failed: 'error',
    dead: 'error',
    confirmed: 'success',
  },
  upstream: {
    closed: 'success',
    open: 'error',
    'half-open': 'warning',
  },
};

interface StatusTagProps {
  status: string;
  kind: StatusKind;
  /** Optional title-style label; defaults to the raw status. */
  label?: string;
}

/** Renders a colored antd Tag for a domain status string. */
export default function StatusTag({ status, kind, label }: StatusTagProps) {
  const color = COLOR_MAPS[kind]?.[status] ?? 'default';
  return <Tag color={color}>{label ?? status}</Tag>;
}
