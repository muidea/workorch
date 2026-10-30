# Shell 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-09-10

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

### 生命周期与排空合同

- `Start` 通过 Base checked SimpleObserver API 逐成功 topic 记账；第 N 个失败保留同一 owner 与成功订阅。Start 关闭后不可重新打开代次。
- `BeginShutdown` 关闭 fresh root 准入；每个实际 EventHub handler 从准入到真实返回都持有 owner Lifecycle 票。cancel/调用者等待超时不释放票，因此子进程等待和 nested receipt commit 都留在实际执行 handler 的排空范围。
- Execute 的显式 drain 只做 sealed command 校验与 exact persisted receipt 查询/重放；未命中必须另取 fresh 票，不能借 drain 创建新 effect、governance receipt 或重新执行。其他 capability/admissibility/feasibility/reconciliation 查询在关门后拒绝 fresh root。
- `Quiesce(ctx)` 等待本 owner 真实在途归零；超时保留 adapter、订阅和同一 owner 引用，可使用独立预算重试。局部归零不封闭合法 drain，最终 Teardown 仍必须由应用共享 Hub/任务全局屏障后的调用方授权，不能把退订当作排空。
- `Biz.Teardown(ctx)` 返回 error，每项退订真实成功才 pop，失败或取消保留未释放依赖。`Module.Stop` 成功才清空 owner；public void `Teardown` 的失败交给 framework guard。生命周期配置/最终释放调用由调用方串行化，不持有跨 Hub action 的 owner 锁。
- `OwnerQuiescenceWaitsForRealHandlerAndRetainsRetryDependencies`、`OwnerQuiescenceExplicitReceiptDrainStillWaitsForCompletion` 与 ResourceLedger/Module tests 验证本地阻塞、取消、关门、retry 和 partial Start；这些测试不证明全进程所有 owner 已适配，也不改变 SM-1～SM-4 业务合同。

### 实际复制输入依赖

非空实际复制集合另以 `evidence-applicability-dependencies/v1` 保留独立快照，不是 currentness certificate。
`QueryRuntimeValidationReadDependenciesCommand/Result` 只重读原复制文件 selector 的完整 bytes、size 与 mode；
接收端反查原 persisted receipt 与完整 `ReadSource`（transaction、operation hash、execution key/fence），再由
Workspace typed read 取得目录，不使用 caller directory。读取以原文件数／总字节边界执行，拒绝 symlink、特殊
对象和越界 selector；实际 missing 与读取错误分开，部分集合不能冒完整观察。返回 exact request/state revision/
receipt hash 与完整 raw-canonical set，零进程、session、effect、receipt 或预算消费。
Orchestrator 的 SM-4 adapter 只提交 owner 事实；SM-2 私有事务判断当前性、stale 与 exact reopen。
新 query 在 BeginShutdown 后拒绝，在途 nested reads 持票直到真实返回。nil/历史协议不升级。
本合同不枚举新文件、不是跨文件原子快照，也不证明子进程所有读取已被追踪。
Unix current-dependency read 逐层以 pinned directory descriptor、`openat/O_NOFOLLOW` 打开原 canonical selector，
避免检查后 parent/final symlink 替换重定向；`O_NONBLOCK` 防止 racing FIFO 在 regular-file gate 前阻塞。
非 Unix 尚无对应安全实现，明确拒绝 current query，不返回完整观察；历史 receipt 的读取不受此查询限制。
这是一次有界逐文件观察，不保证读取后资源持续不变，也不构成跨文件原子快照。
`biz/validation_read_dependencies_test.go` 覆盖实际 bytes/mode、错误来源、missing/error 与零 commit；
`biz/validation_dependency_open_unix_test.go` 固定 parent/final symlink、非规范 selector、FIFO、取消及 binary hash/mode。
`pkg/events/read_dependencies_test.go` 覆盖完整 fence/set、资源身份和返回隔离。

managed validation 的 scratch 复制同时记录每个实际复制文件的全流 bytes SHA-256、字节数、目标和复制权限；
同一次 `io.Copy` 将原字节写入副本并计算 hash，不从先前 catalog、sealed `TargetRefs` 或执行后的 newer 文件猜前像。
文件数与累计实际复制字节受既有 8192 files／256 MiB 边界约束；复制中超限或任何错误均拒绝返回完整依赖集合。
原始字节不因 hash 记录被替换或清洗。

