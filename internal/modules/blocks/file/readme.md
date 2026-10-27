# File Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-09-05

## 功能范围与边界

`blocks/file` 拥有 workspace deliverable 边界内的 list/read/write/edit/patch-preview/patch-apply effect，以及 File
capability catalog、operation admissibility、candidate conformance、feasibility、target selection/snapshot、workspace
manifest/revision、repair-surface confirmation 和 post-effect reconciliation 等 factual observation。它签发 File operation
evidence 与 receipt 内容。

本组件不拥有 Runtime CWI、authority/focus、dispatch lifecycle、receipt persistence、retry/replan、governance decision 或
validation verdict。它不解释 Goal/DoD，不调用其它 capability owner，也不通过 File fact 触发 SM-2/SM-3 transition。

## 组件交互及合同

`file/mutation-historical-effect/v1` 是 File edit/write/patch-apply 的显式用途合同。只有原 sealed all-of
effect+validation binding 派生的 execution context 声明该用途时，owner 才输出 dependency pair v2；否则保持 v1。
actual input reads 与真实 postcondition 均保全，用途不改变 effect 授权、replay proof 或 receipt 身份反查。
后续修改使旧后像 stale 时，历史 receipt 仅证明行为；终验须独立当前普通证明，不提升旧 bytes 为 current。
Core 边界与 current union 门禁见 [Evidence Proof Purpose](../../../../docs/00-core/runtime/evidence-proof-purpose.md)。

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | Orchestrator → File | 11 个 Runtime read/conformance/feasibility command/result；6 个 sealed execute command，统一 `ExecuteOperationResult` | 接受 exact `kernel/orchestrator` source |
| `blocks/git` | Git → File | `AcquireWorkspaceManifestCommand/Result` | Git 在 capability-state/feasibility/effect freshness 中读取 File-owned tree revision；该 topic 额外接受 exact `blocks/git` source |
| `application/workorchd` | Application → File | `AcquireAuthoringResourceCommand/Result` | 创建期单文件有界只读事实；仅接受 exact Application source，不冒充 Runtime prerequisite；File 再按 Workspace owner 的资源范围合同查当前目录和草稿身份，不信任调用方目录；不替代确认时的漂移复核 |
| `blocks/workspace` | Workspace → File | `VerifyAuthoringResourceCommand/Result` | Workspace 派生 reserve、确认 journal 和最终准入前分别要求 File 重新观察 exact 内容与当前草稿 revision；Workspace 随后用 revision-CAS 封存，不是资源的持续有效性保证 |
| `blocks/workspace` | Workspace → File | `ObservePreparationDirectoriesCommand/Result`、`ObservePreparationResourcesCommand/Result`、`ObservePreparationManagedConfigsCommand/Result`、`ReobservePreparationFilesCommand/Result`、`QueryPreparationEpochCommand/Result` | 前四组观察精确目标并绑定原操作；epoch 查询仅返回进程身份。均不签发 readiness、M3 或执行权限 |
| `blocks/workspace` | File → Workspace | `QueryAuthoringResourceScopeCommand/Result` | 每次读前后核对当前草稿 input hash、Template hash、ResourceRef 与 Workspace 目录；按 File source 限定查询，不授予准入 |
| `kernel/orchestrator` | File → Orchestrator | `QueryRuntimeOperationReceiptCommand/Result`、`CommitRuntimeOperationReceiptCommand/Result` | replay fence 与 receipt persistence；File 不直接写 Orchestrator state |
| workspace filesystem | File ↔ filesystem | owner-private `RuntimeAdapter` | effect 只能落在 resolved deliverable root 内 |

File owner 自定义的 24 组入站 command/result（11 组 Runtime read、2 组创建期 read/verify、5 组 STEP04 目录/规格资源/托管配置观察及最终重观察/epoch 查询、6 组 execute）位于 [`pkg/events`](pkg/events)；File 出站使用的 2 组
receipt query/commit 由 Orchestrator 定义，不计入这 24 组。创建期资源查询与验证、STEP04 观察不属于 SM-1～SM-4；其结果不授予准入。
`/file/startup-preparation/directories/query` 仅接受 `blocks/workspace`，scope 为原 Workspace/request/payload/revision 及 owner 解析的两个绝对目录。
Linux 使用无软链接目录描述符、有效凭据 ACL 权限观察和路径重开身份/版本复核，不执行 mkdir、write probe、chmod 或 repair；
其他平台无法精确观察时明确失败。返回完整结构化身份、权限和原错误 bytes/字节数/SHA256；它是历史时点事实，
不证明未来可写、不检查管理配置/规格资源、不签发 readiness/M3。Workspace 私有结算保存该材料；公共 GET 不调用此查询。
此新查询在 shutdown 后拒绝新观察，已接纳 handler 由 owner lifecycle 真实排空。

