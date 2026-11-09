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

原始 stdout/stderr 保持本次实际取得的 bytes，不为脱敏替换，也不按命令名称删除进度行。
通用 FailureMessage 只携带原失败阶段与退出事实，不塞入改写的工具输出；原输出仍由 owner 受限证据链保全。
显式 OutputPreview 继续受原预览边界约束，但不进行敏感词替换，不代表完整原始输出或新的验收 credit。

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
新 query 在 BeginShutdown 后拒绝，在途 nested reads 持票直到真实返回。缺失或不符合当前合同的请求拒绝。
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
typed receipt 保存原集合，后续 exact replay 不重新复制。
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
- activation-declared recipe 可以是固定 transport 或具有明确执行／隔离／诊断范围的 Shell owner-adapter ref；模板不得携带
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
  `*.go:line` 不得成为 implementation repair candidate。明确缺少 module checksum 且 sealed root scope 含 `go.mod` 时，
  仅将 `go.sum` 作为依赖元数据表面：scope 已有该文件时发布 `module_checksum_update_required` 及有界候选，
  否则发布 `module_checksum_create_candidate`，不伪造不存在文件的 structured observation。
  当前 software-dev Profile 分别声明 File existing-scope edit 与 prospective create-only write，File 独立确认当前表面、
  权限及版本；原 importing source 和验收输入不可借依赖诊断进入修复。其他依赖失败不借用 checksum 路由。
  project-build/test/race 的完整、已知、source 未变的构建／依赖失败也发布同一 typed diagnostic；原阶段、退出码、
  原文引用和失败结论保留。超时、cleanup、原文存储或 source 完整性失败不产生修复归属。
- managed Go validation 只在 bounded tool grammar 命中时发布领域中立的可路由 owner code：`module_manifest_parse_failed` 对应
  manifest-scoped `go.mod|go.work|go.sum:line`，`source_compile_failed` 对应 manifest-scoped `*.go:line[:column]`/package conflict，
  `test_failed` 对应 typed test observation。无法精确分类的非零退出保留 `shell_exit_nonzero`，不得借 generic code 进入
  software-dev repair route。`go test -json` 的编译诊断只从有界 `output|build-output` 事件的 `Output` 字段解码，再与
  sealed File manifest scope 求交；JSON envelope、任意外部路径和未封存 target 都不能形成 repair candidate。
- structured validation observation 的 diagnostic hash 必须使用共享 canonical contract，绑定 schema/class、owner code、
  evidence target type、stage/operation、diagnostic targets 与 sealed validation scope；不得使用只覆盖 class/path 的 owner-private
  digest。diagnostic operation kind 必须是实际 sealed Shell operation kind；Shell 不选择匹配 route、repair owner、confirmation
  protocol 或终态。
- 原始 stdout/stderr、secret 和自由文本 diagnostic 不得进入 Runtime authority；公开 diagnostic 只提供有界阶段事实及受限原文引用，不用脱敏改写的工具输出充当原文。
- managed native check 的 failure feedback 从同一 JSON event 关联 check/case 与 assertion location，仅对 sealed validation
  scope 内的唯一相对路径发布 `CheckFailures`（最多64项，超限显式标记）。适用的完整 `AssertionText` 通过受限 typed feedback
  传递，`DiagnosticHash` 绑定精确正文；新失败附带可选 `validation/check-failure` 诊断协议，`ComparisonHash` 只归一本 owner 当前 scratch
  前缀及其中 Go testing 自动生成的目录后缀，不抹掉任意临时路径、文件名或断言值。旧回执及 v1 digest 不改写。
  只有 exact package/case 的 native fail 才可附带 `ReportedAssertion`；passing/skipped case 的日志不能描述另一个失败。
  自报的 expected/observed shape 是有界诊断而非独立事实、执行证明、通过标记或 repair authority。父检查 PASS 但子检查 skip 时发布
  `IncompleteChecks`，transcript 截断时发布 `OutputTruncated`，两者均不能满足 native-check proof。Shell 不裁决修复因果。
- `TestReferenceValidationGraderRejectsWeakAndUnrelatedEvidence` 使用固定行为 grader 校准不同合法文件中的正确实现、不可调用/
  错误实现、静态标记、删除断言、whole/subcheck skip、依赖缺失与 A 失败/B 修改。删除断言由既有 owner declaration
  prerequisite 拒绝；父检查 PASS/子检查 skip 由 native observation 拒绝。此测试只证明该 reference fixture 与协议，不能
  宣称 AST 中出现断言即可证明任意 Goal/DoD，也不构成 DCE producer evidence。


### Validation input boundary

The original Goal/DoD remains the acceptance baseline. Added checks may only
refine or supplement it consistently and conjunctively; a new input version,
approval, reference repository or passing test cannot waive an original criterion.
An old test implementation is not itself the criterion: changing it still requires
a criterion-preserving contract, not merely a failing result. Identity isolation
alone grants no replacement authority. The reviewed entrypoint adapter below requires independent
input review and is not a way to bypass an original failed criterion.

