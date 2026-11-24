package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellProjectInputContentContractRef = "shell/go-test-input/project-conjunction/v1"
const shellProjectReviewScopeContractRef = "shell/go-test-input/project-conjunction-review/v1"
const shellProjectInputContentRequirements = "按 targetRef 创建一个 Go 测试文件，逐一声明所有 nativeCheckRef，签名为 func(t *testing.T)，每项原判据和来源义务的可执行行为部分均须通过真实 testing.T 独立断言。当前证明合同允许将必要的语义责任映射到definition_assessment；必要性/最小性等独立语义评估交definition_assessment消费owner变更事实，不要求native test重建owner回执或自证最小性；原义务及普通验证均不删除，receipt由原lineage消费者核对。全部检查属于同一个不可变 requirement package；不能省略、别名化或替代成员。允许同包命名 helper、type、const 和纯数据 var；禁止 init、TestMain、未声明 native Test、全局 initializer 调用/闭包、嵌套 module、cgo、导入其他 validation package 或使用 runtime.Caller/CallersFrames。测试在测试包目录 cwd 下以 GOFLAGS=-trimpath、GOWORK=off 运行：请用 os.Getwd() 向上定位 go.mod，构建/临时产物只写入 t.TempDir()（禁止写入隔离工作区副本以免 source drift）。全部 helper/fixture 必须留在当前 package，完整评审仍限 16 files/64 KiB，包括现存根 module/workspace 定义。实际调用 workspace delivery，或实际运行继承 owner instrumentation 的新 Go-built executable 观察真实行为（纯 go build 或嵌套 go test 不向 owner GOCOVERDIR 产出覆盖）；依赖缺失、skip、源码标记、测试内替身和 instrumentation override 均不授 proof。Shell 按同一本次封存的 operation deadline（未显式声明时使用 owner 默认期限，允许范围 1000–600000ms） 执行根 module go build ./...、go test -json -count=1 ./...、go test -race -json -count=1 ./...，最后执行当前 package 的全部 exact checks 并独立观察实际非测试 delivery 行为。任一阶段失败、零测试、skip、unfinished、source 漂移、原文容量不足或 cleanup/deadline 失败均不能合取通过；前三阶段不能代替逐 CWI 判据。保留全部历史检查和原判据；创建、readiness、评审均零执行/acceptance credit。scratch 不是 OS/网络/凭据强隔离。"
const shellProjectReviewScopeRequirements = "供应完整当前 requirement package，包括每个 original/supplemental native check、全部 helper/fixture 和现存根 module/workspace 定义；保持 16 files/64 KiB，不静默省略或截断。当前 package 的判据一致性评审不替相邻 harness 补造评审。Shell 以 GOFLAGS=-trimpath、GOWORK=off 在测试包 cwd 执行，根 module build/test/race 另外实际执行，包括现存相邻 package 的失败；最后阶段独立证明当前 CWI 的 exact 判据和真实 delivery 行为。查找 go.mod 须用 os.Getwd()（禁止 runtime.Caller/CallersFrames），临时/构建产物须写 t.TempDir()。全项目成功不能反推逐义务语义正确；任何原 Goal/DoD 判据仍 mandatory，禁止 criterion replacement 或自动放宽。未评审外部 harness 依赖保持明确不完整；允许通过同进程真实 delivery 或实际运行继承 owner GOFLAGS/GOCOVERDIR 的新 Go-built executable 在同一 deadline 内观察行为及 cleanup（纯 go build 或嵌套 go test 不产出 owner GOCOVERDIR 覆盖）；编辑目标列表不限制这些观察方式。缺失交付或真实行为断言失败本身不代表输入不一致，实际失败由普通验证产生；缺失断言、环境/cwd 假设冲突和无条件占位失败仍不满足输入合同。package scope 不提供 OS 安全沙箱。"

var shellProjectNativeIdentity = regexp.MustCompile(`^TestWorkOrchCWI_([A-F0-9]{64})_Check_[A-Z0-9_]+_ProjectInput$`)