`/file/startup-preparation/resources/query` 同样仅接受 Workspace source，精确 echo 原 request/payload/revision、
封存交付目录与有序 refs，不查当前草稿或接受 Application 提交路径。Linux 复用 confined openat2、无软链接/跨挂载、
单链接普通文件边界，读后复核已打开文件及重开路径的 inode/ctime/mtime/size。URI、越界、敏感路径不猜选。
每份最多 32 KiB、总计 128 KiB、最多 128 项；超限/失败不保存前缀，不将部分内容标完整。
结果保全完整原 bytes（包括非 UTF-8/NUL）、字节数/SHA256、文件版本和原错误 bytes；不送往 provider。
原诊断超过 4096 字节时不截断，显式保存 `diagnostic_exceeds_byte_boundary`、原字节数/hash 及未保存原文事实，仍为不完整失败。
Workspace 私有 CAS 归档后只投影历史引用及阻塞码；目录成功、当前内容相等均不代替完整 required 合取或 M3。
GET、完成重放、显式恢复、owner 重开不重跑此读取。此接口不读取或修改 managed config。

`/file/startup-preparation/managed-configs/query` 为独立 Workspace-only 只读合同，绑定原 request/payload/revision、
内部目录及有序 `{Scope, Path}`。system 根由既有 `SystemDirectory` 纯路径规则确定，workspace 根为原内部目录；
不使用 workspace-first fallback，不接收 defaults 或 caller verdict。canonical 相对路径不得逃逸，重复同域目标拒绝。
复用上述 confined/pinned 普通文件读取、每份/总字节上限及完整原始诊断边界；不执行配置创建、覆盖或补缺。
Workspace 从确认来源判定既有 system-exact、workspace-preserve、workspace-mergeMissing 语义，
仅受限私有材料保存完整原文/内容 hash，公共记录只提供独立随机引用，不提供内容派生摘要或原文。
此合同的完整文件观察仍是历史事实，不签发 readiness/M3；原操作恢复不重新读取。

`/file/startup-preparation/revalidation/query` 是 Workspace 独占调用的最终文件重观察：
完整绑定原 Workspace/request/payload/revision、两个目录及全部资源/托管配置目标，重新读取真实原文和元数据。
File 返回 `workorch-file-preparation-revalidation/v1` detached 值、进程随机 epoch、每次随机 token 和
从本次开始时刻计算的 30 秒观察年龄上限；超时、取消或不完整合同不返回成功观察。
原失败诊断完整保留，token 本身不代表检查通过，不保证文件在窗口内不变，也不是执行凭据。
`/file/startup-preparation/epoch/query` 只返回同一 File owner 的内存 epoch，零文件 I/O、零续期；
owner 重建产生新 epoch。两个 topic 都是 typed 只读合同，停机拒绝新请求，不改变 Runtime SM-1～SM-4。
Runtime candidate conformance 与原读取依赖重观察是 SM-3 owner observation，其余 Runtime read/feasibility 合同属于 SM-2 owner observation，
execute 合同属于 SM-3 sealed operation participation；它们都不是 reducer API。`SealedRuntimeOperation` 绑定 canonical
payload hash、operation identity 与 execution scope，handler 不能从自由文本重建 operation。

### 实际读取依赖

成功且目标存在的 `file/read` 从本次完整流式读取的原始 bytes SHA-256 生成
`RuntimeOperationEvidence.OwnerRevisionSet`。集合绑定 sealed transaction 的 exact `DefinitionRevision`，资源身份为
`capability / blocks/file/<WorkspaceID> / workspace_target / <TargetRef>`；`Revision` 是全流内容 hash，
`DependencyHash` 另外绑定 Workspace、target、全流 hash、本次 selected content、行范围、字节上限及
truncated/binary/content-complete 标志。即使返回内容有界，也不能用返回前缀 hash 替代全流 hash。

