import { useState } from 'react';
import {
  App,
  Form,
  Input,
  Modal,
  Select,
  Typography,
} from 'antd';
import type { CreateSubmissionRequest, DeviceCategory, RiskLevel } from '../types';
import { getApiErrorMessage, submissionsApi } from '../api/client';

const CATEGORY_OPTIONS: { label: string; value: DeviceCategory }[] = [
  { label: 'Exoskeleton', value: 'exoskeleton' },
  { label: 'Prosthesis', value: 'prosthesis' },
  { label: 'Orthosis', value: 'orthosis' },
  { label: 'Rehabilitation Robot', value: 'rehabilitation_robot' },
];

const RISK_OPTIONS: { label: string; value: RiskLevel }[] = [
  { label: 'Low (1)', value: 1 },
  { label: 'Medium (2)', value: 2 },
  { label: 'High (3)', value: 3 },
];

interface CreateSubmissionModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

/**
 * Modal form for creating a certification submission. Provides field
 * validation and explicit submit feedback (success / duplicate / error).
 */
export default function CreateSubmissionModal({
  open,
  onClose,
  onSuccess,
}: CreateSubmissionModalProps) {
  const [form] = Form.useForm<CreateSubmissionRequest>();
  const [submitting, setSubmitting] = useState(false);
  const { message } = App.useApp();

  const handleFinish = async (values: CreateSubmissionRequest) => {
    setSubmitting(true);
    try {
      const res = await submissionsApi.create(values);
      if (res.is_duplicate) {
        message.warning(
          `Duplicate submission detected. Returned original ${res.submission.id}.`,
        );
      } else {
        message.success(`Submission ${res.submission.id} created.`);
      }
      form.resetFields();
      onSuccess();
      onClose();
    } catch (err) {
      message.error(getApiErrorMessage(err, 'Failed to create submission'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onClose();
  };

  return (
    <Modal
      title="Create Submission"
      open={open}
      onCancel={handleCancel}
      onOk={() => form.submit()}
      okText="Submit"
      confirmLoading={submitting}
      destroyOnClose
      width={640}
    >
      <Form
        form={form}
        layout="vertical"
        onFinish={handleFinish}
        initialValues={{ risk_level: 1 as RiskLevel, category: 'exoskeleton' as DeviceCategory }}
        preserve={false}
      >
        <Form.Item
          name="enterprise_id"
          label="Enterprise ID"
          rules={[{ required: true, message: 'Enterprise ID is required' }]}
        >
          <Input placeholder="e.g. ent_001" />
        </Form.Item>
        <Form.Item
          name="enterprise_name"
          label="Enterprise Name"
          rules={[{ required: true, message: 'Enterprise name is required' }]}
        >
          <Input placeholder="Enterprise legal name" />
        </Form.Item>
        <Form.Item
          name="model_no"
          label="Model No"
          rules={[{ required: true, message: 'Model number is required' }]}
        >
          <Input placeholder="Device model number" />
        </Form.Item>
        <Form.Item
          name="model_name"
          label="Model Name"
          rules={[{ required: true, message: 'Model name is required' }]}
        >
          <Input placeholder="Device model name" />
        </Form.Item>
        <Form.Item
          name="category"
          label="Category"
          rules={[{ required: true, message: 'Category is required' }]}
        >
          <Select options={CATEGORY_OPTIONS} />
        </Form.Item>
        <Form.Item
          name="risk_level"
          label="Risk Level"
          rules={[{ required: true, message: 'Risk level is required' }]}
        >
          <Select options={RISK_OPTIONS} />
        </Form.Item>
        <Form.Item
          name="standard_codes"
          label="Standard Codes"
          rules={[
            { required: true, message: 'At least one standard is required' },
            {
              validator: (_, value: string[]) =>
                value && value.length > 0
                  ? Promise.resolve()
                  : Promise.reject(new Error('At least one standard is required')),
            },
          ]}
          tooltip="Press Enter to add each standard code, e.g. GB 9706.1"
        >
          <Select
            mode="tags"
            placeholder="Add standard codes"
            tokenSeparators={[',', ' ']}
          />
        </Form.Item>
        <Form.Item
          name="batch_no"
          label="Batch No"
          rules={[{ required: true, message: 'Batch number is required' }]}
        >
          <Input placeholder="Production batch number" />
        </Form.Item>
        <Form.Item name="notes" label="Notes">
          <Input.TextArea rows={2} placeholder="Optional notes" />
        </Form.Item>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Same model + batch is treated as an idempotent duplicate and returns
          the original submission without a new charge.
        </Typography.Text>
      </Form>
    </Modal>
  );
}