const shellProjectFailureDiagnosticProtocolRef = "shell/go-test-json/project-failure-diagnostic-behavior"
const shellProjectMultiFailureDiagnosticProtocolRef = "shell/go-test-json/project-multi-failure-behavior"
const shellProjectMultiFailureArtifactProtocolRef = "shell/go-test-json/project-multi-failure-artifact"
const shellProjectMultiFailureArtifactContentContractRef = "shell/go-test-input/project-multi-failure-artifact/v1"

func shellProjectAdapter(adapter string) bool {
	return adapter == modulecommon.ValidationRecipeAdapterProjectConjunction ||
		adapter == modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic ||
		adapter == modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic ||
		adapter == modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact
}

func shellProjectObservationProtocol(protocol string) bool {
	return protocol == shellProjectConjunctionProtocolRef ||
		protocol == shellProjectFailureDiagnosticProtocolRef ||
		protocol == shellProjectMultiFailureDiagnosticProtocolRef ||
		protocol == shellProjectMultiFailureArtifactProtocolRef
}

func shellProjectMultiFailureProtocol(protocol string) bool {
	return protocol == shellProjectMultiFailureDiagnosticProtocolRef || protocol == shellProjectMultiFailureArtifactProtocolRef
}

func shellProjectRequiresBehavior(protocol string) bool {
	return shellProjectObservationProtocol(protocol) && protocol != shellProjectMultiFailureArtifactProtocolRef
}

func shellProjectFinalStageID(requireBehavior bool) string {
	if requireBehavior {
		return "reviewed-cwi-behavior"
	}
	return "reviewed-cwi-execution"
}

func shellProjectAdapterProtocol(adapter string) string {
	switch adapter {
	case modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact:
		return shellProjectMultiFailureArtifactProtocolRef
	case modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic:
		return shellProjectMultiFailureDiagnosticProtocolRef
	case modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic:
		return shellProjectFailureDiagnosticProtocolRef
	default:
		return shellProjectConjunctionProtocolRef
	}
}

func shellProjectNativePackage(native string) string {
	match := shellProjectNativeIdentity.FindStringSubmatch(strings.TrimSuffix(native, "_Behavior"))
	if len(match) != 2 {
		return ""
	}
	return "workorch_validation/project/" + strings.ToLower(match[1])
}

func shellProjectRequirementIdentity(requirement runtimecontract.RuntimeValidationProofRequirement) string {
	body, _ := json.Marshal(requirement.Normalized())
	digest := sha256.Sum256(append([]byte("shell-project-conjunction-requirement/v1\x00"), body...))
	return hex.EncodeToString(digest[:])
}

func shellProjectExecutionStageIdentities(pkg string, requireBehavior ...bool) []runtimecontract.RuntimeValidationExecutionStageIdentity {
	behavior := true
	if len(requireBehavior) != 0 {
		behavior = requireBehavior[0]
	}
	specs := shellProjectFixedStages(nil, pkg, behavior)
	ret := make([]runtimecontract.RuntimeValidationExecutionStageIdentity, 0, len(specs))
	for index, spec := range specs {
		ret = append(ret, runtimecontract.RuntimeValidationExecutionStageIdentity{ID: spec.ID, Scope: spec.Scope, RequireTests: index > 0, RequireBehavior: index == 3 && behavior})
	}
	return ret
}

func shellProjectExecutionStageIdentitiesForProtocol(pkg, protocol string) []runtimecontract.RuntimeValidationExecutionStageIdentity {
	return shellProjectExecutionStageIdentities(pkg, shellProjectRequiresBehavior(protocol))
}

func shellProjectRecipeDeclared(recipe runtimecontract.RuntimeValidationRecipe) bool {
	return shellProjectAdapter(recipe.AdapterRef) &&
		recipe.InputReviewProtocolRef == runtimecontract.RuntimeValidationInputReviewProtocol &&
		(recipe.Command == "" && len(recipe.Args) == 0 || recipe.Command == "go" && strings.Join(recipe.Args, "\x00") == "test\x00-v\x00./...") &&
		(recipe.TimeoutMillis == 0 || recipe.TimeoutMillis >= 1000 && recipe.TimeoutMillis <= MaxCommandTimeout.Milliseconds())
}

