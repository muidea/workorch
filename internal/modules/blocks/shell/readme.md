# Shell 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-09-10

## 本地生命周期资源合同

- `Start` 只在 SimpleObserver/Hub 实际完成订阅后逐 topic 记账；第 N 个订阅失败返回原 checked error，已成功资源和 owner 引用保留，可检查式清理或从未完成 topic 重试。
- `Biz.Teardown(ctx)` 返回 error，按账本逆序退订，每次成功才移除该项；失败或 context 取消保留剩余订阅与 adapter。`Module.Stop` 只有完整清理成功才清空同一 owner 引用，plugin 的 void `Teardown(ctx)` 将错误交给 framework guard。
- 生命周期调用由调用方串行化；账本不持有跨 Hub control action 的 owner 锁，也不替代输入停止与在途 handler/进程排空。最终释放 adapter 的前提是调用方已完成真实排空，退订完成不等于 quiescence。本组件尚未提供全局停机屏障或阶段准入 guard。
- `TestShellOwnerResourceLedgerEveryStartFailureCanTeardown`、`TestShellOwnerResourceLedgerEveryTeardownFailureRetainsDependenciesAndRetries`、`TestShellOwnerResourceLedgerCancellationRetainsUnreleasedResources` 及 Module resource-ledger tests 覆盖逐 N 故障、取消、retry 和同 owner 引用保留；这些测试不是全局生命周期完成证明。

## 功能范围与边界

`blocks/shell` 是本地 `shell/command` 与 `validation/managed_session` 的 capability owner。

- `shell/command` 在 workspace deliverable directory 中执行受控命令，并以 workspace-durable effect 观察为准。
- `validation/managed_session` 只执行 owner catalog 已确认的 sealed recipe，在隔离的临时 workspace 副本中运行，effect 为
  `ephemeral_session`。

原始进程输出和执行细节保留在 Shell-private adapter；跨 owner 只返回有界 evidence、validation observation、effect fact 与
`RuntimeOperationReceipt`。

## 组件交互及合同

| 交互方 | 方向 | concrete typed 合同 |
| --- | --- | --- |
| Orchestrator | Orchestrator → Shell | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`ProbeRuntimeOwnerFactProspectiveSatisfaction`、`QueryRuntimeCWIReconciliation`、`ExecuteOperation` Command/Result |
| Orchestrator | Shell → Orchestrator | typed receipt query/commit；execute 只消费完整 `RuntimeOperationExecutionContext + SealedRuntimeOperation` |
| File owner | 经 Orchestrator fact 输入 | persisted workspace manifest/observation；Shell 不直接读取 File service/repository |
| OS process | Shell-private | command argv、workspace/scratch、timeout、process-group、bounded output 与 filesystem effect observation |

Prospective satisfaction 是 Shell-owner 的纯 factual query：它只校验一个 activation-sealed recipe declaration 经 owner adapter
按 exact File manifest 解析后的 recipe 在给定 resulting manifest 上
是否满足 owner 声明的 bounded `all_of` target constraint，不选择 target、action、retry 或 Runtime 状态。

## 不变量与非职责

- `shell/command` 不得伪装成 `effect=none` validation；managed validation 不能退化为任意命令字符串。
- execute 前必须验证 producer、同步 Result、sealed payload、owner catalog、target scope、execution key/fence 和 receipt authority。
- managed validation 必须在隔离副本中运行；临时 effect 不得写回 deliverable，scratch 和进程组必须按策略释放。
- activation-declared recipe 可以是固定 transport 或版本化的 `shell/project-native-validation/v1|v2|v3|v4` owner-adapter ref；模板不得携带
  语言 command/args/env。Shell 只依据同一 request 绑定的 File-owner manifest 解析具体 recipe，且该解析只产生 owner fact，
  不触发 Runtime 状态。Shell 只有在 Orchestrator request 携带 exact Definition-owned
  `RuntimeValidationProofRequirement` 时，才可对 matching recipe 发布逐 CWI applicability binding；recipe ID、进程 exit 0、
  zero-work marker 或 surface-wide check union 都不能自行成为 CWI proof。
