# Git 能力组件

## 功能说明

`blocks/git` 提供受控的 Git read/write 运行时能力，并返回规范化结果、变更引用、摘要、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 Git 操作；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 禁止的 执行通知。
- Runtime capability preflight 通过 Orchestrator 定义的 typed EventHub command 查询本 block 实际提供的 Git read/write kinds。
- Git capability-state prerequisite 的 canonical owner identity 是 `blocks/git`；profile 只能通过该 identity 请求 `repository_state`，不得使用 `git` 等 EventHub destination 别名。
- Git capability-state receipt 是 Git owner 的 durable prerequisite evidence，按 `<repository.dataDir>/internal/workspaces/<workspaceId>/runtime/git-capability-state-receipts/` 分区；同一 workspace/acquisition key 只能对应同一快照。它不写入 deliverable，随 workspace internal directory 的受控删除一并清理。
- Runtime Git operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并携带本 owner stable factual
  category、完整安全 original diagnostic 与 evidence；不返回跨组件 Status 双权威。
- request 必须携带 workspace repository、run/authority/dispatch/operation identity、sealed operation 和结构化参数；
  result 返回唯一 receipt outcome、changed refs、bounded evidence/ref 与 original diagnostic。
- Orchestrator 和 Approval 流程在写操作进入本 block 前完成策略准入和必要授权。

## 当前内聚收口切片（2026-08-16）

- `RuntimeAdapter` 仍是唯一 Git capability adapter；原 1,026 行文件已按 capability-state observation、operation
  ingress、read operations、write operations、repository inspection、Git CLI runner、pathspec guard、output codec 与
  result normalization 分成同 package 的 owner-private 文件。没有借用 Shell executor，也没有新增 Block、service
  facade、EventHub round trip、repository、receipt owner 或命令 registry。
- `SupportedRuntimeOperationVariants` 仍与 typed descriptor 做穷尽比对；`ExecuteOperation` 继续静态分派到 read/write，
  `git init`、add、commit 和 status/diff/log/show/branch 的 policy、timeout、pathspec、bounded output 与 error category
  顺序未变。
- `CapabilityState` 只产生 read-only repository snapshot；durable `CapabilityStateReceiptStore` 仍是 Git owner 内唯一
  receipt authority，保持 workspace partition、same-key idempotency/conflict 和 restart reload，不与 operation receipt
  或 Shell state 合并。
- C22-05 Git 切片完成后，非测试 Go 为 24 个文件/2,538 LOC，测试保持 8 个文件/1,345 LOC，package 保持 4；
  最大生产文件从 `git_runtime_adapter.go` 1,026 行降为 `owner.go` 700 行，至少 500 行生产文件从 2 个降为 1 个，
  生产 `CC>30` 继续为 0。

## 相关约束

- Git 操作只能作用于 workspace deliverable repository。
- 不得执行未声明或未准入的远程操作、历史改写或任意 Git CLI 文本。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 completion 状态。
- Git 输出只作为 evidence candidate，不能直接判定 DoD 或 Result accepted。
- 本 owner 私有 code 只在 Git Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
- CWI reconciliation 只返回 repository presence/revision 这一 Git fact；binding 同时要求 check、metric 或 artifact
  时必须返回 `unverifiable/git_proof_incomplete`。commit/revision 本身不能替代 semantic validation proof。