// 只消费 sealed recipe 与 proof mapping，禁止依照当前默认值升级旧执行。
func shellProjectRequestSelected(request RuntimeCommandRequest) (bool, error) {
	recipe, found := runtimecontract.RuntimeValidationRecipeForID(request.ValidationRecipes, request.ValidationRecipeID)
	selected := found && shellProjectAdapter(recipe.AdapterRef)
	projectChecks := false
	for _, check := range request.ValidationCheckAttestations {
		projectChecks = projectChecks || shellProjectObservationProtocol(check.ObservationProtocolRef)
	}
	if !selected && !projectChecks {
		return false, nil
	}
	if !selected || !shellProjectRecipeDeclared(recipe) || len(request.ValidationCheckAttestations) == 0 {
		return false, fmt.Errorf("project conjunction requires exact sealed recipe and complete proof mappings")
	}
	pkg, err := shellIsolatedValidationPackage(request.ValidationCheckAttestations)
	if err != nil || pkg == "" {
		return false, fmt.Errorf("project conjunction requires one exact reviewed requirement package")
	}
	expectedProtocol := shellProjectAdapterProtocol(recipe.AdapterRef)
	requireBehavior := shellProjectRequiresBehavior(expectedProtocol)
	hash := runtimecontract.StableRuntimeValidationExecutionStagesHash(shellProjectExecutionStageIdentities(pkg, requireBehavior))
	for _, check := range request.ValidationCheckAttestations {
		if check.ObservationProtocolRef != expectedProtocol || check.ExecutionStagesHash != hash {
			return false, fmt.Errorf("project conjunction execution stages do not match sealed mappings")
		}
		if !requireBehavior && check.SupplementalNativeCheckRef != "" {
			return false, fmt.Errorf("exact artifact validation does not accept behavior supplements")
		}
	}
	return true, nil
}

func shellProjectContentContractRef(adapter string) string {
	if adapter == modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact {
		return shellProjectMultiFailureArtifactContentContractRef
	}
	return shellProjectInputContentContractRef
}

func shellProjectContentRequirements(adapter string) string {
	switch adapter {
	case modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact:
		return shellProjectMultiFailureArtifactInputContentRequirements
	case modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic:
		return shellProjectMultiFailureInputContentRequirements
	default:
		return shellProjectInputContentRequirements
	}
}

// Use the exact declaration at the new preparation boundary. A zero deadline
// means owner default, never the largest allowed value or a per-stage grant.
func shellValidationDeclaredDeadlineRequirement(timeoutMillis int64) string {
	if timeoutMillis == 0 {
		return " 本 recipe 未显式声明 timeoutMillis（0）；执行使用 owner 默认及更早的父 deadline，不能把允许的最大值当作可用期限。"
	}
	return fmt.Sprintf(" 本 recipe 声明的总 operation timeoutMillis=%d；准备、全部必需阶段、原文保存及 cleanup 共享此期限，更早父 deadline 优先；不得按阶段重置、跳过原义务或缩短必需观测。", timeoutMillis)
}

// Shared generation/review declaration. Exact check/source membership and proof
// consumers still come from the existing typed binding; this is not a second
// topology, scheduler or authority.
const shellProjectProofTopologyRequirements = " Map each current native check to all applicable original obligations, necessary observations and existing proof consumers. Root build/test/race, independent CWI behavior, assessment and terminal receipt consumers remain mandatory and distinct. Do not repeat the whole suite or historical proof graph per check unless an original obligation requires it. Shared helpers preserve each check's assertions and independent behavior; shared results cannot impersonate independent executions. Consume formal results only within scope; new failures require affected-owner review. Preparation, execution, diagnostics, raw publication and cleanup share the original deadline. Report infeasibility; never weaken criteria, shorten samples, extend time or fabricate receipts."