- applicability binding 必须把每个 Definition check 映射到 Shell-owner 的 exact observation protocol 与 native check
  identity，并随 sealed execution authority 一同哈希封存。reconciliation 复用 exact completed dispatch 的完整 validation
  scope 与 recipe，不扫描当前目录补 authority；准备/执行失败保留具体 owner code，缺少 scope 不猜测 target。
  receipt/reconciliation 只承认该 protocol adapter 产生的 typed
  check attestation；raw stdout/stderr（包括 `WORKORCH_CHECK_PASSED=*`）、recipe ID 或任意进程自报 marker 只能是诊断，
  不能填充 passed check authority。另一个 CWI/native check 即使来自同一 recipe 也不能获得当前 CWI credit。
- recipe 的静态/输入 `Readiness` 与 requirement readiness 分开。一个 ready recipe 可以同时发布 ready CWI 的 exact binding、
  missing CWI 的 exact preparation 和另一 CWI 的 `ProofRequirementUnavailability`；preparation 封存 requirement、catalog revision、native mapping 与完整 prospective
  all-of target constraint，但不授予 proof/dispatch。File 完成 Profile 已声明的 acquisition 后，Shell 必须 fresh acquire
  workspace manifest 并重新发布 binding；旧 preparation、别的 CWI 或别的 protocol mapping 不得复用。
- workspace-durable command 的 observed change 必须落在 sealed targets 内；未声明 effect 必须 rejected。
- prospective constraint 只支持 bounded `all_of`；不发布 clause `any_of`、surface `one_of` 或隐式可选 recipe。
- 全部七个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得启动进程、读取 workspace 或访问 receipt authority。
- Shell 只署名 capability、feasibility、validation、effect 和 receipt fact；不拥有 CWI credit、repair、retry、governance 或 terminal。
- managed Go validation 的 bounded tool output 若明确报告依赖元数据问题，Shell 必须发布
  `validation_dependency_mismatch + dependency_contract`，即使 sealed recipe 只携带 generic build hint；incidental
  `*.go:line` 不得成为 implementation repair candidate。Shell 只发布事实；当前默认 Profile 没有该类自动 repair route，
  由 SM-2 有界终止。
- managed Go validation 只在 bounded tool grammar 命中时发布领域中立的可路由 owner code：`module_manifest_parse_failed` 对应
  manifest-scoped `go.mod|go.work|go.sum:line`，`source_compile_failed` 对应 manifest-scoped `*.go:line[:column]`/package conflict，
  `test_failed` 对应 typed test observation。无法精确分类的非零退出保留 `shell_exit_nonzero`，不得借 generic code 进入
  software-dev repair route。`go test -json` 的编译诊断只从有界 `output|build-output` 事件的 `Output` 字段解码，再与
  sealed File manifest scope 求交；JSON envelope、任意外部路径和未封存 target 都不能形成 repair candidate。
- structured validation observation 的 diagnostic hash 必须使用共享 canonical contract，绑定 schema/class、owner code、
  evidence target type、stage/operation、diagnostic targets 与 sealed validation scope；不得使用只覆盖 class/path 的 owner-private
  digest。diagnostic operation kind 必须是实际 sealed Shell operation kind；Shell 不选择匹配 route、repair owner、confirmation
  protocol 或终态。
- 原始 stdout/stderr、secret 和自由文本 diagnostic 不得进入 Runtime authority；公开 diagnostic 必须有界、脱敏。
- managed native check 的 failure feedback 从同一 JSON event 关联 check/case 与 assertion location，仅对 sealed validation
  scope 内的唯一相对路径发布 `CheckFailures`（最多 32 项，超限显式标记）。assertion 正文不导出，`DiagnosticHash` 保留
  精确正文身份；新失败附带可选 `validation/check-failure/v2` 诊断协议，`ComparisonHash` 只归一本 owner 当前 scratch
  前缀及其中 Go testing 自动生成的目录后缀，不抹掉任意临时路径、文件名或断言值。旧回执及 v1 digest 不改写。
  只有 exact package/case 的 native fail 才可附带 `ReportedAssertion`；passing/skipped case 的日志不能描述另一个失败。
  自报的 expected/observed shape 是有界诊断而非独立事实、执行证明、通过标记或 repair authority。父检查 PASS 但子检查 skip 时发布
  `IncompleteChecks`，transcript 截断时发布 `OutputTruncated`，两者均不能满足 native-check proof。Shell 不裁决修复因果。
