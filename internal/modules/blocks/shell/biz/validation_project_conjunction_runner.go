package shell

import (
	"context"
	"fmt"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 在正式 recipe 接线前只允许 owner 内部调用。前三阶段不使用当前 CWI 的
// coverage；最后阶段单独解析原生检查和行为，不能借全项目测试的 coverage 授予 credit。
func runShellProjectConjunction(ctx context.Context, directory string, env []string, request RuntimeCommandRequest) (RuntimeCommandResult, shellProjectConjunctionFact, error) {
	output := shellBaseOutput(request, request.Command, request.Args, directory, "", "go")
	output.Status = "completed"
	output.TimeoutMillis = request.TimeoutMillis
	startedAt := time.Now()
	if ctx == nil {
		output.Status, output.FailureCode = "failed", "validation_project_deadline_invalid"
		result, err := finalizeShellResult(output, fmt.Errorf("复合验证缺少 operation context"))
		return result, sealShellProjectConjunction(nil, nil, nil), err
	}
	timeout, timeoutErr := shellCommandTimeout(request.TimeoutMillis, DefaultCommandTimeout)
	deadline, bounded := ctx.Deadline()
	if timeoutErr != nil || timeout > DefaultCommandTimeout || !bounded || time.Until(deadline) > timeout || ctx.Err() != nil {
		output.Status, output.FailureCode = "failed", "validation_project_deadline_invalid"
		result, err := finalizeShellResult(output, fmt.Errorf("复合验证要求有效且不超过原 60000ms 的单一 deadline"))
		return result, sealShellProjectConjunction(nil, nil, nil), err
	}
	output.TimeoutMillis = timeout.Milliseconds()
	pkg, err := shellIsolatedValidationPackage(request.ValidationCheckAttestations)
	if err != nil || pkg == "" || request.StateChanging || request.Command != "go" || !shellInputHasManagedValidation(request) || !shellRequiresEntrypointBehavior(request) {
		if err == nil {
			err = fmt.Errorf("复合验证要求 exact reviewed package 与 managed entrypoint observation")
		}
		output.Status, output.FailureCode = "failed", "validation_project_binding_invalid"
		result, err := finalizeShellResult(output, err)
		return result, sealShellProjectConjunction(nil, nil, nil), err
	}
	// 原生 selector 和当前输入必须仍由原 owner 合同完整验证；caller 不能以
	// instrumented argv 或 package 名称替代检查映射与当前 source membership。
	behaviorArgs, coveragePath, err := prepareShellBehaviorObservation(request, request.Args, directory)
	processPath := ""
	if err == nil {
		processPath, err = prepareShellProcessBehavior(request, directory)
	}
	if err != nil {
		output.Status, output.FailureCode = "failed", "validation_project_binding_invalid"
		result, err := finalizeShellResult(output, err)
		return result, sealShellProjectConjunction(nil, nil, nil), err
	}
	behaviorArgs = shellProcessBehaviorArgs(behaviorArgs, processPath)
	specs := shellProjectFixedStages(behaviorArgs, pkg)
	output.projectStageSpecs = specs
	var stages []shellProjectStageFact
	var executeErr error
	for index, spec := range specs {
		stageEnv := shellBehaviorEnvironment(env)
		if index == len(specs)-1 {
			stageEnv = shellProcessBehaviorEnvironment(stageEnv, processPath, behaviorArgs)
		}
		stage, stageErr := runShellProjectStage(ctx, directory, stageEnv, spec)
		stages = append(stages, stage)
		output.Stdout, output.Stderr = string(stage.Stdout.Raw), string(stage.Stderr.Raw)
		output.ExitCode = int64(stage.ExitCode)
		output.StdoutBytes, output.StderrBytes = stage.Stdout.Bytes, stage.Stderr.Bytes
		output.StdoutTruncated, output.StderrTruncated = !stage.Stdout.Complete, !stage.Stderr.Complete
		output.OutputTruncated = output.StdoutTruncated || output.StderrTruncated
		if spec.JSON {
			// 每阶段独立解析，前一阶段的失败或 quality 不能作为后一阶段前像。
			output.EvidenceQuality, output.EvidenceReason = "", ""
			output.ZeroWork, output.goTestJSONFacts = false, nil
			attachShellValidationObservation(&output, "go", spec.Args, request.ValidationRecipeID, true)
			attachShellValidationCheckFailures(&output, request)
		}
		if index == len(specs)-1 {
			output.workspaceProcessBehaviorTargetRefs, output.workspaceProcessBehaviorError = shellProcessBehaviorTargets(ctx, "go", directory, processPath, stageEnv, request.ValidationScopeTargetRefs)
			if behaviorErr := attachShellWorkspaceBehavior(&output, request, directory, coveragePath); stageErr == nil && behaviorErr != nil {
				stageErr = behaviorErr
			}
			for _, check := range request.ValidationCheckAttestations {
				for _, native := range check.NativeCheckRefs() {
					if output.ValidationObservation == nil || !runtimecontract.RuntimeValidationCheckPassed(*output.ValidationObservation, native) {
						if stageErr == nil {
							stageErr = fmt.Errorf("复合验证缺少 exact native check 的完整通过：%s", native)
						}
					}
				}
			}
			// coverage 转换仍属于同一次 operation；其后的 source 和总期限也须
			// 复核，不能只验证最后一个 go test 的进程退出时刻。
			after, sourceErr := shellProjectSourceSetHash(directory)
			if stageErr == nil && (sourceErr != nil || after != stage.SourceAfter) {
				stageErr = fmt.Errorf("复合验证行为观察后 source 身份发生变化")
			}
			if stageErr == nil && ctx.Err() != nil {
				stageErr = ctx.Err()
			}
		}
		if stageErr != nil {
			if stages[len(stages)-1].FailureCode == "" {
				stages[len(stages)-1].FailureCode = "validation_project_behavior_incomplete"
			}
			executeErr = stageErr
			output.Status, output.Stage = "failed", spec.ID
			if stage.TimedOut || ctx.Err() == context.DeadlineExceeded {
				output.Status = "timeout"
			}
			output.FailureCode = stages[len(stages)-1].FailureCode
			break
		}
	}
	fact := sealShellProjectConjunction(stages, specs, output.workspaceBehaviorTargetRefs)
	if executeErr == nil && !fact.Complete {
		executeErr = fmt.Errorf("复合验证未完成：%s", fact.FailureCode)
		output.Status, output.FailureCode = "failed", fact.FailureCode
	}
	if executeErr != nil {
		// 即使前一阶段产生了 PASS，也不能把未完成合取投影成当前 CWI 的通过。
		output.EvidenceQuality = string(common.EvidenceQualityInvalid)
		if output.ValidationObservation != nil {
			output.ValidationObservation.ProofClass = ""
			for _, check := range request.ValidationCheckAttestations {
				output.ValidationObservation.IncompleteChecks = append(output.ValidationObservation.IncompleteChecks, check.NativeCheckRefs()...)
			}
		}
	} else {
		output.EvidenceQuality = string(common.EvidenceQualityValid)
	}
	output.DurationMillis = time.Since(startedAt).Milliseconds()
	result, err := finalizeShellResult(output, executeErr)
	return result, fact, err
}