`shell/reviewed-entrypoint-validation` declares an independent native-check identity
(`_EntrypointInput`) and exact `workorch_validation/entrypoint/` input path for a newly sealed
activation. It does not rename, delete or select older checks, and cannot change
an already sealed activation, binding or receipt. The immutable Definition
requirement/check identity is unchanged. A single-input contract cannot advertise
partial support for a multi-check all-of requirement.

Structural readiness and instrumented execution are necessary but not sufficient
semantic calibration. An input that merely has a reachable assertion, starts a
delivery process or passes its own tests cannot authorize replacement of the
previous verification input. A ready reviewed owner binding carries a mandatory exact
`InputReview` subject: the complete versioned input namespace (helpers/fixtures
included) and present Go module/workspace definitions, with content hashes and
the immutable requirement/native mapping. The ordinary envelope is 16 files and
64 KiB per subject (multi-failure uses the explicit larger corpus below); unavailable/invalid-UTF-8/symlink/over-capacity inputs produce an
explicit `validation_input_review_*` unavailability, never truncated review.
Unrepresentable multi-check mappings are also explicit unavailability. Missing
inputs may use the existing exact create-only preparation; creation is not review
or validation credit. Freshness dependencies retain the bytes actually reviewed,
not a later re-read that might hide an intervening edit.

Only a complete consistent LLM report can pass Orchestrator's subsequent binding
gate, and actual validation/assessment remain required. Capability and scripted-provider
tests do not constitute release-grade semantic calibration of arbitrary Goal/DoD.

All review-capable recipes require the sole current `validation/input-consistency` contract.
The complete criterion obligations and declared proof consumers are reviewed together; no earlier report format is selected.
Adapters name distinct execution/isolation/diagnostic scopes, not historical releases.
Unsupported protocol/adapter combinations fail closed. Shell supplies source and scope facts only; it does not grant credit,
open tickets or transition Runtime.

#### Per-CWI review and execution scope

单检查隔离执行显式声明为 `shell/isolated-package-validation`，
使用 `shell/go-test-json/isolated-package-behavior` 与 `_IsolatedInput` native identity；它保持原单检查限制。
Each check has an independent package:
`workorch_validation/isolated/<lowercase-native-identity>/input_test.go`.
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
Runner overrides, effectful initializers, cgo and imports of other validation packages
are unsupported. The selected adapter determines native-test membership: conjunctive/project
packages allow helper methods; multi-failure additionally inventories and independently reviews
ordinary additive native tests, then executes them in the project conjunction. These tests cannot
replace exact required checks or acquire their CWI credit. Nested Go packages/modules and intermediate
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

package 合取 activation 可选择 `shell/conjunctive-package-validation`，且必须显式声明
`validation/input-consistency`；缺失或不支持的 review protocol 在 recipe 解析与检查映射前拒绝。
其 native observation 为 `shell/go-test-json/conjunctive-package-behavior`。完整 canonical requirement 的
SHA256 固定一个 `workorch_validation/conjunctive/<requirement-digest>/input_test.go`，每个 required check
具有不同 `_Check_<check-identity>_ConjunctiveInput` native identity。共同 CWI 检查及各源 Obligation verification
全部进入同一 package、完整 review 和同一合取执行；缺项、重复、别名或跨 requirement/protocol 混合均不可用。

create-only preparation 返回一个 exact target 与完整 native mapping、
`shell/go-test-input/conjunctive-entrypoint/v1`；不是逐项补造部分 proof。
全部 package helpers/fixtures 与根定义合计仍限 16 files / 64 KiB，native checks 仍限 32；
超界显式 unavailable，不截断或提高限制。合取 package 允许同包 helper method；普通合取协议只执行已声明的原／补充 Test。
multi-failure 的额外 Test 采用下述完整独立 inventory/review 合同。仍拒绝 init、TestMain、effectful initializer 和跨 harness 引用；每项仍需真实 testing.T 断言、逐判据一致性评审与实际行为观察。
各成员的 supplement 使用同 package 中不同 exact 文件，不得别名；单输入 compile-repair 不获得多成员修复权限。
当前能力不提供任意输入替换或一般多成员 compile-repair，不能用实现修复绕过输入保护。

完整 scope contract `shell/go-test-input/conjunctive-package-review/v1` 随 catalog/hash/review 封存。
实际执行只选择该 package 的所有 exact checks；任一 failed/skipped/incomplete、输出截断或 source drift
均不能满足完整 proof。源 Criterion/Declaration 原文由 Definition 提供；Shell 不解释自然语言、裁决 credit
或重建 authority。各 adapter 的单检查限制、native identity 和执行 scope 独立，不因选择其他 adapter 扩大原 seal。

`validation_input_conjunction_test.go` 覆盖完整 preparation/review、B 改判据后的新 package、mixed identity 拒绝、
容量与降级拒绝、实际 A+B 执行、B 缺失／失败／跳过、extra Test、unlisted helper 与独立 supplement。
`TestDriveRuntimeSharedSourceObligationOwnerMainline` 使用真实 File/Shell、Run CAS 和 Result→Workspace→Run，
provider 为脚本替身；它不代替公开 authoring、真实模型、任意领域 proof 或 release DCE 验收。

