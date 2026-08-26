# Definition Convergence Verification

Status: normative

Type: verification-contract

Last Verified: 2026-08-26

## Purpose

本文定义 Definition convergence 实现必须通过的稳定验收合同；场景是 required assertions，不是逐项完成状态或实施队列。

## Verification Matrix

### Definition and graph

| Scenario | Required assertion |
| --- | --- |
| reorder Goal/DoD arrays only | semantic graph/rank 不变；presentation可独立变化 |
| rename all opaque IDs | 映射后的相对语义顺序不变；无 lexical/numeric ID influence |
| smaller numeric ID is dependent | prerequisite仍先执行 |
| clarify same semantic item | 保留 ItemRef、创建新 Definition revision；既有 Run 不漂移 |
| materially replace/delete item | old ItemRef retired且不复用；fresh activation使用新 mapping |
| pre-seal candidate | exact proposed Goal/DoD、Template snapshot、分别规范化的 Template/Workspace `EvidencePolicies` snapshot bindings、transitively sealed Profile/policy bindings、完整 normalized typed Profile contract snapshot、required owner-fact/convergence bindings、Profile contract hash、compiler version、resolved directory 与 admission-only relation input 产生稳定 versioned `CandidateHash`；仍无 Definition revision/startup-input/Run |
| current candidate seal | `CandidateHash` 同时覆盖双方每项 `AdmissionPolicyBindings` ref/revision/snapshot hash、acquisition operation variant、operation target policy、operation selector，以及 prospective policy 必需的 `ProspectiveSatisfactionContractRef`；caller/Workspace owner/pure compiler均不能重算 hash 后接受发生漂移的 immutable candidate |
| legacy target in evidence policy | Template 或 Workspace 任一 `EvidencePolicies` snapshot 嵌套出现退役 `prospectiveTargetRef`，都在 candidate submit/promotion 前同步拒绝；零 startup-input/Run/provider/capability work，且不能把该值复制到 Definition、Runtime proposal 或 binding |
| lossy/invalid evidence policy | Template 与 Workspace 分别拒绝空/非字符串 policy ID、同 owner 重复 ID、typed policy validation failure 或 normalize cardinality drift；跨 owner 同 ID 仍是两个独立 snapshot；零 candidate promotion/Run |
| evidence-policy snapshot drift | Template 或 Workspace 任一 policy snapshot 的 canonical bytes/ref/revision/hash 变化都要求同一 lineage 的新 candidate revision/hash；stale candidate/promotion 拒绝，已封存 candidate/admission/Run 不漂移 |
| bootstrap evidence-policy projection drift | final startup seal 将 activation typed policies 与 admitted Workspace raw snapshot 的无损 typed projection exact 比较；缺失、增加或字段漂移同步 conflict且零 Run owner ingress |
| Profile snapshot/hash drift | candidate 的 typed Profile snapshot hash 与`RuntimeProfileContractsHash`不一致时，owner ingress在admission前拒绝；不得仅凭caller提供的hash或bindings继续 |
| candidate lineage/revision | exact normalized duplicate 返回原 CandidateID/revision/hash 与确定性相同的 Workspace-owner admission result；materially changed input 只以 next revision CAS 新增；stale expected revision 零覆盖/零 promotion |
| invalid candidate | 只持久化 candidate/violation/ticket lineage；零 canonical item/relation、`StartupInputHash`、Runtime/provider/capability work |
| admitted candidate promotion | submit 的 `DefinitionAdmissionResult` 由 Workspace owner compiler 生成；identity-only promotion 在 owner-private event 中重新编译 exact-current candidate，并一次 CAS 产生正式 Definition revision/startup input 与 activation-admission fact；caller 无 outcome/result-hash 写权限；无 static ticket 时直接 eligible，corrected ticket lineage 必须 pending；duplicate 返回同一 identity，stale/mismatch/unsupported alternative 零部分写 |
| startup input seal | promoted exact Goal/DoD、Template snapshot、resolved directory 与 Definition revision 共同产生稳定 `StartupInputHash` |
| change any startup input | 新 owner-sealed startup-input snapshot/hash；Goal/DoD/关系变化另产生新 Definition revision；当前 Run/bootstrap/graph 不变 |
| required item has no CWI / CWI has two primary items | static mapping validation fail-closed before Run creation |
| constraint has no consumer / all-of aggregation has no gate | static disposition validation fail-closed |
| required Profile CWI lacks exact proof declaration | candidate promotion 前`definition_profile_contract_invalid + select_or_fix_template_profile + BlockingTicketID`；wildcard evidence policy不能代替exact `scope + id`；零 startup-input/Runtime/provider/dispatch/effect |
| required Profile CWI has empty/unsupported evidence kinds or no proof-capable accepted operation | 同一确定性Profile violation/ticket；不得降级为assessment-only focus；零 startup-input/Runtime/provider/dispatch/effect |
| duplicate required Profile source across profiles | `definition_profile_contract_invalid`精确报告全部Profile source refs；不得按Profile顺序覆盖或合并 |
| required Profile owner fact has no unique acquisition/reobserve closure | candidate promotion 前 `definition_profile_prerequisite_unresolved + select_or_fix_template_profile + BlockingTicketID`；零 startup-input/Runtime/provider/dispatch/effect |
| candidate owner-fact manifest differs from exact Profile snapshot derivation | `definition_profile_prerequisite_unresolved`；Application不能省略、替换或附加binding，Workspace pure compiler独立派生比较 |
| required owner fact declares exact one-shot acquisition | candidate封存missing trigger、acquisition owner/kind/selector、reobserve owner/fact与max=1；promotion/startup hash稳定；semantic obligation 同时保留 terminal 与 current evidence kinds；missing 时只能封存该 exact acquisition；accepted operation 后 reobserve 仍缺失则零第二次 acquisition并 typed exhausted，reobserve ready则重开同一CWI并以fresh generation恢复terminal proof，acquisition零terminal credit |
| prospective owner-fact convergence declaration | `prospective_owner_target` 恰有 nonempty Profile-derived `ProspectiveSatisfactionContractRef`；missing/extra/drift或其它policy携带ref均在promotion前`definition_profile_prerequisite_unresolved`，零Run/provider/capability work |
| prospective target authority split | pre-seal candidate/Definition 无 exact prospective target set，只保留 policy、declaration selector/candidate refs、acquisition closure与contract ref；compiled obligation/manifest 只追加 operation selector/current owner constraints；SM-3 schema只接受一份proposal中1～8个唯一canonical `ProspectiveTargetRefs`，逐项位于两者交集，非空candidate refs要求逐项exact member match，并拒绝singular兼容字段；File/Shell fact精确绑定proposal hash/target set，只有SM-2 sealed binding形成authority；operation candidate只引用binding，并必须按每个target的exact-singleton `RequiredOperationSequence` 齐全、有序提交，dispatch只从binding派生target |
| free text may imply an undeclared prerequisite | static compiler 不做领域猜测或伪报完备；执行期只能走 typed semantic-gap lane |
| dangling/self dependency | fail-closed before Run creation |
| hard/reopen cycle | exact typed cycle path；零 Runtime work |
| explicit one-of in Goal/DoD/Profile/user-submitted candidate | admission-only token 产生 `definition_one_of_unsupported + BlockingTicketID`；零 promotion/startup-input/Run/provider/dispatch/effect；Profile-sourced violation 返回可执行的 template/profile 修订动作 |
| ambiguous providers | 不提供 any-of escape；返回 clarification blocking ticket并要求唯一 provider/required refs |
| duplicate invalid admission | 同一 candidate/violation 返回同一 ticket；Workspace/Decision owner零重复写，仍无 startup-input/Run |
| corrected static-ticket candidate | 重新执行 pure all-of admission；promotion 后 activation admission 保持 pending，Decision resolution + Workspace acknowledgement 精确回绑后才 eligible；不自动 Start/Resume/Restart |
| replacement candidate still one-of | 旧ticket以`definition_candidate_revision_superseded`收口；新candidate取得确定性blocking ticket；仍无promotion/Run |
| stale/mismatched clarification outcome | workspace/candidate/violation/admission/promotion identity不匹配时拒绝关闭current ticket；不影响新candidate |
| Decision ticket persistence/resolution/Workspace ack unavailable | activation admission 保持 pending；所有 Start/Restart ingress 返回 typed pending，零 Run/provider/dispatch/effect；恢复只补齐同 identity resolution/ack |
| fresh Start/Restart ingress while static clarification pending | Workspace owner 即使已有 promoted sealed input 也拒绝选择 pending revision；Application、HTTP、TUI 或重复请求均不能绕过 activation-admission fact |
| Restart with invalid persisted admission candidate | admission 必须精确定位唯一 candidate，并验证 current schema/compiler、candidate/startup/admission binding 与 `admitted_all_of`；缺失/重复/旧 shape/hash/selector closure 漂移同步 conflict，managed-config mutation、execution-intent/admission outcome、Workspace/Run/provider/effect写入均为零；不迁移或补写旧数据 |
| existing-Run Resume while a newer candidate is pending | Resume reuses the Run's original eligible startup identity; it neither absorbs nor promotes the newer candidate |
| semantic duplicates | commutative、合并或 typed ambiguity；绝不按 ID兜底 |
| fan-in/fan-out | 使用 all-of gate，graph size接近 `O(N+E+G)` |
| all-of gate | focus witness逐项覆盖全部 member和 exact current evidence lineage |
| terminal root | 唯一 `TerminalAllOfGate` 覆盖全部 required terminal member；缺一项不能 complete |
| indeterminate/unknown-pass policy in Goal/DoD/Profile/candidate | `definition_terminal_policy_unsupported + BlockingTicketID`；零 promotion/startup-input/Run；当前基线不存在成功例外 |
| required CWI assessment is indeterminate | terminal all-of witness缺失且successful closeout被拒绝；保留reason/missing owner fact，只能继续有界收敛或typed failure |
| input permutation | Definition/scheduling hash稳定 |

