package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellProjectInputContentContractRef = "shell/go-test-input/project-conjunction/v1"
const shellProjectReviewScopeContractRef = "shell/go-test-input/project-conjunction-review/v1"
const shellProjectInputContentRequirements = "按 targetRef 创建一个 Go 测试文件，逐一声明所有 nativeCheckRef，签名为 func(t *testing.T)，每项原判据和来源义务的可执行行为部分均须通过真实 testing.T 独立断言。当前证明合同允许将必要的语义责任映射到definition_assessment；必要性/最小性等独立语义评估交definition_assessment消费owner变更事实，不要求native test重建owner回执或自证最小性；原义务及普通验证均不删除，receipt由原lineage消费者核对。全部检查属于同一个不可变 requirement package；不能省略、别名化或替代成员。允许同包命名 helper、type、const 和纯数据 var；禁止 init、TestMain、未声明 native Test、全局 initializer 调用/闭包、嵌套 module、cgo 或导入其他 validation package。全部 helper/fixture 必须留在当前 package，完整评审仍限 16 files/64 KiB，包括现存根 module/workspace 定义。实际调用 workspace delivery，或通过继承 owner instrumentation 的新 Go-built executable 观察真实行为；依赖缺失、skip、源码标记、测试内替身和 instrumentation override 均不授 proof。Shell 按同一本次封存的 operation deadline（未显式声明时使用 owner 默认期限，允许范围 1000–600000ms） 执行根 module go build ./...、go test -json -count=1 ./...、go test -race -json -count=1 ./...，最后执行当前 package 的全部 exact checks 并独立观察实际非测试 delivery 行为。任一阶段失败、零测试、skip、unfinished、source 漂移、原文容量不足或 cleanup/deadline 失败均不能合取通过；前三阶段不能代替逐 CWI 判据。保留全部历史检查和原判据；创建、readiness、评审均零执行/acceptance credit。scratch 不是 OS/网络/凭据强隔离。"
const shellProjectReviewScopeRequirements = "供应完整当前 requirement package，包括每个 original/supplemental native check、全部 helper/fixture 和现存根 module/workspace 定义；保持 16 files/64 KiB，不静默省略或截断。当前 package 的判据一致性评审不替相邻 harness 补造评审。Shell 的根 module build/test/race 另外实际执行，包括现存相邻 package 的失败；最后阶段独立证明当前 CWI 的 exact 判据和真实 delivery 行为。全项目成功不能反推逐义务语义正确；任何原 Goal/DoD 判据仍 mandatory，禁止 criterion replacement 或自动放宽。未评审外部 harness 依赖保持明确不完整；允许通过真实 delivery 或继承 owner GOFLAGS/GOCOVERDIR 的新 Go-built executable 在同一 operation deadline 内观察行为及 cleanup；编辑目标列表不限制这些观察方式。缺失交付或真实行为断言失败本身不代表输入不一致，实际失败由普通验证产生；缺失断言和无条件占位失败仍不满足输入合同。package scope 不提供 OS 安全沙箱。"

var shellProjectNativeIdentity = regexp.MustCompile(`^TestWorkOrchCWI_([A-F0-9]{64})_Check_[A-Z0-9_]+_ProjectInput$`)

const shellProjectFailureDiagnosticProtocolRef = "shell/go-test-json/project-failure-diagnostic-behavior"
const shellProjectMultiFailureDiagnosticProtocolRef = "shell/go-test-json/project-multi-failure-behavior"

func shellProjectAdapter(adapter string) bool {
	return adapter == modulecommon.ValidationRecipeAdapterProjectConjunction || adapter == modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic || adapter == modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
}

func shellProjectObservationProtocol(protocol string) bool {
	return protocol == shellProjectConjunctionProtocolRef || protocol == shellProjectFailureDiagnosticProtocolRef || protocol == shellProjectMultiFailureDiagnosticProtocolRef
}

func shellProjectAdapterProtocol(adapter string) string {
	if adapter == modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
		return shellProjectMultiFailureDiagnosticProtocolRef
	}
	if adapter == modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic {
		return shellProjectFailureDiagnosticProtocolRef
	}
	return shellProjectConjunctionProtocolRef
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

func shellProjectExecutionStageIdentities(pkg string) []runtimecontract.RuntimeValidationExecutionStageIdentity {
	specs := shellProjectFixedStages(nil, pkg)
	ret := make([]runtimecontract.RuntimeValidationExecutionStageIdentity, 0, len(specs))
	for index, spec := range specs {
		ret = append(ret, runtimecontract.RuntimeValidationExecutionStageIdentity{ID: spec.ID, Scope: spec.Scope, RequireTests: index > 0, RequireBehavior: index == 3})
	}
	return ret
}

