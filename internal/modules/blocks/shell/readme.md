# Shell 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-25

## 功能范围与边界

`blocks/shell` 在受控工作目录、超时和进程组策略下执行本地命令。生产 Runtime 只暴露两类能力：

- `shell/command` 是显式 `stateChanging=true` 的 workspace-durable、at-most-once 操作；任意命令不能再声明
  `effect=none` 或进入 `need_validation`。
- `validation/managed_session` 只接受 owner catalog 已封存的 recipe ID。recipe 在有界临时 workspace 副本内执行，
  effect 固定为 `ephemeral_session`，idempotency 固定为 `at_most_once`。

跨 Owner 的 execute result 只返回 owner 已持久化的 `RuntimeOperationReceipt`。exit code、duration、stdout/stderr 与
raw diagnostic 留在 Shell-private adapter；receipt 只携带有界、脱敏的 evidence/diagnostic 及原始输出被省略的元数据。

## 组件交互及合同

- `kernel/orchestrator` 通过 Shell owner 在 `blocks/shell/pkg/events` 定义的 `ExecuteOperationCommand/Result` 同步投递
  Shell 操作；execute 是强调用，nil result、非 Orchestrator source、receipt query 失败或 sealed data 漂移均在启动进程前
  fail-closed。本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility、semantic-proof feasibility 与 CWI reconciliation
  均使用 Shell owner 定义的 concrete typed command/result。
- 对 profile 明确声明的 managed-validation recipe，SM-2 只通过 typed request 转交 File owner 已持久化的
  `WorkspaceManifest + prerequisite observation`。Shell owner 在自己的 adapter 内判断 recipe 对该 manifest 是否当前
  可执行，并为每个声明 recipe 发布 `ready | prerequisite_required | unavailable` factual surface。只有确认后且可作为
  直接证明的 recipe 才以 `ready + direct` 进入 provider-selectable catalog；例如 `go test` 在没有非排除
  `*_test.go` 时发布带稳定 prerequisite ref 的 `prerequisite_required`，而不是丢失该 Owner fact。静态 executable/policy
  不可用时发布 `unavailable`。Shell 不读取 File service/repository，也不取得 workspace tree 所有权。
- `prerequisite_required` validation surface 同时发布
  `shell/selectable-validation-surface/prospective-workspace-manifest/v1`。Orchestrator 只可通过 Shell owner 的 typed
  prospective-satisfaction query 转交 activation-sealed recipes 和 File owner 已计算的 immutable resulting target refs；
  Shell 先要求每个匹配 contract 的 prerequisite surface 都存在同 ID recipe，且 command、ordered args、timeout 计算的
  owner schema hash 与 surface `InputSchemaHash` 完全一致，再复用 owner-private recipe applicability 规则返回
  `would_satisfy | would_not_satisfy`；若 exact Shell policy/catalog dependency 已漂移，则返回绑定 expected/observed
  revision 的 typed stale nested observation。该 dependency catalog只供 Runtime freshness/CAS/seal，不进入 provider-selectable
  catalog projection。任一 recipe 身份缺失或漂移均 typed fail-closed；该 query 不访问 File、不观察 workspace、不启动进程，
  也不改变 Shell/Runtime 状态。只有 Orchestrator private event可持久化 factual result：SM-2 选择 fact
  refresh/seal/terminal eligibility，SM-3 只在下述有界条件成立时消耗既有 sole semantic correction。exact
  target 来自 persisted SM-3 proposal，Shell 只通过 File delta 验证它，不选择 target；只有 SM-2 sealed
  binding 才形成 authority。
- Shell `would_not_satisfy` 自身不触发 correction。只有它与 exact-current File delta/outer feasibility、
  proposal/batch/manifest/catalog lineage 组成已持久化 full composite，且 Orchestrator semantic attempt=0 仍有
  不同合法 target 时，SM-3 才可复用同一 batch/correction ledger 回到 `Requesting`。新 proposal 必须
  target 不同且仍命中原 selector/constraints/candidate bounds，Shell 对新 File delta 独立重新评估。
  attempt=0 fixed/singleton/无剩余 target、invalid attempt-1 document 或 attempt-1 `would_not_satisfy` 由
  Orchestrator 收口为 `semantic_contract_failed`；第二次 owner rejection 保留 typed original
  reason=`prospective_owner_fact_unsatisfied`，合法 attempt-1 `would_satisfy` 可继续进入 SM-2 seal。typed stale 只刷新
  dependency fact，不消耗 correction。Shell 始终只署名 satisfaction/stale fact 与 reason，不署名或选择
  correction/terminal diagnostic。
