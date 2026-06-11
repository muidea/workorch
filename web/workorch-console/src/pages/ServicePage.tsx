import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { App as AntApp, Button, Descriptions, Space } from 'antd';
import { CardGrid, PageContainer, ProCard } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { compactJSON, errorMessage } from '../utils';
import type { ServiceStatus } from '../types';

function statusDescriptionItems(status: ServiceStatus) {
  return Object.entries(status).map(([key, value]) => ({ key, label: key, children: renderStatusValue(value) }));
}

function renderStatusValue(value: unknown): ReactNode {
  if (value === undefined || value === null) {
    return '-';
  }
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  return <pre className="json-panel">{compactJSON(value)}</pre>;
}

export function ServicePage() {
  const { message } = AntApp.useApp();
  const [health, setHealth] = useState<ServiceStatus>({});
  const [ready, setReady] = useState<ServiceStatus>({});
  const [metrics, setMetrics] = useState('');
  const [loading, setLoading] = useState(false);

  const loadService = async () => {
    setLoading(true);
    try {
      const [nextHealth, nextReady, nextMetrics] = await Promise.all([
        workorchApi.healthz(),
        workorchApi.readyz(),
        workorchApi.metrics()
      ]);
      setHealth(nextHealth);
      setReady(nextReady);
      setMetrics(nextMetrics);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadService();
  }, []);

  return (
    <PageContainer
      title="服务状态"
      subTitle="查看 workorch 健康状态、就绪状态和运行指标"
      extra={<Button onClick={() => void loadService()} loading={loading}>刷新</Button>}
    >
      <CardGrid className="workorch-card-grid" loading={loading}>
        <ProCard title="健康检查" colSpan="50%">
          <Descriptions items={statusDescriptionItems(health)} column={1} />
        </ProCard>
        <ProCard title="就绪检查" colSpan="50%">
          <Descriptions items={statusDescriptionItems(ready)} column={1} />
        </ProCard>
      </CardGrid>
      <ProCard title={modelLabels.metrics} className="workorch-section">
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <pre className="metrics-panel">{metrics}</pre>
        </Space>
      </ProCard>
    </PageContainer>
  );
}
