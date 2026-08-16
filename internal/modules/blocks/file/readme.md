# 文件能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-16

## 功能说明

`blocks/file` 提供受 workspace 路径边界约束的文件 list、read、write、edit、patch preview/apply/revert 等运行时能力，并返回规范化执行结果。

## 组件交互及合同

- `kernel/orchestrator` 通过 File owner 在 `blocks/file/pkg/events` 定义的具体 command/result 同步投递文件操作；本组件直接订阅 EventHub，不启动通用 runtime executor，也不发布第二套执行通知。
- Runtime target snapshot、workspace manifest 与 capability preflight 均使用 File owner 定义的 typed EventHub command/result；结果只表达 owner 观测，manifest revision 对文件路径和内容变化敏感。
- 对 profile 显式绑定的空工作区，File 可以返回不含具体路径的 `scope` selection；它只授权 LLM 提出一个具体 workspace-relative File target，并由同一 owner feasibility/admissibility gate 验证。File 不从 Goal、DoD、目录名或文件后缀推断路径；未绑定 profile 的空工作区仍返回 `absent`。
- Runtime state-change/validation operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并携带本 owner
  stable factual category、完整安全 original diagnostic 与 evidence；不返回跨组件 Status 双权威或 map。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed operation、path/ref 和有界输入；
  result 返回唯一 receipt outcome、target refs、side-effect metadata、bounded evidence/ref 与 original diagnostic。
- Orchestrator 负责 state-changing admission、approval、evidence quality、coverage binding 和 canonical decision。
- 对跨越信任边界的敏感 file/write、patch/apply（密钥、`.env`、`.git`、CI workflow），File owner 先返回 `governance_required` 且不执行副作用；普通 workspace-local 文件的变更规模不构成 approval。resume 只在 exact `ExecutionKey + next Fence` 的 typed grant 经 Orchestrator preflight 验证后消费该 grant 并执行，普通布尔参数或调用方声明不能绕过 approval。

- EventHub handler 只校验并复制外部 command；文件副作用只能由 File-private effect event 选择并调用 adapter。
- `file/write` 是 create-only 操作，目标已存在时必须拒绝且保持原内容不变。File owner 的 operation
  admissibility catalog 因此只为该 kind 发布 `prospective_owner_target`；已有目标只能选择同一 catalog 中允许
  `exact_target | existing_owner_scope` 的 `file/edit` 或 `patch/apply` variant。实际 replacement 仍须绑定 owner
  观测到的目标 revision；其中 `patch/apply file_write` 必须携带在执行时再次核对的 `baseHash`，
  `text_replace | unified_diff` 也必须通过对应的 owner preflight/replay 合同。

## 相关约束

- 所有路径必须限定在 workspace internal/deliverable 允许范围内，并防止路径逃逸。
- 大内容必须使用摘要和 raw ref，不能进入默认事件或投影。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result、Memory 或 coverage 状态。
- 本 owner 私有 code 只在 File Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
- 文件写入成功只表示技术动作成功，不等于业务完成。
- 完整 target snapshot 必须携带 target ref、content、content hash 和明确的 complete content state；missing、truncated、binary 或 redacted 内容不得作为可 patch snapshot 提交。
- CWI reconciliation 仅使用 binding 声明的 exact target、owner selector 或可直接映射到 manifest 的 artifact ref；
  unknown/semantic artifact URI、ambiguous selector 和 workspace absent 分别返回 `unverifiable` 或 `unsatisfied`，不从
  Goal/DoD、目录名或文件名猜测交付目标。