#### 根 module 与逐 CWI 验证合取

Shell-private `runShellProjectConjunction` 复用 `validation_project_conjunction.go` 的事实生产器，
在同一 managed workspace 副本和同一本次封存的 operation deadline（显式声明范围 1000–600000ms，未声明时使用 owner 默认期限） 内依次运行
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
显式新 activation 可声明 `shell/project-conjunction-validation` 和 `validation/input-consistency`。
同一 adapter 使用 `validation/input-consistency` 组合评审：Shell 从原 compiled requirement
复制完整 typed lineage declarations 到实际输入 subject，重新绑定完整 subject hash；不裁决规则是否满足，
不提供 Runtime/Git/未来 ACK 事实，不授 credit。所有输入审查采用唯一当前合同；不同执行范围由各 adapter 明确声明。
`biz/validation_proof_composition_test.go` 固定实际 publication、原声明隔离与 unsupported adapter fail-closed；
组合语义见 [Validation Proof Composition](../../../../docs/00-core/runtime/validation-proof-composition.md)。
它使用 `shell/go-test-json/project-conjunction-behavior`、`_ProjectInput` native identity 及
`workorch_validation/project/<canonical-requirement-digest>/`，每个原／源义务检查完整映射同一 requirement package。
每项 attestation 封存有序四阶段的 `ExecutionStagesHash`；content/review 合同分别为
`shell/go-test-input/project-conjunction/v1` 与 `shell/go-test-input/project-conjunction-review/v1`，
仍要求完整当前 package/helpers/fixtures 与根定义、16 files / 64 KiB，并且不替相邻 harness 补造评审。
recipe/native/scope 的差异由各执行 adapter 显式声明；implementation-work availability 仍独立核验。

显式 `shell/project-failure-diagnostic-validation` 使用独立 observation
`shell/go-test-json/project-failure-diagnostic-behavior`，四项 mandatory stages/hash、完整原准则、native namespace、
输入评审上界与单 operation deadline 均不变。仅在 root test/race 的完整原生 transcript 证明失败仅来自当前
exact reviewed checks、无相邻/未知/编译失败或 test skip、source 不变且原期限未到时，Shell 可追加一个
`reviewed-cwi-failure-diagnostic` 实际阶段，用既有 instrumented exact-package 执行独立诊断。
原项目失败、exit/code、每阶段原始 stdout/stderr 都完整保全；合取仍 incomplete、quality invalid、零 credit。
只有失败 package/case 与每项 assertion detail 完全复现、实际 implementation 行为观测齐全且 source 未漂移，
才能发布原 typed failure→repair candidate。诊断通过、失败集合/断言漂移、unknown、skip、输出/存储缺失、
cleanup 或超时均不能授修复归因。该诊断范围不改变其它 adapter 的失败语义，不自行授予 availability。
`validation_project_failure_diagnostic_test.go` 使用真实 Go/managed copy/raw store 覆盖精确正例、相邻失败、
skip、独立诊断漂移与旧协议；这不代替公开 fullflow、真实 provider 或 live。

显式 `shell/project-multi-failure-validation` + `validation/input-consistency` 使用独立 observation
`shell/go-test-json/project-multi-failure-behavior`，保留其它 adapter 的独立诊断范围，不发布工作权限。
owner 从准确输入 bytes 编译完整项目测试来源（opaque source ref、package-qualified native case、input target），
以 `validation/diagnostic-source-review/v1` 封存到 subject 并要求每个来源独立一致性评审。
有界 corpus 包含全部 project `_test.go`、validation namespace helpers、普通非 Go fixture/documentation 和根定义；
非 test/non-harness Go 文件仍为可修复的交付实现，不成为不可变验证输入。依赖未提供的外部 harness/fixture
不能猜作 consistent。新 envelope 为 64 files/256 KiB、64 sources、64 KiB report；超界明确不可用，不裁剪。
允许普通附加 native Test 和同包 helper method；Go 原生 Test 名称发现与独立诊断 inventory 使用同一规则，method 和 Testhelper 不冒充 native Test。
附加 native Test 必须完整独立评审，并参加全项目合取；失败仍阻止通过，不能替代 required checks。
preparation/supplement 使用实际选中协议的容量和期限说明：multi-failure 为 64 files/256 KiB、64 sources/64 KiB report，
单个 operation 的准备、全部执行阶段与 cleanup 共用本次封存的 deadline（显式声明范围 1000–600000ms，默认不变），不能重置期限、减样本或跳过原准则。
普通 mutation 排除全部原 project test 与 validation namespace；来源评审不开放输入维护授权。

原生测试需要独立子执行时，当前生成与完整来源评审统一声明 owner source channel 方法。
同包 helper 在 `WORKORCH_NATIVE_SOURCE_CHILD == t.Name()` 时执行完整原断言；否则以 `t.Parallel()`
及继承环境的 `exec.Command(WORKORCH_NATIVE_SOURCE_RUNNER, t.Name()).Run()` 请求执行，并将非零状态报告给
`testing.T`。不得回放子测试 stdout/stderr。直接 `os.Executable`（含导入别名）在结构观察中返回
`native_check_unmanaged_self_execution`，沿既有精确受保护输入维护取得新输入与独立评审，不由开发代理改写 live 输入。
原 in-process 方法仍可使用；只有封存 corpus 实际调用 literal runner 环境入口才准备通道。

