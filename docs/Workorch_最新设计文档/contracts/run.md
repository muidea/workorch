# run：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页33个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Run是SM-1唯一语义owner；RuntimeState为typed RuntimeStateRecord，由Orchestrator解释SM-2/3/4语义，Run只负责合法scope、bootstrap/lifecycle fence和物理CAS。StoreRuntimeStateRecord只接受kernel/orchestrator来源，WorkspaceID/RunID非空、ExpectedRuntimeStateRevision非nil、RuntimeState非nil且对应当前bootstrap；任意Run目标状态都不是可接受外部命令。

物理Run记录revision与Runtime aggregate revision不同：最多4次physical CAS rebase，只在aggregate revision不变时允许；semantic mismatch返回AggregateRevisionMismatch而非重放旧计划。LifecycleFactInventory与recovery分页不漏项；开始/继续/终态/retirement由各自private ingress决定。旧Start身份在Run终态后仍不得再次物化新Run。

## 逐topic合同

<a id="ct-run-runblockedonapproval"></a>

### CT-run-RunBlockedOnApproval

- 边界：`TopicRunBlockedOnApproval` = `/run/blocked_on_approval`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runblockedondecision"></a>

### CT-run-RunBlockedOnDecision

- 边界：`TopicRunBlockedOnDecision` = `/run/blocked_on_decision`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runcancelled"></a>

### CT-run-RunCancelled

- 边界：`TopicRunCancelled` = `/run/cancelled`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runcommandfinalizeworkspaceretirement"></a>

### CT-run-RunCommandFinalizeWorkspaceRetirement

- 边界：`TopicRunCommandFinalizeWorkspaceRetirement` = `/run/command/finalize-workspace-retirement`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L47)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L452)
- 接收provider：run；Command [FinalizeWorkspaceRetirementCommand](#schema-finalizeworkspaceretirementcommand) → Result [FinalizeWorkspaceRetirementResult](#schema-finalizeworkspaceretirementresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workspace [driveWorkspaceDeletionRunRetired](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L183)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementCommandSource` 分支谓词（[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceRetirementCommandSource`；`err := cmd.Validate(); err != nil`；`err := result.ValidateFor(cmd); err != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-run-runcommandget"></a>

### CT-run-RunCommandGet

- 边界：`TopicRunCommandGet` = `/run/command/get`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L436)
- 接收provider：run；Command [GetRunCommand](#schema-getruncommand) → Result [GetRunResult](#schema-getrunresult)；target合法caller：workorchd、workspace、orchestrator、kernel/governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ReadRunByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_reads.go#L14)；workorchd [ReadRunByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_reads.go#L34)；governance [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L74)；governance [GetByWorkspaceID](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L94)；orchestrator [Read](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/state_event_adapter.go#L28)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.RunID == ""`；`cmd.WorkspaceID != ""`；`retired, err := m.workspaceRunsRetired(ctx, item.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-run-runcommandgetruntimecheckpoint"></a>

### CT-run-RunCommandGetRuntimeCheckpoint

- 边界：`TopicRunCommandGetRuntimeCheckpoint` = `/run/command/get-runtime-checkpoint`；CF09；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L43)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L448)
- 接收provider：run；Command [GetRuntimeCheckpointCommand](#schema-getruntimecheckpointcommand) → Result [GetRuntimeCheckpointResult](#schema-getruntimecheckpointresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ReadRuntimeCheckpointByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_checkpoint_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.CheckpointID == "" || m.checkpoints == nil`；`cmd.RunID != "" && cmd.RunID != item.RunID`；`cmd.WorkspaceID != "" && item.WorkspaceID != cmd.WorkspaceID`；`err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandgetstep"></a>

### CT-run-RunCommandGetStep

- 边界：`TopicRunCommandGetStep` = `/run/command/get-step`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L40)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L445)
- 接收provider：run；Command [GetStepCommand](#schema-getstepcommand) → Result [GetStepResult](#schema-getstepresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ReadStepByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_step_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.StepID == "" || m.steps == nil`；`cmd.RunID != "" && cmd.RunID != item.RunID`；`err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-run-runcommandlifecyclefactinventory"></a>

### CT-run-RunCommandLifecycleFactInventory

- 边界：`TopicRunCommandLifecycleFactInventory` = `/run/command/lifecycle-fact-inventory`；CF09；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L30)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L439)
- 接收provider：run；Command [QueryRunLifecycleFactInventoryCommand](#schema-queryrunlifecyclefactinventorycommand) → Result [QueryRunLifecycleFactInventoryResult](#schema-queryrunlifecyclefactinventoryresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workspace [queryRunLifecycleFactInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/run_projection_reconciliation.go#L243)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandlist"></a>

### CT-run-RunCommandList

- 边界：`TopicRunCommandList` = `/run/command/list`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L24)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L437)
- 接收provider：run；Command [ListRunsCommand](#schema-listrunscommand) → Result [ListRunsResult](#schema-listrunsresult)；target合法caller：workorchd、workspace、orchestrator、kernel/governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ListRunsByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_reads.go#L51)；governance [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L113)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-run-runcommandlistbyworkspace"></a>

### CT-run-RunCommandListByWorkspace

- 边界：`TopicRunCommandListByWorkspace` = `/run/command/list-by-workspace`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L25)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L438)
- 接收provider：run；Command [ListRunsByWorkspaceCommand](#schema-listrunsbyworkspacecommand) → Result [ListRunsByWorkspaceResult](#schema-listrunsbyworkspaceresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ListRunsByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_reads.go#L64)；orchestrator [queryRunsForWorkspaceQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_runtime_quiescence.go#L206)；workspace [observeDefinitionChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_change_provenance.go#L18)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == ""`；`retired, err := m.workspaceRunsRetired(ctx, cmd.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-run-runcommandlistcheckpointsbyrun"></a>

### CT-run-RunCommandListCheckpointsByRun

- 边界：`TopicRunCommandListCheckpointsByRun` = `/run/command/list-runtime-checkpoints-by-run`；CF09；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L44)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L449)
- 接收provider：run；Command [ListRuntimeCheckpointsByRunCommand](#schema-listruntimecheckpointsbyruncommand) → Result [ListRuntimeCheckpointsByRunResult](#schema-listruntimecheckpointsbyrunresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ListRuntimeCheckpointsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_checkpoint_reads.go#L35)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.RunID == "" || m.checkpoints == nil`；`err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandliststepsummariesbyrun"></a>

### CT-run-RunCommandListStepSummariesByRun

- 边界：`TopicRunCommandListStepSummariesByRun` = `/run/command/list-step-summaries-by-run`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L42)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L447)
- 接收provider：run；Command [ListStepSummariesByRunCommand](#schema-liststepsummariesbyruncommand) → Result [ListStepSummariesByRunResult](#schema-liststepsummariesbyrunresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ListStepSummariesByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_step_reads.go#L53)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.RunID == "" || m.steps == nil`；`err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-run-runcommandliststepsbyrun"></a>

### CT-run-RunCommandListStepsByRun

- 边界：`TopicRunCommandListStepsByRun` = `/run/command/list-steps-by-run`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L41)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L446)
- 接收provider：run；Command [ListStepsByRunCommand](#schema-liststepsbyruncommand) → Result [ListStepsByRunResult](#schema-liststepsbyrunresult)；target合法caller：workorchd、workspace、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workorchd [ListStepsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/run_step_reads.go#L35)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.RunID == "" || m.steps == nil`；`err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-run-runcommandprepareworkspaceretirement"></a>

### CT-run-RunCommandPrepareWorkspaceRetirement

- 边界：`TopicRunCommandPrepareWorkspaceRetirement` = `/run/command/prepare-workspace-retirement`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L46)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L451)
- 接收provider：run；Command [PrepareWorkspaceRetirementCommand](#schema-prepareworkspaceretirementcommand) → Result [PrepareWorkspaceRetirementResult](#schema-prepareworkspaceretirementresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workspace [driveWorkspaceDeletionRunDraining](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L130)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementCommandSource` 分支谓词（[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceRetirementCommandSource`；`err := cmd.Validate(); err != nil`；`err := result.ValidateFor(cmd); err != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-run-runcommandpurgeworkspace"></a>

### CT-run-RunCommandPurgeWorkspace

- 边界：`TopicRunCommandPurgeWorkspace` = `/run/command/purge-workspace`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L48)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L453)
- 接收provider：run；Command [PurgeWorkspaceRunsCommand](#schema-purgeworkspacerunscommand) → Result [PurgeWorkspaceRunsResult](#schema-purgeworkspacerunsresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementCommandSource` 分支谓词（[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceRetirementCommandSource`；`err := cmd.Validate(); err != nil`；`internal.PurgeReceipt == nil || internal.PurgeReceipt.ValidateFor(runWorkspaceRetirementFenceToModel(cmd.RetirementFence())) != nil`；`err := result.ValidateFor(cmd); err != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-run-runcommandruntimerecoveryinventory"></a>

### CT-run-RunCommandRuntimeRecoveryInventory

- 边界：`TopicRunCommandRuntimeRecoveryInventory` = `/run/command/runtime-recovery-inventory`；CF09；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L31)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L440)
- 接收provider：run；Command [QueryRuntimeRecoveryInventoryCommand](#schema-queryruntimerecoveryinventorycommand) → Result [QueryRuntimeRecoveryInventoryResult](#schema-queryruntimerecoveryinventoryresult)；target合法caller：kernel/orchestrator
- 实际注册：[prepareRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L153) → `handleRuntimeRecoveryInventory`（run）
- 接收入口：[handleRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L195)
- 已观察外部引用：orchestrator [queryRuntimeRecoveryInventoryBounded](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L833)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L195)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementFactQuerySource` 分支谓词（[handleRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L195)）
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`ev == nil`；`!ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource`；`_, fenced := retired[item.WorkspaceID]; !fenced`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandruntimestaterecord"></a>

### CT-run-RunCommandRuntimeStateRecord

- 边界：`TopicRunCommandRuntimeStateRecord` = `/run/command/runtime-state-record`；CF09；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L39)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L444)
- 接收provider：run；Command [QueryRuntimeStateRecordCommand](#schema-queryruntimestaterecordcommand) → Result [QueryRuntimeStateRecordResult](#schema-queryruntimestaterecordresult)；target合法caller：kernel/orchestrator
- 实际注册：[prepareRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L153) → `handleRuntimeStateRecord`（run）
- 接收入口：[handleRuntimeStateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L303)
- 已观察外部引用：orchestrator [loadRuntimeStateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_state_repository.go#L35)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeStateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L303)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementFactQuerySource` 分支谓词（[handleRuntimeStateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L303)）
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`ev == nil`；`!ok || cmd.Validate() != nil || m == nil || ev.Source() != workspaceRetirementFactQuerySource`；`cmd.ExpectedRuntimeStateRevision != 0 && (cmd.ExpectedRuntimeStateRevision != response.RuntimeStateRevision || cmd.ExpectedRuntimeStateRef != response.RuntimeStateRef)`；`err := response.Validate(); err != nil`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandruntimestaterecordreadiness"></a>

