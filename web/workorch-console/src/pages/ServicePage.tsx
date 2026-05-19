import { useEffect, useState } from 'react';
import { Descriptions } from 'antd';
import { PageContainer, ProCard } from '@ant-design/pro-components';
import { workorchApi } from '../api';
import { modelLabels } from '../labels';

export function ServicePage() {
  const [health, setHealth] = useState<Record<string, string>>({});
  const [ready, setReady] = useState<Record<string, string>>({});
  const [metrics, setMetrics] = useState('');

  useEffect(() => {
    void workorchApi.healthz().then(setHealth);
    void workorchApi.readyz().then(setReady);
    void fetch('/metrics').then((response) => response.text()).then(setMetrics);
  }, []);

  return (
    <PageContainer title="服务状态" subTitle="查看 workorch 健康状态、就绪状态和运行指标">
      <ProCard gutter={16} wrap>
        <ProCard title="健康检查" colSpan="50%">
          <Descriptions items={Object.entries(health).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
        <ProCard title="就绪检查" colSpan="50%">
          <Descriptions items={Object.entries(ready).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
      </ProCard>
      <ProCard title={modelLabels.metrics} className="workorch-section">
        <pre className="metrics-panel">{metrics}</pre>
      </ProCard>
    </PageContainer>
  );
}
