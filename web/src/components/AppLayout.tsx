import { useMemo } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { Badge, Layout, Menu, theme, Typography } from 'antd';
import {
  ApartmentOutlined,
  AuditOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  ExperimentOutlined,
  SendOutlined,
} from '@ant-design/icons';

const { Header, Sider, Content } = Layout;

const MENU_ITEMS = [
  { key: '/', icon: <DashboardOutlined />, label: <Link to="/">Dashboard</Link> },
  {
    key: '/submissions',
    icon: <SendOutlined />,
    label: <Link to="/submissions">Submissions</Link>,
  },
  {
    key: '/dispatches',
    icon: <CloudServerOutlined />,
    label: <Link to="/dispatches">Dispatch Monitor</Link>,
  },
  {
    key: '/rules',
    icon: <ExperimentOutlined />,
    label: <Link to="/rules">Rules</Link>,
  },
  { key: '/audit', icon: <AuditOutlined />, label: <Link to="/audit">Audit Log</Link> },
];

/** Top-level application shell: collapsible sider + header + routed content. */
export default function AppLayout() {
  const location = useLocation();
  const {
    token: { colorBgContainer },
  } = theme.useToken();

  const selectedKey = useMemo(() => {
    const path = location.pathname;
    const match = MENU_ITEMS.find((item) => item.key !== '/' && path.startsWith(item.key));
    if (match) {
      return match.key;
    }
    if (path === '/' || path === '') {
      return '/';
    }
    return path;
  }, [location.pathname]);

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        breakpoint="lg"
        collapsedWidth={0}
        width={232}
        style={{ position: 'sticky', top: 0, height: '100vh' }}
      >
        <div
          style={{
            height: 56,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#fff',
            gap: 8,
          }}
        >
          <ApartmentOutlined style={{ fontSize: 20 }} />
          <Typography.Text strong style={{ color: '#fff', fontSize: 15 }}>
            RehabCert
          </Typography.Text>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={MENU_ITEMS}
          style={{ borderInlineEnd: 0 }}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            padding: '0 24px',
            background: colorBgContainer,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Typography.Text strong>Rehabilitation Device Certification Platform</Typography.Text>
          <Badge status="success" text="Operator Console" />
        </Header>
        <Content style={{ margin: 24 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