File 不重新打开文件来补造历史版本，不从 `ResourceChanges` 或 current catalog 推断本次读取。typed receipt commit
保存该原始集合；exact persisted receipt replay 返回原依赖，不受后来文件内容或停机影响。未封存 exact Definition
的新读取在文件 I/O 和 receipt commit 前拒绝。集合是本次读取事实，不是 CWI credit 或跨时刻 freshness 凭证。

optional missing read、list/preview、validation、assessment 和正式 artifact 不由该读取合同补造依赖；
现行 Runtime 路径检查与读取仍不是跨文件原子快照，该依赖字段不证明所有 filesystem TOCTOU 已被防住。
独立 producer 回归见 [`biz/file_read_dependencies_test.go`](biz/file_read_dependencies_test.go)。

`QueryRuntimeFileReadDependenciesCommand/Result` 对原 `file/read` 提供当前重新观察：必须有 exact sealed operation、
transaction Definition、原完整 dependency set、source receipt hash 与 request/state-revision echo。File 经 typed
Orchestrator receipt query 的 `ReadSource` 反查原 persisted dispatch/operation/transaction/execution；错 source、
receipt miss/hash/set 漂移均在新读取前拒绝。然后用原 range/MaxBytes 参数读取 current bytes，重新生成完整集合，
不能只回显旧 selector hash。`missing` 显式返回 nil set；目录、工作区缺失、读取错误不伪装为完整观察或 absence proof。
调用端从 Workspace typed read 获取当前目录，不使用自由调用方目录；原 receipt 保持不变，查询不生成新 receipt。
新 query 在 BeginShutdown 后拒绝，已接纳 query 与 nested receipt 查询持票至真实返回。它不提供跨时刻保证或
SM-2 stale/reopen 裁决。非空 `ApplicabilitySource` 显式选择新 mutation 双集合协议：只接受原
`file/edit|file/write|patch/apply` receipt 的完整历史集合与实际后像快照，原 `ReadSource` 反证两侧正文；
重新读取完整实际流并以 `workspace_target_content` selector 返回当前集合，失败不返回观察。
Orchestrator adapter 将事实提交到 SM-2 私有 `EvidenceApplicabilityObserved`，File 不选择 stale/reopen。
旧 nil 协议不升级；owner postcondition、assessment 和 artifact 的完整 current dependencies 不由此补造。
独立 bytes/range oracle、无关/相交变化、missing/error、重启和零额外 commit 见
[`biz/file_read_dependency_query_test.go`](biz/file_read_dependency_query_test.go)；返回端 full raw/hash/identity gate 见
[`pkg/events/read_dependencies_test.go`](pkg/events/read_dependencies_test.go)。

`file/edit`、`file/write` 与 `patch/apply` 在 exact Definition 门禁后保留最终 patch 执行读取的 existing 输入：
owner-private observation 直接取自 `readPatchTarget` 返回的原 bytes，绑定目标、全内容 SHA-256、字节数及
`patch_input` purpose。成功、no-op 和完整读取后发生的 binary/baseHash/replace/write 等失败均保留该事实；
`readPatchTarget` 成功确认目标不存在时，保存 `patch_absence` purpose，资源种类为
`workspace_target_existence`，revision 为 `SHA256("file-target-existence/v1\x00absent")`；它与空文件的内容
revision 分域，字节数为零，不声称读到原 bytes。读取错误不伪装为 absence。`DependencyHash` 绑定 Workspace、完整 sealed operation hash
和 observation；typed receipt 与原 evidence transaction 保全集合，不把 `ResourceChanges` 当 actual-read 来源。
producer 还独立核对 observation 目标与 sealed operation 的唯一目标完全相同；缺失、多目标或错目标不生成依赖。

lost-receipt recovery 若确认 sealed 后像，只保留本次实际读取的 `replay_postcondition` hash，不把 receipt 的历史
`BeforeHash` 或 replay proof 值伪装为本次读取。已保存 receipt 的 duplicate 仍重放原集合，不重读 newer 文件。
该事实不表示所有 preview/feasibility/manifest/metadata 读取已枚举，也不提供原始未保存回执的读前像恢复、
跨文件快照、formal artifact 或 current freshness 证明；既有 effect/unknown/权限/fence 和 SM-1～SM-4 transition 不变。
独立原字节 oracle 和三种 variant 的读取/恢复/失败见 [`biz/file_patch_read_dependencies_test.go`](biz/file_patch_read_dependencies_test.go)；
真实 receipt CAS loss/replay 及 edit/rejected-write 的配对回归见 `biz/file_runtime_adapter_test.go` 和 `module_test.go`。

