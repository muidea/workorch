import React, { lazy, Suspense, useState } from 'react';
import ReactDOM from 'react-dom/client';
import {
  CheckCircleOutlined,
  ClusterOutlined,
  DashboardOutlined,
  ExceptionOutlined,
  ProfileOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { ProLayout } from '@ant-design/pro-components';
import { App as AntApp, ConfigProvider, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { ServicePage } from './pages/ServicePage';
import { GovernanceQueuePage } from './pages/GovernanceQueuePage';
import { WorkspaceBoardPage } from './pages/WorkspaceBoardPage';
import './styles.css';

const RuntimeControlPage = lazy(() => import('./pages/RuntimeControlPage').then(m => ({ default: m.RuntimeControlPage })));
const ConfigurationPage = lazy(() => import('./pages/ConfigurationPage').then(m => ({ default: m.ConfigurationPage })));
const WorkspaceDetailPage = lazy(() => import('./pages/WorkspaceDetailPage').then(m => ({ default: m.WorkspaceDetailPage })));

type ViewKey = 'board' | 'workspaceDetail' | 'governance' | 'runtimeControl' | 'configuration' | 'service';

const routes = [
  { path: '/board', name: '业务总览', icon: <DashboardOutlined /> },
  { path: '/workspaceDetail', name: '工作空间', icon: <ProfileOutlined /> },
  { path: '/governance', name: '治理队列', icon: <ExceptionOutlined /> },
  { path: '/runtimeControl', name: '运行控制', icon: <ClusterOutlined /> },
  { path: '/configuration', name: '系统配置', icon: <SettingOutlined /> },
  { path: '/service', name: '服务状态', icon: <CheckCircleOutlined /> }
];

function ConsoleApp() {
  const [view, setView] = useState<ViewKey>('board');
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | undefined>(undefined);
  const renderContent = () => {
    switch (view) {
      case 'board':
        return <WorkspaceBoardPage onOpenWorkspace={(workspaceId) => {
          setSelectedWorkspaceId(workspaceId);
          setView('workspaceDetail');
        }} />;
      case 'governance':
        return <GovernanceQueuePage />;
      case 'workspaceDetail':
        return <WorkspaceDetailPage selectedWorkspaceId={selectedWorkspaceId} />;
      case 'runtimeControl':
        return <RuntimeControlPage />;
      case 'configuration':
        return <ConfigurationPage />;
      case 'service':
        return <ServicePage />;
      default:
        return <WorkspaceBoardPage onOpenWorkspace={(workspaceId) => {
          setSelectedWorkspaceId(workspaceId);
          setView('workspaceDetail');
        }} />;
    }
  };

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#1668dc',
          borderRadius: 8
        }
      }}
    >
      <AntApp>
        <ProLayout
          title="workorch 控制台"
          logo={false}
          route={{ path: '/', routes }}
          location={{ pathname: `/${view}` }}
          menuItemRender={(item, dom) => (
            <a
              onClick={(event) => {
                event.preventDefault();
                setView((item.path?.replace('/', '') as ViewKey) || 'board');
              }}
            >
              {dom}
            </a>
          )}
          layout="mix"
          contentWidth="Fluid"
        >
          <Suspense fallback={<div style={{ padding: 24, textAlign: 'center' }}>加载中...</div>}>
            {renderContent()}
          </Suspense>
        </ProLayout>
      </AntApp>
    </ConfigProvider>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConsoleApp />
  </React.StrictMode>
);