- `TestReferenceValidationGraderRejectsWeakAndUnrelatedEvidence` 使用固定行为 grader 校准不同合法文件中的正确实现、不可调用/
  错误实现、静态标记、删除断言、whole/subcheck skip、依赖缺失与 A 失败/B 修改。删除断言由既有 owner declaration
  prerequisite 拒绝；父检查 PASS/子检查 skip 由 native observation 拒绝。此测试只证明该 reference fixture 与协议，不能
  宣称 AST 中出现断言即可证明任意 Goal/DoD，也不构成 DCE producer evidence。


### Versioned validation input boundary

The original Goal/DoD remains the acceptance baseline. Added checks may only
refine or supplement it consistently and conjunctively; a new input version,
approval, reference repository or passing test cannot waive an original criterion.
An old test implementation is not itself the criterion: changing it still requires
a criterion-preserving contract, not merely a failing result. Identity isolation
alone grants no replacement authority. The v3 adapter below requires independent
input review and is not a way to bypass an original failed criterion.

`shell/project-native-validation/v3` declares an independent native-check identity
(`_InputV3`) and exact `workorch_validation/v3/` input path for a newly sealed
activation. It does not rename, delete or select older checks, and cannot change
an already sealed v1/v2 activation, binding or receipt. The immutable Definition
requirement/check identity is unchanged. A single-input contract cannot advertise
partial support for a multi-check all-of requirement.

Structural readiness and instrumented execution are necessary but not sufficient
semantic calibration. An input that merely has a reachable assertion, starts a
delivery process or passes its own tests cannot authorize replacement of the
previous verification input. A ready v3 owner binding carries a mandatory exact
`InputReview` subject: the complete versioned input namespace (helpers/fixtures
included) and present Go module/workspace definitions, with content hashes and
the immutable requirement/native mapping. The current envelope is 16 files and
64 KiB per subject; unavailable/invalid-UTF-8/symlink/over-capacity inputs produce an
explicit `validation_input_review_*` unavailability, never truncated review.
Unrepresentable multi-check mappings are also explicit unavailability. Missing
inputs may use the existing exact create-only preparation; creation is not review
or validation credit. Freshness dependencies retain the bytes actually reviewed,
not a later re-read that might hide an intervening edit.

Only a complete consistent LLM report can pass Orchestrator's subsequent binding
gate, and actual validation/assessment remain required. Capability and scripted-provider
tests do not constitute release-grade semantic calibration of arbitrary Goal/DoD.

The review protocol is separately selected by the activation-sealed recipe's
`InputReviewProtocolRef`. Current built-in activations explicitly select
`validation/input-consistency/v2`; an absent selection retains the v1 subject and
historical hashes. v3/v4 adapters publish the selected protocol in the exact subject
and catalog commitment without changing the source bytes, native identities or paths.
Unsupported protocol/adapter combinations fail closed. Shell provides source and
scope facts only; it does not classify obligations, grant additions, open tickets or
transition Runtime. `TestInputReviewProtocolSelectionPreservesOriginalInputsAndNativeMapping`
covers the read-only selection and downgrade boundary.

#### Per-CWI review and execution scope

New built-in software-development activations select `shell/project-native-validation/v4`,
with `shell/go-test-json/workspace-behavior/v5` and a `_InputV4` native identity.
Each check has an independent package:
`workorch_validation/v4/<lowercase-native-identity>/input_test.go`.
Its optional conjunctive supplement is `behavior_test.go` in the same package.
The review subject contains this entire package directory (helpers/fixtures included)
plus present root `go.mod`, `go.sum`, `go.work` and `go.work.sum`, still bounded by
16 files / 64 KiB. Other CWI packages do not consume this subject's budget or change
its hash. All namespace inputs remain protected mutation targets, independently of
the current CWI's review scope. v3 retains its namespace-wide contract for old seals.
Before review and execution readiness, Shell compares this package's actual files
and present root definitions with the bound File manifest. Unlisted/missing helpers
or fixtures report `validation_input_review_source_set_changed`; symlinks and nested
modules/packages fail closed. The read-only directory scan is bounded to 256 entries
and never traverses neighboring CWI packages. Neither the file set nor review bytes
are truncated to obtain a binding.
The subject also seals `ScopeContractRef + ScopeRequirements`, an explicit bounded
owner description of this execution boundary. LLM review receives it in full in C3;
changing the scope invalidates old reports even when input bytes are unchanged.

