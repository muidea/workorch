# Workspace Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-22

## 功能范围与边界

`blocks/workspace` 管理 Workspace、Goal、DoD 和 WorkspaceTemplate 组成的 workspace-definition 聚合，包括定义版本、激活绑定和模板查询。它只管理该聚合的正式状态，不承担 Run 调度或跨组件治理。

Workspace 还独占 immutable pre-seal `DefinitionAdmissionCandidate`、promoted `WorkspaceStartupInputContract` 与
`WorkspaceActivationAdmission`。candidate 可以保存尚未获准的 relation input；正式 startup input 只允许 pure compiler
确认的完整 `all_of`，并以 `StartupInputHash` 封存 Definition、Template snapshot、目录和 candidate-item mapping。

## 组件交互及合同

- Workspace、Goal、DoD 和 Template 的 CRUD input，以及 Template defaults ensure input，均由
  `blocks/workspace/pkg/events` 定义具体 command 类型；普通 CRUD 当前共享 owner-local typed result union
  `WorkspaceCommandResult`。该 union 不包含 `map[string]any` authority，也不替代各操作的具体 command。
- Workspace 创建、状态变化、删除以及 Goal/DoD 定义变化由本 block 发布具体通知。
- `application/workorchd` 与需要 definition fact 的 owner 通过 EventHub 读取 Workspace、Goal、DoD、Template 和
  definition revision；其他 owner 不直接访问 Workspace repository。
- `application/workorchd` 通过 owner command/read port 管理和展示 workspace definition。
- Definition admission 使用三个独立 concrete command/result：submit candidate、promote admitted candidate、acknowledge
  clarification resolution。promotion 若绑定 blocking ticket，只能先进入 `pending_clarification_resolution`；逐字段匹配
  Decision resolution 后才可 CAS 为 `eligible`。eligible revision 只关闭不晚于自身的旧 clarification lineage；其后的
  invalid candidate 仍须由后续 corrected candidate 精确 reconcile。三个 handler 只把 immutable fact 翻译成
  exactly-one owner-private event；candidate/startup/admission 的状态选择和 CAS 只经过一个 dispatcher，调用方不能携带
  target state。任一步失败都不能把 admission 解释成可启动。
- `kernel/system` 发送 `EnsureWorkspaceTemplateDefaultsCommand`；本 block 只为缺失的内置 ID 创建默认模板，已存在对象保持不变，并返回具体 result。
- `WorkspaceTemplate` 是 workspace owner 的 DTO；EventHub result 只返回该 owner DTO，并在 module 内部完成与 repository entity 的双向 mapping、slice/map/payload clone。repository 类型不得泄漏给 application 或其他 owner。
- 外部定义命令保持唯一入口 `handleCommand -> executeCommand`；Owner 内部按 Workspace、Goal、DoD、Template
  四个命令族做穷尽且 fail-closed 的静态分派。各 handler 只是同一 Biz 内的职责切片，不是新的 Owner、状态机、
  repository/CAS 边界或 EventHub facade。

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
- `fresh_activation_required` 只是 Run owner 的不可继续 disposition；普通 Run notification 不释放 Workspace slot。
  对不兼容 development data 的 restart，Run owner返回 `data_reset_required`，Workspace 不代替 Run 创建 successor、
  不读取旧 aggregate，也不通过旁路 fact 清除 active slot。
- 默认 WorkspaceTemplate 是可由用户编辑的首次启动种子；普通 ensure 不得覆盖内置 ID 上的当前对象。嵌入默认值变更若需迁移，必须使用独立的显式迁移合同。
- file repository 只在 Workspace 持久化读边界把旧版零值 `ContextHints:null` 解释为空对象，且不得因此改写历史文件；数组、标量等非对象值仍 fail-closed。该兼容不放宽 EventHub、HTTP 或 Runtime payload 合同。

## 已覆盖测试点

- [x] memory/file backend 上 Template defaults 的部分已存在、缺失补建、重复 ensure 与内置 ID 用户修改非覆盖语义。
- [x] Workspace CAS revision，以及 Goal/DoD create/update、`A → B → A` 的单调 revision、相同内容 noop 和 stale restart definition 拒绝。
- [x] Goal/DoD 非递增 ID 的 canonical leaf 谱系、历史 definition 更新拒绝和 current leaf 删除拒绝。
- [x] memory/file backend 上 Goal/DoD 局部 identity、错误 Workspace scope 的 Get/Update/Delete 与无 scope 歧义拒绝。
- [x] sealed terminal outcome 的匹配释放、空 slot/其他 active Run/冲突 identity 拒绝和同 identity 重投递幂等。
- [x] immutable candidate create/duplicate/stale revision、all-of promotion、pending clarification acknowledgement、
  exactly-one private-event payload、duplicate acknowledgement noop 与 startup-input hash clone/persistence。
- [x] application-owned directory resolver 的空目录默认值、相对/绝对路径规范化、workspace ID/相对路径逃逸拒绝和
  多 workspace 默认目录隔离。
- [x] execution intent 的缺失 Goal/DoD、context scope mismatch、duplicate replay、active Run result fence，以及
  `data_reset_required` 零 Workspace 元数据副作用。
- [x] 普通/治理/冲突 Run notification 与 `fresh_activation_required` 均不能释放 active slot；
  `data_reset_required` admission 同样保持 slot，只有 matching terminal delivery 可释放。
- [x] Template command、repository DTO、EventHub DTO 与 list mapping 对 slice/map/nested payload 的完整深拷贝。
- [x] 旧版 `ContextHints:null` 的只读兼容、历史 Workspace 文件不改写，以及非对象 context hints 拒绝。

当前没有与本合同对应的已知可执行测试缺口。
