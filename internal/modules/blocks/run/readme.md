# Run Block-class Owner

## 组件类型

`blocks/run` 按职责属于 Block：它只拥有 Run 聚合、Runtime V2 aggregate 与其
CAS/查询合同，不组合多个 owner 选择下一动作。目录、module ID 和 EventHub
合同身份均为 `blocks/run`。

## 功能说明

`blocks/run` 管理 Run、Step、RuntimeCheckpoint 与 Runtime V2 aggregate 的正式持久化、状态准入和查询合同。Runtime V2 的动作选择、执行和收敛由 Orchestrator 负责，Run 不承担跨组件编排。

## 组件交互及合同

- Run owner 通过 `TopicRunCommandRuntimeV2RecoveryInventory` 提供分页 typed recovery inventory，分类 active/no-dispatch、active/sealed、blocked governance 与 terminal delivery pending；该窄只读 topic 在 `Setup`（Configure 后）即订阅，早于常规 Run CRUD topic，因此 Orchestrator 的 SM-4 启动盘点不依赖 module `Weight()` 或 Run `Start()`；Orchestrator 只消费该分类，不直接扫描 Run repository。

- Run 自身的 create/ensure/get/list/update/delete、Step 查询和 Checkpoint 查询由 `blocks/run/pkg/events` 定义具体 command 与 `RunCommandResult`。
- workspace/run purge 通过 `PurgeRuntimeResourcesCommand` 由 Run owner 删除该 scope 下的 Step 与 RuntimeCheckpoint；命令必须先确认 Run 属于指定 workspace，并返回实际删除数量。
- Runtime V2 正式状态使用 typed `RuntimeStateRecord` 随 Run 持久化，其中包含 facts、authority、dispatch、candidate attempt 及初始化时封存的 Goal/DoD definition context；update command 可携带 expected runtime revision，Run owner 在同一 EventHub lane 内执行 CAS 后写入。
- Run 创建合同可携带 typed `RuntimeBootstrap`，保存 effective Goal/DoD definition、ordered focus queue 与 prerequisite requirements；bootstrap 在 RuntimeState 初始化前保持不可变。
- 创建或启动 Run 前，通过 EventHub 读取 `blocks/workspace` 的有效 workspace definition 和 revision。
- 启动和恢复由 application 向 Run owner发送具体 activation / continuation intent；Run Biz 在自身 reducer 中完成状态准入、
  CAS 持久化并发布 `RunStateChangedData`。Orchestrator 只能消费该通知触发 drive，不能投影 Run lifecycle。
  Runtime V2 Resume 只携带 workspace/run scope，不接受 loaded Run 或上下文 patch。
- governed blocker 必须携带对应 DecisionTicket 或 Approval 的 workspace/run scope。
- Run 创建及状态变化使用 `RunStateChangedData` typed notification；Governance、Eventstream 和 application 按需订阅该 owner 通知。
- Governance、Eventstream 和 application 按需导入 `blocks/run/pkg/events` 订阅或发送命令。
- Runtime V1 retirement apply 使用 typed update command 将非终态 V1 Run 标记为 `superseded`，保留 lineage 与时间字段并写入固定 failure reason；owner 随更新发布正式 Run 状态通知。

## 相关约束

- Run、Step 和 Checkpoint 操作必须携带足够的 workspace/run/step scope。
- 生产装配只接收 Run 自身 repository 与 EventHub，不得注入 WorkspaceReader、Orchestrator service、callback 或其他 owner repository。
- `Stop`/`Teardown` 必须逆序、幂等地解除 recovery inventory 与常规 topic；recovery inventory 不得在 owner 停止后继续响应。
- Run 不负责 coverage、evidence quality、Result acceptance 或 LLM 输出正确性判断。
- 不允许形成 active Run 无 next action、无 blocker 且当前 Step 已失败的悬挂状态。
- Step 合同失败不得先记录 completed 再回滚为 failed。
- `RuntimeStateRecord` 必须通过 canonical invariant 校验并与 workspace/run scope 一致；所有 V2 state update 都必须携带 expected revision，缺失 CAS 或 stale revision 必须拒绝，不能覆盖更新后的 authority transaction。
- update 是完整 owner state replacement：GoalID、DoDID、DefinitionRevision、InitialStepKind、CreatedAt 与 RuntimeBootstrap 创建后不可漂移；V2 contract/state 不可降级或清空，blocker 字段必须与 RunState 一致，bootstrap/state 的 definition 与 activation 必须相互匹配。
- candidate attempt history 属于 typed Runtime V2 state；Run owner 必须拒绝 attempt 1 无对应 failed attempt 0、accepted attempt 无 sealed dispatch、或同 snapshot 重复 attempt 的状态写入。
- governance resolution consumption record 随 Runtime V2 state 持久化，并必须关联已完成的 governed dispatch；缺少对应 dispatch 或重复 resolution/snapshot 的状态不得写入。
- open governance request binding 同样随 Runtime V2 state 持久化，记录 ticket、authority snapshot、dispatch 和 dependency；绑定必须关联 active 或 completed governed dispatch。
- Runtime V2 facts 中的当前 coverage focus 与 remaining focus queue 必须原样持久化；Run owner 不负责重排或推导 focus。
- bootstrap definition identity/revision 必须与 Run 的 GoalID、DoDID 和 DefinitionRevision 一致；不一致的创建或 ensure command 必须返回 conflict。
- 持久化 definition context 必须完整且 revision 与 RuntimeFacts 一致；Run owner 不接受定义正文、identity 或 capability contract 缺失的 Runtime V2 状态。
- retirement update 不得修改带 typed Runtime V2 bootstrap/state 的 Run，也不得删除历史 Step、Checkpoint 或 evidence 审计记录。
- Resume 的状态准入与迁移完全由 Run owner reducer 完成；成功后才发布不可变 Run state fact。不得通过 Run command
  传递 `map[string]any`、指定外部 owner state 或调用 Orchestrator service facade。
- Run 不提供 Validate/Complete service command；禁止 application 或其他 module 绕过 Runtime V2 reducer/evidence gate 直接把 active Run 标记为 validated/completed。
- Run owner handler 直接调用本 owner `biz`；不保留 `Service()` 或 `service/` forwarding facade。Start/Resume/Block/Cancel/Supersede/Fail 等跨组件状态变更必须发送 owner 的具体 EventHub command。
- Run biz 不再持有或构造 Orchestrator adapter；已删除仅用于 V1 lifecycle 转发的 service DTO alias 和 test-only constructor，Run owner 的生产依赖只包含自身 repository、Workspace typed read port 与状态通知 Hub。