Shell selects only the exact reviewed package and original/supplemental native checks;
it does not compile or select neighboring checks through `./...`. Coverage uses exact
module-qualified implementation package identities from the sealed manifest, excluding
the harness namespace, also for Go-built children. This is package membership isolation,
not an OS sandbox, semantic proof or proof that the full project test suite ran. An original
full-suite/build criterion still requires explicit actual execution/assertions by its input;
historical failures may not be silently omitted to satisfy that criterion.

Every current-package Go source is structurally inspected, including non-test helpers.
Runner overrides, effectful initializers, other tests/methods, cgo and imports of other
validation packages are unsupported. Nested Go packages/modules and intermediate
module/workspace definitions are explicit `validation_input_review_scope_unsupported`;
invalid helpers, symlinks, unreadable or oversized inputs fail closed. This bounded
closure does not certify arbitrary dynamic file/process dependencies: a review cannot
declare consistency when it requires unreviewed external harness code or fixtures.

Exact create-only preparation carries the bounded owner-authored
`ContentContractRef + ContentRequirements` (`shell/go-test-input/isolated-entrypoint/v1`).
The catalog hash binds both; semantic acquisition and operation C3 preserve them with
the exact target/native mapping. Core and File do not infer Go rules. Generation does
not grant review, validation or acceptance credit, and does not permit editing old inputs.

`validation_input_package_test.go` covers 25 simultaneous CWI subjects, complete helper
freshness, actual/manifest membership mismatch, scope/capacity rejection and real Go
execution with broken neighboring inputs and unlisted helper rejection.
The Orchestrator isolated-review mainline covers multiple CWI completion, tri-state review,
ticket/re-review, five restart cuts and duplicate Result→Workspace→Run delivery.
The normal HTTP authoring/restart-compiler test covers 24 original criteria / 27 CWIs
using the shipped template without patching its bootstrap. None is real-model or release
DCE evidence, nor acceptance of the user's actual multilingual bridge.

Shell reports only facts. Orchestrator's existing owner-private SM-2/3/4 events
remain responsible for binding, credit, bounded failure and delivery; there is no
calibration state machine in the template, File owner, Soul or projection.

An external reference repository is optional supporting material, not a prerequisite
for ordinary workspace execution. Neither its CI result nor process coverage proves
that an arbitrary verification input preserves the workspace's Goal/DoD semantics.

#### Executable input-maintenance contracts

Shell reports observed validation inputs through each surface's typed
`InputTargetOwnerModuleID + InputTargetRefs`: existing files in its private
`workorch_validation/` namespace, plus native entry files identified by the current
requirement inspection. Namespace inputs remain listed before their proof batch;
ambiguous or non-ready inputs do not become ordinary implementation targets.
Collection is bounded, read-only and part of the catalog identity. It neither
authorizes an edit nor manages File/Runtime state. Orchestrator compiles these facts
into sealed mutation exclusions; File consumes the generic target contract without
recognizing the namespace. `biz/validation_input_protection_test.go` verifies these
facts separately from readiness and checks that repeated queries preserve bytes.

| Input change | Required proof | Current authority boundary |
| --- | --- | --- |
| Conjunctive addition | Exact original check mapping, fresh supplemental identity, declared content contract and resource-owner feasibility | Original and supplemental checks remain mandatory; creation grants no validation credit |
| Criterion-preserving symbol repair | Owner-observed compiler location, bounded exact symbol delta and matching before/after hashes | Only the attested input delta is writable; original and supplemental checks remain mandatory |
| Different validation implementation or observation path | LLM consistency review against the original criteria, bound to exact input content, followed by owner execution facts | v3/v4 adapters supply mandatory review subjects; only Orchestrator can seal a consistent report after the independent request, with no waiver or review credit |

The last row cannot borrow either of the first two authorizations. In particular,
an unattributed test failure does not establish an input defect, and a compatible
check name does not establish that its assertions represent the original criteria.
Existing governance approval alone supplies neither fact. The consistency judgment
belongs to a bounded LLM review, not to Shell's syntax or execution observer. A
consistent review is not an execution pass; a conflicting review cannot be
overridden by approval. An inconclusive completed review requires a clarification
ticket via Orchestrator and the Decision owner. Shell neither creates that ticket
nor advances review/binding state. Ticket information is re-reviewed through the
same semantic transaction, not consumed by Shell as permission or acceptance.
`biz/validation_input_review_test.go` covers bounded exact source and freshness;
the Orchestrator mainline tests exercise real owner execution and ticket recovery.

### Validation failure attribution

