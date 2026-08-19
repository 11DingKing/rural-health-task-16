import { Card, Statistic } from 'antd';
import type { ReactNode } from 'react';

interface StatCardProps {
  title: string;
  value: number | string;
  prefix?: ReactNode;
  suffix?: string;
  valueStyle?: React.CSSProperties;
  loading?: boolean;
}

/** A bordered statistic card used on the dashboard. */
export default function StatCard({
  title,
  value,
  prefix,
  suffix,
  valueStyle,
  loading = false,
}: StatCardProps) {
  return (
    <Card bordered hoverable loading={loading} bodyStyle={{ padding: 20 }}>
      <Statistic
        title={title}
        value={value}
        prefix={prefix}
        suffix={suffix}
        valueStyle={valueStyle}
      />
    </Card>
  );
}