### 生命周期合同

- 普通及 matched 订阅统一按成功获取顺序登记，部分 Start 失败保留 adapter/清单，未清理前不重复注册；退订成功才移除对应项。
- BeginShutdown 非等待式关闭新规划/执行。既有 authoring read/verify、workspace manifest 与 post-effect reconciliation 保留原 typed/source/scope 校验供 drain；不以 Source 泛放行新工作。
- 六种 execute 外层 drain 只支持 sealed 校验和 exact persisted receipt 重放；未命中必须另取 fresh 票，关闭后零新文件操作/receipt commit，包括 read/list/preview。
- handler 票持有至真实返回及 nested receipt I/O 完成。Quiesce 超时保留全部依赖；Stop 封最终准入后再次排空，逆序 checked 退订，全部成功才清 adapter/Module 引用。未知 panic 不吞掉，不遗留清理 claim。
- 调用方串行 Configure/Start/Stop，不能覆盖未释放 owner；plugin guard 保留 cause。本地排空不替代共享任务/Hub 屏障，SM-1～SM-4 的业务权威、scope、fence、replay/权限合同不变。
- `biz/owner_lifecycle_test.go`、`module_lifecycle_test.go` 覆盖全部普通/matched 订阅切点、失败退订、超时重试与 guard；既有 patch receipt-loss 回归同时验证停机 exact 重放及未命中拒绝。

## 不变量与非职责

- `Configure` 缺 EventHub 立即失败；`Start` 重复调用不重复订阅，`Stop` 释放订阅和 adapter。
- 所有 Runtime ingress 在 owner query、effect 或 receipt I/O 前校验 producer；除 workspace manifest 显式允许 Git owner读取外，
  其余只接受 exact Orchestrator source。execute 还校验 workspace/run/dispatch、authority transaction、operation ID/kind、sealed payload
  hash、target 与 execution fence。
- path 必须是 canonical workspace-relative regular-file target；absolute path、`..` escape、symlink escape、binary edit、
  ambiguous replace、stale base hash 和越界 inventory 均 fail-closed、零 effect。
- preview、catalog、selection、manifest、feasibility、repair confirmation 与 reconciliation 都是只读事实，不得创建目录/文件。
- 创建期查询不接收调用方目录；每次读前后经 Workspace owner 的 `QueryAuthoringResourceScopeCommand/Result` 取当前草稿 hash、Template hash、资源引用和目录。Linux 上从 `/` 逐段 `openat2` 打开工作区目录（禁止符号链接），其后使用 `RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS|RESOLVE_NO_XDEV` 打开目标并核对 mount ID；拒绝非普通文件、硬链接和读中版本漂移；其他平台不返回资源内容。Workspace 的派生 reserve、确认和准入分别经专用 File 验证合同核对即时内容、ref 与 Workspace revision，并在 owner CAS 记账前再检查 revision；此观察不是跨文件原子快照，也不保证文件在 File 校验与 Workspace CAS 之间不变。该机制不证明工作区目录本身的来源、没有防住特权进程修改 mount namespace 或并发同字节重写后恢复，也不能完整识别秘密内容；资源进入 provider 前仍需评估数据敏感性。
- `file/write`、`file/edit`、`patch/apply` descriptor 共同声明 File-owned regular-target confirmation protocol；File 只按 concrete
  typed command 对 observation candidates 做 current manifest/revision 检查并返回事实，不选择 Profile route、repair action、
  retry 或 SM-2 transition。command 的 `RequestID + IssuedAtStateRevision` 是 Orchestrator-issued opaque request fence，File 必须
  校验其存在并在 result 中 exact echo，不能解释、保存或改写；confirmation 的 `SubjectRef` 也不是 owner 自选字段，只能返回该
  protocol 固定的 `workspace_deliverable`。缺失/错误 echo 或 subject 必须在 adapter 边界 fail closed。
  `RecordedAtStateRevision` 只属于 Orchestrator 持久化 record，并由其 CAS 固定为 `IssuedAtStateRevision + 1`；File 不生成或保存该
  revision。未声明该协议的 owner operation 不能被 pre-seal 为 validation-repair route。