A behavior-protocol test failure retains its original test-failure class, exit code
and check failures. Only complete, bounded observations of workspace implementation
execution can propose implementation repair targets. With no such observations, or
truncated output, Shell publishes `validation_test_failure_unattributed` with
`validation_output` and no targets, input addition or input repair authorization.
This means attribution is unavailable, not that the deliverable or its tests are
known to be defective. Ordinary `test_failed` implementation repair must not match
this fact. Environment, dependency, build and input-compile diagnostics retain their
existing precedence; legacy protocols retain their original interpretation.

Shell does not choose a successor, repair route or terminal state. The existing
Orchestrator SM-2 private receipt event resolves the exact sealed profile route;
SM-3 dispatch and SM-4 delivery/recovery remain unchanged. Result and projections
consume the judged result without reinterpreting this owner diagnostic.

## 能力验收

### Native check readiness and behavior observation

新 Start/Restart 可显式选择 `shell/project-native-validation/v2`，其 Go binding 固定使用
`shell/go-test-json/workspace-behavior/v4`。该版本同时观测 in-process 测试与本次 Go 构建的子进程；v1 resolver 和已封存
v1/v2/v3 observation 的原有语义不变。Resume/recovery 不根据当前默认配置升级协议。

v4 新发布的 `shell/go-test-input/conjunctive-entrypoint-test/v3` 允许 exact 补充文件中的同包具名 helper、type、const、无调用/闭包
初始化的 var，仍拒绝 init、TestMain、method、额外 Test 定义与不可达的 testing.T 断言。旧输入只被重新检查，不被改写；
原检查和补充检查仍是 all-of。结构无效或语义证据不足不授权改写判据，也不自动扩大 File repair surface。

新增输入内容合同可以声明可选失败报告语法：实际 testing.T 断言失败时，在独立一行输出
`WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array`。必须精确四个有序 token；封闭 code/shape 配对
由 shared typed DTO 校验，不接受额外正文、实际值或秘密。缺失/无效报告保留位置与 digest，并由 LLM 标记诊断不完整；
不为补诊断改写已有测试，也不把报告语法纳入 native pass/fail 判据。旧 single-test v1 / entrypoint v2 内容合同保持封存，
新声明分别为 v2 / v3 并进入 owner catalog revision，不能从新配置重建旧 authority。
`TestStdioEnvelopeReferenceGraderCalibration` 校准固定真实进程的正例、错误 result/framing/id、缺失工具、marker-only 和
comment-only 反例；`TestShellAssertionComparisonUsesRealManagedTemporaryPaths` 校准真实两次执行的精确 digest 不同而比较
身份相同。它们不是完整 MCP 或五语言验收器，也不提供 release DCE 证据。

v4 对 exact bound supplement 的编译失败另发布最多 8 项 `InputLocations`（相对路径、行、列、封闭 code/symbol），
它们是诊断而非 writable target。仅当实际 compiler 的全部相关错误均为同一文件中的 undefined ASCII identifier，且
唯一同文件 package const/var 声明只差大小写、无局部 shadow、位置命中 unresolved AST identifier、执行副本源码未变且
输出未截断时，Shell 才发布 `InputRepair`：原 required/protocol/native/supplement identity、exact target、
before/after SHA-256 和有限 corrections。只读解析上限 256 KiB；函数/类型、歧义、断言失败或其它编译错误均不生成修复。
该 postimage 只替换指定 identifier，所有其它字节（包括断言与 expected value）不变。事实须在清理执行副本前产生；Shell
不得修改 workspace。Profile 独立授权、File 确认/预览和 SM-3 seal 之后才可能产生 durable repair，随后仍须 fresh all-of。
`biz/validation_input_repair_test.go` 校准真实三处大小写编译错误、诊断完整反馈，以及歧义/shadow/旧协议/截断/源码变化负例。

Shell 在同一个 managed scratch/session 内生成固定 test-exec wrapper，恢复被 go test 覆盖的 owner GOCOVERDIR；
GOFLAGS 为 Go-built 子进程启用覆盖插桩，真实执行数据由 Go covdata 转换，不从 stdout 自报或 caller 路径导入。转换共用
operation deadline，并额外限时 10 秒；最多 128 个 artifact、8 MiB 输入以及既有 coverage record 上界，非 regular-file、
不完整/未知数据、converter warning/truncation 均 fail closed。只构建未运行不产生证明，Go delivery 外的依赖、非 Go 后端、
测试/harness 文件和零执行记录不计入实现执行。观测路径归一去重，防止多个 coverage block 放大 File repair scope。
执行前后核对隔离副本中的 Go 源码、模块与验证输入；发生改变则证据不可验证。测试临时项目/数据应使用独立临时目录。
这不是防恶意代码的 OS sandbox，也不是语义 grader；具体协议响应、各语言后端与性能验收仍需独立对应的原始断言。