新 managed execute 必须在物理执行前具有 exact transaction `DefinitionRevision`。Shell 将实际复制集合转为
`RuntimeOperationEvidence.OwnerRevisionSet`，资源身份为 `capability / blocks/shell/<WorkspaceID> / workspace_target /
<TargetRef>`，`Revision` 为复制流 hash。新读取使用 `shell-validation-copied-input/v2`：`DependencyHash` 绑定
Workspace、文件 bytes/size/mode 与 actual full-copy 协议、排除目录、文件/字节上界；recipe、command/args、check
attestations 与 validation scope 不选择复制输入，仍由原 sealed operation、receipt 和 validation observation 完整绑定，
不混入实际读取选择器。相同 bytes/mode 与复制策略的多 CWI 读取可幂等合并，版本或读取选择器冲突仍拒绝。
旧 v1 原集合不升级、不重算；typed receipt 保存原集合，后续 exact replay 不重新复制。
执行失败但已完成复制时同样保留该读取来源；未完成复制不补造完整集合。receipt-to-fact/hash/reload 的接收合同由
Orchestrator 的原事务负责，不增加 SM-1～SM-4 transition、availability 或 credit。

这是 Shell 复制输入的 owner observation，不是子进程 syscall 读取追踪，也不证明外部 module/cache、网络或环境
全部依赖已被枚举。完整 workspace 复制会实际读取未被 native check 选中的文件；不能按 operation targets 裁去这些
事实，也不能将这些读取称为逐 CWI semantic proof。scratch 不是 OS／网络／凭据强隔离，复制和 source version 观察
不构成跨文件原子快照；该集合也不替代 current owner query、stale/reopen、assessment/artifact 或 terminal union。
独立字节与真实 typed owner 回归见 `biz/validation_read_dependencies_test.go`。

### 执行与证明不变量

- `shell/command` 不得伪装成 `effect=none` validation；managed validation 不能退化为任意命令字符串。
- execute 前必须验证 producer、同步 Result、sealed payload、owner catalog、target scope、execution key/fence 和 receipt authority。
- managed validation 必须在隔离副本中运行；临时 effect 不得写回 deliverable，scratch 和进程组必须按策略释放。
- activation-declared recipe 可以是固定 transport 或版本化的 `shell/project-native-validation/v1|v2|v3|v4|v5|v6|v7` owner-adapter ref；模板不得携带
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
- 全部八个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
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

单检查 v4 显式声明为 `shell/project-native-validation/v4`，
使用 `shell/go-test-json/workspace-behavior/v5` 与 `_InputV4` native identity；它保持原单检查限制。
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

#### 多检查 requirement 输入包

新内置软件开发 activation 选择 `shell/project-native-validation/v5`，且必须显式声明
`validation/input-consistency/v2`；缺失或降级 review protocol 在 recipe 解析与检查映射前拒绝。
其 native observation 为 `shell/go-test-json/workspace-behavior/v6`。完整 canonical requirement 的
SHA256 固定一个 `workorch_validation/v5/<requirement-digest>/input_test.go`，每个 required check
具有不同 `_Check_<check-identity>_InputV5` native identity。共同 CWI 检查及各源 Obligation verification
全部进入同一 package、完整 review 和同一合取执行；缺项、重复、别名或跨 requirement/protocol 混合均不可用。

create-only preparation 返回一个 exact target 与完整 native mapping、
`shell/go-test-input/conjunctive-entrypoint/v1`；不是逐项补造部分 proof。
全部 package helpers/fixtures 与根定义合计仍限 16 files / 64 KiB，native checks 仍限 32；
超界显式 unavailable，不截断或提高限制。结构检查只允许已声明的原／补充 Test，拒绝额外 Test、init、
TestMain、effectful initializer 和跨 harness 引用；每项仍需真实 testing.T 断言、逐判据一致性评审与实际行为观察。
各成员的 supplement 使用同 package 中不同 exact 文件，不得别名；旧单输入 compile-repair 不因 v5 获得多成员修复权限。
当前能力不提供任意输入替换或一般多成员 compile-repair，不能用实现修复绕过输入保护。

完整 scope contract `shell/go-test-input/conjunctive-package-review/v1` 随 catalog/hash/review 封存。
实际执行只选择该 package 的所有 exact checks；任一 failed/skipped/incomplete、输出截断或 source drift
均不能满足完整 proof。源 Criterion/Declaration 原文由 Definition 提供；Shell 不解释自然语言、裁决 credit
或重建 authority。v3/v4 的单检查、native identity 与 scope 保持不变，旧 seal 不隐式升级。