通道属于原 managed session 内部，不新增 dispatch、执行权限或 Runtime authority。
Shell 在原 operation deadline 内编译固定最小 client；每 stage 使用随机独立 Linux abstract Unix socket。
client 仅可提交 native 名，不能提交 binary、package、argv、环境、正文或结果。Shell 通过 SO_PEERCRED、
实际父进程及其编译身份，要求父测试 binary 由该 stage 的 go-test 进程直接启动；匹配封存 source/native/package
及准确输入 package cwd 后，由 Shell 执行该实际 binary 的 exact native selector。记录 binary SHA256、
实际 argv、时间、exit、终止及 cleanup；打开实际父 executable inode 并通过继承描述符执行同一文件，
不因路径被替换或删除而重开另一个 binary。替换父进程、递归、越 scope 和重复来源明确拒绝。
这不是任意子进程拦截器，也不是 OS sandbox；不能按嵌套 RUN 字符串恢复旧混合输出来源。

子执行 stdout/stderr 在共享父流之前分开保全，再对已采集 stdout 作确定性 test2json 投影，不重执行业务。
使用普通 verbose 模式以保持 stderr 独立；禁止使用会合并 stderr 的 test2json command 模式或 test.v=test2json。
普通验证、单源/多源独立诊断与断言提取消费同一个 acquisition 事实；父子 outcome 冲突、foreign case、
未结束、skip、原文缺失或超容量均不授完整来源或 repair。覆盖率沿原 owner 目录收集，不能借其他 source 的行为归属。
全部 child 共用原期限和单流16MiB/执行64MiB预算，取消时终止 process group、完整 join 后再发布。

`RuntimeValidationExecutionStage.NativeExecutions` 只携带 owner 事实及受限原文引用；`NativeSourceFailure`
精确保留拒绝，不等于原失败未知。子原文按原 stage 加 `/native/<sourceRef>` 进入同一 raw store/query 边界，
沿原 receipt hash、executionKey/fence、Workspace/Run 和 bytes/SHA256 读取；保存失败撤销归因资格，已保存历史仍可读取。
`validation_native_channel_test.go` 覆盖三个实际并行子进程、相同正文、分块 stderr、乱序、单源复现、替换父进程、
重复/伪造来源、容量、取消及 Workspace 隔离；组合 owner 主线覆盖输入维护、不同失败、fresh all-of、assessment 和三 ACK。
这些本地机制回归不证明线上项目已收敛，原判据、期限和全部验收责任不变。

root test/race 完整失败后，只有全部 failed package/case 能唯一匹配封存来源、没有 skip/build/unknown/source drift，
才可在原单 operation deadline 内并行重跑至多五个 top-level native 来源（总 stage 仍不超过八项）。每源独立的完整有界 workspace 副本、HOME/TMP/cache 环境、
coverage file 和 child-process instrumentation，精确保留 race 模式，不能借另一失败源的 aggregate coverage 归属。
owner 新生成的计数使用 atomic 模式，允许原输入中继承 instrumentation 的真实嵌套 `go test -race`；不能因 owner
注入 count 模式使原工程命令失败。完整 count/atomic owner profiles 都按原 exact manifest scope、正计数、非
test/harness 和原容量门禁读取；set/未知格式、缺失/截断仍拒绝，旧 count 原事实不改写。
各子执行共享原 context/deadline 与单执行 64MiB 原文总预算；有界 goroutine 全部 join、进程排空及副本 cleanup 后才发布。按封存 source catalog 顺序汇总，不以完成顺序选择结论。
所有原失败 case/subcase 与 source-bound 失败身份集合必须完整复现，且每源实际触达有限 scope 内非 test/non-harness
实现；source/输出/cleanup/deadline/storage 任一不完整均不发布修复诊断。阶段保全 `DiagnosticSourceRef`、exact `DiagnosticOriginStageID`、唯一副本 `DiagnosticIsolationRef` 与
独立 `BehaviorTargetRefs`、实际 argv、原 raw identity/store refs；普通 receipt/hash/reload/duplicate 链仍绑定完整正文。
若实际归属后原文发布失败，已发生的逐源行为、全部原输出 bytes/hash 身份、已保存流和明确 omission 原因仍保全为
rejected history；不抹除历史，也不发布 structured repair。`TestProjectDiagnosticPublicationFailureKeepsActualSourceHistory`
使用真实 runner/raw store 的发布失败与取消边界核验完整原文及零授权；不是公开 fullflow 或 live。
原工程失败与全部测试不改写，四阶段 conjunction 仍 incomplete、quality invalid、零 CWI credit。