func shellProjectRecipeDeclared(recipe runtimecontract.RuntimeValidationRecipe) bool {

	return shellProjectAdapter(recipe.AdapterRef) &&
		recipe.InputReviewProtocolRef == runtimecontract.RuntimeValidationInputReviewProtocol &&
		(recipe.Command == "" && len(recipe.Args) == 0 || recipe.Command == "go" && strings.Join(recipe.Args, "\x00") == "test\x00-v\x00./...") &&
		(recipe.TimeoutMillis == 0 || recipe.TimeoutMillis >= 1000 && recipe.TimeoutMillis <= MaxCommandTimeout.Milliseconds())
}

func shellProjectReconciliationRawScope(command shellevents.QueryRuntimeCWIReconciliationCommand, recipeID string) shellProjectRawScope {
	// 独立域隔离诊断存储；不是新的 execution/receipt journal，也不授原操作 replay 权。
	body, _ := json.Marshal(struct {
		WorkspaceID        string
		RunID              string
		RequestID          string
		Generation         uint64
		DefinitionRevision string
		BindingSetHash     string
		BindingHash        string
		BaselineRevision   string
		BaselineRef        string
		RecipeID           string
	}{string(command.WorkspaceID), string(command.RunID), command.RequestID, command.Generation, command.DefinitionRevision, command.BindingSetHash, command.Binding.BindingHash, command.WorkspaceBaselineRevision, command.WorkspaceBaselineRef, recipeID})
	digest := sha256.Sum256(append([]byte("shell-project-reconciliation-raw/v1\x00"), body...))
	return shellProjectRawScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID, ExecutionKey: "reconciliation/" + hex.EncodeToString(digest[:])}
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
	hash := runtimecontract.StableRuntimeValidationExecutionStagesHash(shellProjectExecutionStageIdentities(pkg))
	for _, check := range request.ValidationCheckAttestations {
		if check.ObservationProtocolRef != shellProjectAdapterProtocol(recipe.AdapterRef) || check.ExecutionStagesHash != hash {
			return false, fmt.Errorf("project conjunction execution stages do not match sealed mappings")
		}
	}
	return true, nil
}

func shellProjectContentRequirements(adapter string) string {
	if adapter != modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic {
		return shellProjectInputContentRequirements
	}
	return shellProjectMultiFailureInputContentRequirements
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

const shellProjectMultiFailureInputContentRequirements = "按 targetRef 创建或按原授权维护当前 Go requirement package。每个原 nativeCheckRef 必须唯一完整，签名为 func(t *testing.T)，并按原判据和全部来源义务的可执行行为通过真实 testing.T 独立断言。采用显式definition_assessment职责映射时，独立必要性/最小性评估由原assessment消费owner变更事实，native test不得伪造baseline、回执或自证语义结论；原义务和行为断言继续完整保留。允许普通附加 native Test、同包命名 helper/method、type、const 和纯数据 var；所有附加 native Test 都进入完整独立审查及全项目合取执行，不能替代 required checks 或自动取得 CWI credit。禁止 init、TestMain、全局 initializer 调用/闭包、嵌套 module、cgo、导入其他 validation package 或覆盖 owner instrumentation。全部 helper/fixture 留在当前 package，完整项目 corpus 评审限 64 files/256 KiB、最多 64 native sources，报告限 64 KiB；不得省略或截断。调用实际 workspace delivery，或继承 owner instrumentation 的新 Go-built executable；依赖缺失、skip、源码标记、测试内替身均不授 proof。根 module build/test/race 和当前 exact checks 的实际行为观察为 all-of，全部使用同一本次封存的 operation deadline（未显式声明时使用 owner 默认期限，允许范围 1000–600000ms），包含准备及 cleanup，不为某个阶段重置期限。任何原 Goal/DoD 判据仍为 mandatory；若所需观测不能在原期限内完成，应如实指出不可行，不能跳过、缩短样本或改判。保全当前仍适用的全部原检查和原判据；effect、readiness 和评审均零执行/验收 credit。scratch 不是 OS/网络/凭据强隔离。" + shellProjectProofTopologyRequirements

func shellProjectAdapterForObservation(protocol string) string {
	switch protocol {
	case shellProjectMultiFailureDiagnosticProtocolRef:
		return modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	case shellProjectFailureDiagnosticProtocolRef:
		return modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
	default:
		return modulecommon.ValidationRecipeAdapterProjectConjunction
	}
}