### CT-run-RunCommandRuntimeStateRecordReadiness

- 边界：`TopicRunCommandRuntimeStateRecordReadiness` = `/run/command/runtime-state-record-readiness`；CF09；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L38)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L443)
- 接收provider：run；Command [QueryRuntimeStateRecordReadinessCommand](#schema-queryruntimestaterecordreadinesscommand) → Result [QueryRuntimeStateRecordReadinessResult](#schema-queryruntimestaterecordreadinessresult)；target合法caller：kernel/orchestrator
- 实际注册：[prepareRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L153) → `handleRuntimeStateRecordReadiness`（run）
- 接收入口：[handleRuntimeStateRecordReadiness](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L278)
- 已观察外部引用：orchestrator [queryRuntimeStateRecordReadiness](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L791)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeStateRecordReadiness](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L278)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementFactQuerySource` 分支谓词（[handleRuntimeStateRecordReadiness](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L278)）
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`ev == nil`；`!ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandstoreruntimestaterecord"></a>

### CT-run-RunCommandStoreRuntimeStateRecord

- 边界：`TopicRunCommandStoreRuntimeStateRecord` = `/run/command/store-runtime-state-record`；CF09；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L45)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L450)
- 接收provider：run；Command [StoreRuntimeStateRecordCommand](#schema-storeruntimestaterecordcommand) → Result [StoreRuntimeStateRecordResult](#schema-storeruntimestaterecordresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：orchestrator [SaveValidated](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_state_repository.go#L97)
- 本topic入口/具体类型分支的Source审查索引：[executeRuntimeStateStoreCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/runtime_state_commands.go#L14)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "kernel/orchestrator"` 分支谓词（[executeRuntimeStateStoreCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/runtime_state_commands.go#L14)）
- 特化承诺：只kernel/orchestrator可写typed RuntimeStateRecord（Run不拥有其SM-2/3/4语义）；ExpectedRuntimeStateRevision指针不能nil（0仅首次合法aggregate）；runtime revision mismatch不得用physical rebase掩盖，最多4次rebase后OwnerTransient。响应丢失query同Run/ref/revision；不解释SM-2/3/4。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.RunID == "" || cmd.WorkspaceID == "" || ev.Source() != "kernel/orchestrator"`；`current == nil || current.ID != cmd.RunID || current.WorkspaceID != cmd.WorkspaceID`；`current.RuntimeBootstrap == nil || cmd.RuntimeState == nil || cmd.ExpectedRuntimeStateRevision == nil`；`cmd.LifecycleFence != nil`；`err := fence.Validate(); err != nil || fence.WorkspaceID != cmd.WorkspaceID || fence.RunID != cmd.RunID || fence.RuntimeStateRevision != cmd.RuntimeState.StateRevision || fence.RuntimeStateRef != runevents.RuntimeStateRef(cmd.WorkspaceID, cmd.RunID, cmd.RuntimeState.StateRevision) || fence.RuntimeStateHash != runtimemodel.StableRuntimeStateRecordHash(*cmd.RuntimeState) || fence.BootstrapHash != runtimemodel.StableRuntimeBootstrapHash(*current.RuntimeBootstrap)`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-run-runcommandworkspaceexecutionintent"></a>

### CT-run-RunCommandWorkspaceExecutionIntent

- 边界：`TopicRunCommandWorkspaceExecutionIntent` = `/run/command/workspace-execution-intent`；CF02；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L57)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L454)
- 接收provider：run；Command [WorkspaceExecutionIntentCommand](#schema-workspaceexecutionintentcommand) → Result [WorkspaceExecutionIntentResult](#schema-workspaceexecutionintentresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleWorkspaceExecutionIntent`（run）
- 接收入口：[handleWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go#L25)
- 已观察外部引用：workspace [sendRunExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/execution_intent.go#L213)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go#L45)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceExecutionIntentRunSource` 分支谓词（[executeWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go#L45)）
- 特化承诺：[CS10](../Workorch_组件固定合同与分层验收规范.md#cs10)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`err := response.Validate(); err != nil`；`m == nil || ev == nil`；`!ok || ev.Source() != workspaceExecutionIntentRunSource`；`err := cmd.Validate(); err != nil`
- 验收适用集：[CP06](../Workorch_组件固定合同与分层验收规范.md#cp06)

<a id="ct-run-runcommandworkspaceretirementfact"></a>

### CT-run-RunCommandWorkspaceRetirementFact

- 边界：`TopicRunCommandWorkspaceRetirementFact` = `/run/command/workspace-retirement-fact`；CF12；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L32)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L441)
- 接收provider：run；Command [QueryWorkspaceRetirementFactCommand](#schema-queryworkspaceretirementfactcommand) → Result [QueryWorkspaceRetirementFactResult](#schema-queryworkspaceretirementfactresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleCommand`（run）；[prepareRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L153) → `handleCommand`（run）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)；[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/command_router.go#L68)
- 已观察外部引用：orchestrator [queryRunWorkspaceRetirementFact](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L248)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementFactQuerySource` 分支谓词（[executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceRetirementFactQuerySource`；`err := command.Validate(); err != nil`；`command.DeletionID != "" && fence.DeletionID != command.DeletionID`；`err := runmodel.WorkspaceRetirementEligible(items, command.WorkspaceID); err != nil`；`err := runmodel.ValidateWorkspaceRetirementRuntimePhysicalState(item, command.WorkspaceID); err != nil`；`err := result.ValidateFor(command); err != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-run-runcommandworkspaceretirementinventory"></a>

### CT-run-RunCommandWorkspaceRetirementInventory

- 边界：`TopicRunCommandWorkspaceRetirementInventory` = `/run/command/workspace-retirement-inventory`；CF12；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L33)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L442)
- 接收provider：run；Command [QueryWorkspaceRetirementInventoryCommand](#schema-queryworkspaceretirementinventorycommand) → Result [QueryWorkspaceRetirementInventoryResult](#schema-queryworkspaceretirementinventoryresult)；target合法caller：kernel/orchestrator
- 实际注册：[prepareRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L153) → `handleWorkspaceRetirementInventory`（run）
- 接收入口：[handleWorkspaceRetirementInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L250)
- 已观察外部引用：orchestrator [queryRunWorkspaceRetirementInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L288)
- 本topic入口/具体类型分支的Source审查索引：[handleWorkspaceRetirementInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L250)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementFactQuerySource` 分支谓词（[handleWorkspaceRetirementInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L250)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-run-runcompleted"></a>

### CT-run-RunCompleted

- 边界：`TopicRunCompleted` = `/run/completed`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runcontinuationrequested"></a>

### CT-run-RunContinuationRequested

- 边界：`TopicRunContinuationRequested` = `/run/continuation-requested`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L472)
- 唯一事实owner/发布者：run；Data [RunContinuationRequestedData](#schema-runcontinuationrequesteddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRunContinuationRequested`（orchestrator）
- 已读事实来源引用（含发布/分派路径）：[publishContinuationRequested](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L229)
- 已观察外部引用：orchestrator [subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runcreated"></a>

### CT-run-RunCreated

- 边界：`TopicRunCreated` = `/run/created`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、process/workorch、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：workorchd [enrichSSEEnvelope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/sse_enrichment.go#L15)；workorchd [isSelectableWorkspaceRunEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_run_event_filter.go#L10)；workorchd [latestWorkspaceOutcomeRunEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_outcome_event_filter.go#L11)；workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runfactcontinuationeligible"></a>

### CT-run-RunFactContinuationEligible

- 边界：`TopicRunFactContinuationEligible` = `/run/fact/continuation-eligible`；CF02；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L53)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L456)
- 接收provider：run；Command [RuntimeContinuationEligibleFact](#schema-runtimecontinuationeligiblefact) → Result [RunLifecycleCommandResult](#schema-runlifecyclecommandresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleLifecycleCommand`（run）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L33)
- 已观察外部引用：orchestrator [applyRunContinuationEligible](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/lifecycle_fact_adapter.go#L85)
- 本topic入口/具体类型分支的Source审查索引：[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimeLifecycleFactSource` 分支谓词（[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)）
- 特化承诺：[CS10](../Workorch_组件固定合同与分层验收规范.md#cs10)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != runtimeLifecycleFactSource`
- 验收适用集：[CP06](../Workorch_组件固定合同与分层验收规范.md#cp06)

<a id="ct-run-runfactgovernancewait"></a>

### CT-run-RunFactGovernanceWait

- 边界：`TopicRunFactGovernanceWait` = `/run/fact/governance-wait`；CF02；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L52)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L455)
- 接收provider：run；Command [RuntimeGovernanceWaitFact](#schema-runtimegovernancewaitfact) → Result [RunLifecycleCommandResult](#schema-runlifecyclecommandresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleLifecycleCommand`（run）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L33)
- 已观察外部引用：orchestrator [applyRunGovernanceWait](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/lifecycle_fact_adapter.go#L20)
- 本topic入口/具体类型分支的Source审查索引：[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimeLifecycleFactSource` 分支谓词（[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)）
- 特化承诺：[CS10](../Workorch_组件固定合同与分层验收规范.md#cs10)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != runtimeLifecycleFactSource`
- 验收适用集：[CP06](../Workorch_组件固定合同与分层验收规范.md#cp06)

<a id="ct-run-runfactterminaldelivery"></a>

### CT-run-RunFactTerminalDelivery

- 边界：`TopicRunFactTerminalDelivery` = `/run/fact/terminal-delivery`；CF10；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L54)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L457)
- 接收provider：run；Command [RuntimeTerminalDeliveryFact](#schema-runtimeterminaldeliveryfact) → Result [RunLifecycleCommandResult](#schema-runlifecyclecommandresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L108) → `handleLifecycleCommand`（run）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L33)
- 已观察外部引用：orchestrator [deliverRuntimeRun](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go#L185)
- 本topic入口/具体类型分支的Source审查索引：[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimeLifecycleFactSource` 分支谓词（[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != runtimeLifecycleFactSource`；`m == nil || event.WorkspaceID == "" || event.RunID == ""`；`cmd.AuthoritySnapshotID == "" || cmd.DefinitionRevision == "" || cmd.EvidenceRevision == ""`；`fence.TerminalFenceID != cmd.TerminalDeliveryID || fence.WorkspaceID != cmd.WorkspaceID || fence.RunID != cmd.RunID || fence.RuntimeStateRevision != current.RuntimeState.StateRevision || fence.RuntimeStateRef != runevents.RuntimeStateRef(current.WorkspaceID, current.ID, current.RuntimeState.StateRevision)`
- 验收适用集：[CP04](../Workorch_组件固定合同与分层验收规范.md#cp04)

<a id="ct-run-runfailed"></a>

### CT-run-RunFailed

- 边界：`TopicRunFailed` = `/run/failed`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runruntimecommandsready"></a>

### CT-run-RunRuntimeCommandsReady

- 边界：`TopicRunRuntimeCommandsReady` = `/run/runtime-commands-ready`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L473)
- 唯一事实owner/发布者：run；Data [RuntimeCommandsReadyData](#schema-runtimecommandsreadydata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRunRuntimeCommandsReady`（orchestrator）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleRunRuntimeCommandsReady`（workspace）
- 已读事实来源引用（含发布/分派路径）：[publishRuntimeCommandsReady](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L363)
- 已观察外部引用：orchestrator [subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269)；workspace [subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner_lifecycle.go#L50)；workspace [Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runstarted"></a>

### CT-run-RunStarted

- 边界：`TopicRunStarted` = `/run/started`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runstatechanged"></a>

### CT-run-RunStateChanged

- 边界：`TopicRunStateChanged` = `/run/state/changed`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、governance、orchestrator、process/workorch、workorchd、workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L148) → `Start`（governance）；[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRunStateChanged`（orchestrator）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleRunStateChanged`（workspace）；[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）；[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53) → `handleRunStateChanged`（workorchd）
- 已读事实来源引用（含发布/分派路径）：[publishLifecycleTransition](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L208)
- 已观察外部引用：workorchd [enrichSSEEnvelope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/sse_enrichment.go#L15)；workorchd [isEventStreamCriticalTopic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/server_stream.go#L74)；workorchd [Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53)；workorchd [isSelectableWorkspaceRunEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_run_event_filter.go#L10)；workorchd [latestWorkspaceOutcomeRunEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_outcome_event_filter.go#L11)；workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；governance [Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L148)；orchestrator [subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269)；workspace [subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner_lifecycle.go#L50)；workspace [Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86)；eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-run-runsuperseded"></a>

### CT-run-RunSuperseded

- 边界：`TopicRunSuperseded` = `/run/superseded`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：run；Data [RunStateChangedData](#schema-runstatechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、orchestrator、workorchd、workspace
- 实际注册：[subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448) → `subscribeRunStateNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeRunStateNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L448)；eventstream [validRunStateNotification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L582)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-finalizeworkspaceretirementcommand"></a>

### FinalizeWorkspaceRetirementCommand

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:330](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L330)
- 字段：`QuiescenceToken deletioncontract.WorkspaceRuntimeQuiescenceToken`
- 字段引用类型：[WorkspaceRuntimeQuiescenceToken（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/runtime_quiescence.go#L65)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L334)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L339)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L347)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil || r.Fence.Validate() != nil`；`r.Fence.WorkspaceID != token.WorkspaceID || r.Fence.DeletionID != token.DeletionID || r.Fence.DeletionGeneration != token.DeletionGeneration || r.Fence.DataDispositionIntent != token.DataDispositionIntent || r.Fence.WorkspaceIntentHash != token.WorkspaceIntentHash || r.Fence.Phase != runmodel.WorkspaceRetirementPhaseRetired || r.Fence.FenceRevision != token.RunDrainingRevision+1 || r.Fence.WorkspaceRunGeneration != token.WorkspaceRunGeneration || r.Fence.QuiescenceID != token.QuiescenceID || r.Fence.RuntimeInventoryHash != token.RuntimeInventoryHash`

<a id="schema-finalizeworkspaceretirementresult"></a>

### FinalizeWorkspaceRetirementResult

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:343](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L343)
- 字段：`Fence RunWorkspaceRetirementFence`
- 字段引用类型：[RunWorkspaceRetirementFence](#schema-runworkspaceretirementfence)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L347)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil || r.Fence.Validate() != nil`；`r.Fence.WorkspaceID != token.WorkspaceID || r.Fence.DeletionID != token.DeletionID || r.Fence.DeletionGeneration != token.DeletionGeneration || r.Fence.DataDispositionIntent != token.DataDispositionIntent || r.Fence.WorkspaceIntentHash != token.WorkspaceIntentHash || r.Fence.Phase != runmodel.WorkspaceRetirementPhaseRetired || r.Fence.FenceRevision != token.RunDrainingRevision+1 || r.Fence.WorkspaceRunGeneration != token.WorkspaceRunGeneration || r.Fence.QuiescenceID != token.QuiescenceID || r.Fence.RuntimeInventoryHash != token.RuntimeInventoryHash`

<a id="schema-getruncommand"></a>

### GetRunCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L24)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-getrunresult"></a>

### GetRunResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:376](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L376)
- 字段：`Run *runmodel.Run`
- 字段引用类型：[Run](#schema-run)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L392)

<a id="schema-getruntimecheckpointcommand"></a>

### GetRuntimeCheckpointCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:210](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L210)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`CheckpointID common.CheckpointID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[CheckpointID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L11)

<a id="schema-getruntimecheckpointresult"></a>

### GetRuntimeCheckpointResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:382](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L382)
- 字段：`Checkpoint *runmodel.RuntimeCheckpoint`
- 字段引用类型：[RuntimeCheckpoint](#schema-runtimecheckpoint)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L416)

<a id="schema-getstepcommand"></a>

### GetStepCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:197](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L197)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)

<a id="schema-getstepresult"></a>

### GetStepResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:379](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L379)
- 字段：`Step *runmodel.Step`
- 字段引用类型：[Step](#schema-step)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L404)

<a id="schema-listrunsbyworkspacecommand"></a>

### ListRunsByWorkspaceCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:34](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L34)
- 字段：`WorkspaceID common.WorkspaceID`；`States []common.RunState`；`Compact bool`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)

<a id="schema-listrunsbyworkspaceresult"></a>

### ListRunsByWorkspaceResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:378](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L378)
- 字段：`Runs []*runmodel.Run`
- 字段引用类型：[Run](#schema-run)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L400)

<a id="schema-listrunscommand"></a>

### ListRunsCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:29](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L29)
- 字段：`States []common.RunState`；`Compact bool`
- 字段引用类型：[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)

<a id="schema-listrunsresult"></a>

### ListRunsResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:377](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L377)
- 字段：`Runs []*runmodel.Run`
- 字段引用类型：[Run](#schema-run)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L396)

<a id="schema-listruntimecheckpointsbyruncommand"></a>

### ListRuntimeCheckpointsByRunCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:216](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L216)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-listruntimecheckpointsbyrunresult"></a>

### ListRuntimeCheckpointsByRunResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:383](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L383)
- 字段：`Checkpoints []*runmodel.RuntimeCheckpoint`
- 字段引用类型：[RuntimeCheckpoint](#schema-runtimecheckpoint)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L420)

<a id="schema-liststepsummariesbyruncommand"></a>

### ListStepSummariesByRunCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:208](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L208)
- 实际alias/同形类型：[ListStepsByRunCommand](#schema-liststepsbyruncommand)
- 字段引用类型：[ListStepsByRunCommand](#schema-liststepsbyruncommand)

<a id="schema-liststepsummariesbyrunresult"></a>

### ListStepSummariesByRunResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:381](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L381)
- 字段：`Steps []*runmodel.Step`
- 字段引用类型：[Step](#schema-step)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L412)

<a id="schema-liststepsbyruncommand"></a>

### ListStepsByRunCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:203](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L203)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-liststepsbyrunresult"></a>

### ListStepsByRunResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:380](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L380)
- 字段：`Steps []*runmodel.Step`
- 字段引用类型：[Step](#schema-step)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L408)

<a id="schema-prepareworkspaceretirementcommand"></a>

### PrepareWorkspaceRetirementCommand

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:57](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L57)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`WorkspaceIntentHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L65)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L73)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L93)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.DeletionID == "" || c.DeletionGeneration != 1`；`c.WorkspaceIntentHash == ""`；`err := command.Validate(); err != nil`；`err := r.Fence.Validate(); err != nil`；`r.Fence.WorkspaceID != command.WorkspaceID || r.Fence.DeletionID != command.DeletionID || r.Fence.DeletionGeneration != command.DeletionGeneration || r.Fence.DataDispositionIntent != command.DataDispositionIntent || r.Fence.WorkspaceIntentHash != command.WorkspaceIntentHash`
- 显式分支集合（只表列源码case，default语义见validator）：`deletioncontract.ModeRetain, deletioncontract.ModePurge`

<a id="schema-prepareworkspaceretirementresult"></a>

### PrepareWorkspaceRetirementResult

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:89](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L89)
- 字段：`Fence RunWorkspaceRetirementFence`
- 字段引用类型：[RunWorkspaceRetirementFence](#schema-runworkspaceretirementfence)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L93)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`err := r.Fence.Validate(); err != nil`；`r.Fence.WorkspaceID != command.WorkspaceID || r.Fence.DeletionID != command.DeletionID || r.Fence.DeletionGeneration != command.DeletionGeneration || r.Fence.DataDispositionIntent != command.DataDispositionIntent || r.Fence.WorkspaceIntentHash != command.WorkspaceIntentHash`

<a id="schema-purgeworkspacerunscommand"></a>

### PurgeWorkspaceRunsCommand

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:228](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L228)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`WorkspaceIntentHash string`；`FenceRevision uint64`；`WorkspaceRunGeneration uint64`；`RuntimeInventoryHash string`；`QuiescenceID string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L255)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L266)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L292)
- 空值/identity/版本/范围原式：`err := c.RetirementFence().Validate(); err != nil || c.QuiescenceID == "" || c.DataDispositionIntent != deletioncontract.ModePurge`；`err := command.Validate(); err != nil`；`r.WorkspaceID != command.WorkspaceID || r.DeletionID != command.DeletionID || r.DeletionGeneration != command.DeletionGeneration || r.FenceRevision != command.FenceRevision || r.WorkspaceRunGeneration != command.WorkspaceRunGeneration || r.QuiescenceID != command.QuiescenceID || r.PurgeID != StableRunWorkspacePurgeID(command) || runmodel.ValidateEmptyWorkspaceRetirementPurgeObservations(r.Observations) != nil || r.PurgeProofHash != StableRunWorkspacePurgeProofHashForObservations(command, r.Observations) || r.AckHash != StableRunWorkspacePurgeAckHashForObservations(command, r.Observations)`

<a id="schema-purgeworkspacerunsresult"></a>

### PurgeWorkspaceRunsResult

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:274](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L274)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceRevision uint64`；`WorkspaceRunGeneration uint64`；`QuiescenceID string`；`PurgeID string`；`AckHash string`；`PurgeProofHash string`；`Observations []runmodel.WorkspaceRetirementPurgeObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[WorkspaceRetirementPurgeObservation](#schema-workspaceretirementpurgeobservation)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L287)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L292)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != command.WorkspaceID || r.DeletionID != command.DeletionID || r.DeletionGeneration != command.DeletionGeneration || r.FenceRevision != command.FenceRevision || r.WorkspaceRunGeneration != command.WorkspaceRunGeneration || r.QuiescenceID != command.QuiescenceID || r.PurgeID != StableRunWorkspacePurgeID(command) || runmodel.ValidateEmptyWorkspaceRetirementPurgeObservations(r.Observations) != nil || r.PurgeProofHash != StableRunWorkspacePurgeProofHashForObservations(command, r.Observations) || r.AckHash != StableRunWorkspacePurgeAckHashForObservations(command, r.Observations)`

<a id="schema-queryrunlifecyclefactinventorycommand"></a>

### QueryRunLifecycleFactInventoryCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:43](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L43)
- 字段：`Cursor string`；`Limit int`

<a id="schema-queryrunlifecyclefactinventoryresult"></a>

### QueryRunLifecycleFactInventoryResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:48](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L48)
- 字段：`Items []RunStateChangedData`；`NextCursor string`
- 字段引用类型：[RunStateChangedData](#schema-runstatechangeddata)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L53)
- 空值/identity/版本/范围原式：`len(r.Items) > 500`；`err := item.Validate(); err != nil`；`previous != "" && key <= previous`；`r.NextCursor != "" && (len(r.Items) == 0 || r.NextCursor != previous)`
- 显式分支集合（只表列源码case，default语义见validator）：`common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval`

<a id="schema-queryruntimerecoveryinventorycommand"></a>

### QueryRuntimeRecoveryInventoryCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:79](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L79)
- 字段：`Cursor string`；`Limit int`

<a id="schema-queryruntimerecoveryinventoryresult"></a>

### QueryRuntimeRecoveryInventoryResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:94](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L94)
- 字段：`Items []RuntimeRecoveryInventoryItem`；`NextCursor string`
- 字段引用类型：[RuntimeRecoveryInventoryItem](#schema-runtimerecoveryinventoryitem)

<a id="schema-queryruntimestaterecordcommand"></a>

### QueryRuntimeStateRecordCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:112](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L112)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ExpectedRuntimeStateRef string`；`ExpectedRuntimeStateRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L119)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.RunID == ""`；`hasRef != hasRevision`；`hasRef && c.ExpectedRuntimeStateRef != RuntimeStateRef(c.WorkspaceID, c.RunID, c.ExpectedRuntimeStateRevision)`

<a id="schema-queryruntimestaterecordreadinesscommand"></a>

### QueryRuntimeStateRecordReadinessCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:102](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L102)
- 声明：`type QueryRuntimeStateRecordReadinessCommand struct{}`

<a id="schema-queryruntimestaterecordreadinessresult"></a>

### QueryRuntimeStateRecordReadinessResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:104](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L104)
- 字段：`Ready bool`

<a id="schema-queryruntimestaterecordresult"></a>

### QueryRuntimeStateRecordResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:137](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L137)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StatePresent bool`；`RuntimeStateRef string`；`RuntimeStateRevision uint64`；`RuntimeState *runtimecontract.RuntimeStateRecord`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeStateRecord（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_state.go#L294)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L146)
- 空值/identity/版本/范围原式：`r.WorkspaceID == "" || r.RunID == ""`；`!r.StatePresent`；`r.RuntimeState != nil || r.RuntimeStateRef != "" || r.RuntimeStateRevision != 0`；`r.RuntimeState == nil || r.RuntimeStateRevision == 0 || r.RuntimeState.StateRevision != r.RuntimeStateRevision || r.RuntimeStateRef != RuntimeStateRef(r.WorkspaceID, r.RunID, r.RuntimeStateRevision)`

<a id="schema-queryworkspaceretirementfactcommand"></a>

### QueryWorkspaceRetirementFactCommand

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:109](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L109)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L114)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L120)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L178)
- 空值/identity/版本/范围原式：`c.WorkspaceID == ""`；`err := command.Validate(); err != nil`；`err := r.Fence.Validate(); err != nil || r.Fence.WorkspaceID != command.WorkspaceID || (command.DeletionID != "" && r.Fence.DeletionID != command.DeletionID)`；`err := item.Validate(r.Fence.WorkspaceID); err != nil || item.WorkspaceRunGeneration <= previousGeneration || item.WorkspaceRunGeneration > r.Fence.WorkspaceRunGeneration`

<a id="schema-queryworkspaceretirementfactresult"></a>

### QueryWorkspaceRetirementFactResult

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:168](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L168)
- 字段：`Fence RunWorkspaceRetirementFence`；`Runs []RunWorkspaceRetirementRuntimeFact`
- 字段引用类型：[RunWorkspaceRetirementFence](#schema-runworkspaceretirementfence)；[RunWorkspaceRetirementRuntimeFact](#schema-runworkspaceretirementruntimefact)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L173)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L178)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`err := r.Fence.Validate(); err != nil || r.Fence.WorkspaceID != command.WorkspaceID || (command.DeletionID != "" && r.Fence.DeletionID != command.DeletionID)`；`err := item.Validate(r.Fence.WorkspaceID); err != nil || item.WorkspaceRunGeneration <= previousGeneration || item.WorkspaceRunGeneration > r.Fence.WorkspaceRunGeneration`

<a id="schema-queryworkspaceretirementinventorycommand"></a>

### QueryWorkspaceRetirementInventoryCommand

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:196](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L196)
- 字段：`Cursor string`；`Limit int`

<a id="schema-queryworkspaceretirementinventoryresult"></a>

### QueryWorkspaceRetirementInventoryResult

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:201](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L201)
- 字段：`Items []RunWorkspaceRetirementFence`；`NextCursor string`
- 字段引用类型：[RunWorkspaceRetirementFence](#schema-runworkspaceretirementfence)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L206)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L211)
- 空值/identity/版本/范围原式：`len(r.Items) > 500`；`err := item.Validate(); err != nil || (previous != "" && string(item.WorkspaceID) <= previous)`；`r.NextCursor != "" && (len(r.Items) == 0 || r.NextCursor != previous)`

<a id="schema-replacementintent"></a>

### ReplacementIntent

- 源码：[internal/modules/blocks/run/pkg/models/run.go:61](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L61)
- 字段：`RequestID string`；`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`；`DoDID common.DoDID`；`DefinitionRevision string`；`StartupInputHash string`；`ReplacementIntentID string`；`ReplacementIntentHash string`；`ClarificationTicketID common.TicketID`；`PredecessorStartupInputHash string`；`SuccessorStartupInputHash string`；`ClarificationResolutionID common.ResolutionID`；`ClarificationResolutionBindingHash string`；`ClarificationConflictKind string`；`ConflictingReplacementIntentID string`；`ConflictingReplacementIntentHash string`；`Bootstrap *runtimecontract.RuntimeBootstrap`；`ReconciliationScope runtimecontract.RuntimeCWIReconciliationScope`；`Kind runtimecontract.RuntimeTerminationKind`；`Reason string`；`State ReplacementIntentState`；`Recovery ReplacementRecoveryLease`；`TerminalDeliveryID string`；`RuntimeStateRevision uint64`；`SuccessorRunID common.RunID`；`AcknowledgedAt string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[RuntimeBootstrap（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_bootstrap.go#L12)；[RuntimeCWIReconciliationScope（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L21)；[RuntimeTerminationKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_repair_directive.go#L168)；[ReplacementIntentState](#schema-replacementintentstate)；[ReplacementRecoveryLease](#schema-replacementrecoverylease)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[ValidFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L90)；[validFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L165)；[StableReplacementIntentHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L194)；[CloneReplacementIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L328)
- 空值/identity/版本/范围原式：`predecessor == nil || strings.TrimSpace(i.RequestID) == "" || i.WorkspaceID == "" || i.WorkspaceID != predecessor.WorkspaceID || i.GoalID == "" || i.DoDID == "" || strings.TrimSpace(i.DefinitionRevision) == "" || strings.TrimSpace(i.StartupInputHash) == "" || i.Bootstrap == nil || !runtimecontract.ValidRuntimeTerminationKind(i.Kind) || strings.TrimSpace(i.Reason) == ""`；`runtimecontract.ValidateRuntimeBootstrapRaw(*i.Bootstrap) != nil`；`i.Kind != runtimecontract.RuntimeTerminationSuperseded || i.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull`；`definition.GoalID != i.GoalID || definition.DoDID != i.DoDID || definition.Revision != i.DefinitionRevision || definition.StartupInputHash != i.StartupInputHash`；`clarification`；`strings.TrimSpace(i.ReplacementIntentID) == "" || strings.TrimSpace(i.ReplacementIntentHash) == "" || i.ReplacementIntentHash != StableReplacementIntentHash(i) || strings.TrimSpace(i.PredecessorStartupInputHash) == "" || strings.TrimSpace(i.SuccessorStartupInputHash) == "" || i.SuccessorStartupInputHash != i.StartupInputHash || i.PredecessorStartupInputHash == i.SuccessorStartupInputHash || strings.TrimSpace(predecessor.DefinitionRevision) == "" || strings.TrimSpace(i.DefinitionRevision) == strings.TrimSpace(predecessor.DefinitionRevision)`；`i.ReplacementIntentID != "" || i.ReplacementIntentHash != "" || i.PredecessorStartupInputHash != "" || i.SuccessorStartupInputHash != "" || i.ClarificationResolutionID != "" || i.ClarificationResolutionBindingHash != "" || i.ClarificationConflictKind != "" || i.ConflictingReplacementIntentID != "" || i.ConflictingReplacementIntentHash != ""`；`!i.Recovery.validFor(i)`；`!clarification || i.ClarificationResolutionID != "" || i.ClarificationResolutionBindingHash == "" || i.SuccessorRunID != "" || strings.TrimSpace(i.TerminalDeliveryID) != "" || i.RuntimeStateRevision != 0 || strings.TrimSpace(i.AcknowledgedAt) != ""`；`i.Recovery.Stage != ReplacementRecoveryClarificationResolution || i.Recovery.State != ReplacementRecoveryCompleted`；`r.RequestID == "" || r.RequestID != intent.RequestID || r.IntentHash == "" || r.IntentHash != StableReplacementIntentHash(intent) || r.Generation == 0 || r.MaxAttempts == 0 || r.Attempt > r.MaxAttempts`；`i.Bootstrap == nil`
- 显式分支集合（只表列源码case，default语义见validator）：`ReplacementIntentPending`；`ReplacementIntentPendingClarificationResolution`；`ReplacementIntentClarificationResolutionBound`；`ReplacementIntentClarificationResolutionRejected`；`"scope_mismatch"`；`"ticket_already_bound"`；`ReplacementIntentTerminationAcknowledged`；`ReplacementIntentSuccessorCreated`；`ReplacementIntentCompleted`；`ReplacementRecoveryClarificationResolution, ReplacementRecoveryAdministrativeTermination, ReplacementRecoverySuccessorMaterialization`；`ReplacementRecoveryScheduled`；`ReplacementRecoveryRunning`；`ReplacementRecoveryExhausted`；`ReplacementRecoveryCompleted`

<a id="schema-replacementintentstate"></a>

### ReplacementIntentState

- 源码：[internal/modules/blocks/run/pkg/models/run.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L13)
- 声明：`type ReplacementIntentState string`
- 源码枚举值：`ReplacementIntentPending = "pending"`；`ReplacementIntentPendingClarificationResolution = "pending_clarification_resolution"`；`ReplacementIntentClarificationResolutionBound = "clarification_resolution_bound"`；`ReplacementIntentClarificationResolutionRejected = "clarification_resolution_rejected"`；`ReplacementIntentTerminationAcknowledged = "termination_acknowledged"`；`ReplacementIntentSuccessorCreated = "successor_created"`；`ReplacementIntentCompleted = "completed"`

<a id="schema-replacementrecoverylease"></a>

### ReplacementRecoveryLease

- 源码：[internal/modules/blocks/run/pkg/models/run.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L46)
- 字段：`RequestID string`；`IntentHash string`；`Stage ReplacementRecoveryStage`；`State ReplacementRecoveryState`；`Generation uint64`；`Attempt uint64`；`MaxAttempts uint64`；`DueAt string`；`LastError string`
- 字段引用类型：[ReplacementRecoveryStage](#schema-replacementrecoverystage)；[ReplacementRecoveryState](#schema-replacementrecoverystate)
- 实际方法：[validFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L165)
- 空值/identity/版本/范围原式：`r.RequestID == "" || r.RequestID != intent.RequestID || r.IntentHash == "" || r.IntentHash != StableReplacementIntentHash(intent) || r.Generation == 0 || r.MaxAttempts == 0 || r.Attempt > r.MaxAttempts`
- 显式分支集合（只表列源码case，default语义见validator）：`ReplacementRecoveryClarificationResolution, ReplacementRecoveryAdministrativeTermination, ReplacementRecoverySuccessorMaterialization`；`ReplacementRecoveryScheduled`；`ReplacementRecoveryRunning`；`ReplacementRecoveryExhausted`；`ReplacementRecoveryCompleted`

<a id="schema-replacementrecoverystage"></a>

### ReplacementRecoveryStage

- 源码：[internal/modules/blocks/run/pkg/models/run.go:25](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L25)
- 声明：`type ReplacementRecoveryStage string`
- 源码枚举值：`ReplacementRecoveryClarificationResolution = "clarification_resolution"`；`ReplacementRecoveryAdministrativeTermination = "administrative_termination"`；`ReplacementRecoverySuccessorMaterialization = "successor_materialization"`

<a id="schema-replacementrecoverystate"></a>

### ReplacementRecoveryState

- 源码：[internal/modules/blocks/run/pkg/models/run.go:33](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L33)
- 声明：`type ReplacementRecoveryState string`
- 源码枚举值：`ReplacementRecoveryScheduled = "scheduled"`；`ReplacementRecoveryRunning = "running"`；`ReplacementRecoveryExhausted = "exhausted"`；`ReplacementRecoveryCompleted = "completed"`

<a id="schema-run"></a>

### Run

- 源码：[internal/modules/blocks/run/pkg/models/run.go:211](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L211)
- 字段：`Revision uint64`；`WorkspaceRunGeneration uint64`；`ID common.RunID`；`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`；`DoDID common.DoDID`；`Lineage RunLineage`；`DefinitionRevision string`；`StartupInputHash string`；`InitialStepKind string`；`RuntimeBootstrap *runtimecontract.RuntimeBootstrap`；`ActivationRequestID string`；`ReplacementIntent *ReplacementIntent`；`RetiredReplacementIntents []ReplacementIntent`；`ReconciliationRequestID string`；`ReconciliationScope runtimecontract.RuntimeCWIReconciliationScope`；`RuntimeState *runtimecontract.RuntimeStateRecord`；`RuntimeLifecycleFence *runtimecontract.RuntimeOpaqueLifecycleFence`；`State common.RunState`；`BlockingTicketID common.TicketID`；`BlockingApprovalID common.ApprovalID`；`FailureReason string`；`CreatedAt string`；`StartedAt string`；`EndedAt string`；`TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity`；`TerminalResultID common.ResultID`
- 字段引用类型：[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[RunLineage](#schema-runlineage)；[RuntimeBootstrap（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_bootstrap.go#L12)；[ReplacementIntent](#schema-replacementintent)；[RuntimeCWIReconciliationScope（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L21)；[RuntimeStateRecord（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_state.go#L294)；[RuntimeOpaqueLifecycleFence（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_owner_identity.go#L131)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[RuntimeTerminalDeliveryIdentity（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L35)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)
- 实际方法：[ValidateOpaqueRuntimeStateCAS](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/runtime_state_cas.go#L14)；[ValidateWorkspaceRetirementRuntimePhysicalState](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/workspace_retirement.go#L203)；[ValidateWorkspaceRetirementFinalization](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/workspace_retirement.go#L231)；[ValidFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L90)；[CloneRun](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L294)；[CloneRuns](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L337)；[ValidateRunRuntimeBootstrapsRaw](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L352)
- 空值/identity/版本/范围原式：`current == nil || next == nil || current.ID == "" || current.WorkspaceID == "" || current.ID != next.ID || current.WorkspaceID != next.WorkspaceID || current.Revision != next.Revision || current.WorkspaceRunGeneration == 0 || current.WorkspaceRunGeneration != next.WorkspaceRunGeneration || next.RuntimeState == nil`；`current.RuntimeState != nil`；`currentRuntimeRevision != expectedRuntimeStateRevision || next.RuntimeState.StateRevision != expectedRuntimeStateRevision+1`；`!reflect.DeepEqual(expected, next)`；`item == nil || workspaceID == "" || item.WorkspaceID != workspaceID || item.ID == "" || item.WorkspaceRunGeneration == 0 || item.Revision == 0 || !workspaceRetirementTerminalRunState(item.State)`；`item.RuntimeState == nil`；`item.RuntimeLifecycleFence != nil || item.TerminalDelivery != nil`；`item.RuntimeState.StateRevision == 0 || item.RuntimeLifecycleFence == nil || item.TerminalDelivery == nil || strings.TrimSpace(item.TerminalDelivery.TerminalDeliveryID) == ""`；`lifecycle.Validate() != nil || lifecycle.WorkspaceID != workspaceID || lifecycle.RunID != item.ID || lifecycle.RuntimeStateRevision != item.RuntimeState.StateRevision || lifecycle.RuntimeStateRef != expectedRef || lifecycle.RuntimeStateHash != expectedHash || lifecycle.TerminalFenceID != strings.TrimSpace(item.TerminalDelivery.TerminalDeliveryID)`；`err := fence.Validate(); err != nil || !fence.IsDraining() || token.Validate() != nil`；`fence.WorkspaceID != token.WorkspaceID || fence.DeletionID != token.DeletionID || fence.DeletionGeneration != token.DeletionGeneration || fence.DataDispositionIntent != token.DataDispositionIntent || fence.WorkspaceIntentHash != token.WorkspaceIntentHash || fence.FenceRevision != token.RunDrainingRevision || fence.WorkspaceRunGeneration != token.WorkspaceRunGeneration`；`err := WorkspaceRetirementEligible(items, fence.WorkspaceID); err != nil`；`items[i].WorkspaceRunGeneration != items[j].WorkspaceRunGeneration`；`len(items) != len(token.Runs)`；`item == nil || item.ID != fact.RunID || item.WorkspaceRunGeneration != fact.WorkspaceRunGeneration || item.Revision != fact.RunRevision || item.State != fact.RunState`；`err := ValidateWorkspaceRetirementRuntimePhysicalState(item, fence.WorkspaceID); err != nil`；`fact.RuntimeStatePresent || item.RuntimeLifecycleFence != nil || item.TerminalDelivery != nil`；`!fact.RuntimeStatePresent || item.RuntimeState.StateRevision != fact.RuntimeStateRevision || fact.RuntimeStateRef != fmt.Sprintf("runtime-state/%s/%s/%d", fence.WorkspaceID, item.ID, item.RuntimeState.StateRevision) || fact.RuntimeStateHash != runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState) || !fact.TerminalDeliveryAcked || item.TerminalDelivery == nil || item.TerminalDelivery.TerminalDeliveryID != fact.TerminalDeliveryID || item.RuntimeLifecycleFence == nil`；`lifecycle.WorkspaceID != fence.WorkspaceID || lifecycle.RunID != item.ID || lifecycle.RuntimeStateRevision != fact.RuntimeStateRevision || lifecycle.RuntimeStateRef != fact.RuntimeStateRef || lifecycle.RuntimeStateHash != fact.RuntimeStateHash || lifecycle.TerminalFenceID != fact.TerminalDeliveryID`；`predecessor == nil || strings.TrimSpace(i.RequestID) == "" || i.WorkspaceID == "" || i.WorkspaceID != predecessor.WorkspaceID || i.GoalID == "" || i.DoDID == "" || strings.TrimSpace(i.DefinitionRevision) == "" || strings.TrimSpace(i.StartupInputHash) == "" || i.Bootstrap == nil || !runtimecontract.ValidRuntimeTerminationKind(i.Kind) || strings.TrimSpace(i.Reason) == ""`；`runtimecontract.ValidateRuntimeBootstrapRaw(*i.Bootstrap) != nil`；`i.Kind != runtimecontract.RuntimeTerminationSuperseded || i.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull`；`definition.GoalID != i.GoalID || definition.DoDID != i.DoDID || definition.Revision != i.DefinitionRevision || definition.StartupInputHash != i.StartupInputHash`；`clarification`；`strings.TrimSpace(i.ReplacementIntentID) == "" || strings.TrimSpace(i.ReplacementIntentHash) == "" || i.ReplacementIntentHash != StableReplacementIntentHash(i) || strings.TrimSpace(i.PredecessorStartupInputHash) == "" || strings.TrimSpace(i.SuccessorStartupInputHash) == "" || i.SuccessorStartupInputHash != i.StartupInputHash || i.PredecessorStartupInputHash == i.SuccessorStartupInputHash || strings.TrimSpace(predecessor.DefinitionRevision) == "" || strings.TrimSpace(i.DefinitionRevision) == strings.TrimSpace(predecessor.DefinitionRevision)`；`i.ReplacementIntentID != "" || i.ReplacementIntentHash != "" || i.PredecessorStartupInputHash != "" || i.SuccessorStartupInputHash != "" || i.ClarificationResolutionID != "" || i.ClarificationResolutionBindingHash != "" || i.ClarificationConflictKind != "" || i.ConflictingReplacementIntentID != "" || i.ConflictingReplacementIntentHash != ""`；`!i.Recovery.validFor(i)`；`!clarification || i.ClarificationResolutionID != "" || i.ClarificationResolutionBindingHash == "" || i.SuccessorRunID != "" || strings.TrimSpace(i.TerminalDeliveryID) != "" || i.RuntimeStateRevision != 0 || strings.TrimSpace(i.AcknowledgedAt) != ""`；`i.Recovery.Stage != ReplacementRecoveryClarificationResolution || i.Recovery.State != ReplacementRecoveryCompleted`；`item == nil`；`bootstrap == nil`；`err := runtimecontract.ValidateRuntimeBootstrapRaw(*bootstrap); err != nil`；`err := validate("runtime bootstrap", item.RuntimeBootstrap); err != nil`；`item.ReplacementIntent != nil`；`err := validate("replacement bootstrap", item.ReplacementIntent.Bootstrap); err != nil`；`err := validate(fmt.Sprintf("retired replacement bootstrap %d", idx), item.RetiredReplacementIntents[idx].Bootstrap); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`ReplacementIntentPending`；`ReplacementIntentPendingClarificationResolution`；`ReplacementIntentClarificationResolutionBound`；`ReplacementIntentClarificationResolutionRejected`；`"scope_mismatch"`；`"ticket_already_bound"`；`ReplacementIntentTerminationAcknowledged`；`ReplacementIntentSuccessorCreated`；`ReplacementIntentCompleted`

<a id="schema-runcontinuationrequesteddata"></a>

### RunContinuationRequestedData

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:253](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L253)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`CausationID string`；`RequestID string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L278)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L286)
- 空值/identity/版本/范围原式：`d.WorkspaceID == "" || d.RunID == "" || d.CausationID == "" || d.RequestID == ""`；`d.RequestID == "" && d.CausationID != ""`

<a id="schema-runlifecyclecommandresult"></a>

### RunLifecycleCommandResult

- 源码：[internal/modules/blocks/run/pkg/events/lifecycle_messages.go:223](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L223)
- 字段：`Run *runmodel.Run`；`Transitioned bool`；`PreviousState common.RunState`；`DeliveryAck *runtimecontract.RuntimeDeliveryAcknowledgement`
- 字段引用类型：[Run](#schema-run)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)；[RuntimeDeliveryAcknowledgement（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L160)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L230)

<a id="schema-runlineage"></a>

### RunLineage

- 源码：[internal/modules/blocks/run/pkg/models/run.go:268](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L268)
- 字段：`PredecessorRunID common.RunID`；`PredecessorResultID common.ResultID`；`SuccessorRunID common.RunID`；`LineageState RunLineageState`
- 字段引用类型：[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)；[RunLineageState](#schema-runlineagestate)

<a id="schema-runlineagestate"></a>

### RunLineageState

- 源码：[internal/modules/blocks/run/pkg/models/run.go:277](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/run.go#L277)
- 声明：`type RunLineageState string`
- 源码枚举值：`RunLineageStateActive = "active"`；`RunLineageStateSuperseded = "superseded"`

<a id="schema-runstatechangeddata"></a>

### RunStateChangedData

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:221](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L221)
- 字段：`RunID common.RunID`；`WorkspaceID common.WorkspaceID`；`WorkspaceRunGeneration uint64`；`RunRevision uint64`；`GoalID common.GoalID`；`DoDID common.DoDID`；`State common.RunState`；`BlockingTicketID common.TicketID`；`BlockingApprovalID common.ApprovalID`；`FailureReason string`；`CausationID string`；`RuntimeStateRevision uint64`；`RuntimeStateRef string`；`ReconciliationRequestID string`
- 字段引用类型：[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L354)
- 空值/identity/版本/范围原式：`d.WorkspaceID == "" || d.RunID == "" || d.WorkspaceRunGeneration == 0 || d.RunRevision == 0 || d.CausationID == ""`；`d.CausationID != RunStateChangedCausationID(d.RunID, d.WorkspaceRunGeneration, d.RunRevision, d.RuntimeStateRevision)`；`d.RuntimeStateRevision > 0 && strings.TrimSpace(d.RuntimeStateRef) == ""`；`d.RuntimeStateRevision == 0 && strings.TrimSpace(d.RuntimeStateRef) != ""`；`d.RuntimeStateRevision > 0 && d.RuntimeStateRef != RuntimeStateRef(d.WorkspaceID, d.RunID, d.RuntimeStateRevision)`

<a id="schema-runworkspaceretirementfence"></a>

### RunWorkspaceRetirementFence

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L22)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`WorkspaceIntentHash string`；`Phase runmodel.WorkspaceRetirementPhase`；`FenceRevision uint64`；`WorkspaceRunGeneration uint64`；`QuiescenceID string`；`RuntimeInventoryHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)；[WorkspaceRetirementPhase](#schema-workspaceretirementphase)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L35)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L46)

<a id="schema-runworkspaceretirementruntimefact"></a>

### RunWorkspaceRetirementRuntimeFact

- 源码：[internal/modules/blocks/run/pkg/events/workspace_retirement.go:128](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L128)
- 字段：`RunID common.RunID`；`WorkspaceRunGeneration uint64`；`RunRevision uint64`；`RunState common.RunState`；`RuntimeStatePresent bool`；`RuntimeStateRef string`；`RuntimeStateRevision uint64`；`RuntimeStateHash string`；`TerminalDeliveryID string`
- 字段引用类型：[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/workspace_retirement.go#L140)
- 空值/identity/版本/范围原式：`workspaceID == "" || f.RunID == "" || f.WorkspaceRunGeneration == 0 || f.RunRevision == 0 || !runWorkspaceRetirementTerminalState(f.RunState)`；`!f.RuntimeStatePresent`；`f.RuntimeStateRef != "" || f.RuntimeStateRevision != 0 || f.RuntimeStateHash != "" || f.TerminalDeliveryID != ""`；`f.RuntimeStateRevision == 0 || f.RuntimeStateRef != RuntimeStateRef(workspaceID, f.RunID, f.RuntimeStateRevision) || f.RuntimeStateHash == "" || f.TerminalDeliveryID == ""`

<a id="schema-runtimecheckpoint"></a>

### RuntimeCheckpoint

- 源码：[internal/modules/blocks/run/pkg/models/checkpoint.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/checkpoint.go#L6)
- 字段：`ID common.CheckpointID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`CheckpointType common.CheckpointType`；`CurrentStepID common.StepID`；`RunStateSnapshot common.RunState`；`BlockingTicketID common.TicketID`；`BlockingApprovalID common.ApprovalID`；`ThreadRef string`；`OccurredAt string`；`RestorableUntil string`；`Data workorcheventpayload.Payload`
- 字段引用类型：[CheckpointID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L11)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[CheckpointType（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L18)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)
- 实际方法：[CloneRuntimeCheckpoint](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/checkpoint.go#L21)；[CloneRuntimeCheckpoints](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/checkpoint.go#L30)

<a id="schema-runtimecommandsreadydata"></a>

### RuntimeCommandsReadyData

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:267](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L267)
- 字段：`CausationID string`
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L271)
- 空值/identity/版本/范围原式：`strings.TrimSpace(d.CausationID) == ""`

<a id="schema-runtimecontinuationeligiblefact"></a>

### RuntimeContinuationEligibleFact

- 源码：[internal/modules/blocks/run/pkg/events/lifecycle_messages.go:146](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L146)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`SourceOwner string`；`CausationID string`；`AuthoritySnapshotID string`；`DefinitionRevision string`；`EvidenceRevision string`；`DispatchID string`；`ResolutionID common.ResolutionID`；`TicketID common.TicketID`；`ApprovalID common.ApprovalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L160)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L170)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.RunID == ""`；`c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" || c.AuthoritySnapshotID == "" || c.DefinitionRevision == "" || c.EvidenceRevision == "" || c.DispatchID == "" || c.ResolutionID == ""`；`hasTicket == hasApproval`

<a id="schema-runtimegovernancewaitfact"></a>

### RuntimeGovernanceWaitFact

- 源码：[internal/modules/blocks/run/pkg/events/lifecycle_messages.go:103](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L103)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`SourceOwner string`；`CausationID string`；`AuthoritySnapshotID string`；`DefinitionRevision string`；`EvidenceRevision string`；`DispatchID string`；`BlockingTicketID common.TicketID`；`BlockingApprovalID common.ApprovalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L116)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L126)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.RunID == ""`；`c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" || c.AuthoritySnapshotID == "" || c.DefinitionRevision == "" || c.EvidenceRevision == "" || c.DispatchID == ""`；`hasTicket == hasApproval`

<a id="schema-runtimerecoveryinventoryitem"></a>

### RuntimeRecoveryInventoryItem

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:84](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L84)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`State common.RunState`；`HasRuntimeBootstrap bool`；`HasRuntimeState bool`；`RuntimeStateRef string`；`RuntimeStateRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)

<a id="schema-runtimestateupdateresult"></a>

### RuntimeStateUpdateResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:167](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L167)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ExpectedRuntimeRevision uint64`；`ObservedRuntimeRevision uint64`；`Feedback runtimecontract.RuntimeFeedback`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeFeedback（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_feedback.go#L63)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L175)
- 空值/identity/版本/范围原式：`r.WorkspaceID == "" || r.RunID == ""`；`r.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainPersistence || r.Feedback.WorkspaceID != r.WorkspaceID || r.Feedback.RunID != r.RunID`；`err := r.Feedback.Validate(); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`runtimecontract.RuntimeFeedbackCategoryAccepted, runtimecontract.RuntimeFeedbackCategoryAggregateRevisionMismatch, runtimecontract.RuntimeFeedbackCategorySemanticRejection, runtimecontract.RuntimeFeedbackCategoryOwnerTransient`

<a id="schema-runtimeterminaldeliveryfact"></a>

### RuntimeTerminalDeliveryFact

- 源码：[internal/modules/blocks/run/pkg/events/lifecycle_messages.go:191](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L191)
- 字段：`<embedded> runtimecontract.RuntimeTerminalDeliveryFact`；`ResultID common.ResultID`
- 字段引用类型：[RuntimeTerminalDeliveryFact（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L74)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L199)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L205)；[Identity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L216)
- 空值/identity/版本/范围原式：`c.SourceOwner != runtimeLifecycleSourceOwner || c.ResultID == ""`；`err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil`

<a id="schema-step"></a>

### Step

- 源码：[internal/modules/blocks/run/pkg/models/step.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/step.go#L8)
- 字段：`ID common.StepID`；`RunID common.RunID`；`Kind string`；`State common.StepState`；`Input workorcheventpayload.Payload`；`Output workorcheventpayload.Payload`
- 字段引用类型：[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L9)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)
- 实际方法：[CloneStep](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/step.go#L17)；[CloneSteps](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/step.go#L27)

<a id="schema-storeruntimestaterecordcommand"></a>

### StoreRuntimeStateRecordCommand

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:16](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L16)
- 字段：`RunID common.RunID`；`WorkspaceID common.WorkspaceID`；`RuntimeState *runtimecontract.RuntimeStateRecord`；`LifecycleFence *runtimecontract.RuntimeOpaqueLifecycleFence`；`ExpectedRuntimeStateRevision *uint64`
- 字段引用类型：[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RuntimeStateRecord（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_state.go#L294)；[RuntimeOpaqueLifecycleFence（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_owner_identity.go#L131)

<a id="schema-storeruntimestaterecordresult"></a>

### StoreRuntimeStateRecordResult

- 源码：[internal/modules/blocks/run/pkg/events/messages.go:388](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L388)
- 字段：`RuntimeStateUpdate *RuntimeStateUpdateResult`
- 字段引用类型：[RuntimeStateUpdateResult](#schema-runtimestateupdateresult)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go#L424)

<a id="schema-workspaceexecutionintentcommand"></a>

### WorkspaceExecutionIntentCommand

- 源码：[internal/modules/blocks/run/pkg/events/lifecycle_messages.go:19](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L19)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`RequestID string`；`Kind runtimecontract.RuntimeExecutionIntentKind`；`GoalID common.GoalID`；`DoDID common.DoDID`；`DefinitionRevision string`；`StartupInputHash string`；`Bootstrap *runtimecontract.RuntimeBootstrap`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeExecutionIntentKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_execution_intent.go#L12)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[RuntimeBootstrap（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_bootstrap.go#L12)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L31)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L43)
- 空值/identity/版本/范围原式：`c.Bootstrap != nil`；`err := runtimecontract.ValidateRuntimeBootstrapRaw(*c.Bootstrap); err != nil`；`c.WorkspaceID == "" || c.RequestID == ""`；`c.RunID == "" || c.Bootstrap != nil`；`c.RunID != "" || c.Bootstrap == nil || c.GoalID == "" || c.DoDID == "" || c.DefinitionRevision == "" || c.StartupInputHash == ""`；`err := c.Bootstrap.Validate(); err != nil`；`definition.GoalID != c.GoalID || definition.DoDID != c.DoDID || definition.Revision != c.DefinitionRevision || definition.StartupInputHash != c.StartupInputHash`
- 显式分支集合（只表列源码case，default语义见validator）：`runtimecontract.RuntimeExecutionIntentResume`；`runtimecontract.RuntimeExecutionIntentRestart`

<a id="schema-workspaceexecutionintentresult"></a>

### WorkspaceExecutionIntentResult

- 源码：[internal/modules/blocks/run/pkg/events/lifecycle_messages.go:77](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L77)
- 字段：`<embedded> runtimecontract.RuntimeExecutionIntentAdmission`
- 字段引用类型：[RuntimeExecutionIntentAdmission（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_execution_intent.go#L60)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go#L81)
- 空值/identity/版本/范围原式：`err := r.RuntimeExecutionIntentAdmission.Validate(); err != nil`

<a id="schema-workspaceretirementphase"></a>

### WorkspaceRetirementPhase

- 源码：[internal/modules/blocks/run/pkg/models/workspace_retirement.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/workspace_retirement.go#L22)
- 声明：`type WorkspaceRetirementPhase string`
- 源码枚举值：`WorkspaceRetirementPhaseDraining = "draining"`；`WorkspaceRetirementPhaseRetired = "retired"`

<a id="schema-workspaceretirementpurgeobservation"></a>

### WorkspaceRetirementPurgeObservation

- 源码：[internal/modules/blocks/run/pkg/models/workspace_retirement.go:162](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/workspace_retirement.go#L162)
- 字段：`Surface string`；`ObservedBefore uint64`；`DeletedCount uint64`；`RemainingCount uint64`
- 实际方法：[ValidateEmptyWorkspaceRetirementPurgeObservations](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/workspace_retirement.go#L177)；[StableWorkspaceRetirementPurgeProofHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/models/workspace_retirement.go#L189)
- 空值/identity/版本/范围原式：`len(observations) != len(workspaceRetirementPurgeSurfaces)`；`strings.TrimSpace(observation.Surface) != workspaceRetirementPurgeSurfaces[idx] || observation.RemainingCount != 0 || observation.DeletedCount > observation.ObservedBefore || observation.ObservedBefore-observation.DeletedCount != observation.RemainingCount`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [executeWorkspaceRetirementCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_retirement.go#L17)：`!ok || ev.Source() != workspaceRetirementFactQuerySource`
  - 当前来源常量：`workspaceRetirementFactQuerySource = "kernel/orchestrator"`；`workspaceRetirementCommandSource = "blocks/workspace"`
- [handleRuntimeRecoveryInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L195)：`!ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource`
  - 当前来源常量：`workspaceRetirementFactQuerySource = "kernel/orchestrator"`
- [handleRuntimeStateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L303)：`!ok || cmd.Validate() != nil || m == nil || ev.Source() != workspaceRetirementFactQuerySource`
  - 当前来源常量：`workspaceRetirementFactQuerySource = "kernel/orchestrator"`
- [handleRuntimeStateRecordReadiness](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L278)：`!ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource`
  - 当前来源常量：`workspaceRetirementFactQuerySource = "kernel/orchestrator"`
- [executeRuntimeStateStoreCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/runtime_state_commands.go#L14)：`!ok || cmd.RunID == "" || cmd.WorkspaceID == "" || ev.Source() != "kernel/orchestrator"`
- [executeWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go#L45)：`!ok || ev.Source() != workspaceExecutionIntentRunSource`
  - 当前来源常量：`workspaceExecutionIntentRunSource = "blocks/workspace"`
- [handleWorkspaceRetirementInventory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L250)：`!ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource`
  - 当前来源常量：`workspaceRetirementFactQuerySource = "kernel/orchestrator"`
- [executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/lifecycle_handler.go#L49)：`!ok || ev.Source() != runtimeLifecycleFactSource`
  - 当前来源常量：`runtimeLifecycleFactSource = "kernel/orchestrator"`