### Semantic priority

| Scenario | Required assertion |
| --- | --- |
| high-priority goal has low-level prerequisite | prerequisite继承 effective high priority |
| one item directly unlocks many high-priority dependents | 优先于无关小项 |
| deep fan-out with large transitive closure | 不计算 descendant cardinality/reachability materialization |
| ready-queue comparison | comparator不遍历graph；通常`O(1)`，signature hash collision时仅做`O(L)` canonical bytes比较 |
| lower-ranked CWI shares current partition | cache affinity不能越过更高rank的cross-partition CWI |
| equal unlock but different critical depth | longer critical path先行 |
| equal semantic value but different cost band | smaller declared prerequisite先行 |
| presentation priority changes | execution rank不变 |
| Definition `Order`/ID changes | execution rank不变 |
| semantic hash collision, different signatures | 比较 canonical bytes，不误判 duplicate、不读 ID |
| exact semantic tie | 明确 ambiguity/commutative rule，不读 ID |
| prerequisite reopened | dependent退出 ready；只重投影影响闭包 |
| current focus bound/effect-fenced | 任何新 rank都不能替换它 |

### Context and LLM

| Scenario | Required assertion |
| --- | --- |
| manual startup authoring | Template + Goal/DoD + blank directory 无需强制 setup-draft LLM 即可生成candidate、完成all-of admission/promotion，目录解析为默认路径；不限制后续 Runtime LLM |
| execution-time LLM identity | 每个 request/result 绑定 exact Workspace/Run/Definition/`StartupInputHash`；stale/mismatch 在 provider/authority mutation 前拒绝 |
| large Definition | provider只收到 bounded exact C1 partition；aggregate保留完整 authority |
| focus changes inside one partition | C1/StablePrefixHash不变，只改变C3 |
| focus changes across partitions | 先持久化partition-switch generation，再创建新logical request/prefix |
| proof batch contains multiple CWI | 全部home partition相同；cross-partition CWI必须拆为另一logical request |
| omitted unrelated partition | 不进入 prompt noise，但在 partition/context audit 可见 |
| required external context | 只可请求 manifest-authorized refs；C5 有界扩展且 C1 不变 |
| attempt 0/1 | DefinitionPartitionHash、C0～C3、C5 expansion和StablePrefixHash精确相同 |
| LLM returns global order | strict schema rejection；零 focus mutation |
| LLM orders by IDs | 输出无该字段/权限，不能影响 scheduler |
| LLM proposes rewritten Goal/DoD/Template/directory | schema/integrity fail-closed；零 Workspace/bootstrap/graph/startup-input mutation |
| LLM exposes implicit one-of | 只封存 bounded diagnostic；SM-2/SM-3 建 current Run scoped governed ticket，affected focus 零 candidate/dispatch/effect |
| LLM exposes undeclared prerequisite for CWI with sealed acquisition | pure manifest admission 返回`rejected/owner_fact_acquisition_declared`并进入同request bounded correction；零gap observation、governance、ticket、Definition mutation或effect |
| LLM exposes admitted undeclared prerequisite/semantic ambiguity | 不自动补 relation 或扩图；只有通过exact manifest/expected-document admission后才绑定 current authority 的 `revise_definition_and_restart` ticket |
| Profile required owner fact closure omits operation selector/variant/policy | pre-seal `definition_profile_prerequisite_unresolved`；零 Run/provider/capability work |
| Definition selector differs from capability operation selector | manifest 同时封存两份 identity；provider schema只允许 operation selector，返回 Definition selector时 attempt-0 typed correction、零 dispatch |
| prospective dependency catalog provider isolation | required-owner `OwnerFactDependencyCatalogs` 进入 manifest/hash/freshness，但不进入 C3 owner catalog、allowed operation/surface、schema enum 或 correction allowed values；LLM 无法选择 Shell readiness/satisfaction |
| prospective target typed constraint | required-owner catalog发布唯一provider-safe `ProspectiveTargetSatisfactionConstraint`，只允许`all_of` mandatory clauses与clause内合取predicate，不存在`one_of`/`any_of`；同一值封存到acquisition/manifest/C3，dependency recipe identity/command不进入provider；1～8个target覆盖全部clause、每个target至少贡献一个clause后才允许File/Shell probe |
| assisted draft returns one-of | raw draft 只形成不可采用 diagnostic，UI/schema 不把它显示为合法 relation；只有用户另行显式提交的 admission candidate 才可保留 token 并进入 candidate-scoped ticket |
| malformed/timeout context request | whole-document recovery或typed terminal；不扩 dependency/priority |

