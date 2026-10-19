# Workspace Definition Clarification Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-26

## Scope

本合同只定义 pre-seal Workspace Definition admission failure 的 blocking ticket。显式 one-of、已声明多 provider
可替代语义与 unresolved declared prerequisite 不属于 canonical Definition，也不属于 SM-2 Runtime governance。
Decision owner 独占 ticket lifecycle；Workspace owner 独占 candidate、promotion 与 activation admission。Definition admission
对外只有 `DefinitionAdmissionIntentCommand/Result`；Workorchd 只组合 Workspace/Decision 用例，不选择 Workspace
target state。

## Static pre-Run clarification

显式 one-of、已声明多 provider 可替代语义与 unresolved declared prerequisite 属于 pre-seal Workspace Definition admission
failure，不属于 canonical Workspace Definition 或 SM-2 Runtime governance。Workorchd application use case 必须围绕
Workspace owner factual intent result 按以下顺序收口：

1. Workorchd 编译 immutable candidate，并发送 single `DefinitionAdmissionIntentCommand`。Workspace private reducer 在
   owner-local CAS 路径持久化 candidate、运行 pure compiler 并选择 owner transition；command 不得携带
   target state、promotion/ack 意图或 caller 编译的 admission outcome；
2. pure compiler 返回 `definition_one_of_unsupported | declared_prerequisite_unresolved |
   definition_profile_contract_invalid | definition_profile_prerequisite_unresolved |
   definition_clarification_required` 时，intent result 必须为 `clarification_required`，且只报告显式
   relation/ref/mapping/Profile proof contract，不能声称已理解任意自由文本；
3. 在 promotion、创建 Workspace 对应 Run/Runtime aggregate 或发起 provider/capability work 前，Workorchd 通过
   同步 EventHub `Send` 向 Decision owner 的 `pkg/events` 提交 concrete typed
   `OpenWorkspaceDefinitionClarificationRequestCommand`，并取得 concrete result；调用 context 必须已绑定 matching
   `WorkspaceID`。Workorchd 必须先执行 result 的 `ValidateFor(exact command)`；映射出的 immutable
   `WorkspaceDefinitionClarificationTicketFact` 只携带 owner-neutral request identity/ticket binding values，不能直接写入
   Workspace state，也不能凭自洽 pair 声称 Decision 已持久化；
4. ticket scope 固定为 `WorkspaceID + CandidateID + CandidateRevision + CandidateHash`，不含尚不存在的
   `DefinitionRevision/StartupInputHash`，也不含伪造的
   `RunID/DispatchID/AuthorityTransaction`。Run-scoped runtime-definition-clarification ticket/command 不得复用；
5. ticket 至少持久化 violation code、offending item/relation/Profile source refs、canonical violation signature/hash、typed
   `RequiredActionKind` 和原始 candidate identity/hash。candidate-owned relation/ref 使用 `rewrite_definition_all_of`；
   `definition_profile_contract_invalid` 与 `definition_profile_prerequisite_unresolved` 使用
   `select_or_fix_template_profile`，其问题文本明确要求选择或修订 Workspace Template/Profile proof contract。不得向用户提供
   无法修改相应 owner 输入的动作；不得使用 `map[string]any`、自由文本 reason 或
   UI payload 作为 admission authority，也不得只凭 hash equality 合并 canonical signature bytes 不同的 violation；
6. ticket identity 由 `WorkspaceID + CandidateID + CandidateRevision + CandidateHash + violation code + canonical violation hash`
   确定性派生。同一无效 candidate 的重复 setup/admission 返回同一 `BlockingTicketID`，零重复写；
7. required action 只能要求用户通过 Workspace owner intent 提交新的 immutable candidate：Definition-owned 输入将可替代
   分支改写为完整 all-of、唯一 provider 和明确 required refs；Profile-owned 输入则选择或修订 proof-complete
   Template/Profile snapshot。Decision owner 不读取 Workspace repository，也不替用户修改 candidate/Definition/Profile；
8. Workorchd 将新 candidate revision 放入同一 single intent。新 candidate 通过 all-of admission 且存在既往
   blocking clarification 时，Workspace private reducer 先返回 `clarification_ticket_required` 与 exact
   `PriorClarification`，不执行 promotion。Workorchd 打开/读取 matching Decision ticket，将其结果映射为
   immutable ticket fact 并重新发送同一 intent；Workspace Biz 必须先通过 Decision-owned concrete persisted-fact query
   取得 matching open Ticket record，且 Decision 必须同时核对 binding 与外层 canonical ticket 语义。Found=false、已
   resolved/retired ticket 或任一逐字段漂移均零 CAS；只有随后 Workspace reducer
   重算 ticket identity/violation binding 并逐字段验证该 fact 后，才原子
   promotion 并将 `WorkspaceActivationAdmission` 封存为 `pending_clarification_resolution`，result 返回
   `clarification_resolution_required`；
