import { Alert } from 'antd';

export function AuthoringFailedRecoveryNotice() {
  return <Alert type="warning" showIcon message="原请求已失败；再次编译不会重发该请求" description="刷新、重新打开页面或部署修复都不会替换已保存的失败结果，也不会取得新的 provider 调用许可。只有真实的目标、模板或资源引用修订才应保存为新输入并重新审查；修订保留失败历史，仍受既有预算限制。若仅修复了服务代码，请保留原诊断，另行显式新建验证事务，不要为了重试而制造无意义的输入变化。此操作不会启动 Run。" />;
}
