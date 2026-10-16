# workorchruntime：CF00生命周期合同

- 版本v0.3；日期2026-10-12；只读静态核对，未执行项目测试/服务/live
- 稳定设计ID：`CT-workorchruntime-Lifecycle`（生命周期索引，不是源码topic）
- 源码：[internal/initiators/workorchruntime/module.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/initiators/workorchruntime/module.go)
- source：existing；target：retain；topic/Command/Data/Result：无，不伪造消息补数

workorchruntime是唯一Initiator，只拥有单一持久化基础能力的进程句柄与阶段生命周期，没有EventHub/Base或业务topic，不解释Run/Core状态，也不承担跨owner saga。

## 生命周期承诺

适用[CF00共同生命周期承诺](../Workorch_组件固定合同与分层验收规范.md#cf00-common-behavior)；本页只补本单元的真实装配、出站能力与无业务topic边界，不重复共享停止/预算/恢复规则。