### State machine and recovery

| Scenario | Required assertion |
| --- | --- |
| scheduling contract missing | SM-3 provider request和dispatch均为零 |
| internal caller bypasses Profile admission | Runtime focus compiler在任何SM-2 focus mutation/SM-3 request前重复执行同一pure Profile validation并fail-closed；不建第二状态机或ticket |
| duplicate schedule/private event | zero revision/CAS |
| CAS conflict during seal | reload + pure recompute；无 process-local order复用 |
| crash after schedule before proof | exact contract/window恢复；一次 logical request |
| crash after dispatch/receipt | 不重放 durable effect；一次 evidence credit |
| owner-fact acquisition reobserve ready | `assessment_observed`一次CAS保留operation/evidence/assessment与attempt lineage，但CWI仍open/reopened；current binding清除、generation退休、successor manifest恢复原terminal evidence；零terminal credit/ticket，restart零重复CAS/effect/request |
| owner-fact acquisition reobserve missing | exact accepted attempt计入`MaxAcquisitionAttempts=1`并`owner_fact_convergence_exhausted`；零第二acquisition/reopen/provider escape |
| prospective satisfaction would-satisfy | File outer feasibility生成base-revision-bound immutable delta；Shell只消费delta+exact recipe+dependency catalog/observation并返回nested observation；CAS与seal双重验后才可封存，probe全程零File/Shell effect |
| prospective satisfaction dependency stale/missing | Shell typed stale保留为nested owner fact并只消费bounded refresh generation；不得伪造File outer stale或直接terminal；不改变semantic attempt/directive/rejection ledger，不消耗correction；probe前missing dependency退休generation且零Shell call；refresh后新manifest/request，duplicate/restart幂等 |
| prospective target constraint mismatch | schema/document admission返回manifest/CWI/`prospectiveTargetRefs` typed violation；empty/partial/extra/duplicate/越界或未覆盖全部clause都允许一次普通完整文档/schema correction，复用transaction/manifest/C0～C3/stable-prefix且C4只引用违规field、不复制constraint；全程零File/Shell probe/binding/effect/coverage/ticket |
| prospective satisfaction would-not-satisfy | 先持久化唯一exact-current File delta + Shell observation composite；因proposal已通过published `all_of` constraint，该结果只能形成`prospective_satisfaction_contract_violated -> semantic_contract_failed`，不得生成replacement target、Allowed、第二provider request或第二owner probe；duplicate/restart零重复terminal/probe，所有分支零ticket/governance/acquisition/operation/effect/coverage |
| prospective observation CAS race | File tree、File catalog/observation、Shell dependency catalog/observation或recipe schema在probe→CAS/持久化→seal任一窗口漂移时，exact-current composite校验拒绝seal并有界刷新；零binding/dispatch/effect |
| settled CWI unlocks next wave | SM-2重投影并继续正向推进 |
| reopen after resource effect | exact successor closure重开；无全局重排/全量 prompt |
| all-of gate witness member reopened | 原 witness 立即 stale且 dependent 退出ready；全部member再次current后才可重封存 |
| gate witness stale after effect fence | 不换focus/witness、不重放effect；原receipt收敛后拒绝新credit并targeted reopen |
| ordinary governance wait/resolution | startup input/graph/partition/focus/witness保持；既有 continue/fail 与 continuation fact 行为不受 Definition-clarification 专用合同影响 |
| duplicate runtime gap observation/redrive | 同一 lineage 返回同一 `BlockingTicketID`；零重复 provider request/ticket write/CAS |
| stale or mismatched runtime gap observation | 拒绝建票、改 graph 或增 revision；current authority 不变 |
| runtime definition-gap ticket open | 只有 persisted clarification dispatch 可调用 `OpenRuntimeDefinitionClarificationRequestCommand/Result`；result 的 canonical request binding hash 精确回绑同一 dispatch；普通 `OpenRuntimeGovernanceRequestCommand` 调用为零；保持 governed wait，无 `continue`/Resume resolution |
| duplicate/stale clarification open | duplicate 返回同一 ticket/result且零重复 Decision write；stale dispatch/scope拒绝建票和 Run mutation |
| oversized clarification payload | signature/ref/safe-diagnostic 任一超限在 Decision Send 前 typed fail-closed；零 ticket/Decision CAS，current governed authority不变 |
| crash after Decision clarification-ticket CAS before Runtime binding CAS | 重发 exact open command并接收 duplicate result，只补齐同一 clarification request record；零第二 ticket/provider request |
| revised input while current Run waits | Workspace 经新 candidate admission/promotion 产生新 revision/hash，current Run 仍绑定旧 startup input；不自动 Resume/Restart |
| Resume | 复用同一aggregate/`StartupInputHash`/schedule/partition/focus；不吸收新 revision、不重编译、不重放effect |
| Restart intent after definition gap | explicit Restart 只先持久化 ticket/old-new-startup-input-bound `ReplacementIntent=pending_clarification_resolution`；Decision ack 前零 predecessor terminal/delivery/successor |
| clarification replacement resolution | 只有 Run owner 可提交专用 typed command；Decision `resolved` / `duplicate` 的 resolution binding hash 精确回绑同一 intent，且 `definition_replacement_intent_bound` 不声称 successor 已创建；不调用 Orchestrator governance resolution且零 `RuntimeContinuationEligibleFact`/active transition |
| crash after replacement intent CAS | 重投同一 Decision command；无第二 intent/resolution/successor |
| crash after Decision resolution before Run ack | duplicate result join后 Run CAS ack；不重复 Decision effect，不恢复 predecessor active |
| stale replacement already bound | exact persisted intent只返回duplicate；不同identity时Decision返回current intent identity/hash，Run持久化invariant-breach rejection并fail-closed，零hash反推、自动覆盖或循环重试 |
| mismatched replacement scope | Run 将 intent CAS 为 `clarification_resolution_rejected`；只有不同RequestID的显式新 Restart 可将scope-mismatch rejection移入immutable history后重建，零自动 supersede/delivery/successor |
| Restart after definition gap | 只在 explicit Restart + matching intent + Decision ack + predecessor delivery 后fresh compile；不继承schedule/binding/credit/LLM document/context expansion |
| predecessor already has owner effects | successor reconciliation 重新观察；不直接继承 credit |
| dynamic capability unavailable | Run后先由SM-2读取owner-defined typed recovery合同；具备exact condition、trigger/deadline和budget时持久化bounded recoverable runtime wait且provider/dispatch/effect均为零，否则typed fail-closed；duplicate/restart不创建第二wait或重复消费 |
| all CWI settled | artifact/coverage closeout后 sealed terminal |
| partial delivery | 仅重试缺失的 Result/Workspace/Run acknowledgement |
| invalid/retired persisted aggregate | strict load failure；零 owner transition、provider/capability work |