原生失败观察在 Shell 内唯一编译：只对 exact sealed input/hash 中静态确认的直接 `*testing.T.Log/Logf` 及闭合参数表达式，
排除普通观察 frame（含归属其缩进的嵌套 stdout）；完整原 bytes 仍保全。静态类型不完整、helper 调用、用户 Stringer/Formatter、
shadowed/mixed call site 均保留为潜在断言。支持参数限已解析 builtin primitive/byte-rune slices、标准 time 值和闭合的
builtin len/cap/conversion、time/runtime/strings 值操作；不执行输入、不猜用户函数纯度，也不按消息词汇分类。
`go/types` 使用本机 Go toolchain 的导入类型信息；无法解析时只减少排除，不产生观察推断。

复现使用唯一 `shellSameFailureAssertions` 比较 check/case/package/target、body ComparisonHash 和 reported shape 的完整集合；
ComparisonHash 只沿用 owner 已声明的 scratch noise 规则，不清洗耗时/值。helper 装饰行号和同一断言的重复次数不是额外失败身份，
各自原文/位置/hash 仍留在原 receipt。单源与多源诊断共用该规则；真实 body/value/case/source 改变或缺项仍拒绝。
断言 count 上限为64，单条16KiB、总正文128KiB不变；count/body/aggregate/frame分别报 exact ownerCode。
这不意味着64条任意大小都能发送：含旧正式报告及全部metadata的整个 execution feedback JSON 仍受64KiB限制，
共同 typed compiler 显式测量并报告 actual/limit，不截断、不重试、不扩预算。

生成与 source review 共用 proof topology 声明，沿已有 typed check/source/consumer binding：每个当前 native check 保留全部适用义务，
除原义务明确需要，不在每个检查重复根项目 suite 或完整历史证明图；独立 root all-of、当前行为、assessment 与末端 receipt 继续分别履责。
隔离并行仅解决诊断串行成本，不能证明任何具体项目在原300秒内可行，也不授 credit。
`TestNative*`、`TestInapplicableLargeObservationIsExcludedBeforeAssertionCapacity` 覆盖原生观察/真失败、动态值、用户格式化、
31/32/33/39/64/65条及单条/总量边界；`TestProjectMultiFailureDiagnosticActualSourcesAndBehavior` 覆盖动态日志、互斥文件/并行屏障、
运行中取消、partial/unobserved/changed-assertion/source drift。取消后原文未发布时按已有边界撤销诊断，保留真实阶段及 omission。

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
完整 scope 隔离，独立校验全部接收 bytes 的 SHA256 与实际已保存 bytes 的 SHA256。
原文存储与单次读取容量分离：每流最多 16MiB，同一执行（含普通阶段及诊断阶段）合计最多 64MiB；
reconciliation batch 的共同阶段与各独立阶段共享同一 64MiB quota，不能按 consumer 重置。
64KiB 以内使用可逆 base64 JSON，较大流使用绑定完整 saved-stream hash 和有序 chunk hash 的小型 metadata，
原 bytes 以不可变 64KiB chunk 文件保存。读取指定 offset 时只加载相关 chunk，不重复展开整流。
任一容量超界仍 hash/count 全部接收 bytes，明确保留 incomplete／omitted reason，不能变成 PASS 或完整诊断。
发布使用已 fsync 的临时文件和不可覆盖的 hard-link，再删除临时别名及同步目录；任何失败不返回持久成功 ref，
重试核对原 canonical bytes 后才确认，异常已存在文件不覆盖。它不是第二 operation journal，也不提供执行恢复权威。
`validation_project_raw_store_test.go` 覆盖原 bytes（中文／CRLF／NUL／非法 UTF-8）、空输出、并发 duplicate、scope 漂移、
容量、symlink／权限／篡改拒绝、发布前／后故障重载，以及首次／duplicate 同步期间取消不返回成功引用；
真实四阶段 runner 测试同时保存并 fresh-store 读回全部实际输出。
受限存储只保全诊断，不替代原 receipt/aggregate 恢复权威；fresh-process 恢复另行验收。
本节不表示公开 authoring/activation/fullflow、fresh-process 或 live 已验收。

`validation_project_execution.go` 将准备、四阶段、行为转换、原文发布、句柄关闭和实际 scratch cleanup
纳入从私有封套入口开始的同一最早 deadline。只接受原 execution scope 的 ephemeral managed validation，
不接受 caller scratch、超过本次声明或超过 owner 最大 600000ms 的期限；阶段规格从执行前的 owner 编译产物取得，不从结果反推。
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
每次按 offset 最多返回该原流的 64KiB 已保存 bytes，携带准确 offset、chunk SHA256 与完整 saved-stream SHA256；
应用沿同一 receipt/fence/identity 分块读取，逐块核验并在完整拼接后复核 saved-stream hash，任一漂移不返回部分成功。
不完整前缀保持原 incomplete/omission，不改写或摘要原文，也不从保留前缀推断总测试数或缺失失败。
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
| Different validation implementation or observation path | LLM consistency review against the original criteria, bound to exact input content, followed by owner execution facts | Entrypoint and package adapters supply mandatory review subjects; only Orchestrator can seal a consistent report after the independent request, with no waiver or review credit |

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