`validation_input_conjunction_test.go` 覆盖完整 preparation/review、B 改判据后的新 package、mixed identity 拒绝、
容量与降级拒绝、实际 A+B 执行、B 缺失／失败／跳过、extra Test、unlisted helper 与独立 supplement。
`TestDriveRuntimeSharedSourceObligationOwnerMainline` 使用真实 File/Shell、Run CAS 和 Result→Workspace→Run，
provider 为脚本替身；它不代替公开 authoring、真实模型、任意领域 proof 或 release DCE 验收。

#### 根 module 与逐 CWI 验证合取

Shell-private `runShellProjectConjunction` 复用 `validation_project_conjunction.go` 的事实生产器，
在同一 managed workspace 副本和同一不超过 60000ms 的原 operation deadline 内依次运行
`go build ./...`、`go test -json -count=1 ./...`、`go test -race -json -count=1 ./...`，
最后独立运行准确映射的当前 requirement 原／补充检查及实现行为观察。
前三阶段不使用最后阶段的 coverage；完整项目成功不能替代当前 CWI 的准确检查通过或实际行为。
最后行为转换之后再次反证完整 source/member 身份和总期限；所有阶段仍保留原始 stdout/stderr bytes、
总字节数、SHA256、容量不足标记、实际 argv/scope、测试数、进程组清理及 source 前后身份。
旧输入或相邻 harness 失败、skip、race、source 变化、缺检查映射或行为未观测均不能完成合取。
已知工程失败保留真实 exit code，不能因当前 CWI 尚未执行而降成结果未决。
最后的 exact-reviewed 阶段缺少真实行为或原生检查失败时，沿既有 typed diagnostic 合同报告；
不将前三阶段或相邻测试的通过／失败借为本 CWI 的语义结论。缺行为的补充输入仍须匹配原检查，
保留所有原输入，经过 fresh owner 观察与独立审查后才可再验证；诊断本身零 credit。

`validation_project_conjunction_runner_test.go` 使用真实 Go 和真实 managed copy 验证完整四阶段正例、
旧输入失败、相邻编译失败、跳过、race、前三阶段成功而最后行为失败、source 变化、缺映射及准确期限。
显式新 activation 可声明 `shell/project-native-validation/v6` 和 `validation/input-consistency/v2`。
同一 v6 adapter 还支持显式 `validation/input-consistency/v3` 组合评审：Shell 从原 compiled requirement
复制完整 typed lineage declarations 到实际输入 subject，重新绑定完整 subject hash；不裁决规则是否满足，
不提供 Runtime/Git/未来 ACK 事实，不授 credit。其它现有 adapter 不能选择 v3，旧 activation 不升级。
`biz/validation_proof_composition_test.go` 固定实际 publication、原声明隔离与 unsupported adapter fail-closed；
组合语义见 [Validation Proof Composition](../../../../docs/00-core/runtime/validation-proof-composition.md)。
它使用 `shell/go-test-json/project-conjunction-behavior/v1`、`_InputV6` native identity 及
`workorch_validation/v6/<canonical-requirement-digest>/`，每个原／源义务检查完整映射同一 requirement package。
每项 attestation 封存有序四阶段的 `ExecutionStagesHash`；content/review 合同分别为
`shell/go-test-input/project-conjunction/v1` 与 `shell/go-test-input/project-conjunction-review/v1`，
仍要求完整当前 package/helpers/fixtures 与根定义、16 files / 64 KiB，并且不替相邻 harness 补造评审。
旧 v1～v5 recipe/native/scope 不升级；内置默认仍为 v5，implementation-work availability 不因此开放。

显式 `shell/project-native-validation/v7` 使用独立 observation
`shell/go-test-json/project-conjunction-behavior/v2`，四项 mandatory stages/hash、完整原准则、native namespace、
输入评审上界与单 operation deadline 均不变。仅在 root test/race 的完整原生 transcript 证明失败仅来自当前
exact reviewed checks、无相邻/未知/编译失败或 test skip、source 不变且原期限未到时，Shell 可追加一个
`reviewed-cwi-failure-diagnostic` 实际阶段，用既有 instrumented exact-package 执行独立诊断。
原项目失败、exit/code、每阶段原始 stdout/stderr 都完整保全；合取仍 incomplete、quality invalid、零 credit。
只有失败 package/case 与每项 assertion detail 完全复现、实际 implementation 行为观测齐全且 source 未漂移，
才能发布原 typed failure→repair candidate。诊断通过、失败集合/断言漂移、unknown、skip、输出/存储缺失、
cleanup 或超时均不能授修复归因。v6 原失败语义不升级；默认和公开 availability 不由此开启。
`validation_project_failure_diagnostic_test.go` 使用真实 Go/managed copy/raw store 覆盖精确正例、相邻失败、
skip、独立诊断漂移与旧协议；这不代替公开 fullflow、真实 provider 或 live。

