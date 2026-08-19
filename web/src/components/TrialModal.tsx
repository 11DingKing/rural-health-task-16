import { useState } from 'react';
import {
  Alert,
  App,
  Button,
  Input,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  getApiErrorMessage,
  rulesApi,
} from '../api/client';
import type { RuleTrial, TrialEvidence } from '../types';

interface TrialModalProps {
  open: boolean;
  ruleId: string;
  ruleName: string;
  onClose: () => void;
  onSuccess: () => void;
}

const DEFAULT_CONTEXT = JSON.stringify(
  {
    device_category: 'exoskeleton',
    risk_level: 2,
    standards: ['GB 9706.1'],
    enterprise_region: 'CN',
  },
  null,
  2,
);

const evidenceColumns: ColumnsType<TrialEvidence> = [
  { title: 'Node', dataIndex: 'node_id', width: 160 },
  { title: 'Type', dataIndex: 'node_type', width: 120 },
  { title: 'Source', dataIndex: 'source', ellipsis: true },
  { title: 'Got', dataIndex: 'got_value', width: 120 },
  { title: 'Expected', dataIndex: 'expected', width: 120, render: (v?: string) => v ?? '-' },
  {
    title: 'Passed',
    dataIndex: 'passed',
    width: 90,
    render: (v: boolean) => (v ? <Tag color="success">yes</Tag> : <Tag color="error">no</Tag>),
  },
];

/** Modal for running a rule trial: enter context JSON, inspect matched
 *  evidence, then approve or reject the trial. */
export default function TrialModal({
  open,
  ruleId,
  ruleName,
  onClose,
  onSuccess,
}: TrialModalProps) {
  const { message } = App.useApp();
  const [context, setContext] = useState(DEFAULT_CONTEXT);
  const [trial, setTrial] = useState<RuleTrial | null>(null);
  const [running, setRunning] = useState(false);
  const [reason, setReason] = useState('');
  const [acting, setActing] = useState(false);

  const handleRun = async () => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(context);
    } catch (e) {
      message.error(`Invalid JSON: ${(e as Error).message}`);
      return;
    }
    setRunning(true);
    setTrial(null);
    try {
      const res = await rulesApi.startTrial(ruleId, JSON.stringify(parsed));
      setTrial(res.trial);
      if (res.trial.status === 'failed') {
        message.error(res.trial.error_message || 'Trial calculation failed');
      } else if (res.trial.matched) {
        message.success('Rule matched the provided context.');
      } else {
        message.info('Rule did not match the provided context.');
      }
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Trial failed'));
    } finally {
      setRunning(false);
    }
  };

  const handleApprove = async () => {
    setActing(true);
    try {
      await rulesApi.approveTrial(ruleId, trial!.id, 'auditor');
      message.success('Trial approved; rule activated.');
      onSuccess();
      handleClose();
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Approve failed'));
    } finally {
      setActing(false);
    }
  };

  const handleReject = async () => {
    if (!reason.trim()) {
      message.warning('Please provide a rejection reason.');
      return;
    }
    setActing(true);
    try {
      await rulesApi.rejectTrial(ruleId, trial!.id, 'auditor', reason.trim());
      message.success('Trial rejected.');
      onSuccess();
      handleClose();
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Reject failed'));
    } finally {
      setActing(false);
    }
  };

  const handleClose = () => {
    setTrial(null);
    setReason('');
    onClose();
  };

  const canReview = trial && trial.status === 'completed';

  return (
    <Modal
      title={`Trial — ${ruleName}`}
      open={open}
      onCancel={handleClose}
      width={760}
      footer={[
        <Button key="close" onClick={handleClose}>
          Close
        </Button>,
        <Button key="run" type="primary" loading={running} onClick={handleRun}>
          Run Trial
        </Button>,
        canReview ? (
          <Button key="reject" danger loading={acting} onClick={handleReject}>
            Reject
          </Button>
        ) : null,
        canReview ? (
          <Button key="approve" type="primary" loading={acting} onClick={handleApprove}>
            Approve
          </Button>
        ) : null,
      ]}
    >
      <Typography.Text strong>Context JSON</Typography.Text>
      <Input.TextArea
        value={context}
        onChange={(e) => setContext(e.target.value)}
        rows={8}
        style={{ fontFamily: 'monospace', marginTop: 4, marginBottom: 12 }}
        placeholder="{}"
      />

      {trial ? (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Space size="middle">
            <Typography.Text>Matched:</Typography.Text>
            <Tag color={trial.matched ? 'success' : 'default'}>
              {trial.matched ? 'YES' : 'NO'}
            </Tag>
            <Typography.Text>Trial Status:</Typography.Text>
            <Tag>{trial.status}</Tag>
          </Space>
          {trial.error_message ? (
            <Alert
              type="error"
              showIcon
              message={trial.error_code || 'Trial error'}
              description={trial.error_message}
            />
          ) : null}
          {canReview ? (
            <div>
              <Typography.Text strong>Reject reason (required to reject):</Typography.Text>
              <Input.TextArea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                rows={2}
                style={{ marginTop: 4 }}
                placeholder="Why is this trial being rejected?"
              />
            </div>
          ) : null}
          {trial.evidence && trial.evidence.length > 0 ? (
            <Table<TrialEvidence>
              rowKey={(r) => r.node_id + r.source}
              columns={evidenceColumns}
              dataSource={trial.evidence}
              size="small"
              pagination={false}
              scroll={{ x: 600 }}
            />
          ) : (
            <Alert type="info" message="No evaluation evidence produced." />
          )}
        </Space>
      ) : null}
    </Modal>
  );
}
