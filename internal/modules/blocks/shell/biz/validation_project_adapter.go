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
const shellProjectInputContentRequirements = "按 targetRef 创建一个 Go 测试文件，逐一声明所有 nativeCheckRef，签名为 func(t *testing.T)，每项原判据和来源义务均须通过真实 testing.T 独立断言。全部检查属于同一个不可变 requirement package；不能省略、别名化或替代成员。允许同包命名 helper、type、const 和纯数据 var；禁止 init、TestMain、method、未声明 Test、全局 initializer 调用/闭包、嵌套 module、cgo 或导入其他 validation package。全部 helper/fixture 必须留在当前 package，完整评审仍限 16 files/64 KiB，包括现存根 module/workspace 定义。实际调用 workspace delivery，或通过继承 owner instrumentation 的新 Go-built executable 观察真实行为；依赖缺失、skip、源码标记、测试内替身和 instrumentation override 均不授 proof。Shell 按同一不超过 60000ms 的 operation deadline 执行根 module go build ./...、go test -json -count=1 ./...、go test -race -json -count=1 ./...，最后执行当前 package 的全部 exact checks 并独立观察实际非测试 delivery 行为。任一阶段失败、零测试、skip、unfinished、source 漂移、原文容量不足或 cleanup/deadline 失败均不能合取通过；前三阶段不能代替逐 CWI 判据。保留全部历史检查和原判据；创建、readiness、评审均零执行/acceptance credit。scratch 不是 OS/网络/凭据强隔离。"
const shellProjectReviewScopeRequirements = "供应完整当前 requirement package，包括每个 original/supplemental native check、全部 helper/fixture 和现存根 module/workspace 定义；保持 16 files/64 KiB，不静默省略或截断。当前 package 的判据一致性评审不替相邻 harness 补造评审。Shell 的根 module build/test/race 另外实际执行，包括现存相邻 package 的失败；最后阶段独立证明当前 CWI 的 exact 判据和真实 delivery 行为。全项目成功不能反推逐义务语义正确；任何原 Goal/DoD 判据仍 mandatory，禁止 criterion replacement 或自动放宽。未评审外部 harness 依赖保持明确不完整；package scope 不提供 OS 安全沙箱。"

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
		recipe.TimeoutMillis >= 0 && recipe.TimeoutMillis <= DefaultCommandTimeout.Milliseconds()
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