显式 `shell/project-native-validation/v8` + `validation/input-consistency/v3` 使用独立 observation
`shell/go-test-json/project-conjunction-behavior/v3`，不升级 v6/v7，也不发布默认工作权限。
owner 从准确输入 bytes 编译完整项目测试来源（opaque source ref、package-qualified native case、input target），
以 `validation/diagnostic-source-review/v1` 封存到 subject 并要求每个来源独立一致性评审。
有界 corpus 包含全部 project `_test.go`、validation namespace helpers、普通非 Go fixture/documentation 和根定义；
非 test/non-harness Go 文件仍为可修复的交付实现，不成为不可变验证输入。依赖未提供的外部 harness/fixture
不能猜作 consistent。新 envelope 为 64 files/256 KiB、64 sources、64 KiB report；超界明确不可用，不裁剪。
普通 mutation 排除全部原 project test 与 validation namespace；来源评审不开放输入维护授权。

root test/race 完整失败后，只有全部 failed package/case 能唯一匹配封存来源、没有 skip/build/unknown/source drift，
才可在原单 operation deadline 内分别重跑至多五个 top-level native 来源（总 stage 仍不超过八项）。每源独立
coverage file 和 child-process instrumentation，精确保留 race 模式，不能借另一失败源的 aggregate coverage 归属。
owner 新生成的计数使用 atomic 模式，允许原输入中继承 instrumentation 的真实嵌套 `go test -race`；不能因 owner
注入 count 模式使原工程命令失败。完整 count/atomic owner profiles 都按原 exact manifest scope、正计数、非
test/harness 和原容量门禁读取；set/未知格式、缺失/截断仍拒绝，旧 count 原事实不改写。
所有原失败 case/subcase 与 assertion details 必须完整复现，且每源实际触达有限 scope 内非 test/non-harness
实现；source/输出/cleanup/deadline/storage 任一不完整均不发布修复诊断。阶段保全 `DiagnosticSourceRef` 与
独立 `BehaviorTargetRefs`、实际 argv、原 raw identity/store refs；普通 receipt/hash/reload/duplicate 链仍绑定完整正文。
若实际归属后原文发布失败，已发生的逐源行为、全部原输出 bytes/hash 身份、已保存流和明确 omission 原因仍保全为
rejected history；不抹除历史，也不发布 structured repair。`TestProjectDiagnosticPublicationFailureKeepsActualSourceHistory`
使用真实 runner/raw store 的发布失败与取消边界核验完整原文及零授权；不是公开 fullflow 或 live。
原工程失败与全部测试不改写，四阶段 conjunction 仍 incomplete、quality invalid、零 CWI credit。
`TestProjectMultiFailureDiagnosticActualSourcesAndBehavior/disjoint` 用两个实际失败来源分别触达不同实现文件，
固定每源独立的精确行为 target 及最终完整 union；不得借另一来源的 coverage 为本来源归属。
只有完整来源归属后才沿既有 typed diagnostic→resource-owner confirmation→exact repair→fresh all-of 路线；
SM-2/3/4 没有新 transition、counter 或预算，SM-1 不变。真实 owner producer 回归不代表 live 或公开默认准入。

公开 Shell owner 在原 seal、scope、receipt miss、生命周期票与 catalog revision 核验之后选择该私有 runner；
runner 的成功观察保持未分类；只有 owner completion 确认完整四阶段、准确 native checks、
真实行为、原输出持久引用、清理及原 deadline 全部成立后，才署名 `behavioral` 模态。
失败历史保留空模态；模态是 capability fact，不裁决 CWI credit。
无原 execution 的 direct adapter 调用拒绝降级执行。reconciliation 使用 completed dispatch 的原 scope、recipe 和
binding stage hash，经同一封套执行全部四阶段；它不产生 operation receipt 或 CWI credit。
reconciliation 原文使用绑定 Workspace/Run、request/generation、Definition、binding-set/binding、baseline 与 recipe 的
独立 `reconciliation/<digest>` scope；它不是执行恢复账本，也不能借原 receipt read topic 读取。