const shellProjectMultiFailureInputContentRequirements = "Create/maintain targetRef in this requirement package; declare every nativeCheckRef once as func(t *testing.T), asserting its criterion and obligations via real testing.T. definition_assessment verifies necessity/minimality from owner facts; never fabricate baselines/receipts. Added Tests need independent review/execution all-of, never replacements. Local helpers/methods, types/consts and pure vars allowed; no init, TestMain, global calls/closures, nested modules, cgo, other validation imports, runtime.Caller/CallersFrames or env/instrumentation overrides. Run with GOFLAGS=-trimpath, GOWORK=off in package cwd: locate go.mod via os.Getwd() and write temp/build outputs only to t.TempDir() (never workspace copy). Review full package, shared data/docs and root defs (64 files/256 KiB, 64 sources, report 64 KiB), no truncation; adjacent packages run all-of. Owner records provenance and failed-source bodies. Exercise actual delivery in-process or run freshly go-built binaries inheriting owner env; owner GOCOVERDIR observes in-process calls or executed built binaries only, not go-build-only or nested go test. Skips/markers/substitutes grant no proof. Root build/test/race and CWI behavior run all-of under one deadline (1000-600000ms or default), including prep/raw/cleanup. Preserve Goal/DoD/checks; readiness/review/effect grant no credit; scratch is not a security sandbox." + shellProjectProofTopologyRequirements + shellNativeSourceExecutionRequirements

const shellProjectMultiFailureArtifactInputContentRequirements = "Create/maintain targetRef in this requirement package; declare every nativeCheckRef once as func(t *testing.T), asserting its criterion and obligations via real testing.T against actual workspace artifacts (docs, plans, configs), static/lint checks, or owner-grounded facts. definition_assessment consumes owner facts for necessity/minimality, VCS commits, and terminal result/evidence-index duties. Do NOT read private Runtime storage, fabricate receipts/summaries, or inject unrelated Go delivery calls solely to manufacture code coverage. Added Tests require independent review/execution all-of, never replacements. Local helpers/methods, types/consts and data-only vars allowed; no init, TestMain, global calls/closures, nested modules, cgo, other validation imports, runtime.Caller/CallersFrames or env/instrumentation overrides. Run with GOFLAGS=-trimpath, GOWORK=off in package cwd: locate go.mod via os.Getwd() and write temp/build outputs only to t.TempDir() (never workspace copy). Review complete package, shared data/docs and root defs (64 files/256 KiB, 64 sources, report 64 KiB); adjacent packages run all-of. Root build/test/race and reviewed-cwi-execution exact checks execute all-of under one deadline (1000-600000ms or default), yielding ProofClass executed without requiring Go production code coverage. Exact CWI execution, assessment and terminal receipt consumers remain mandatory and distinct. Preserve Goal/DoD/checks; skips/placeholders grant no proof; readiness/review/effect grant no credit; scratch is not a security sandbox." + shellNativeSourceExecutionRequirements

func shellProjectAdapterForObservation(protocol string) string {
	switch protocol {
	case shellProjectMultiFailureArtifactProtocolRef:
		return modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact
	case shellProjectMultiFailureDiagnosticProtocolRef:
		return modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	case shellProjectFailureDiagnosticProtocolRef:
		return modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
	default:
		return modulecommon.ValidationRecipeAdapterProjectConjunction
	}
}

// This method is supplied at the same owner-authored generation/review boundary
// as the proof topology. The input still owns its actual criterion assertions.
const shellNativeSourceExecutionRequirements = ` Subprocesses must use owner channel (never os.Executable or shared test.v/JSON): helper calls t.Helper(); if os.Getenv("WORKORCH_NATIVE_SOURCE_CHILD")==t.Name() run all assertions and return; else t.Parallel(), require nonempty os.Getenv("WORKORCH_NATIVE_SOURCE_RUNNER"), exec.Command(runner,t.Name()).Run() with inherited env, fail testing.T on error. Shell runs exact check with separate raw stdout/stderr; return exit status only, never override env, recurse, print child raw, or skip on missing runner. Observations, criteria, deadline, cleanup, validation, assessment remain mandatory; provenance grants no credit.`
