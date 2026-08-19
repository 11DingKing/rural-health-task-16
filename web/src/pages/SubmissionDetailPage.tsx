import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import {
  Alert,
  App,
  Button,
  Card,
  Descriptions,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  ArrowLeftOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import {
  dispatchesApi,
  getApiErrorMessage,
  submissionsApi,
  summariesApi,
} from '../api/client';
import type {
  ConsistencyCheck,
  DispatchTask,
  Submission,
  Summary,
} from '../types';
import ErrorResult from '../components/ErrorResult';
import PageHeader from '../components/PageHeader';
import StatusTag from '../components/StatusTag';

function formatCharge(amount: number | string, currency: string): string {
  return `${currency} ${String(amount)}`;
}

export default function SubmissionDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const { message } = App.useApp();

  const [submission, setSubmission] = useState<Submission | null>(null);
  const [tasks, setTasks] = useState<DispatchTask[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [summaryError, setSummaryError] = useState<string | null>(null);

  const [loading, setLoading] = useState(false);
  const [tasksLoading, setTasksLoading] = useState(false);
  const [summaryLoading, setSummaryLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [checking, setChecking] = useState(false);
  const [checkResult, setCheckResult] = useState<ConsistencyCheck | null>(null);

  const fetchSubmission = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await submissionsApi.get(id);
      setSubmission(res.submission);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to load submission'));
    } finally {
      setLoading(false);
    }
  }, [id]);

  const fetchTasks = useCallback(async () => {
    setTasksLoading(true);
    try {
      const res = await dispatchesApi.list({ page: 1, page_size: 200 });
      const filtered = (res.items ?? []).filter((t) => t.submission_id === id);
      setTasks(filtered);
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Failed to load dispatch tasks'));
    } finally {
      setTasksLoading(false);
    }
  }, [id, message]);

  const fetchSummary = useCallback(async () => {
    setSummaryLoading(true);
    setSummaryError(null);
    try {
      const res = await summariesApi.get(id);
      setSummary(res.summary);
    } catch (err) {
      setSummary(null);
      setSummaryError(getApiErrorMessage(err, 'No summary available yet'));
    } finally {
      setSummaryLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchSubmission();
    fetchTasks();
    fetchSummary();
  }, [fetchSubmission, fetchTasks, fetchSummary]);

  const handleCheck = async () => {
    setChecking(true);
    try {
      const res = await summariesApi.check(id);
      setCheckResult(res.check);
      if (res.check.consistent) {
        message.success('Consistency check passed.');
      } else {
        message.warning('Inconsistency detected — model may be frozen.');
      }
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Consistency check failed'));
    } finally {
      setChecking(false);
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 48 }}>
        <Spin tip="Loading submission…" />
      </div>
    );
  }

  if (error || !submission) {
    return <ErrorResult message={error ?? 'Submission not found'} onRetry={fetchSubmission} />;
  }

  const taskColumns: ColumnsType<DispatchTask> = [
    {
      title: 'Task ID',
      dataIndex: 'id',
      width: 180,
      render: (taskId: string) => <Typography.Text code>{taskId}</Typography.Text>,
    },
    { title: 'Standard', dataIndex: 'standard_code', width: 140 },
    { title: 'Agency', dataIndex: 'agency_name', ellipsis: true },
    {
      title: 'Status',
      dataIndex: 'status',
      width: 120,
      render: (status: string) => <StatusTag status={status} kind="dispatch" />,
    },
    { title: 'Attempt', key: 'attempt', width: 100, render: (_, r) => `${r.attempt}/${r.max_attempts}` },
    { title: 'Upstream', dataIndex: 'last_upstream_id', width: 140, render: (v?: string) => v ?? '-' },
    {
      title: 'Assigned',
      dataIndex: 'assigned_at',
      width: 170,
      render: (v: string) => (v ? new Date(v).toLocaleString() : '-'),
    },
  ];

  return (
    <div>
      <PageHeader
        title={`Submission ${submission.id}`}
        subtitle={submission.enterprise_name}
        extra={
          <Space>
            <Button href="/submissions" icon={<ArrowLeftOutlined />}>
              Back
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => { fetchSubmission(); fetchTasks(); fetchSummary(); }}>
              Refresh
            </Button>
          </Space>
        }
      />

      <Card style={{ marginBottom: 16 }}>
        <Descriptions column={{ xs: 1, sm: 2, lg: 3 }} size="small" bordered>
          <Descriptions.Item label="Status">
            <StatusTag status={submission.status} kind="submission" />
          </Descriptions.Item>
          <Descriptions.Item label="Enterprise ID">{submission.enterprise_id}</Descriptions.Item>
          <Descriptions.Item label="Enterprise">{submission.enterprise_name}</Descriptions.Item>
          <Descriptions.Item label="Model No">{submission.model_no}</Descriptions.Item>
          <Descriptions.Item label="Model Name">{submission.model_name}</Descriptions.Item>
          <Descriptions.Item label="Batch No">{submission.batch_no}</Descriptions.Item>
          <Descriptions.Item label="Category">{submission.category}</Descriptions.Item>
          <Descriptions.Item label="Risk Level">{submission.risk_level}</Descriptions.Item>
          <Descriptions.Item label="Dispatch Round">{submission.dispatch_round}</Descriptions.Item>
          <Descriptions.Item label="Standard Codes">
            <Space size={[4, 4]} wrap>
              {submission.standard_codes.map((c) => (
                <Tag key={c}>{c}</Tag>
              ))}
            </Space>
          </Descriptions.Item>
          <Descriptions.Item label="Base Charge">
            {formatCharge(submission.base_charge, submission.charge.currency || 'CNY')}
          </Descriptions.Item>
          <Descriptions.Item label="Charge Amount">
            {submission.charge.is_duplicate ? (
              <Tag color="magenta">Duplicate — 0</Tag>
            ) : (
              formatCharge(submission.charge.amount, submission.charge.currency || 'CNY')
            )}
          </Descriptions.Item>
          <Descriptions.Item label="Created By">{submission.audit.created_by}</Descriptions.Item>
          <Descriptions.Item label="Created At">
            {new Date(submission.audit.created_at).toLocaleString()}
          </Descriptions.Item>
          <Descriptions.Item label="Notes">{submission.notes || '-'}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title="Summary"
        style={{ marginBottom: 16 }}
        extra={
          <Button
            icon={<SafetyCertificateOutlined />}
            loading={checking}
            onClick={handleCheck}
          >
            Run Consistency Check
          </Button>
        }
      >
        {summaryLoading ? (
          <Spin />
        ) : summary ? (
          <Descriptions column={{ xs: 1, sm: 2, lg: 3 }} size="small" bordered>
            <Descriptions.Item label="Status">
              <StatusTag status={summary.status} kind="summary" />
            </Descriptions.Item>
            <Descriptions.Item label="Overall Passed">
              {summary.overall_passed ? <Tag color="success">PASS</Tag> : <Tag color="error">FAIL</Tag>}
            </Descriptions.Item>
            <Descriptions.Item label="Version">{summary.version}</Descriptions.Item>
            <Descriptions.Item label="Standards">{summary.standard_count}</Descriptions.Item>
            <Descriptions.Item label="Passed">{summary.passed_count}</Descriptions.Item>
            <Descriptions.Item label="Failed">{summary.failed_count}</Descriptions.Item>
            <Descriptions.Item label="Computed At">
              {new Date(summary.computed_at).toLocaleString()}
            </Descriptions.Item>
            {summary.frozen_reason ? (
              <Descriptions.Item label="Frozen Reason">{summary.frozen_reason}</Descriptions.Item>
            ) : null}
          </Descriptions>
        ) : (
          <Alert
            type="info"
            showIcon
            message="No summary computed yet"
            description={summaryError ?? 'The summary is derived once dispatch results arrive.'}
          />
        )}

        {checkResult ? (
          <Alert
            style={{ marginTop: 12 }}
            type={checkResult.consistent ? 'success' : 'warning'}
            showIcon
            message={checkResult.consistent ? 'Consistency verified' : 'Inconsistency detected'}
            description={
              <>
                Details: {checkResult.detail_count} · Summary: {checkResult.summary_count}
                {checkResult.divergence_note ? ` · ${checkResult.divergence_note}` : ''}
                <br />
                Checked: {new Date(checkResult.checked_at).toLocaleString()}
              </>
            }
          />
        ) : null}
      </Card>

      <Card title="Dispatch Tasks" extra={<Button size="small" onClick={fetchTasks}>Refresh</Button>}>
        <Table<DispatchTask>
          rowKey="id"
          columns={taskColumns}
          dataSource={tasks}
          loading={tasksLoading}
          size="small"
          scroll={{ x: 900 }}
          locale={{ emptyText: 'No dispatch tasks for this submission' }}
          pagination={false}
        />
      </Card>
    </div>
  );
}