这些变化只发生在能力 owner 的事实生产中：SM-1 不变，SM-2/3/4 沿用原有 private-event、sealed dispatch、receipt 和
recovery/delivery 合同。`TestEntrypointBehaviorObservesBuiltGoProcessWithoutRewritingInputs` 与 Orchestrator 的
`TestDriveRuntimeEntrypointValidationUsesSameOwnerTransactionsAcrossRestart` 覆盖校准、旧协议隔离及四机联合回归。

当前 `shell/project-native-validation/v1` adapter 的 Go-native binding 使用 `shell/go-test-json/workspace-behavior/v2`；已封存的
v1/v2 binding 仍按原协议执行/重放，不修改历史 receipt、binding 或归档。fresh manifest 若含 exact CWI 补充输入，使用
`shell/go-test-json/workspace-behavior/v3`。协议属于 Shell adapter，
不进入模板身份或 Orchestrator 的状态选择逻辑。显式固定 command 的 legacy recipe 保持 v1 语义，不能被静默增加 coverage
要求，也不能被声称具有 v2 的行为 grounding；需要该 grounding 的 Profile 应声明 project-native adapter。

- 结构就绪通过真实 `testing` import、`*testing.T` 对象身份和同包具名 helper / `t.Run` 子测试解析断言。
  未调用闭包、未引用 helper、遮蔽的同名变量、跨包 helper 与歧义入口不能代替断言；不支持的结构明确报告未解析。
  检查有 256 源文件、8 MiB、32768 AST 节点、64 层 helper 调用的上界，且读取限制在 workspace root。
- preparation 的可选 `OriginalDiagnostic` 保留 exact requirement/CWI/check、manifest revision、catalog revision 与
  缺失输入原因；不能通过新建输入补齐的结构无效、未解析、歧义、读取失败或资源上界，改由独立的
  `ProofRequirementUnavailability` 保留 exact requirement 与原始原因，不发布另一条 create 路径或 prospective constraint。
  同一 requirement 不能同时为 ready、preparable 与 unavailable；其它 CWI 的合法 binding 不受影响。以上均不是
  target、repair、retry 或 terminal authority。
- v2 每次执行绑定同一 proof 的完整 native-check 集合（最多 32 项），强制 fresh JSON test observation，并在 managed
  scratch 内生成 owner-controlled workspace coverage。外部模块、测试源、`workorch_validation` harness 与零执行记录
  不提供实现执行证据；无可核对的 workspace 实现执行或输出不完整时，返回
  `validation_workspace_behavior_unobserved | validation_workspace_behavior_unverifiable` 与 `IncompleteChecks`，不授予 proof。
  caller 不能覆盖 coverage/runner/overlay 参数；环境中的 `GOFLAGS` 和外部 `GOWORK` 不得替换该观测合同。
- v2 test failure 的 assertion location 仍完整作为 check diagnostic 保留，但只把 coverage 观测到的实现文件作为
  File confirmation 的候选；不把断言文件自动路由成实现修复。File 确认和 SM-2 seal 仍不可省略。
- 执行覆盖是必要的行为 grounding，不是逐 Goal/DoD 的充分语义证明、独立可信 grader 或对抗性 OS sandbox。
  子进程/其它语言的交付入口若不被该 adapter 观测，必须提供对应的 typed owner proof adapter；不能用同名测试、
  source-text、dependency-only success 或模型声明补齐。验证输入/验收判据修订需要独立合同，默认实现修复不授予该权限。

