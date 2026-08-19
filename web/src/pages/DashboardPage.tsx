import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Alert, Badge, Button, Card, Col, Row, Space, Table, Tag, Tooltip, Typography } from "antd";
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';
import { adminApi, getApiErrorMessage, healthApi } from '../api/client';
import type { BacklogStats, UpstreamStatus } from '../types';
import ErrorResult from '../components/ErrorResult';
import PageHeader from '../components/PageHeader';
import StatCard from '../components/StatCard';
import StatusTag from '../components/StatusTag';

interface HealthState {
  status: string;
  detail?: string;
}

export default function DashboardPage() {
  const navigate = useNavigate();
  const [backlog, setBacklog] = useState<BacklogStats | null>(null);
  const [upstreams, setUpstreams] = useState<UpstreamStatus[]>([]);
  const [healthz, setHealthz] = useState<HealthState | null>(null);
  const [readyz, setReadyz] = useState<HealthState | null>(null);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [backlogRes, gatewayRes] = await Promise.all([
        adminApi.backlog(),
        adminApi.gatewayStatus(),
      ]);
      setBacklog(backlogRes);
      setUpstreams(gatewayRes.upstreams ?? []);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to load dashboard data'));
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchHealth = useCallback(async () => {
    try {
      const h = await healthApi.healthz();
      setHealthz({ status: h.status });
    } catch {
      setHealthz({ status: 'down', detail: 'no response' });
    }
    try {
      const r = await healthApi.readyz();
      setReadyz({ status: r.status, detail: r.reason || (r.problems ? r.problems.join(', ') : undefined) });
    } catch {
      setReadyz({ status: 'not ready', detail: 'no response' });
    }
  }, []);

  useEffect(() => {
    fetchData();
    fetchHealth();
  }, [fetchData, fetchHealth]);

  const upstreamColumns: ColumnsType<UpstreamStatus> = [
    { title: 'Upstream', dataIndex: 'name', ellipsis: true },
    { title: 'Base URL', dataIndex: 'base_url', ellipsis: true, render: (v: string) => <Typography.Text code style={{ fontSize: 12 }}>{v}</Typography.Text> },
    { title: 'State', dataIndex: 'state', width: 110, render: (v: string) => <StatusTag status={v} kind="upstream" /> },
    { title: 'Failures', dataIndex: 'failures', width: 100 },
    { title: 'Priority', dataIndex: 'priority', width: 90 },
  ];

  return (
    <div>
      <PageHeader
        title="Dashboard"
        subtitle="Platform backlog, gateway, and health overview"
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => { fetchData(); fetchHealth(); }} loading={loading}>
            Refresh
          </Button>
        }
      />

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={8}>
          <StatCard
            title="Pending Dispatches"
            value={backlog?.dispatch_backlog ?? 0}
            loading={loading}
            valueStyle={{ color: '#1677ff' }}
          />
        </Col>
        <Col xs={24} sm={8}>
          <StatCard
            title="Pending Callbacks"
            value={backlog?.callback_backlog ?? 0}
            loading={loading}
            valueStyle={{ color: '#fa8c16' }}
          />
        </Col>
        <Col xs={24} sm={8}>
          <StatCard
            title="Dead Letters"
            value={backlog?.dead_letters ?? 0}
            loading={loading}
            valueStyle={{ color: '#cf1322' }}
          />
        </Col>
      </Row>

      <Card size="small" style={{ marginBottom: 16 }} title="Health Checks">
        <Space size="large" wrap>
          <Space>
            <Typography.Text>Health:</Typography.Text>
            {healthz ? (
              <Badge status={healthz.status === 'ok' ? 'success' : 'error'} text={healthz.status} />
            ) : (
              <Tag>checking…</Tag>
            )}
          </Space>
          <Space>
            <Typography.Text>Ready:</Typography.Text>
            {readyz ? (
              <Tooltip title={readyz.detail || ''}>
                <Badge status={readyz.status === 'ok' ? 'success' : 'warning'} text={readyz.status} />
              </Tooltip>
            ) : (
              <Tag>checking…</Tag>
            )}
          </Space>
        </Space>
      </Card>

      {error ? (
        <ErrorResult message={error} onRetry={fetchData} />
      ) : (
        <Card
          title="Gateway Upstream Status"
          extra={
            <Space>
              <Button size="small" onClick={() => navigate('/dispatches')}>
                Open Dispatch Monitor
              </Button>
            </Space>
          }
        >
          <Table<UpstreamStatus>
            rowKey="id"
            columns={upstreamColumns}
            dataSource={upstreams}
            loading={loading}
            size="small"
            locale={{ emptyText: 'No upstreams configured' }}
            pagination={false}
          />
          {upstreams.length === 0 && !loading ? (
            <Alert
              style={{ marginTop: 12 }}
              type="info"
              showIcon
              message="No gateway upstreams registered. Configure upstreams in the backend config."
            />
          ) : null}
        </Card>
      )}
    </div>
  );
}