- workspace-sensitive Shell catalog 的 observation 必须依赖该 File manifest 的 exact workspace-tree revision；后续
  File receipt 改变该 revision 时，SM-2 仅按依赖交集退休并重获 catalog。该 freshness lineage 不允许 Shell 修改
  Manifest、CWI 或 Runtime 状态。
- managed-validation execute command 还携带 fresh File manifest 的 bounded target scope。Shell 只在 owner-private
  adapter 中解析明确支持的结构化工具位置；当前 Go recipe 仅接受 `*.go:line[:column]:` 位置，归一后必须属于该 manifest
  scope 且最多八项。validation entry、operation input target、coverage hint 和无结构化 stderr/stdout 都不能自动成为
  failure target；Shell 只发布 candidate observation，File confirmation 与 Orchestrator SM-2 才能封存 repair surface。
- Shell 发布的 `ready + direct` validation surface 同时声明 `ephemeral_session + at_most_once`，并包含由 recipe identity
  确定性派生、与 recipe ID 不同的 aggregate proof check。tool-native check 继续作为 audit fact；只有 recipe 真实执行了
  至少一个无冲突 pass，aggregate proof check 才进入 receipt。该 surface 只是当前 Owner fact；它不关闭 profile 的
  `selectable_validation_surface` postcondition、不重开 CWI，也不选择 semantic successor。Orchestrator SM-2 只在
  assessment 内消费该 typed catalog，缺失时的 bounded CWI replan 仍完全属于 Orchestrator aggregate。
- managed validation 在 owner 创建的临时目录复制有界、File manifest 可见的 regular-file workspace；`.git` 与生成目录
  不复制，任何被复制范围内的 workspace symlink/special file 均拒绝。HOME/TMP/XDG cache 被隔离，副本在成功、失败或
  timeout 后删除。Go build/module cache 仅复用 owner 技术 cache，属于已声明的 ephemeral effect，不是
  workspace/Runtime authority。原 workspace 的 authority-visible tree（另含 `.git` control state 与 symlink identity）仍做
  前后有界观察；出现任何回写即拒绝 receipt。进程组在正常完成、失败、取消和 timeout 后均清理。
- 对 state-changing `shell/command`，本 owner 在执行前后以包含 `.git`、mode 与 symlink identity 的有界 workspace
  snapshot 计算真实 changed refs；外指 symlink 在进程启动前拒绝。所有真实 changed ref 必须是 sealed `TargetRefs` 的
  子集，否则返回 `effect_mismatch/undeclared_workspace_effect` rejected receipt；不能通过同时 touch 一个已声明目标掩盖额外写入。
- accepted state-changing receipt 的 `TargetRefs` 与 `ResourceChanges` 来自 owner observation，而非直接回显候选输入；无可观察 workspace delta 时返回 no-progress，而不是 durable evidence。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed operation data、working directory/ref、
  timeout 和 policy。`shell/command` 的 command/args/effect/targets 必须与 sealed execution 一致；managed validation 的
  command/args 只能由 Shell owner 根据 sealed recipe snapshot 回填，candidate 不能提交执行正文。
- Runtime evidence policy 使用 Orchestrator command 携带的具体 `RuntimeEvidencePolicy`，用于 zero-work marker、work-evidence marker、evidence quality、reason 和 tag 判定；本组件不从 operation parameter map 解析跨组件 policy。
- Orchestrator 与 Approval 流程在高风险命令进入本 block 前完成准入；只有本 block 返回独立 typed
  `GovernanceRequirement` 时才可进入治理，普通失败或 budget exhaustion 不能创建 ticket。
- effect 前的 Orchestrator receipt query 同时是 exact sealed-dispatch authorization：只有当前
  `workspace/run/dispatch/operation/ExecutionKey/Fence` 仍处于 claimed 且无 receipt 时才允许执行。已有 exact receipt 只重放
  receipt；找不到授权、查询错误或 stale fence 均零进程调用。managed validation 在 claimed/no-receipt 重启时进入
  `outcome_unknown`，不会以“验证只读”为由盲重放。Found/Receipt 一致性、query identity 与 commit 完整 receipt equality
  由 capability owners 共享的 typed validator 校验。Shell 不以进程退出状态代写 Runtime 状态。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | 双向 | capability/admissibility/feasibility/prospective-satisfaction/reconciliation/execute、manifest prerequisite；receipt query/commit | 完整 |