`shellProjectConjunctionObservation` 将实际阶段事实 detached 映射为中立
`RuntimeValidationExecutionConjunction`，核对原四阶段身份、准确 argv 和 owner raw bytes/hash；
缺阶段的已知失败只保全为 incomplete，不能变成完整合取。最终事实随实际 observation/receipt 保全；
前三阶段的 PASS 不投影为当前 CWI 的 exact native proof。
`validation_project_conjunction_observation_test.go` 验证完整映射、身份／原 bytes 篡改拒绝和副本隔离。
中立接收合同要求 native PASS 与完整 all-of 同时成立；事实本身仍不授予 CWI credit。

`validation_project_raw_store.go` 提供 owner-private 的受限原文存储／读取边界，由执行封套与受限 read topic 使用。
它只接受现有绝对 Workspace internal directory，并用 `os.Root` 固定目录句柄；owner 子目录拒绝 symlink 和宽权限，
逐次复核实际打开 inode，文件仅以 0600 保存。每条原文以 Workspace／Run／原 ExecutionKey／stage／stdout 或 stderr
完整 scope 隔离，原 `[]byte` 使用可逆 base64 JSON 保存，独立校验全部接收 bytes 的 SHA256 与实际已保存前缀的 SHA256。
每流沿用 64KiB 上界；超界保留明确 incomplete／omitted reason，不能变成 PASS 或完整诊断。
发布使用已 fsync 的临时文件和不可覆盖的 hard-link，再删除临时别名及同步目录；任何失败不返回持久成功 ref，
重试核对原 canonical bytes 后才确认，异常已存在文件不覆盖。它不是第二 operation journal，也不提供执行恢复权威。
`validation_project_raw_store_test.go` 覆盖原 bytes（中文／CRLF／NUL／非法 UTF-8）、空输出、并发 duplicate、scope 漂移、
容量、symlink／权限／篡改拒绝、发布前／后故障重载，以及首次／duplicate 同步期间取消不返回成功引用；
真实四阶段 runner 测试同时保存并 fresh-store 读回全部实际输出。
受限存储只保全诊断，不替代原 receipt/aggregate 恢复权威；fresh-process 恢复另行验收。
本节不表示公开 authoring/activation/fullflow、fresh-process 或 live 已验收。

