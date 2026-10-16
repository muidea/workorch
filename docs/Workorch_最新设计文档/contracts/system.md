# system：CF00生命周期合同

- 版本v0.3；日期2026-10-12；只读静态核对，未执行项目测试/服务/live
- 稳定设计ID：`CT-system-Lifecycle`（生命周期索引，不是源码topic）
- 源码：[internal/modules/kernel/system/biz/system.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go)
- source：existing；target：retain；topic/Command/Data/Result：无，不伪造消息补数

System当前是Module，固定SHA没有pkg/events接收topic。现有Start顺序调用Soul、WorkspaceTemplate、MCPRegistry三个EnsureDefaults；每次按实际typed结果校验。通用能力readiness聚合是目标，不能冒充已有System API，也不能把health readiness当M3或execution admission。

## 生命周期承诺

适用[CF00共同生命周期承诺](../Workorch_组件固定合同与分层验收规范.md#cf00-common-behavior)；本页只补本单元的真实装配、出站能力与无业务topic边界，不重复共享停止/预算/恢复规则。

## 实际消费的owner合同

- [Soul EnsureDefaults](soul.md#ct-soul-soulcommandensuredefaults)：[ensureSoulDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L71)
- [WorkspaceTemplate EnsureDefaults](workspace.md#ct-workspace-workspacetemplatecommandensuredefaults)：[ensureWorkspaceTemplateDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L87)
- [MCPRegistry EnsureDefaults](mcpregistry.md#ct-mcpregistry-mcpcommandensuredefaults)：[ensureMCPDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L104)
