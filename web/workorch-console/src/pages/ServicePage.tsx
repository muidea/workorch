import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Space } from 'antd';
import { CardGrid, PageContainer, ProCard } from '../components/proCompat';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import { errorMessage } from '../utils';

export function ServicePage() {
  const { message } = AntApp.useApp();
  const [health, setHealth] = useState<Record<string, string>>({});
  const [ready, setReady] = useState<Record<string, string>>({});
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
          <Descriptions items={Object.entries(health).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
        <ProCard title="就绪检查" colSpan="50%">
          <Descriptions items={Object.entries(ready).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
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