9. Workorchd 根据上述 factual result 通过 Decision owner-defined concrete
   `ResolveWorkspaceDefinitionClarificationByAdmissionCommand/Result` 提交 old/new candidate identity、admission-result hash 与
   owner promotion 产生的 `DefinitionRevision + StartupInputHash`，旧 ticket 以 `definition_candidate_adjusted` resolution 收口。
   Workorchd 必须先执行 result 的 `ValidateFor(exact command)`；matching result 只能映射为 immutable resolution
   Identity/Binding values 并附在同一 intent 上重新提交；Workspace Biz 先从 Decision ticket aggregate 与 resolution repository
   查询 exact persisted Ticket+Resolution record，partial/missing/mismatch 零 CAS；Workspace private reducer 再次执行
   `ValidateFor`、重算 resolution identity/binding，且逐字段匹配 candidate、ticket、resolution、`DefinitionRevision` 与
   `StartupInputHash` 后才 CAS 为 `eligible`。任一步失败均不得创建 Run；
10. 新 candidate 仍无效时，Workspace intent result 保持 `clarification_required` 并可返回 exact
   `PriorClarification`；Workorchd 通过 Decision owner-defined concrete
   `ReconcileWorkspaceDefinitionClarificationCandidateCommand/Result` 将旧 candidate ticket 以
   `definition_candidate_revision_superseded` 收口，并为新 invalid candidate 返回其确定性 blocking ticket；Decision owner
   先 durable ensure 新 ticket，再以可重放的 resolution/ticket lifecycle write 退休旧 ticket。该顺序逐字段验证 old/new
   candidate 与 violation identity，并保证任一切点至少保留一张 blocker；它不是跨两个 aggregate 的伪原子 CAS；
   两种情况都不得自动 Start/Resume/Restart 或创建 successor；
11. HTTP/Web/TUI 的 setup result 固定为
   `definition_clarification_required + ViolationCode + BlockingTicketID` 并提供可读修改入口；显示文本不能反向决定
   violation、ticket identity 或 resolution。

若 candidate persistence、Decision owner 建票、Workspace owner promotion、ticket resolution 或 owner 对 resolution fact 的验证/迁移任一失败，
setup/activation 整体 fail-closed，仍不得创建 Run。已成功 promotion 的 startup input 可以作为 owner audit/current sealed input
保留，但其 activation admission 必须保持 pending；恢复只重放 exact intent 与 Decision ticket/resolution fact identity。
资格变为 eligible 后仍由用户
再次显式 Start/Restart，不得因 promotion 或 ticket closure 自动启动。

### 已确认 authoring 的票据交付恢复

已确认 authoring 的 journal 为 `materialized`，且 Workspace owner 查询已核验当前 admission candidate
与 journal 的完整 authoring ref 一致时，若原 blocking ticket 缺失或交付尚未验证，Web 可以提供显式的
“恢复原确认／票据交付”。仅复用既有 confirm 的五字段及原 Workspace；重开只消费 owner 投影的
`confirmationRecovery`，不得从客户端拼造或自动选择新候选。内存中的未决原意图不得被回读的新 revision 替换。

GET 只查询：Decision 不可用、原票据缺失、创建通知标记未持久化或开放生命周期不匹配时，均不得宣称票据交付完成。
显式恢复仍由 Workspace 独立重验原 journal、candidate、当前绑定与必要栅栏，再由 Decision 创建同一确定性票据，
或逐字段验证已持久原票据并补交创建通知。不得换 candidate、改规格、解决票据、调用模型或创建 Run。
恢复完成仍是 `clarification_required`，不授予 eligible；其余恢复信息缺失／漂移保持只读阻塞。

HTTP 409、网络中断与异常响应不证明无写入，可能发生在正式物化之后。页面必须保留原意图并 GET 对账，
只允许显式精确重放；关闭仅停止观察，不取消后台事实。上述交付恢复不是通用修订／澄清重试入口。

## Related Contracts

- execution-time implicit semantic gap：[Runtime Definition Clarification](runtime-clarification.md)
- candidate、promotion 与 activation admission：[Definition Admission](definition-admission.md)
