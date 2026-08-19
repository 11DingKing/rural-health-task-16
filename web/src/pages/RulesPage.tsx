import { useCallback, useEffect, useState } from 'react';
import {
  App,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Table,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ExperimentOutlined, ReloadOutlined } from '@ant-design/icons';
import {
  getApiErrorMessage,
  rulesApi,
} from '../api/client';
import type { Rule } from '../types';
import ErrorResult from '../components/ErrorResult';
import PageHeader from '../components/PageHeader';
import StatusTag from '../components/StatusTag';
import TrialModal from '../components/TrialModal';

interface CreateRuleForm {
  standard_code: string;
  name: string;
  expression: string;
  priority: number;
}

const DEFAULT_EXPRESSION = 'risk_level >= 2 && contains(standards, "GB 9706.1")';
const EXPRESSION_PLACEHOLDER = 'risk_level >= 2 && contains(standards, "GB 9706.1")';

export default function RulesPage() {
  const { message } = App.useApp();
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [trialOpen, setTrialOpen] = useState(false);
  const [activeRule, setActiveRule] = useState<Rule | null>(null);

  const [form] = Form.useForm<CreateRuleForm>();

  const fetchRules = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await rulesApi.list();
      setRules(res.rules ?? []);
    } catch (err) {
      setError(getApiErrorMessage(err, 'Failed to load rules'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRules();
  }, [fetchRules]);

  const handleCreate = async (values: CreateRuleForm) => {
    setSubmitting(true);
    try {
      await rulesApi.create({
        standard_code: values.standard_code.trim(),
        name: values.name.trim(),
        expression: values.expression,
        priority: values.priority,
      });
      message.success('Rule created (status: draft).');
      form.resetFields();
      fetchRules();
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Failed to create rule'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleRunTrial = (rule: Rule) => {
    setActiveRule(rule);
    setTrialOpen(true);
  };

  const columns: ColumnsType<Rule> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 180,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    { title: 'Standard', dataIndex: 'standard_code', width: 150 },
    { title: 'Name', dataIndex: 'name', ellipsis: true },
    {
      title: 'Expression',
      dataIndex: 'expression',
      ellipsis: true,
      render: (expr: string) => (
        <Tooltip title={expr} styles={{ root: { maxWidth: 480 } }}>
          <Typography.Text code style={{ fontSize: 12 }}>
            {expr}
          </Typography.Text>
        </Tooltip>
      ),
    },
    { title: 'Priority', dataIndex: 'priority', width: 100 },
    {
      title: 'Status',
      dataIndex: 'status',
      width: 130,
      render: (status: string) => <StatusTag status={status} kind="rule" />,
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 120,
      render: (_, record) => (
        <Button
          size="small"
          icon={<ExperimentOutlined />}
          disabled={record.status === 'retired' || record.status === 'superseded'}
          onClick={() => handleRunTrial(record)}
        >
          Run Trial
        </Button>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="Rules Management"
        subtitle="Create certification rules, run trials, and approve"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchRules} loading={loading}>
            Refresh
          </Button>
        }
      />

      <Card title="Create Rule" style={{ marginBottom: 16 }}>
        <Form
          form={form}
          layout="inline"
          onFinish={handleCreate}
          initialValues={{ priority: 10, expression: DEFAULT_EXPRESSION }}
          style={{ flexWrap: 'wrap', rowGap: 12 }}
        >
          <Form.Item
            name="standard_code"
            label="Standard"
            rules={[{ required: true, message: 'Required' }]}
          >
            <Input style={{ width: 180 }} placeholder="GB 9706.1" />
          </Form.Item>
          <Form.Item name="name" label="Name" rules={[{ required: true, message: 'Required' }]}>
            <Input style={{ width: 220 }} placeholder="Rule name" />
          </Form.Item>
          <Form.Item name="priority" label="Priority" rules={[{ required: true }]}>
            <InputNumber min={1} max={1000} style={{ width: 100 }} />
          </Form.Item>
          <Form.Item
            name="expression"
            label="Expression"
            rules={[{ required: true, message: 'Required' }]}
            style={{ flex: 1, minWidth: 280 }}
          >
            <Input.TextArea
              autoSize={{ minRows: 1, maxRows: 3 }}
              style={{ fontFamily: 'monospace' }}
              placeholder={EXPRESSION_PLACEHOLDER}
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={submitting}>
              Create
            </Button>
          </Form.Item>
        </Form>
      </Card>

      {error ? (
        <ErrorResult message={error} onRetry={fetchRules} />
      ) : (
        <Card title="Rules">
          <Table<Rule>
            rowKey="id"
            columns={columns}
            dataSource={rules}
            loading={loading}
            size="middle"
            scroll={{ x: 1000 }}
            locale={{ emptyText: 'No rules defined yet' }}
            pagination={false}
          />
        </Card>
      )}

      {activeRule ? (
        <TrialModal
          open={trialOpen}
          ruleId={activeRule.id}
          ruleName={activeRule.name}
          onClose={() => setTrialOpen(false)}
          onSuccess={fetchRules}
        />
      ) : null}
    </div>
  );
}
