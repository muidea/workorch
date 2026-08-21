# Git 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-21

## 功能范围与边界

`blocks/git` 提供受控的 Git read/write 运行时能力。Git-private adapter 保留有界规范化输出；跨 Owner
只返回权威 `RuntimeOperationReceipt`，不会把 stdout、stderr、diff 或完整私有结果 DTO 暴露为第二套结果合同。

## 组件交互及合同

- `kernel/orchestrator` 通过 Git owner 在 `blocks/git/pkg/events` 定义的 `ExecuteOperationCommand/Result` 同步投递
  Git 操作；本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility、semantic-proof feasibility、CWI reconciliation
  与 capability-state acquisition 均使用 Git owner 定义的 concrete typed command/result。
- Git capability-state prerequisite 的 canonical owner identity 是 `blocks/git`；profile 只能通过该 identity 请求 `repository_state`，不得使用 `git` 等 EventHub destination 别名。
- Git capability-state receipt 是 Git owner 的 durable prerequisite evidence，按 `<repository.dataDir>/internal/workspaces/<workspaceId>/runtime/git-capability-state-receipts/` 分区；同一 workspace/acquisition key 只能对应同一快照。它不写入 deliverable，随 workspace internal directory 的受控删除一并清理。
- Runtime Git operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果。`ExecuteOperationCommand` 只携带
  execution context、完整 `SealedRuntimeOperation` 与 prerequisite facts；Git 校验发送方必须是 Orchestrator、
  payload hash、operation/target/CWI scope，并从 seal 内派生 adapter 参数，不再接受可独立漂移的 `Data`。
- `git add` 的 pathspec 必须由同一 sealed operation 的 exact target refs（或 owner selector
  `workspace-repository`）授权。执行前后比较完整的有界 porcelain state；已有 staged 文件不构成本次变化，
  重复 add 返回 `no_op`，不会生成 repository resource change。
- Git read 的 `ref/baseRef` 拒绝 option-shaped、控制字符和超长输入，并在 argv 中使用
  `--end-of-options`；声明为 `effect=none` 的 read 不能借 ref 写文件。
- public receipt 保留 summary、exit code、规范化内容 hash、最多 64 个 changed/commit evidence refs、
  add 的 before/after hash 与 repository revision change。原始 stdout/stderr/diff 只留在 Git-private result；
  stdout/stderr 在子进程 pipe 写入时即分别按 64 KiB（diff 128 KiB）限长保留，同时独立累计真实总字节数；失败反馈仅记录
  安全摘要、原始字节数、截断和 `raw_git_command_output` omitted metadata，不声称完整 raw diagnostic。
- Orchestrator 和 Approval 流程在写操作进入本 block 前完成策略准入和必要授权。
- workspace repository manifest 通过 File owner 的 `AcquireWorkspaceManifestCommand/Result` 获取；Git 对
  workspace/run/authority/prerequisite scope 和完整 manifest binding 做精确校验，并在 operation feasibility 与
  effect 前重新读取 File workspace-tree revision。Git 不读取 File repository/service，也不取得 workspace tree 所有权。
- Git repository revision 使用 `git-state-v2` 对有界完整 porcelain observation 哈希；File revision 单独覆盖
  “路径仍 dirty 但内容已变化”的 freshness。
- effect 前后通过 Orchestrator 的 receipt query/commit concrete command 完成幂等握手。receipt query 的
  `found=false` 只对当前 active dispatch 中 exact operation/execution-key/fence 且状态为 `claimed` 的记录成立；
  sealed、stale、completed、invalidated 或伪造 identity 均 fail-closed。query 的 Found/Receipt 一致性与 receipt identity、
  commit 的完整 receipt equality 使用 capability owners 共享的 typed event-contract validator。
- execute handler 要求同步 result；误投为 `Post` 时在 receipt 查询和 Git 调用前返回，不产生副作用。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/reconciliation/state acquisition/execute；receipt query/commit | 当前生产路径完整；execute 与 receipt authorization 均为 exact sealed identity |
| `blocks/file` | Git → File | `AcquireWorkspaceManifestCommand/Result` | 当前生产路径完整；只读 scoped binding，并在 feasibility/effect 前重验 |
| Git repository | Git-private adapter | sealed read/write operation、bounded evidence 与 revision observation | 当前 operation catalog 完整；无任意 CLI 文本入口，raw output 不跨 Owner |

## 相关约束

- Git 操作只能作用于 workspace deliverable repository。
- repository root 必须与解析后的 deliverable root 相同，Git control directory 也必须位于该 root 内；父仓库继承、外置
  `gitdir`、`.git` symlink 与 bare repository 均不得取得 workspace Git authority。workspace 内指向外部文件的 symlink
  只按 Git symlink identity 暂存，不跟随或修改外部目标。
- 不得执行未声明或未准入的远程操作、历史改写或任意 Git CLI 文本。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 completion 状态。
- Git 输出只作为 evidence candidate，不能直接判定 DoD 或 Result accepted。
- 本 owner 私有 code 只在 Git Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
- CWI reconciliation 只返回 repository presence/revision 这一 Git fact；binding 同时要求 check、metric 或 artifact
  时必须返回 `unverifiable/git_proof_incomplete`。commit/revision 本身不能替代 semantic validation proof。

## 测试覆盖状态

已覆盖：

- File workspace-tree binding、dirty-content stale feasibility 与 effect 前 stale 拒绝。
- durable capability receipt 的重启加载、重复 acquisition、同 key 不同 snapshot 冲突。
- exact owner receipt 重放、Orchestrator active/claimed query authorization、resource-change crash/restart。
- ref option injection、sealed payload/target drift、非 Orchestrator source、Post 强调用和重复 add no-op。
- CWI reconciliation 仅形成 repository fact，check/metric/artifact 要求返回 unverifiable。
- File responder missing/nil/wrong type、owner error、workspace/run/authority/prerequisite identity 与 manifest binding 漂移。
- 未声明 remote/history-rewrite operation 的 read/write 全矩阵，ref/baseRef option/control/oversize 输入，以及 operation variant
  catalog 与 descriptor 的精确一致性。
- 父仓库、非 repository、外置 `.git` file/symlink、workspace symlink identity 与 capability receipt 跨 workspace 隔离。
- 大 diff 与 900-file dirty repository 的 retained bytes、changed-files/evidence-ref 上限回归及 allocation benchmark。

## 当前验证结论

- 当前没有已知的本地可执行合同或测试缺口。不同部署环境中的 Git binary/version 兼容性保留为 deployed verification；
  版本差异不得扩展固定 operation catalog、revision grammar 或 workspace 边界。