`validation_project_execution.go` 将准备、四阶段、行为转换、原文发布、句柄关闭和实际 scratch cleanup
纳入从私有封套入口开始的同一最早 deadline。只接受原 execution scope 的 ephemeral managed validation，
不接受 caller scratch 或超过既有 60000ms 的期限；阶段规格从执行前的 owner 编译产物取得，不从结果反推。
首次执行事实与 raw refs 绑定同一 Workspace／Run／原 ExecutionKey；未保存的流显式保留 omission 原因。
cleanup、原文发布或总期限失败使合取 incomplete／quality invalid，已知阶段失败的原 exit/code 不因此改成未决。
启动前失败保留实际空 stdout/stderr 的 bytes/hash，不补造 Started。该封套不提供 replay/读取准入；
公开 execute 仍复用原 receipt query/commit，reconciliation 只返回当前事实，不创建第二 journal。
SM-3 仍独占 sealed dispatch/receipt，SM-2 仍裁决 proof/current reconciliation，SM-4 仍从原 aggregate 恢复；
SM-1、既有转换和预算不变。
`validation_project_execution_test.go` 覆盖真实四阶段／旧工程失败、原 execution scope 重载、实际 scratch 消失、
前置 scope/budget 拒绝和 cleanup/deadline/raw gap 反例；这些私有测试仍不代替原 seal/receipt 的公开恢复验收。
封套将每流的已发布原 ref 或明确 omission 投影到中立输出身份的 `Storage`，使其能够随原 receipt/fact
保全和绑定 hash；stderr 发布失败不丢弃已发布 stdout。原接收 bytes/hash/容量事实不因持久失败改写，
未保存引用不标为完整合取。`Storage` 只提供数据身份，不允许 caller 指定目录读取。
`QueryRuntimeValidationRawOutput` 是仅接受 exact Orchestrator strong-call 的受限 owner read topic：caller 只提供
Workspace／Run／Dispatch／Operation／原 ExecutionKey／Fence、原 receipt hash 和 stage/stream selector，不能传目录或 ref。
Shell 经原 receipt-owner query 反查同 owner 的 persisted receipt，从原合取事实取得 ref，再从 Workspace typed query
取得当前 internal directory；原 scope、引用正文、bytes、全输出 SHA256 与已保存 bytes SHA256 均须匹配。
receipt miss、未保存流、错 hash/scope/类型、异常文件均返回明确错误，不授 execute、credit 或恢复权限。
rejected managed validation 原 receipt 的 effect 可以仍为 unknown；原诊断读取不清除 unknown、不改预算，
也不把已知阶段失败改成结果未决。accepted unknown 与 workspace-durable 等其他 scope 仍拒绝。
新 project observation 的通用 Runtime summary 只含 owner code/stage/exit 事实，不拷贝或生成原 stdout/stderr 摘要；
原 bytes 独立保全在受限 store。
每次最多返回该原流的 64KiB 已保存 bytes；不完整前缀保持原 incomplete/omission，不改写或摘要原文。
查询沿原 context/lane 进行，BeginShutdown 后拒绝新读取，已接纳的 nested receipt/Workspace/read 等真实结束后才释放依赖。
`validation_raw_output_query_test.go` 配对实际私有 producer/raw store 与 typed receipt/Workspace fixture，
`pkg/events/validation_raw_output_test.go` 固定原 bytes、bounds 与 detached result。
Orchestrator 的 `runtime_shell_raw_output_receipt_integration_test.go` 配对实际 Shell read、receipt handler/controller
和文件型 Run owner，沿完整 semantic/candidate/claim/receipt 准入验证原持久来源、零变更读取、异引用冲突、句柄重建及退休拒绝；
原文 wire 和 Workspace 为显式 fixture，不代表公开 project producer、fresh-process 恢复或 live 验收。
该 read topic 不产生新的 SM-1～SM-4 transition、预算或 availability。
`validation_project_adapter_test.go` 覆盖显式 v6 映射、完整 preparation/review 容量、公开 Shell 四阶段与旧工程失败、
原 receipt duplicate、reconciliation scope/budget 拒绝及诊断域隔离。
Orchestrator 的 `runtime_project_conjunction_owner_mainline_test.go` 使用真实准备、File/Shell、文件型 Run CAS 和
Result→Workspace→Run；实际 producer 的合取/raw identity 必须与 persisted receipt、EvidenceFact 和 exact stage mapping
相等。重建 Shell owner、重新打开 Run repository 后，duplicate 仍返回原 receipt，全部实际 stage/stream 经原 receipt
与真实 Workspace owner 读取，错误 fence 拒绝且 aggregate/预算零变化；旧工程失败保留 rejected/unknown receipt 和
未完成合取，不授当前 native proof。测试还在真实 validation sealed dispatch 上执行原 Shell operation，
于 SM-3 owner receipt/消费 CAS 已保存、SM-2 evidence/CWI credit 尚未产生时丢失响应；成功四阶段与原工程
known failure 都重建 Shell/驱动对象并独立重读 Run 前像，再沿原 receipt 续驱。实际原工程测试写出外部计数，
成功的 ordinary/race 恰各一次、ordinary 失败恰一次；恢复和 duplicate 不增加计数、receipt、原 execution 分配/
消费或 recovery authorization。既有消费事实、原 rejected/unknown effect 与全部 raw 引用保持不变。
provider 为脚本替身且使用同一 Hub；这不是公开 authoring、fresh-process 或 live 验收。

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

## Validation generation observations

For isolated, conjunctive and project Go adapters that publish a content-bound missing-input preparation, the surface
also publishes one `shell/go-validation-generation-context/v1` observation. It reads actual non-test `.go` source and
present root `go.mod`/`go.sum`/`go.work`/`go.work.sum` from the declared workspace manifest, excluding `workorch_validation/`.
Selection is adapter-specific, not Core policy. Up to 64 files / 256 KiB are supplied byte-for-byte with content hashes;
missing/unreadable/non-UTF-8/symlink files or capacity overflow produce an explicit observation with no partial files.
The context is shared across the surface's checks and bound to the owner catalog revision/snapshot. Original source
obligation checks are preserved from the exact requested requirement. These observations supply implementation context
for generation; they neither prove behavior nor change the independent review scope, native identity or create-only
input rules. Existing input replacement and review-conflict retry remain unauthorized by this observation alone.
