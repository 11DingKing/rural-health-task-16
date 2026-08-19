import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { App, Button, Input, Select, Space, Table, Tag } from "antd";
import type { ColumnsType } from 'antd/es/table';
import { PlusOutlined, ReloadOutlined } from "@ant-design/icons";
import {
  getApiErrorMessage,
  submissionsApi,
} from '../api/client';
import type { Submission } from '../types';
import CreateSubmissionModal from '../components/CreateSubmissionModal';
import ErrorResult from '../components/ErrorResult';
import PageHeader from '../components/PageHeader';
import StatusTag from '../components/StatusTag';

const STATUS_OPTIONS = [
  { label: 'All', value: '' },
  { label: 'Draft', value: 'draft' },
  { label: 'Pending', value: 'pending' },
  { label: 'Dispatched', value: 'dispatched' },
  { label: 'Testing', value: 'testing' },
  { label: 'Review', value: 'review' },
  { label: 'Completed', value: 'completed' },
  { label: 'Rejected', value: 'rejected' },
  { label: 'Frozen', value: 'frozen' },
  { label: 'Cancelled', value: 'cancelled' },
];

function formatCharge(amount: number | string, currency: string): string {
  return `${currency} ${String(amount)}`;
}

export default function SubmissionsPage() {
  const navigate = useNavigate();
  const { message, modal } = App.useApp();
  const [data, setData] = useState<Submission[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [status, setStatus] = useState('');
  const [enterprise, setEnterprise] = useState('');

  const [createOpen, setCreateOpen] = useState(false);
  const [actingId, setActingId] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await submissionsApi.list({ page, page_size: pageSize, status, enterprise });
      setData(res.items ?? []);
      setTotal(res.total ?? 0);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to load submissions'));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, status, enterprise]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleCancel = (record: Submission) => {
    modal.confirm({
      title: `Cancel submission ${record.id}?`,
      content: 'This will stop further processing for this submission.',
      okText: 'Cancel submission',
      okType: 'danger',
      cancelText: 'Close',
      onOk: async () => {
        setActingId(record.id);
        try {
          await submissionsApi.cancel(record.id);
          message.success(`Submission ${record.id} cancelled.`);
          fetchData();
        } catch (err) {
          message.error(getApiErrorMessage(err, 'Cancel failed'));
        } finally {
          setActingId(null);
        }
      },
    });
  };

  const handleDispatch = (record: Submission) => {
    modal.confirm({
      title: `Dispatch submission ${record.id}?`,
      content: 'This will create dispatch tasks for each applicable agency.',
      okText: 'Dispatch',
      onOk: async () => {
        setActingId(record.id);
        try {
          const res = await submissionsApi.dispatch(record.id);
          message.success(`Dispatched: ${res.count} task(s) created.`);
          fetchData();
        } catch (err) {
          message.error(getApiErrorMessage(err, 'Dispatch failed'));
        } finally {
          setActingId(null);
        }
      },
    });
  };

  const columns: ColumnsType<Submission> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 180,
      render: (id: string) => (
        <Button type="link" size="small" style={{ padding: 0 }} onClick={() => navigate(`/submissions/${id}`)}>
          {id}
        </Button>
      ),
      sorter: false,
    },
    { title: 'Enterprise', dataIndex: 'enterprise_name', ellipsis: true, width: 180 },
    { title: 'Model No', dataIndex: 'model_no', width: 140 },
    { title: 'Batch No', dataIndex: 'batch_no', width: 140 },
    {
      title: 'Status',
      dataIndex: 'status',
      width: 120,
      render: (status: string) => <StatusTag status={status} kind="submission" />,
    },
    {
      title: 'Charge',
      key: 'charge',
      width: 130,
      render: (_, record) =>
        record.charge
          ? formatCharge(record.charge.amount, record.charge.currency || 'CNY')
          : '-',
    },
    {
      title: 'Duplicate',
      dataIndex: ['charge', 'is_duplicate'],
      width: 100,
      render: (dup?: boolean) =>
        dup ? <Tag color="magenta">Yes</Tag> : <Tag>No</Tag>,
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 180,
      render: (_, record) => (
        <Space size="small">
          <Button
            size="small"
            danger
            loading={actingId === record.id}
            disabled={record.status === 'cancelled' || record.status === 'completed'}
            onClick={() => handleCancel(record)}
          >
            Cancel
          </Button>
          <Button
            size="small"
            type="primary"
            loading={actingId === record.id}
            disabled={
              record.status === 'dispatched' ||
              record.status === 'testing' ||
              record.status === 'completed' ||
              record.status === 'cancelled'
            }
            onClick={() => handleDispatch(record)}
          >
            Dispatch
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="Submissions"
        subtitle="Certification submissions with server-side pagination"
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
              Refresh
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setCreateOpen(true)}
            >
              Create Submission
            </Button>
          </Space>
        }
      />

      <Space style={{ marginBottom: 16 }} size="middle" wrap>
        <Select
          style={{ width: 180 }}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={STATUS_OPTIONS}
          placeholder="Filter by status"
        />
        <Input.Search
          style={{ width: 260 }}
          placeholder="Filter by enterprise"
          allowClear
          enterButton
          onSearch={(v) => {
            setEnterprise(v);
            setPage(1);
          }}
        />
      </Space>

      {error ? (
        <ErrorResult message={error} onRetry={fetchData} />
      ) : (
        <Table<Submission>
          rowKey="id"
          columns={columns}
          dataSource={data}
          loading={loading}
          size="middle"
          scroll={{ x: 1100 }}
          locale={{ emptyText: 'No submissions found' }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (t) => `Total ${t} submissions`,
            onChange: (p, ps) => {
              setPage(p);
              setPageSize(ps);
            },
          }}
        />
      )}

      <CreateSubmissionModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onSuccess={() => {
          setPage(1);
          fetchData();
        }}
      />
    </div>
  );
}
