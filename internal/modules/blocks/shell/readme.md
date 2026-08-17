# Shell 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/shell` 在受控工作目录、超时和资源策略下执行本地 shell command，并返回 exit code、stdout/stderr 摘要、错误分类、耗时和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Shell owner 在 `blocks/shell/pkg/events` 定义的 `ExecuteOperationCommand/Result` 同步投递
  Shell 操作；本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility、semantic-proof feasibility 与 CWI reconciliation
  均使用 Shell owner 定义的 concrete typed command/result。
- 对 profile 明确声明的 managed-validation recipe，SM-2 只通过 typed request 转交 File owner 已持久化的
  `WorkspaceManifest + prerequisite observation`。Shell owner 在自己的 adapter 内判断 recipe 对该 manifest 是否当前
  可执行，并为每个声明 recipe 发布 `ready | prerequisite_required | unavailable` factual surface。只有确认后且可作为
  直接证明的 recipe 才以 `ready + direct` 进入 provider-selectable catalog；例如 `go test` 在没有非排除
  `*_test.go` 时发布带稳定 prerequisite ref 的 `prerequisite_required`，而不是丢失该 Owner fact。静态 executable/policy
  不可用时发布 `unavailable`。Shell 不读取 File service/repository，也不取得 workspace tree 所有权。
- workspace-sensitive Shell catalog 的 observation 必须依赖该 File manifest 的 exact workspace-tree revision；后续
  File receipt 改变该 revision 时，SM-2 仅按依赖交集退休并重获 catalog。该 freshness lineage 不允许 Shell 修改
  Manifest、CWI 或 Runtime 状态。
- Shell 发布的 `ready + direct` validation surface 只是当前 Owner fact；它不关闭 profile 的
  `selectable_validation_surface` postcondition、不重开 CWI，也不选择 semantic successor。Orchestrator SM-2 只在
  assessment 内消费该 typed catalog，缺失时的 bounded CWI replan 仍完全属于 Orchestrator aggregate。
- stdout/stderr 不直接跨组件传递，只输出有界 typed evidence 摘要与引用。
- 对 state-changing `shell/command`，本 owner 在执行前后以有界 workspace regular-file snapshot 计算真实 changed refs；`.git`、symlink 与不可安全内容寻址的对象不作为交付物 effect。所有真实 changed ref 必须是 sealed `TargetRefs` 的子集，否则返回 `effect_mismatch/undeclared_workspace_effect` rejected receipt；不能通过同时 touch 一个已声明目标掩盖额外写入。
- accepted state-changing receipt 的 `TargetRefs` 与 `ResourceChanges` 来自 owner observation，而非直接回显候选输入；无可观察 workspace delta 时返回 no-progress，而不是 durable evidence。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed command、working directory/ref、
  timeout 和 policy；result 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回 exit code、有界输出、duration、
  owner stable factual category、完整安全 original diagnostic 和 evidence/ref，不返回跨组件 Status 双权威。
- Runtime evidence policy 使用 Orchestrator command 携带的具体 `RuntimeEvidencePolicy`，用于 zero-work marker、work-evidence marker、evidence quality、reason 和 tag 判定；本组件不从 operation parameter map 解析跨组件 policy。
- Orchestrator 与 Approval 流程在高风险命令进入本 block 前完成准入；只有本 block 返回独立 typed
  `GovernanceRequirement` 时才可进入治理，普通失败或 budget exhaustion 不能创建 ticket。
- effect 执行前后通过 Orchestrator receipt query/commit concrete command 完成幂等握手；Shell 不以进程退出状态代写 Runtime 状态。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/reconciliation/execute、manifest prerequisite；receipt query/commit | 完整 |
| `blocks/file` | 间接 typed fact | File owner manifest 由 Orchestrator 转交，Shell 不直连 File | 完整 |
| 本地进程/workspace | Shell-private adapter | sealed command、cwd、timeout、effect observation | 完整；无任意长期后台任务入口 |

## 相关约束

- command、工作目录、运行时间和输出大小必须有界，不能默认启动后台长期任务。
- destructive、unbounded 或不符合 policy 的命令必须拒绝。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 Memory 状态。
- stdout/stderr 只是 evidence input，不能直接判定业务完成。
- evidence policy 只调整本次 Shell evidence 的结构化质量结论，不得修改 Runtime authority、coverage focus 或 Run 状态。
- 本 owner 私有 code 只在 Shell Biz 边界映射为 factual category；不得选择 correction、retry、terminal 或下一
  canonical state。unknown code 保留原 code/message 并 typed fail-fast。
- CWI reconciliation 只能执行 binding 封存的 validation recipe；recipe 成功且 exit 0 但没有
  `RequiredCheckRefs` 时返回 `unverifiable/validation_check_declaration_unavailable`，不得把 generic `go test` 或
  空 TODO test 扇出为多个 CWI 的 semantic proof。非工具原生检查必须由封存 recipe 在 stdout/stderr 输出
  `WORKORCH_CHECK_PASSED=<check-ref>`（或对应 `FAILED`/`SKIPPED`）行，由 Shell owner 解析为结构化事实；
  candidate 不能直接提交这些标记或 check ref。
- 无当前适用 recipe 时，Shell 返回空的 confirmed recipe set，同时在同一 catalog 保留声明 recipe 的
  `prerequisite_required`/`unavailable` surface 供 aggregate/audit 使用；它不得把这些 surface 加入 provider-selectable
  refs、选择 File preparation 变更、请求 correction，或把 feasibility rejection 解释为新的 retry/terminal 状态。

## 待完善测试点

- [ ] 冻结每个 profile 声明 recipe 的 ready/prerequisite_required/unavailable 与 direct/auxiliary 矩阵。
- [ ] 覆盖 File manifest revision 变化后的 catalog 退休/重获，以及 stale manifest 不执行 recipe。
- [ ] 覆盖未声明 workspace effect、symlink/`.git` effect、只 touch 声明目标掩盖额外写入均被拒绝。
- [ ] 覆盖 candidate 伪造 `WORKORCH_CHECK_*` 标记、未知 check ref 和 marker 冲突。
- [ ] 覆盖 timeout、取消、输出截断和后台子进程清理。
- [ ] 覆盖命令已执行但 receipt commit 前崩溃的查询重放，确认不会重复 durable effect。
