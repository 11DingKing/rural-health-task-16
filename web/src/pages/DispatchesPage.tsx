import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  App,
  Button,
  Card,
  Empty,
  Input,
  Select,
  Space,
  Table,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons';
import {
  adminApi,
  dispatchesApi,
  getApiErrorMessage,
} from '../api/client';
import type { DispatchTask, UpstreamStatus } from '../types';
import ErrorResult from '../components/ErrorResult';
import PageHeader from '../components/PageHeader';
import StatusTag from '../components/StatusTag';

const AGENCY_STATUS_OPTIONS = [
  { label: 'All', value: '' },
  { label: 'Created', value: 'created' },
  { label: 'Assigned', value: 'assigned' },
  { label: 'In Progress', value: 'in_progress' },
  { label: 'Completed', value: 'completed' },
  { label: 'Failed', value: 'failed' },
  { label: 'Timeout', value: 'timeout' },
  { label: 'Retrying', value: 'retrying' },
  { label: 'Failover', value: 'failover' },
  { label: 'Dead Letter', value: 'dead_letter' },
  { label: 'Cancelled', value: 'cancelled' },
];

function fmtTime(v?: string): string {
  return v ? new Date(v).toLocaleString() : '-';
}

export default function DispatchesPage() {
  const navigate = useNavigate();
  const { message } = App.useApp();

  const [data, setData] = useState<DispatchTask[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [agency, setAgency] = useState('');
  const [status, setStatus] = useState('');
  const [retryingId, setRetryingId] = useState<string | null>(null);

  const [upstreams, setUpstreams] = useState<UpstreamStatus[]>([]);
  const [gatewayLoading, setGatewayLoading] = useState(false);
  const [resettingId, setResettingId] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await dispatchesApi.list({ page, page_size: pageSize, agency, status });
      setData(res.items ?? []);
      setTotal(res.total ?? 0);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to load dispatches'));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, agency, status]);

  const fetchGateway = useCallback(async () => {
    setGatewayLoading(true);
    try {
      const res = await adminApi.gatewayStatus();
      setUpstreams(res.upstreams ?? []);
    } catch {
      setUpstreams([]);
    } finally {
      setGatewayLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    fetchGateway();
  }, [fetchGateway]);

  const handleRetry = async (record: DispatchTask) => {
    setRetryingId(record.id);
    try {
      await dispatchesApi.retry(record.id);
      message.success(`Retry triggered for task ${record.id}.`);
      fetchData();
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Retry failed'));
    } finally {
      setRetryingId(null);
    }
  };

  const handleResetCircuit = async (upstreamId: string) => {
    setResettingId(upstreamId);
    try {
      await adminApi.resetCircuit(upstreamId);
      message.success(`Circuit breaker reset for ${upstreamId}.`);
      fetchGateway();
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Reset failed'));
    } finally {
      setResettingId(null);
    }
  };

  const columns: ColumnsType<DispatchTask> = [
    {
      title: 'Task ID',
      dataIndex: 'id',
      width: 180,
      render: (id: string) => <Typography.Text code>{id}</Typography.Text>,
    },
    {
      title: 'Submission',
      dataIndex: 'submission_id',
      width: 160,
      render: (subId: string) => (
        <Button type="link" size="small" style={{ padding: 0 }} onClick={() => navigate(`/submissions/${subId}`)}>
          {subId}
        </Button>
      ),
    },
    { title: 'Model', dataIndex: 'model_no', width: 120 },
    { title: 'Standard', dataIndex: 'standard_code', width: 140 },
    { title: 'Agency', dataIndex: 'agency_name', ellipsis: true },
    {
      title: 'Status',
      dataIndex: 'status',
      width: 120,
      render: (status: string) => <StatusTag status={status} kind="dispatch" />,
    },
    {
      title: 'Attempt',
      key: 'attempt',
      width: 90,
      render: (_, r) => `${r.attempt}/${r.max_attempts}`,
    },
    {
      title: 'Next Retry',
      dataIndex: 'next_retry_at',
      width: 170,
      render: fmtTime,
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 110,
      render: (_, record) => (
        <Button
          size="small"
          icon={<ThunderboltOutlined />}
          loading={retryingId === record.id}
          disabled={
            record.status !== 'failed' &&
            record.status !== 'timeout' &&
            record.status !== 'dead_letter'
          }
          onClick={() => handleRetry(record)}
        >
          Retry
        </Button>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="Dispatch Monitor"
        subtitle="Dispatch tasks with agency & status filters"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
            Refresh
          </Button>
        }
      />

      <Card
        title="Gateway Upstream Status"
        size="small"
        style={{ marginBottom: 16 }}
        extra={
          <Button size="small" onClick={fetchGateway} loading={gatewayLoading}>
            Refresh
          </Button>
        }
      >
        {upstreams.length === 0 ? (
          <Empty description="No upstreams configured" />
        ) : (
          <Space wrap>
            {upstreams.map((up) => (
              <Card
                key={up.id}
                size="small"
                style={{ width: 260 }}
                styles={{ body: { padding: 12 } }}
              >
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                    <Typography.Text strong>{up.name}</Typography.Text>
                    <StatusTag status={up.state} kind="upstream" />
                  </Space>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {up.base_url}
                  </Typography.Text>
                  <Space size="large">
                    <Typography.Text style={{ fontSize: 12 }}>
                      Failures: <b>{up.failures}</b>
                    </Typography.Text>
                    <Typography.Text style={{ fontSize: 12 }}>
                      Priority: <b>{up.priority}</b>
                    </Typography.Text>
                  </Space>
                  <Button
                    size="small"
                    danger
                    block
                    loading={resettingId === up.id}
                    disabled={up.state !== 'open'}
                    onClick={() => handleResetCircuit(up.id)}
                  >
                    Reset Circuit
                  </Button>
                </Space>
              </Card>
            ))}
          </Space>
        )}
      </Card>

      <Space style={{ marginBottom: 16 }} size="middle" wrap>
        <Input
          style={{ width: 220 }}
          placeholder="Filter by agency ID"
          allowClear
          onPressEnter={(e) => {
            setAgency((e.target as HTMLInputElement).value);
            setPage(1);
          }}
        />
        <Select
          style={{ width: 180 }}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={AGENCY_STATUS_OPTIONS}
          placeholder="Filter by status"
        />
      </Space>

      {error ? (
        <ErrorResult message={error} onRetry={fetchData} />
      ) : (
        <Table<DispatchTask>
          rowKey="id"
          columns={columns}
          dataSource={data}
          loading={loading}
          size="middle"
          scroll={{ x: 1200 }}
          locale={{ emptyText: 'No dispatch tasks found' }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `Total ${t} tasks`,
            onChange: (p, ps) => {
              setPage(p);
              setPageSize(ps);
            },
          }}
        />
      )}
    </div>
  );
}