The complete diagnostic/marker union and its typed observation share the existing
16-target bound with the File confirmation consumer. An overflow is never clipped
or reduced to the parser subset. `ValidationFailureDiagnostic.TargetRejection`
retains the closed rejection code, candidate count at the rejecting boundary and
shared limit; it does not claim the size of a later unconstructed union. The original
failure class, exit code, source conjunction and raw bytes remain factual. Invalid
markers and out-of-scope candidates also reject the whole transaction. A rejection
cannot coexist with partial targets or a repair observation and grants no credit.

Shell does not choose a successor, repair route or terminal state. The existing
Orchestrator SM-2 private receipt event resolves the exact sealed profile route;
SM-3 dispatch and SM-4 delivery/recovery remain unchanged. Result and projections
consume the judged result without reinterpreting this owner diagnostic.

## 能力验收

### Native check readiness and behavior observation

新 Start/Restart 可显式选择 `shell/entrypoint-validation`，其 Go binding 固定使用
`shell/go-test-json/entrypoint-behavior`。该 adapter 同时观测 in-process 测试与本次 Go 构建的子进程；Resume/recovery 仍使用原封存执行输入。

`shell/go-test-input/conjunctive-entrypoint-test` 允许 exact 补充文件中的同包具名 helper、type、const、无调用/闭包
初始化的 var，仍拒绝 init、TestMain、method、额外 Test 定义与不可达的 testing.T 断言。此单检查协议的限制不套用于合取/project package。旧输入只被重新检查，不被改写；
原检查和补充检查仍是 all-of。结构无效或语义证据不足不授权改写判据，也不自动扩大 File repair surface。

新增输入内容合同可以声明可选失败报告语法：实际 testing.T 断言失败时，在独立一行输出
`WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array`。必须精确四个有序 token；封闭 code/shape 配对
由 shared typed DTO 校验，不接受额外正文、实际值或秘密。缺失/无效报告保留位置与 digest，并由 LLM 标记诊断不完整；
不为补诊断改写已有测试，也不把报告语法纳入 native pass/fail 判据。single-test 和 entrypoint 使用各自唯一当前内容合同；
owner catalog 身份绑定真实能力声明，不维护旧格式选择或从新配置重建已封存 authority。
`TestStdioEnvelopeReferenceGraderCalibration` 校准固定真实进程的正例、错误 result/framing/id、缺失工具、marker-only 和
comment-only 反例；`TestShellAssertionComparisonUsesRealManagedTemporaryPaths` 校准真实两次执行的精确 digest 不同而比较
身份相同。它们不是完整 MCP 或五语言验收器，也不提供 release DCE 证据。

隔离包 adapter 对 exact bound supplement 的编译失败另发布最多 8 项 `InputLocations`（相对路径、行、列、封闭 code/symbol），
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

当前 `shell/project-native-validation` adapter 的 Go-native binding 使用 `shell/go-test-json/workspace-behavior`；
包含 exact CWI 补充输入时使用 `shell/go-test-json/supplemented-behavior`。显式固定 command 的 recipe 仅证明其直接执行结果，
不具有交付实现 grounding；需要 grounding 的 Profile 应声明 project-native adapter。两者是独立能力，不是历史格式兼容。
既有 receipt、binding 或归档不被当前目录改写。

- 结构就绪通过真实 `testing` import、`*testing.T` 对象身份和同包具名 helper / `t.Run` 子测试解析断言。
  未调用闭包、未引用 helper、遮蔽的同名变量、跨包 helper 与歧义入口不能代替断言；不支持的结构明确报告未解析。
  检查有 256 源文件、8 MiB、32768 AST 节点、64 层 helper 调用的上界，且读取限制在 workspace root。
- preparation 的可选 `OriginalDiagnostic` 保留 exact requirement/CWI/check、manifest revision、catalog revision 与
  缺失输入原因；不能通过新建输入补齐的结构无效、未解析、歧义、读取失败或资源上界，改由独立的
  `ProofRequirementUnavailability` 保留 exact requirement 与原始原因，不发布另一条 create 路径或 prospective constraint。
  同一 requirement 不能同时为 ready、preparable 与 unavailable；其它 CWI 的合法 binding 不受影响。以上均不是
  target、repair、retry 或 terminal authority。
- 对已有验证包的可处理语法/断言结构缺陷，unavailability 可附 typed `RuntimeValidationInputMaintenance`：原 requirement/binding、
  完整原始 bounded files 与当前同包输入目标。读取复用独立 review 的同一 source/bytes reader，但不生成 review subject/hash、
  一致性结论或 readiness。范围、读取、容量与不支持协议等不可用原因不取得维护许可；Shell 不选择后继、不修改业务。
  SM-2 只有在原 Profile 已授权 input revision 且 exact 当前 CWI/owner/source 均匹配时才可使用现有维护事务；否则保留原不可用原因。
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
skip、初始化/runner override、输入编译错误、精确重放与补充 all-of 正反例。
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

### Validation generation observations

