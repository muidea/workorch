# Workspace Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-25

## 功能范围与边界

`blocks/workspace` 管理 Workspace、Goal、DoD 和 WorkspaceTemplate 组成的 workspace-definition 聚合，包括定义版本、激活绑定和模板查询。它只管理该聚合的正式状态，不承担 Run 调度或跨组件治理。

Workspace 还独占 immutable pre-seal `DefinitionAdmissionCandidate`、promoted `WorkspaceStartupInputContract` 与
`WorkspaceActivationAdmission`。candidate 可以保存尚未获准的 relation input；正式 startup input 只允许 pure compiler
确认的完整 `all_of`，并以 `StartupInputHash` 封存 Definition、Template/Profile contract hash、目录和 candidate-item mapping。
candidate 还封存 required owner-fact binding 与唯一 acquisition/reobserve closure；Workspace 只持久化/校验该事实，
不读取 capability catalog 或选择运行期动作。`CandidateHash` 覆盖 acquisition operation variant、operation target policy、
operation selector 与 Definition declaration selector/candidate refs；owner ingress 不替 caller 重新计算 hash。pre-seal
candidate/startup input 不携带 exact `ProspectiveTargetRef`；该值只能由运行期 SM-3 semantic proposal 在这份
declaration 与 current owner constraints 的交集中提出，Workspace 不补默认值、不回写 proposal/owner fact/binding。
candidate 的 `AdmissionPolicyBindings` 分别保存 Template 与 Workspace 两侧每项 canonical evidence-policy
ref/revision/snapshot hash，`CandidateHash` 覆盖这些 immutable bindings。任一 raw policy snapshot 含退役
`prospectiveTargetRef`、空/非字符串 ID、同 owner 重复 ID、invalid typed policy 或 normalize cardinality drift 时必须在
submit/promotion 前拒绝；Workspace 不把 policy binding 解释为 target authority，也不让 live policy 漂移修改既有
candidate/admission lineage。Application final bootstrap gate 另行 exact 比较 admitted Workspace policy 的 typed projection。

candidate 同时封存完整 normalized typed `RuntimeProfileContracts` snapshot，而不只保存 hash。pure compiler 从该 snapshot
独立派生 owner-fact/convergence manifest，并要求与 candidate bindings 逐项相等；每个 Profile-required acceptance/deliverable
CWI 还必须有 exact `scope + id` evidence requirement、非空受支持 evidence kinds，以及至少一个能返回对应 typed owner receipt
proof 的 accepted operation。wildcard policy 不能替代 exact Profile-owned CWI proof，也不能把无证明交付项降级为
assessment-only Runtime focus。

pure compiler 位于 Workspace owner package，只做 immutable candidate 的静态计算。submit owner-private event 在同一 CAS
路径生成并返回 exact `DefinitionAdmissionResult`；promotion command 不接收 caller 计算的 admission outcome，promotion
owner-private event 必须重新编译 repository 中的 current exact candidate，且只允许 `admitted_all_of`。Application 对同一
pure helper 的 read-side 调用只是防御校验，不产生或迁移 admission state。

## 组件交互及合同

- Workspace、Goal、DoD 和 Template 的 CRUD input，以及 Template defaults ensure input，均由
  `blocks/workspace/pkg/events` 定义具体 command 类型；普通 CRUD 当前共享 owner-local typed result union
  `WorkspaceCommandResult`。该 union 不包含 `map[string]any` authority，也不替代各操作的具体 command。
- Workspace creation command 只携带 identity、描述和已解析目录，不携带 `State` 或 active slot。
  `workspace.creation` owner-private event/dispatcher 是唯一 repository Create 入口，由 Workspace owner
  选择 initial `active` 并绑定 matching workspace context；protocol/service 不得构造 caller-selected initial state。
- Workspace 创建、状态变化、删除以及 Goal/DoD 定义变化由本 block 发布具体通知。
- `application/workorchd` 与需要 definition fact 的 owner 通过 EventHub 读取 Workspace、Goal、DoD、Template 和
  definition revision；其他 owner 不直接访问 Workspace repository。
- `application/workorchd` 通过 owner command/read port 管理和展示 workspace definition。
- Definition admission 使用三个独立 concrete command/result：submit candidate、promote admitted candidate、acknowledge
  clarification resolution。submit result 携带 Workspace owner 生成的 exact admission result；promotion command 只携带
  candidate identity/hash 与可选 ticket，不携带 admission result/hash。promotion 若绑定 blocking ticket，只能先进入 `pending_clarification_resolution`；逐字段匹配
  Decision resolution 后才可 CAS 为 `eligible`。eligible revision 只关闭不晚于自身的旧 clarification lineage；其后的
  invalid candidate 仍须由后续 corrected candidate 精确 reconcile。三个 handler 只把 immutable fact 翻译成
  exactly-one owner-private event；candidate/startup/admission 的状态选择和 CAS 只经过一个 dispatcher，调用方不能携带
  target state。任一步失败都不能把 admission 解释成可启动。
