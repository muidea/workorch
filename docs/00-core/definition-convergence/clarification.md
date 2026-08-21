# Workspace Definition Clarification Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-18

## Scope

本合同只定义 pre-seal Workspace Definition admission failure 的 blocking ticket。显式 one-of、已声明多 provider
可替代语义与 unresolved declared prerequisite 不属于 canonical Definition，也不属于 SM-2 Runtime governance。
Decision owner 独占 ticket lifecycle；Workspace owner 独占 candidate、promotion 与 activation admission。

## Static pre-Run clarification

显式 one-of、已声明多 provider 可替代语义与 unresolved declared prerequisite 属于 pre-seal Workspace Definition admission
failure，不属于 canonical Workspace Definition 或 SM-2 Runtime governance。Workorchd application use case 必须在 pure static
candidate compiler 返回 violation 后按以下顺序收口：

1. pure compiler 返回 `definition_one_of_unsupported | declared_prerequisite_unresolved |
   definition_clarification_required`；只报告显式 relation/ref/mapping，不能声称已理解任意自由文本；
2. 在 promotion、创建 Workspace 对应 Run/Runtime aggregate 或发起 provider/capability work 前，通过同步 EventHub `Send` 向
   Decision owner 的 `pkg/events` 提交 concrete typed `OpenWorkspaceDefinitionClarificationRequestCommand`，并取得 concrete
   result；调用 context 必须已绑定 matching `WorkspaceID`；
3. ticket scope 固定为 `WorkspaceID + CandidateID + CandidateRevision + CandidateHash`，不含尚不存在的
   `DefinitionRevision/StartupInputHash`，也不含伪造的
   `RunID/DispatchID/AuthorityTransaction`。Run-scoped runtime-definition-clarification ticket/command 不得复用；
4. ticket 至少持久化 violation code、offending item/relation refs、canonical violation signature/hash、typed
   `RequiredActionKind` 和原始 candidate identity/hash。candidate-owned relation 使用 `rewrite_definition_all_of`；来自独立
   Template/Profile snapshot 的违规使用 `select_or_fix_template_profile`，不得向用户提供无法修改相应 owner 输入的动作；不得使用 `map[string]any`、自由文本 reason 或
   UI payload 作为 admission authority，也不得只凭 hash equality 合并 canonical signature bytes 不同的 violation；
5. ticket identity 由 `WorkspaceID + CandidateID + CandidateRevision + CandidateHash + violation code + canonical violation hash`
   确定性派生。同一无效 candidate 的重复 setup/admission 返回同一 `BlockingTicketID`，零重复写；
6. required action 只能要求用户通过 Workspace owner command 提交新的 immutable candidate，将可替代分支改写为完整
   all-of、唯一 provider 和明确 required refs。Decision owner 不读取 Workspace repository，也不替用户修改 candidate/Definition；
7. Workorchd 对新 candidate revision 重新执行同一 pure validation。新 candidate 通过 all-of admission 后，Workspace owner
   先原子 promotion，并将 `WorkspaceActivationAdmission` 封存为 `pending_clarification_resolution`；Workorchd 再通过 Decision owner-defined concrete
   `ResolveWorkspaceDefinitionClarificationByAdmissionCommand/Result` 提交 old/new candidate identity、admission-result hash 与
   promotion 产生的 `DefinitionRevision + StartupInputHash`，旧 ticket 以 `definition_candidate_adjusted` resolution 收口。
   matching result 返回后，Workorchd 必须使用 Workspace owner-defined
   `AcknowledgeWorkspaceDefinitionClarificationResolutionCommand/Result` 绑定 ticket/resolution/result hash；Workspace owner
   只有在逐字段匹配 promotion fact 后才把 activation admission CAS 为 `eligible`。任一步失败均不得创建 Run；
8. 新 candidate 仍无效时，Workorchd 通过 Decision owner-defined concrete
   `ReconcileWorkspaceDefinitionClarificationCandidateCommand/Result` 将旧 candidate ticket 以
   `definition_candidate_revision_superseded` 收口，并为新 invalid candidate 返回其确定性 blocking ticket；Decision owner
   在一个 owner-local CAS/reducer transition 中验证 old/new candidate 与 violation identity，避免两张票之间出现可运行窗口；
   两种情况都不得自动 Start/Resume/Restart 或创建 successor；
9. HTTP/Web/TUI 的 setup result 固定为
   `definition_clarification_required + ViolationCode + BlockingTicketID` 并提供可读修改入口；显示文本不能反向决定
   violation、ticket identity 或 resolution。

若 candidate persistence、Decision owner 建票、promotion、ticket resolution 或 Workspace acknowledgement 任一失败，
setup/activation 整体 fail-closed，仍不得创建 Run。已成功 promotion 的 startup input 可以作为 owner audit/current sealed input
保留，但其 activation admission 必须保持 pending；恢复只重放 exact command/result/ack identity。资格变为 eligible 后仍由用户
再次显式 Start/Restart，不得因 promotion 或 ticket closure 自动启动。

## Related Contracts

- execution-time implicit semantic gap：[Runtime Definition Clarification](runtime-clarification.md)
- candidate、promotion 与 activation admission：[Definition Admission](definition-admission.md)