For isolated, conjunctive and project Go adapters that publish a content-bound missing-input preparation, the surface
also publishes one `shell/go-validation-generation-context/v1` observation. It reads actual non-test `.go` source and
present root `go.mod`/`go.sum`/`go.work`/`go.work.sum` from the declared workspace manifest, excluding `workorch_validation/`.
Selection is adapter-specific, not Core policy. Up to 64 files / 256 KiB are supplied byte-for-byte with content hashes;
missing/unreadable/non-UTF-8/symlink files or capacity overflow produce an explicit observation with no partial files.
The context is shared across the surface's checks and bound to the owner catalog revision/snapshot. Original source
obligation checks are preserved from the exact requested requirement. These observations supply implementation context
for generation; they neither prove behavior nor change the independent review scope, native identity or create-only
input rules. Existing input replacement and review-conflict retry remain unauthorized by this observation alone.

### Ready 输入的修订依赖

需要独立 input review 的 recipe 在已发布 exact proof binding 后，仍提供当前 owner revision 的完整 generation context，
而非仅在 prospective preparation 时提供。选择和 bytes/SHA256 校验沿 Shell adapter 合同执行；unavailable/over_capacity
保留明确状态且不发布部分正文。该上下文供 LLM 修订测试输入，不赋予 mutation authority 或行为/回执 credit。
Profile/template 中的正式 owner receipt/Result/ACK 规则由其既定 Runtime 消费点核对，项目 native test 不读取私有 owner 存储。


### 冻结输入修订的写前结构观察

`InspectRuntimeValidationInputStructure` 为只读 SM-3 query，仅接受 Orchestrator 强来源调用。
File 的现有 patch preview 返回精确后像 bytes/hash 和原 replay proof；Shell 对原完整冻结输入 corpus
仅替换该后像，复用 readiness 同一 parser、声明规则及断言遍历，不写文件、不运行进程、不调用模型。
观察绑定 Workspace/Run、authority transaction、candidate manifest、原 revision、operation 与 File 前后像。
缺检查、重复、非法签名、runner interception 等结构问题沿原有一次候选纠正，原文不修补。
此接点只覆盖已有授权的 input-revision File edit/patch，不宣称普通业务源码或首次创建输入都已做写前语言校验。
具体 operation 未触及原 InputTargetRefs 时不调用该结构 query；普通交付依赖仍由 File 正常 feasibility/权限负责。
通过仍须执行 File 原 CAS/权限门禁、fresh readiness、新独立 review 和普通 validation；零验收 credit。

project input-review scope 保留可执行方法事实：可在同一 deadline/cleanup 下调用实际 delivery，或编译并运行继承
owner GOFLAGS/GOCOVERDIR 的新程序。编辑 scope 不能被推断成进程禁止。语义评审判断输入是否真实检查原义务，
不要求当前实现已通过其断言；实际缺交付/行为失败交普通验证。缺失断言、未评审 harness 依赖和无条件占位失败
仍不能判 consistent，不能用本说明删除判据或授 credit。

project adapter 遵守原 recipe/operation 及更早父 context 的单一总期限；准备、build/test/race、当前 CWI
行为、原文发布和 cleanup 都不能重置它。去除 adapter 私有 60s 上限不增加任何既有 Run 期限/预算。
原已封存期限容不下 60s 完整观察与其他阶段时仍不可行，不能缩短样本。当前 software-dev 内置方法对后续合法新事务声明 300000ms 总期限；这不是对旧 Run/已发操作的扩容，也不保证任意项目都能在其中完成。

跨 CWI test/race 证明复用尚未实现。`observeShellValidationCopiedInputs` 仅观察原复制文件，明确不跟踪
全部子进程读取；source hash 和这些依赖相同不能证明网络、时钟、外部文件或工具链依赖未变。
现阶段保留全部物理阶段、完整 Goal/DoD 合取和当前 CWI 的独立行为证明，不能将历史 PASS 改成当前执行。


新 owner input-review subject 显式声明本次 `Recipe` 方法（含总 TimeoutMillis），并将其纳入原 subject hash；
与 catalog 的原 recipe 逐值一致，不能通过自重算 hash 更换期限或方法。新 preparation 和项目 supplement 同步说明
精确已声明总期限；0 仅表示使用 owner 默认，不代表无界或最大值。结构维护保存同一原 recipe，零 review/readiness credit。
重读已冻结 subject 仅保留它原先声明的方法，不给旧原件追加声明。未声明 executable metadata 的 owner 不受 Shell transport 要求。
输入修订继续由原权威、权限和预算控制；新的期限信息不改变全部必需阶段、样本和准则。
新语义 subject 同时声明 `EvaluationScope=input_semantics` 并绑定 hash；结构维护只提供完整原文件，不声明
该语义范围或 review hash。输入审查只判断原义务是否由输入保留及后续证明责任是否正确映射，不要求先证明
实际总耗时或执行成功。真实耗时、超时、失败和 cleanup 继续由 Shell 在原封存总期限及更早父 deadline 内产生，
不能以审查一致授执行 credit，也不能为未知耗时省略阶段或扩大期限。冻结原 subject 不注入新声明。
结构 query 同时登记在 owner topic 与 concrete typed message 合同，均为 Orchestrator SM-3 的只读 participant。

