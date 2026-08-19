import { Button, Result } from 'antd';

interface ErrorResultProps {
  /** Human-readable error message. */
  message: string;
  /** Optional backend error code (e.g. VALIDATION_ERROR). */
  code?: string;
  /** Retry callback; when provided a Retry button is shown. */
  onRetry?: () => void;
}

/** Full-panel error state for failed data fetches. */
export default function ErrorResult({ message, code, onRetry }: ErrorResultProps) {
  return (
    <Result
      status="error"
      title={code ? `${code}` : 'Request failed'}
      subTitle={message}
      extra={
        onRetry ? (
          <Button type="primary" onClick={onRetry}>
            Retry
          </Button>
        ) : null
      }
    />
  );
}
