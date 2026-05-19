import type { ReactNode } from 'react';
import { Alert, Skeleton } from 'antd';

interface LoadingStateProps {
  loading: boolean;
  error?: string;
  children: ReactNode;
}

/**
 * 加载状态组件
 * 统一处理加载、错误和正常状态的显示
 */
export function LoadingState({ loading, error, children }: LoadingStateProps) {
  if (loading) {
    return <Skeleton active />;
  }

  if (error) {
    return (
      <Alert
        type="error"
        message="加载失败"
        description={error}
        showIcon
      />
    );
  }

  return <>{children}</>;
}