| `blocks/file` | 间接 typed fact | File owner manifest 由 Orchestrator 转交，Shell 不直连 File | 完整 |
| 本地进程/workspace | Shell-private adapter | sealed command/recipe、临时副本、cwd、timeout、进程组清理、effect observation | 完整；不承诺 host/container 级沙箱 |

## 相关约束

- command、工作目录、workspace snapshot/copy、运行时间和输出大小必须有界。已知 detached/background 语法在 policy
  阶段拒绝；所有进入执行的进程仍统一放入独立进程组并在主命令结束后清理。
- destructive、unbounded 或不符合 policy 的命令必须拒绝。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 Memory 状态。
- stdout/stderr 只是 evidence input，不能直接判定业务完成。
- evidence policy 只调整本次 Shell evidence 的结构化质量结论，不得修改 Runtime authority、coverage focus 或 Run 状态。
- 本 owner 私有 code 只在 Shell Biz 边界映射为 factual category；不得选择 correction、retry、terminal 或下一
  canonical state。unknown code 保留原 code/message 并 typed fail-fast。
- CWI reconciliation 只能执行 binding 封存的 validation recipe；recipe ID 不是 check ID，单独 exit 0 也不是证明。
  binding 的 `RequiredCheckRefs` 来自 owner `ready + direct` surface；缺失 exact unambiguous pass 时返回 unverifiable，
  不得把 generic `go test` 或空 TODO test 扇出为多个 CWI 的 semantic proof。非工具原生检查必须由封存 recipe 在 stdout/stderr 输出
  `WORKORCH_CHECK_PASSED=<check-ref>`（或对应 `FAILED`/`SKIPPED`）行，由 Shell owner 解析为结构化事实；
  candidate 不能直接提交这些标记或 check ref。同一 check 同时出现在 passed 与 failed/skipped 时整次 observation 为
  invalid，Orchestrator binding 与 assessment gate 也按相同规则拒绝 credit。
- 无当前适用 recipe 时，Shell 返回空的 confirmed recipe set，同时在同一 catalog 保留声明 recipe 的
  `prerequisite_required`/`unavailable` surface 供 aggregate/audit 使用；它不得把这些 surface 加入 provider-selectable
  refs、选择 File preparation 变更、请求 correction，或把 feasibility rejection 解释为新的 retry/terminal 状态。

## 测试覆盖

- 已覆盖 catalog 的 ready/prerequisite-required/unavailable、manifest freshness、空 confirmed recipe 与 owner selector。
- 已覆盖 prospective-satisfaction 对 ordinary script 与 `*_test.go` resulting target 的纯值判定，以及错误 source/scope、
  typed stale Shell catalog、duplicate request、多 prerequisite surface 和 recipe command/args/timeout 身份漂移；测试确认
  dependency catalog不进入provider choice、不创建target、不执行命令且不改变owner状态。
- 已覆盖强调用 nil result、伪造 source、receipt query failure、sealed data 漂移均在进程调用前拒绝。
- 已覆盖 managed validation 临时副本、原 workspace/`.git` 隔离、外指 symlink、正常完成与 timeout 的进程组清理。
- 已覆盖默认 software-development recipe 的 owner proof check、generic success/zero-work/all-skipped、显式 marker 与
  passed+failed / passed+skipped 冲突。
- 已覆盖 durable Shell 的零变化、未声明 effect、多目标合法 effect、receipt exact replay 与 claimed/no-receipt
  at-most-once recovery。
- 已覆盖输出截断、脱敏 diagnostic、timeout、非法/危险命令和 managed resource cleanup。
- 已覆盖 validation entry 不晋升 failure target、无结构化输出零 candidate，以及 managed Go diagnostic 仅报告实际
  manifest-scoped source target 并拒绝越界位置。

非阻断增强项：增加大型 workspace copy/observation benchmark，以及在更多 Unix 平台运行进程组清理兼容性矩阵。