- candidate constraint 分两阶段。conformance 必须复用 effect owner 的 exact parser/matcher，在零 workspace I/O 下验证 unified
  diff 的 single-file/target identity，以及 add-file (`/dev/null`) 或 existing-update 形态是否匹配 sealed target policy，只返回
  manifest-bound 的 `conformant | nonconformant | owner_contract_defect` 静态事实。feasibility 对合法但无法应用到 current
  owner-observed target 的 diff 返回 manifest-bound `candidate_constraint_violated`，携带 exact target/revision 与封闭 shape。
  File 在两阶段都不选择 correction、retry、terminal 或 Runtime state transition。
- `patch/apply|patch/preview` variant、parameter description 与 provider-safe syntax guidance 只由 File operation contract 声明并
  随 sealed manifest 投影；Profile/Soul 不固定 `unified_diff`、不复制 parser 规则。`unified_diff` 的 canonical standard form 包含
  单一 `---/+++` file-header pair 和至少一个 explicit `@@ ... @@` hunk，hunk-only content 在 owner conformance 中 fail closed。
- File catalog 对 `file/edit`、`patch/apply`、`patch/preview` 发布 target-bound `baseHash` preimage；literal 必须来自生成
  workspace manifest revision 的同一次 regular-file scan，并限制在 catalog 的 bounded target set。File 只提供事实；SM-3
  从 sealed manifest 为已选 singleton target 派生参数，LLM 不回显或猜测 hash。prospective target 无 existing preimage，
  省略参数；`file/write` 的 create-only 合同不变。File feasibility ingress 重验 exact catalog/authorization/preimage，
  current revision 漂移仍返回 typed stale，执行 hash guard 与 replay proof 不因本地派生而放宽。
- `file/edit` 的 operation-level constraint 与 `patch/apply|patch/preview:text_replace` 的 variant-level constraint 共享
  `oldText` applicability 合同。feasibility 复用 execution 的 text replacement/patch parser 与 matcher 做只读应用；
  current revision 下缺失/歧义/occurrence 越界的匹配返回 bounded applicability violation，不能冒充 stale 或扩大 target。
  baseHash、binary/path/resource failure 保持普通 owner infeasibility；无 current snapshot 时仅返回既有 typed prerequisite。
  `file/edit` 仍是 at-most-once effect，不因新增预检而取得 patch replay authority。
- `oldText` 非空、`oldText != newText` 与 `occurrence >= 1` 属于 manifest v7 的纯 `ValueRules`，由 shared strict admission 在任何
  File I/O 前判断；File 不保存该结果、不发布对应 EventHub fact，也不触发 correction。File 只拥有上述依赖 parser/current
  target 的 conformance/applicability 事实及自身 effect/receipt 状态。
- feasibility 必须按 sealed target policy 对资源事实进行完备分类：prospective target 已存在，以及 exact/existing target
  已缺失，返回 `policy_disallowed + sealed_target_authorization_contradicted` 和 exact owner policy/revision；candidate-local
  patch conflict 不得冒充 binding contradiction。
- File 的 target-selection 在 owner 封存前统一合同规定的 canonical 集合表示，snapshot hash 与返回前像一致；空 scope 的 target/candidate 集合保留 canonical absence。该规格规范化不覆盖原始诊断或业务文档，也不授 effect。`file/workspace_deliverable` 只读声明 scope 可涵盖空目录、唯一目标或完整有界多文件 inventory；工作轮目标上限不能裁剪它。
- `workspace-relative-new-file-target` 是 create-only selector：prospective path 缺失时返回 `create`；File effect 后相同 path
  已存在时必须返回 `unsupported`，不能把它重解释为普通 existing-target `selected`。
- `workspace-relative-existing-file-target` 是 `file/edit` 与 `patch/apply:text_replace` 的 existing/exact owner authorization；它只授权 manifest 中已列出的
  current regular file，不能创建路径。该 selector 只提供 capability fact，repair target 仍须来自 validation observation 与
  File confirmation，File 不选择 repair transition。
- 若 sealed operation manifest 携带 `InputRepair`，File 的只读 feasibility preview 必须与它的 exact target、before/after
  SHA-256 全部相同，才返回普通 `ReplayProof`。错配返回 `validation_input_postimage_mismatch / policy_disallowed`，零写入，
  不冒充 candidate correction。真正 apply 和 receipt-loss replay 继续核对已封存 ReplayProof，不能借新 catalog 应用旧诊断。
  File 不解释编译错误、不改变验收标准、不自行选择修复或 validation；修复 receipt 不构成 DoD credit。