- `kernel/system` 发送 `EnsureWorkspaceTemplateDefaultsCommand`；本 block 只为缺失的内置 ID 创建默认模板，已存在对象保持不变，并返回具体 result。
- `WorkspaceTemplate` 是 workspace owner 的 DTO；EventHub result 只返回该 owner DTO，并在 module 内部完成与 repository entity 的双向 mapping、slice/map/payload clone。repository 类型不得泄漏给 application 或其他 owner。
- 外部定义命令保持唯一入口 `handleCommand -> executeCommand`；Owner 内部按 Workspace、Goal、DoD、Template
  四个命令族做穷尽且 fail-closed 的静态分派。各 handler 只是同一 Biz 内的职责切片，不是新的 Owner、状态机、
  repository/CAS 边界或 EventHub facade。
- Goal/DoD/Workspace repository 协调器是 Biz 包内私有实现；本 block 不导出可绕过 EventHub ingress 的 manager 或
  CRUD service facade。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Workspace | Workspace/Goal/DoD/Template CRUD、pre-seal candidate submit/promotion/ack、definition read 与 execution intent ingress | 完整 |
| `kernel/system` | System → Workspace | `EnsureWorkspaceTemplateDefaultsCommand` | 完整 |
| `blocks/run` | 双向 | Workspace → Run 的 scoped execution intent；Run → Workspace 的 `RunStateChangedData` | 完整；状态转换分别留在各自 owner |
| `kernel/orchestrator` | Orchestrator → Workspace | typed definition/activation fact 读取；sealed terminal delivery fact | 完整；不访问 repository，不直接写 Workspace model |
| `blocks/eventstream` | Workspace → Eventstream | create/state/delete、Goal/DoD definition 通知 | 完整 |

## 相关约束

- 只访问 Workspace、Goal、DoD 和 WorkspaceTemplate repository，不得持有 Run、Result 或其他 owner repository/service。
- 所有 Goal/DoD command 必须显式携带 WorkspaceID；owner command boundary 绑定同一 execution scope，memory/file
  repository 对 scoped Get/Update/Delete 使用一致语义。同一 GoalID/DoDID 可在不同 Workspace 中重复；无 scope 且
  identity 不唯一时必须 fail-closed。
- Goal/DoD definition revision 使用 owner-local `<kind>:<20 位递增代次>:<content-hash>`；同一 Workspace、同一
  definition kind 内可按代次/字符串单调比较，`A → B → A` 仍产生新 revision。相同规范化内容的重复 update 是零
  revision。新 identity 只能连接当前唯一 canonical leaf，不得按 ID、列表顺序或 repository 顺序推断前驱；历史
  definition 不可直接更新，当前 canonical leaf 不可物理删除。激活绑定变化必须使旧定义选择可检测为 stale。
- Goal/DoD 当前 definition 必须显式携带 `LineageState=active`。空 lineage 或其他不完整 current 字段直接冲突且零写入；
  Workspace owner 不补默认值、不重写对象，也不产生任何 schema upgrade 结果。
- Workspace block 不驱动 Runtime、不判定 Result、不执行治理恢复；execution-intent ingress 只校验当前
  Workspace startup input/admission、选择 active Run，并向 Run owner 发送 concrete typed intent。Restart 必须逐字段匹配
  current eligible `DefinitionRevision + StartupInputHash`；Resume 只选择现有 Run，不重新读取或附带最新 Goal/DoD。它消费 scoped sealed terminal outcome，按
  outcome identity 在自身 reducer 中释放 `ActiveRunID` 并维护 Workspace state projection；首次确认必须满足
  `ActiveRunID == outcome.RunID`，空 slot 不得补写新的 acknowledgement fence，已持久化的同 identity 重投递才是
  idempotent noop。它不读取
  `ConvergenceState`、RuntimeAction、focus、repair 或 error message 重新裁决；delivery feedback 保留 stable
  category 与完整安全 original diagnostic。
- 正式 DoD definition resource 只保存 `ready`。pre-seal draft 属于 `DefinitionAdmissionCandidate`；每个 Run 的
  satisfied/unsatisfied/indeterminate assessment 与 terminal closeout 属于 Orchestrator，不能回写 DoD state。
- `fresh_activation_required` 只表示可读 current Run 缺少 sealed bootstrap；普通 Run notification 不释放 Workspace slot。
  无法按当前结构读取的持久化数据在 execution admission 前失败，Workspace 不创建 successor、不读取或修补该数据，
  也不通过旁路 fact 清除 active slot。