### 同一 reconciliation 批次的共同执行事实

`QueryRuntimeCWIReconciliationBatchCommand` 是 Shell 接收的有界 typed 查询：2–8 个成员必须逐值来自
同一个已持久 request/generation/binding-set/baseline，不能扩展消费者。Orchestrator 只提供原 completed
SM-3 seal 的 exact scope、recipe 和 accepted receipt hash；Shell 独立检查方法和各成员当前输入。
同一当前项目、相同完整 project recipe、各自不重复的 exact native checks 才适用共同执行；成员的
原行为 scope 不合并。任何不适用都在执行前回到各自原方法，不授共享证明。

适用时一次完整 scratch 副本执行共同 build/test/race，随后逐 CWI 执行完整 exact 行为。
每个 `RuntimeValidationExecutionUse` 保留共同阶段同一实际时间、source/argv、原输出身份和受限引用，
以及自己的独立行为、原 accepted receipt 和原 scope/recipe hash。该用途属于 fresh reconciliation
事实，不是另一操作 receipt，不新增普通 validation 或 assessment credit。全批原文发布、关闭、source
完整成员复核和 scratch cleanup 完成，且未超过本次 recipe 与父 context 的最早总期限，才返回满足。
任一成员失败使整批不可满足，保留实际 failure code；执行后的错误不退回重跑。

scope 包含当前完整副本文件集合，新增/删除/替换相邻输入也使本批失效。只在这次查询内共享实际事实，
不以 copied-input hash 宣称网络、时钟、外部文件、环境或工具链具有跨时间依赖闭包，不提供历史 PASS cache。
原文继续保存在受限 Shell store，共同阶段用 reconciliation/group scope，独立阶段另含 binding identity；
它们不是 operation receipt 的输出，不借原 receipt read topic 获得读取权限。
持久 batch 后的 duplicate drive 读原 aggregate；在 batch CAS 前中断仍遵守原 fresh-query 语义，
不承诺查询物理执行 at-most-once，也不另造恢复账本。


### 验证输入的证明职责

当前项目验证输入合同区分可执行行为断言、独立语义 assessment 与封存 lineage 消费者。
每个原 native check 和来源义务的行为部分仍完整执行；必要性/最小性可显式映射 `definition_assessment`，
由独立评估消费原判据与 Orchestrator 当前 scoped 变更事实，不让 native test 伪造 owner baseline/receipt 或自证最小性。
无条件 evidence-gap 报错、未观测行为、缺项和冲突仍阻断；input review/readiness/file effect 不授 credit。
根 module build/test/race、exact checks、行为观测、单次总 deadline 和原 terminal receipt/三 ACK 不变。

### 大输出失败到修复的当前合同回归

`TestProjectMultiFailureDiagnosticActualSourcesAndBehavior/large-output` 在真实 Go 输出 2MiB 之后产生失败断言，
要求完整存储、逐源行为归因与准确实现 target；容量不足不能猜测失败来源。
`TestDriveRuntimeProjectLargeOutputConfirmedRepair` 经过实际 Shell/File/Run owners、独立输入审查、
原失败 receipt、确认修复、新普通验证、assessment 及 Result→Workspace→Run 三 ACK；
验证与修复 receipt 边界恢复核对原身份及预算唯一消费。provider/reviewer 是固定 fixture，测试不调用收费模型。
这些本地回归不替代部署后的真实项目验收，不追溯补全旧 Run 缺失原文或给旧结果 credit。

### 原生失败帧与独立诊断不可用

原生 package/case/source frame 先按缩进拼接精确多行正文（含空行和更深缩进的源码样文本），
退回外层的独立输出结束当前 frame；同层未知续行仍标 incomplete，不按日志关键词裁剪。
原始 bytes/读取引用继续完整保全。JSON stage 只解析一次，独立归因消费其完整 native observation，
不能因未取得 repair target 丢掉原 FailedChecks/断言，或把已运行的普通失败投影成空观察。

multi-source adapter 的所有非成功出口返回 `ValidationFailureDiagnostic.Unavailability` 的闭合 typed 原因及
精确 owner code/stage/原 exit；不返回静默 nil、不授部分行为 union 完整修复权限。原生断言不完整、来源未解析、
容量、独立重现、行为、输入变化、执行条件和原操作 deadline 各自区分。context cancellation 为 execution unavailable，
不能冒充 deadline；诊断不创建更长 deadline。独立阶段已执行事实和原失败 raw/receipt继续保留。
Orchestrator 消费其适用事实决定输入重审或精确终止，Shell 不选择下一步，也不修改旧结果。

`TestShellValidationAssertionFramesSeparateOuterLogsAndNestedSourceText`、原生多行/空行/未知续行测试、
`TestProjectMultiFailureDiagnosticActualSourcesAndBehavior` 和 context disposition 覆盖 producer；
`TestDriveRuntimeProjectMultiFailureOwnerMainline/project_history_failure` 覆盖完整断言→独立重审→一致但无行为归因→
精确失败交付/恢复，已归因失败仍覆盖原 File repair→新 all-of→assessment→有序三 ACK。