- File ingress 共同消费 sealed target authorization 的 `ExcludedTargetRefs`，普通修改不能通过既有 scope 或 selector
  写入排除目标。File 不按路径/扩展名判断是否为验证输入，也不调用 Shell、LLM 或 Decision 取得额外权限；排除集合和
  exact repair 例外只来自 Orchestrator sealed manifest。`biz/validation_input_protection_test.go` 覆盖真实 EventHub
  对排除输入的零副作用拒绝，以及合法实现目标的重复只读 feasibility。
- workspace manifest/revision 对 deliverable topology/content 变化敏感；admissibility/target snapshot 漂移返回 typed stale，
  不自行刷新 authority。
- state-changing effect 返回 exact before/after revision 和 bounded evidence。execution key 已有 committed receipt 时返回 replay
  fact，不执行第二次写；effect 成功但 receipt commit 丢失时只允许 sealed effect-aware recovery。
- File 不保存 Runtime continuation、FailureTarget、CWI 或 decision；Orchestrator receipt commit 成功后才向 caller 返回 committed receipt。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| normal list/read/write/edit/preview/apply | [`module_test.go`](module_test.go)、[`biz/file_runtime_adapter_test.go`](biz/file_runtime_adapter_test.go) | 覆盖 create-only 与 existing-target selector、读写编辑及 patch 主路径 |
| actual existing-read dependency | [`biz/file_read_dependencies_test.go`](biz/file_read_dependencies_test.go)、`kernel/orchestrator/biz/runtime/owner_read_dependencies_test.go`、`workorchruntimecontract/runtime_receipt_owner_dependencies_test.go` | 真实全流 bytes hash、独立 selected-content/selector 前像、exact Definition 拒绝、typed receipt 保存和停机重放；消费者覆盖 CAS 前失败/响应丢失、原集合 hash/clone/reload 与来源篡改，非 current applicability 或 terminal union 证明 |
| actual patch-read dependency | [`biz/file_patch_read_dependencies_test.go`](biz/file_patch_read_dependencies_test.go)、`module_test.go`、`biz/file_runtime_adapter_test.go::TestFileOwnerPatchApplyReplaysSealedWriteAfterReceiptCommitFailure` | 三种 variant 原 bytes/独立 hash、读取后失败、创建/缺失不补造、exact Definition 零 effect、原输入与 recovery 后像区分、typed receipt 丢失恢复与停机重放；非完整读取 inventory/current applicability 证明 |
| canonical scope preimage | [`biz/file_runtime_target_canonical_test.go`](biz/file_runtime_target_canonical_test.go)、`kernel/orchestrator/biz/runtime_implementation_work_scope_test.go` | 真实 File 空／唯一／完整多文件 scope、unsupported、canonical hash/当前 JSON 重载、只读与 stale revision；不替代 working binding、公开 activation 或终验权限 |
| text edit applicability | [`edit_applicability_test.go`](edit_applicability_test.go) | `file/edit` 与 `patch/apply|patch/preview` text_replace 的真实 EventHub 矩阵：exact/byte-exact、空 replacement、occurrence、缺失/歧义/越界、baseHash/binary 与 duplicate；probe 零写入/receipt，owner constraint 不含执行或纠正权限 |
| exact validation input delta | [`biz/validation_input_repair_test.go`](biz/validation_input_repair_test.go) | EventHub feasibility 的 exact/夹带断言/旧输入/旧诊断矩阵，零预检副作用，真实 apply、重启重放与 newer input 保护 |
| target-bound parameter preimage | [`target_parameter_test.go`](target_parameter_test.go)、`workorchruntimecontract/runtime_target_parameter_binding_test.go` | 多文件 exact target hash、provider echo 拒绝、required/auxiliary 与 prospective 编译、catalog/manifest/wire hash、真实 owner restart、direct ingress drift 拒绝与外部修改后的 stale；read-only probe 零 effect |
| typed contract / sealed identity | [`pkg/events/operation_test.go`](pkg/events/operation_test.go)、`kernel/orchestrator/biz/prerequisite_event_adapter_test.go::TestAcquireRuntimeTargetSelectionPrerequisiteUsesTypedEventHubContract` | 24 topic inventory、payload/scope binding、create-only selector effect 前 `create`/effect 后 `unsupported` 覆盖 |
| candidate constraints | [`biz/file_runtime_candidate_conformance_test.go`](biz/file_runtime_candidate_conformance_test.go)、`biz/file_runtime_adapter_test.go::TestRuntimeAdapterRuntimeFeasibilityRejectsStaleUnifiedDiff`、`module_test.go::TestFileModuleReportsUnifiedDiffTargetApplicabilityAsTypedCandidateConstraint`、[`pkg/events/operation_test.go`](pkg/events/operation_test.go) | static valid/invalid unified diff、target/target-policy mode、零 workspace I/O conformance，以及真实 owner EventHub 路径中 current target conflict 的 manifest/revision-bound typed applicability violation 与零 effect 覆盖 |
| exact source | `TestFileRuntimeOwnerCommandsRejectUntrustedSources`、`TestFileWorkspaceManifestAllowsGitOwnerSource`、`TestFileModuleRejectsDriftedSealedOperationBeforeEffect` | 11 个 read topic 的 producer matrix 与 execute wrong-source 覆盖 |
| path/binary/symlink/ambiguous negative | `TestRuntimeAdapterRejectsPathEscape`、`TestRuntimeAdapterRejectsSymlinkEscape`、`TestRuntimeAdapterEditRejectsBinaryFile`、`TestRuntimeAdapterEditRejectsAmbiguousMatch` | 覆盖拒绝且零 effect |
| stale/freshness / manifest revision | `TestRuntimeAdapterProbeRejectsStaleAdmissibilitySnapshotWithoutMutation`、`TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation`、`TestRuntimeAdapterEditRejectsBaseHashMismatch`、`TestAcquireWorkspaceManifestRevisionTracksContent`、`TestOwnerFeasibilityCategoriesAreTotal` | 覆盖 stale、resource state 与 sealed target policy contradiction 的完备分类 |
| duplicate/idempotency/receipt loss | `TestFileOwnerStateChangingReceiptReplaysWithoutSecondWrite`、`TestRuntimeAdapterPatchApplyFileWriteConfirmsSealedReplayAfterReceiptLoss`、`TestFileOwnerPatchApplyReplaysSealedWriteAfterReceiptCommitFailure` | 覆盖 |
| reconciliation / restart | [`biz/owner_reconciliation_test.go`](biz/owner_reconciliation_test.go) | fresh owner restart、changed/deleted target 覆盖 |
| repair confirmation | [`biz/repair_surface_confirmation_test.go`](biz/repair_surface_confirmation_test.go) 中的 `TestFileOwnerConfirmsOnlyCurrentRegularRepairSurface`、`kernel/orchestrator/biz/prerequisite_event_adapter_test.go::TestConfirmFileRuntimeRepairSurfaceRequiresExactRequestFence`、`kernel/orchestrator/biz/runtime/shell_owner_receipt_integration_test.go::TestShellOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot`、`kernel/orchestrator/biz/runtime/repair_surface_confirmation_plan_test.go::TestRuntimeRepairSurfaceConfirmationRequestIDBindsRevisionAndRefreshLineage`、`workorchruntimecontract::TestRuntimeRepairSurfaceConfirmationRecordProvidesTypedReplayFence` | expected workspace-tree revision 与 current 相同时的 regular target confirmed/rejected、mutation 后 stale+零 target、request/issued/expected revision exact echo、stale refresh 新 fence与旧 response 越窗拒绝、protocol-bound subject、recorded revision 及 cross-owner/unsupported 拒绝覆盖 |
| lifecycle / dependency fail-fast | [`module_test.go`](module_test.go)、`TestFileOwnerStartIsIdempotentAndTeardownClearsLifecycle`、`TestFileOwnerWrongDTOAndLifecycleMatrixCoversAllTopics` | 覆盖 24 topic 的重复 Start 单次投递与 Stop 后零 payload/effect |
| wrong DTO matrix | `TestFileOwnerWrongDTOAndLifecycleMatrixCoversAllTopics` | 覆盖全部 24 topic 的 wrong concrete command，并校验每次恰有一个 owner subscription |
| creation-time resource facts | [`biz/authoring_resources_test.go`](biz/authoring_resources_test.go)、`workspace/biz/definition_derivation_persistence_test.go`、`application/workorchd/biz/definition_authoring_resources_test.go` | exact source、受限路径/字节/敏感内容拒绝、File 核验调用方内容与 Workspace revision、重启后派生 reserve 不重发 permit；正式资源 provider 路径尚未启用 |
| provider / external network | 不适用 | 不适用 |
