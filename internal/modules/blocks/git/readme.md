# Git 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/git` 提供受控的 Git read/write 运行时能力，并返回规范化结果、变更引用、摘要、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Git owner 在 `blocks/git/pkg/events` 定义的 `ExecuteOperationCommand/Result` 同步投递
  Git 操作；本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility、semantic-proof feasibility、CWI reconciliation
  与 capability-state acquisition 均使用 Git owner 定义的 concrete typed command/result。
- Git capability-state prerequisite 的 canonical owner identity 是 `blocks/git`；profile 只能通过该 identity 请求 `repository_state`，不得使用 `git` 等 EventHub destination 别名。
- Git capability-state receipt 是 Git owner 的 durable prerequisite evidence，按 `<repository.dataDir>/internal/workspaces/<workspaceId>/runtime/git-capability-state-receipts/` 分区；同一 workspace/acquisition key 只能对应同一快照。它不写入 deliverable，随 workspace internal directory 的受控删除一并清理。
- Runtime Git operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并携带本 owner stable factual
  category、完整安全 original diagnostic 与 evidence；不返回跨组件 Status 双权威。
- request 必须携带 workspace repository、run/authority/dispatch/operation identity、sealed operation 和结构化参数；
  result 返回唯一 receipt outcome、changed refs、bounded evidence/ref 与 original diagnostic。
- Orchestrator 和 Approval 流程在写操作进入本 block 前完成策略准入和必要授权。
- workspace repository manifest 通过 File owner 的 `AcquireWorkspaceManifestCommand/Result` 获取；Git 不读取 File repository/service，也不取得 workspace tree 所有权。
- effect 执行前后通过 Orchestrator 的 receipt query/commit concrete command 完成幂等握手，Git 只返回 repository owner fact。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/reconciliation/state acquisition/execute；receipt query/commit | 完整 |
| `blocks/file` | Git → File | `AcquireWorkspaceManifestCommand/Result` | 完整；只读 typed snapshot |
| Git repository | Git-private adapter | sealed read/write operation 与 revision observation | 完整；无任意 CLI 文本入口 |

## 相关约束

- Git 操作只能作用于 workspace deliverable repository。
- 不得执行未声明或未准入的远程操作、历史改写或任意 Git CLI 文本。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 completion 状态。
- Git 输出只作为 evidence candidate，不能直接判定 DoD 或 Result accepted。
- 本 owner 私有 code 只在 Git Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
- CWI reconciliation 只返回 repository presence/revision 这一 Git fact；binding 同时要求 check、metric 或 artifact
  时必须返回 `unverifiable/git_proof_incomplete`。commit/revision 本身不能替代 semantic validation proof。

## 待完善测试点

- [ ] 覆盖 File manifest 缺失、stale、wrong scope/revision 时 capability state 与 operation admission fail-closed。
- [ ] 覆盖 prerequisite receipt 在重启、重复 acquisition 和同 key 不同 snapshot 下的幂等/冲突语义。
- [ ] 覆盖远程操作、历史改写、未声明 ref 与危险参数均不能绕过结构化 operation catalog。
- [ ] 覆盖 workspace repository 路径逃逸、非 repository 和跨 workspace scope。
- [ ] 覆盖 effect 后 receipt commit 前崩溃的查询重放，确认 Git 副作用不重复。
- [ ] 覆盖 commit/revision 只能形成 repository fact，不能单独满足 semantic check/metric/artifact proof。
- [ ] 覆盖 File/Orchestrator 返回 nil/wrong result 与 safe original diagnostic 保留。
