import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Input,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { auditApi, getApiErrorMessage } from '../api/client';
import type { AuditEntry } from '../types';
import ErrorResult from '../components/ErrorResult';
import PageHeader from '../components/PageHeader';

export default function AuditPage() {
  const [data, setData] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // Applied filter values (what the last query used).
  const [filters, setFilters] = useState({ entity_type: '', entity_id: '', actor: '' });
  // Input values being edited in the filter bar.
  const [inputs, setInputs] = useState({ entity_type: '', entity_id: '', actor: '' });

  const isFiltered = Boolean(
    (filters.entity_type && filters.entity_id) || filters.actor,
  );

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await auditApi.list({
        page,
        page_size: pageSize,
        entity_type: filters.entity_type,
        entity_id: filters.entity_id,
        actor: filters.actor,
      });
      if (isFiltered) {
        // Filtered queries return { logs, count } (backend returns all matches).
        setData(res.logs ?? []);
        setTotal(res.count ?? 0);
      } else {
        // Unfiltered queries return a paginated { items, total, page, page_size }.
        setData(res.items ?? []);
        setTotal(res.total ?? 0);
      }
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to load audit logs'));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, filters, isFiltered]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const applyFilters = () => {
    setFilters({ ...inputs });
    setPage(1);
  };

  const clearFilters = () => {
    setInputs({ entity_type: '', entity_id: '', actor: '' });
    setFilters({ entity_type: '', entity_id: '', actor: '' });
    setPage(1);
  };

  const columns: ColumnsType<AuditEntry> = [
    {
      title: 'Time',
      dataIndex: 'occurred_at',
      width: 180,
      render: (v: string) => (v ? new Date(v).toLocaleString() : '-'),
    },
    { title: 'Entity Type', dataIndex: 'entity_type', width: 130 },
    {
      title: 'Entity ID',
      dataIndex: 'entity_id',
      width: 200,
      render: (v: string) => (
        <Tooltip title={v}>
          <Typography.Text code style={{ fontSize: 12 }}>
            {v}
          </Typography.Text>
        </Tooltip>
      ),
    },
    {
      title: 'Action',
      dataIndex: 'action',
      width: 150,
      render: (v: string) => <Tag>{v}</Tag>,
    },
    { title: 'Actor', dataIndex: 'actor', width: 140 },
    {
      title: 'Detail',
      dataIndex: 'detail',
      ellipsis: true,
      render: (v?: string) => v ?? '-',
    },
  ];

  return (
    <div>
      <PageHeader
        title="Audit Log"
        subtitle="Append-only audit trail of platform actions"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
            Refresh
          </Button>
        }
      />

      <Space style={{ marginBottom: 16 }} size="small" wrap>
        <Input
          style={{ width: 180 }}
          placeholder="Entity type"
          value={inputs.entity_type}
          onChange={(e) => setInputs((s) => ({ ...s, entity_type: e.target.value }))}
        />
        <Input
          style={{ width: 200 }}
          placeholder="Entity ID"
          value={inputs.entity_id}
          onChange={(e) => setInputs((s) => ({ ...s, entity_id: e.target.value }))}
        />
        <Input
          style={{ width: 180 }}
          placeholder="Actor"
          value={inputs.actor}
          onChange={(e) => setInputs((s) => ({ ...s, actor: e.target.value }))}
        />
        <Button type="primary" icon={<SearchOutlined />} onClick={applyFilters}>
          Search
        </Button>
        <Button onClick={clearFilters}>Clear</Button>
      </Space>

      {error ? (
        <ErrorResult message={error} onRetry={fetchData} />
      ) : (
        <Table<AuditEntry>
          rowKey="id"
          columns={columns}
          dataSource={data}
          loading={loading}
          size="middle"
          scroll={{ x: 1000 }}
          locale={{ emptyText: 'No audit entries found' }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `Total ${t} entries`,
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