- Start/Restart 在任何 Run-owner command 前必须从 `WorkspaceActivationAdmission` 精确定位唯一 immutable candidate，并验证
  `candidate -> startup input -> admission` 的完整 current-shape 绑定。缺 candidate、重复 identity、hash 漂移、旧 schema/compiler
  或不再满足 `admitted_all_of` 的 closure 都直接 conflict；不得重封存、补字段、创建替代 candidate 或生成兼容 admission outcome。
- Start/Restart 必须逐字段匹配 promoted `RuntimeProfileContractsHash` 与 candidate 中的 exact typed Profile snapshot。
  Profile-required CWI proof 不闭包时，pure admission 在 promotion 前返回 `definition_profile_contract_invalid`；Profile-derived
  owner fact 缺少 preexisting 声明或 exact one-shot acquisition/reobserve closure，或 candidate manifest 与 snapshot 派生值不同时，
  返回 `definition_profile_prerequisite_unresolved`。两者都要求 `select_or_fix_template_profile` blocking ticket；Workspace 不得用
  live owner 事实、LLM 文本、wildcard evidence 或默认动作修补 candidate。candidate-owned relation/ref failure 仍使用
  `declared_prerequisite_unresolved + rewrite_definition_all_of`。
- 默认 WorkspaceTemplate 是可由用户编辑的首次启动种子；普通 ensure 不得覆盖内置 ID 上的当前对象。当前阶段不提供
  默认资源升级或数据迁移；需要新默认值时清理数据后重新初始化。
- current candidate schema/compiler 严格要求完整 Profile snapshot（schema v4/compiler v4）；旧 shape 不迁移、不补写，必须
  清理并按 current Template 重新创建 Workspace。
- file repository 只接受 current object-shaped `ContextHints`；`null`、数组、标量、未知/退役字段与空持久化文件均 fail-closed。

## 已覆盖测试点

- [x] memory/file backend 上 Template defaults 的部分已存在、缺失补建、重复 ensure 与内置 ID 用户修改非覆盖语义。
- [x] Workspace creation 经唯一 private dispatcher 选择 initial state，错误 workspace scope/非 creation event 零写入，静态门禁拒绝其他 Biz `State` literal 或 repository Create 旁路。
- [x] Workspace CAS revision，以及 Goal/DoD create/update、`A → B → A` 的单调 revision、相同内容 noop 和 stale restart definition 拒绝。
- [x] Goal/DoD 非递增 ID 的 canonical leaf 谱系、历史 definition 更新拒绝和 current leaf 删除拒绝。
- [x] 缺失 current lineage 的 Goal/DoD 更新直接拒绝且不会补写 `active`。
- [x] memory/file backend 上 Goal/DoD 局部 identity、错误 Workspace scope 的 Get/Update/Delete 与无 scope 歧义拒绝。
- [x] sealed terminal outcome 的匹配释放、空 slot/其他 active Run/冲突 identity 拒绝和同 identity 重投递幂等。
- [x] immutable candidate create/duplicate/stale revision、owner-generated admission result、identity-only promotion 重编译、
  unsupported alternative 零 promotion、all-of promotion、pending clarification acknowledgement、
  exactly-one private-event payload、duplicate acknowledgement noop 与 startup-input hash clone/persistence。
- [x] Template/Workspace evidence-policy snapshot bindings 完整进入 candidate hash；任一侧 legacy target 字段或 snapshot
  漂移在 promotion 前 fail-closed，且零 Run/provider/capability work。
- [x] application-owned directory resolver 的空目录默认值、相对/绝对路径规范化、workspace ID/相对路径逃逸拒绝和
  多 workspace 默认目录隔离。
- [x] execution intent 的缺失 Goal/DoD、context scope mismatch、duplicate replay 与 active Run result fence。
- [x] 无效持久化数据不会生成 execution admission outcome，也不会修改 Workspace 元数据。
- [x] Restart 对 admission-bound candidate 的缺失/重复/旧 schema/hash 与 acquisition selector 漂移严格拒绝，零 Run command；
  current exact candidate 可正常进入 Run owner。
- [x] Profile snapshot/hash 漂移、required CWI 缺 exact proof/evidence kind/owner receipt operation、重复 Profile source，以及
  snapshot-derived owner-fact manifest 缺失/附加/漂移均在 promotion 前 fail-closed，并映射到可执行的 Template/Profile 修订动作。
- [x] 普通/治理/冲突 Run notification 与 `fresh_activation_required` 均不能释放 active slot；只有 matching terminal delivery 可释放。
- [x] Template command、repository DTO、EventHub DTO 与 list mapping 对 slice/map/nested payload 的完整深拷贝。
- [x] `ContextHints:null`、非对象 payload、未知/退役字段与空文件严格拒绝。

当前没有与本合同对应的已知可执行测试缺口。