v2 对只有一个 native attestation 的缺失实现执行事实返回 bounded `InputAddition`：保留原 required/native check，另给
一个 exact workspace-relative 新输入与 supplemental native identity。该事实不授权写文件，必须匹配 Profile 的独立
create-only route，并经 File feasibility 和 SM-2 seal。多 attestation、不可验证或其它失败不推断补充合同。
fresh v3 catalog 将原检查与补充检查绑定为 all-of，并沿用最多 32 个 native input 的执行/观测上界。补充文件只允许
imports 和该 exact test function；不能借 top-level helper、全局变量、`init`、`TestMain` 或重定义原检查替换实现/判据。
`InputAddition.contentContractRef + contentRequirements` 随 owner receipt/hash、repair manifest 与 provider 投影携带同一
有界内容合同：辅助常量、变量、类型与闭包须位于 test function 内，断言须直接位于 test body 或 `t.Run` callback
中，不能只藏在辅助闭包；真实入口必须受该协议观测。
既有补充输入无效时，fresh catalog 只发布该 CWI 的 unavailability 和 exact 原因；pure prospective query 没有可解析的
preparation，不能宣称另建文件会满足它。Shell policy revision 绑定这份合同，旧 snapshot 按 typed stale 重获；历史
binding/receipt 不升级或改写。
补充仍未观测实现时返回 `validation_workspace_behavior_supplement_unobserved`，不再提供补充合同；补充的编译错误也
不能把测试/owner harness 文件交给普通实现修复。具体 CWI 的真实交付入口断言仍不可由“任意函数被覆盖”替代。
`TestValidationInputSupplementRequiresOriginalAndDeliveryBehavior` 校准原检查失败、实现错误、源码/dependency/fake bypass、
skip、初始化/runner override、输入编译错误、v2 replay 与 v3 fresh all-of 正反例。
`TestSupplementContentContractAndFreshReadinessAgreeWithoutReplacementPreparation` 覆盖 inline 正例与顶层 helper/const/var/type、
runner/init、语法、无断言反例，并验证另一 CWI 不受影响、零替代 preparation 与零 `would_satisfy`。

`TestNativeCheckStructureResolvesOnlyReachableTestingAssertions`、
`TestWorkspaceBehaviorProofRequiresDeliveryExecutionAndProtectsAssertionInputs` 与
`TestWorkspaceBehaviorObservationRejectsForeignHarnessAndIncompleteCoverage` 覆盖上述正反例；v1 compatibility、
instrumentation override、skip、临时资源清理及现有 typed receipt/restart 测试共同验证执行边界。

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | command、managed validation、structured check、isolated workspace 与 concrete module EventHub execute tests | 已覆盖两类能力的主要正常路径 |
| invalid + scope | dangerous command、timeout/output bound、symlink/scratch、masked exit、missing target、undeclared effect、failure-target bounds、`TestShellValidationDiagnosticClassifiesGoDependencyMetadataBeforeGenericBuildFailure`、`TestShellValidationDiagnosticPublishesExactGoOwnerCodes`、`TestShellManagedGoModuleDiagnosticReportsOnlyManifestScopedModuleTarget`、`TestShellManagedGoJSONDiagnosticReportsActualManifestScopedFailureTarget`、`TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要输入/process scope、精确 Go owner code、dependency-contract、Go JSON build output 与 implementation target 分离，并穷举七 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | zero-work receipt replay、`TestShellOwnerStateChangingReceiptReportsFileResourcesAndReplays`、prospective duplicate pure-value tests | 已覆盖同 owner 与 fresh owner 的 exact receipt 重放和纯 query 重复调用 |
| stale + freshness | admissibility manifest prerequisite、target revision stale、prospective catalog stale tests | 已覆盖 owner/File fact freshness |
| restart / owner 重建 | `TestShellOwnerStateChangingReceiptReportsFileResourcesAndReplays` | fresh owner 从 persisted receipt 重放，外部改写后的 target 保持不变，证明零二次进程执行 |
| lifecycle | `TestShellOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、`TestShellOwnerLifecycleCoversEveryRuntimeTopic`、process-group 与 scratch cleanup tests | 覆盖全部七 topic 的重复 Start 单次投递、Stop 后零投递，以及独立 process/scratch cleanup |
| effect + receipt | declared/undeclared workspace effects、managed-session observation、accepted/rejected receipt、diagnostic hash field-drift tests | 已覆盖主要 effect、receipt 分类及 route 字段哈希完整性 |
| boundary | `TestShellOwnerAcquiresManagedValidationCapabilityThroughTypedEventHubContract`、`TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、`TestShellOwnerFeasibilityAndSemanticProofHappyPathsUseTypedEventHub`、module execute、seal/authority tests | 覆盖七 topic 的 exact producer/strong-call/concrete payload，以及 owner adapter 的 typed EventHub admissibility、operation-feasibility 与 state-change semantic feasible 路径 |