### Scale and performance

至少提供 1,000 CWI、线性/分支/聚合图的 deterministic fixture，并证明：

- graph traversal、cycle validation、priority propagation、critical depth、direct unlock 和 full ready fact evaluation 为
  `O(N+E+G)`；设 ready set 为 `R`、bounded window 为 `W`，ready heap/window selection 为
  `O(R + W log R + W log W)`；完整 ready-set serialization/hash 另计 `O(R log R)`；graph canonical
  normalization/hash 的最坏上界另计 `O((N+E+G) log(N+E+G))`；
- 不构造 transitive reachability matrix/bitmap/count；单项 settle/reopen 的增量路径为
  `O(A+E_A+G_A+Q log R)`，其中 `Q` 是受影响 ready-heap entry 数；最坏不超过一次 full fact evaluation + window selection；
- static rank input 只在 scheduling seal/reseal 时 reverse-topological 计算；ready comparator 不做 graph walk，通常 `O(1)`，
  hash collision fallback 为 `O(L)`；direct unlock counter 只读取 current facts + sealed contract 的纯派生
  unsatisfied-member counter，不形成第二份 authority；
- 无 source × dependent 的无界 `N×M` materialization；
- active focus、semantic proof batch、C1 partition和C5 external context始终不超过各自 sealed budget；
- duplicate projection近似零分配/零 CAS，单项 settle/reopen只重算必要闭包；
- stable graph/scheduling/request hash 在多次运行和输入 permutation后相同；
- provider请求大小不随全部已完成/无关 Definition正文线性增长；同 partition 多个 focus 复用同一 stable prefix。
