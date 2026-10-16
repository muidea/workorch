package shell

import (
	"context"
	"fmt"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 私有执行封套不提供 dispatch/replay 准入；公开 owner 必须先核验原 seal 与 receipt。
// 原文引用仅绑定本次原 execution，不授予读取权限或另造恢复账本。
type shellProjectStageRawReferences struct {
	StageID string
	Stdout  string
	Stderr  string
	// 同一总期限内未发布的流显式保留原因，不以空引用假称完整诊断。
	OmittedReason string
}

type shellProjectExecutionResult struct {
	Result      RuntimeCommandResult
	Conjunction *runtimecontract.RuntimeValidationExecutionConjunction
	Raw         []shellProjectStageRawReferences
	StartedAt   time.Time
	FinishedAt  time.Time
	Deadline    time.Time
}

func (a *RuntimeAdapter) executeShellProjectConjunction(ctx context.Context, request RuntimeCommandRequest, execution runtimecontract.RuntimeOperationExecutionContext) (result shellProjectExecutionResult, executeErr error) {
	result.StartedAt = time.Now().UTC()
	if a == nil || ctx == nil || execution.Execution.Validate() != nil || runtimecontract.ValidateRuntimeOperationExecutionContextScope(execution) != nil ||
		request.WorkspaceID != execution.WorkspaceID || request.RunID != execution.RunID ||
		request.WorkspaceDeliverableDirectory != execution.WorkspaceDeliverableDirectory || request.WorkspaceInternalDirectory != execution.WorkspaceInternalDirectory ||
		execution.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectEphemeralSession || request.StateChanging || !shellInputHasManagedValidation(request) ||
		request.ValidationScratchDirectory != "" || !shellRequiresEntrypointBehavior(request) {
		return result, common.ErrInvalidArgument
	}
	return a.executeShellProjectScope(ctx, request, shellProjectRawScope{WorkspaceID: execution.WorkspaceID, RunID: execution.RunID, ExecutionKey: execution.Execution.ExecutionKey})
}

// scope 来自已核验的 execute seal 或现有 reconciliation identity，不提供 caller 路径/引用入口。
func (a *RuntimeAdapter) executeShellProjectScope(ctx context.Context, request RuntimeCommandRequest, scope shellProjectRawScope) (result shellProjectExecutionResult, executeErr error) {
	result.StartedAt = time.Now().UTC()
	probe := scope
	probe.StageID, probe.Stream = "scope-check", "stdout"
	if a == nil || ctx == nil || probe.Validate() != nil || request.WorkspaceID != scope.WorkspaceID || request.RunID != scope.RunID ||
		request.StateChanging || !shellInputHasManagedValidation(request) || request.ValidationScratchDirectory != "" || !shellRequiresEntrypointBehavior(request) {
		return result, common.ErrInvalidArgument
	}
	timeout, err := shellCommandTimeout(request.TimeoutMillis, a.timeout)
	if err != nil || timeout > DefaultCommandTimeout {
		return result, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// 准备、四阶段、行为转换、原文发布及 cleanup 使用同一最早 deadline，绝不重置。
	runCtx, cancel := context.WithDeadline(ctx, result.StartedAt.Add(timeout))
	defer cancel()
	result.Deadline, _ = runCtx.Deadline()
	request.TimeoutMillis = timeout.Milliseconds()
	request.validationCopiedInputs = nil
	defer func() {
		output := result.Result
		if output.Command == "" {
			output = shellBaseOutput(request, request.Command, request.Args, request.WorkspaceDeliverableDirectory, "", "go")
		}
		attachManagedValidationResources(&output, request, 0, result.StartedAt, time.Now().UTC(), "released")
		var cleanupErr error
		for _, resource := range output.ValidationResources {
			if resource.Status != "released" {
				cleanupErr = fmt.Errorf("复合验证 scratch 未释放：%s", resource.Status)
				break
			}
		}
		result.Result = output
		result.FinishedAt = time.Now().UTC()
		contextErr := runCtx.Err()
		if contextErr == nil && result.FinishedAt.After(result.Deadline) {
			contextErr = context.DeadlineExceeded
		}
		executeErr = finishShellProjectExecution(&result, executeErr, cleanupErr, contextErr)
	}()
	store, err := openShellProjectRawStore(request.WorkspaceInternalDirectory)
	if err != nil {
		return result, err
	}
	defer func() {
		if err := store.Close(); err != nil && executeErr == nil {
			executeErr = err
		}
	}()
	directory, err := prepareManagedValidationSandbox(&request, request.WorkspaceDeliverableDirectory)
	if err != nil {
		return result, err
	}
	if err := runCtx.Err(); err != nil {
		return result, err
	}
	env := managedValidationEnvironment(a.filteredEnv(), request.ValidationScratchDirectory)
	output, fact, err := runShellProjectConjunction(runCtx, directory, env, request)
	result.Result, executeErr = output, err
	if len(fact.Stages) == 0 {
		return result, executeErr
	}
	// 规格取自 runner 在启动进程前编译的精确合同，不从结果 argv 反推声明。
	conjunction, err := shellProjectConjunctionObservation(fact, output.projectStageSpecs, output.projectDiagnosticStageSpecs...)
	if err != nil {
		return result, err
	}
	result.Conjunction = &conjunction
	if err := persistShellProjectStageRaw(runCtx, store, scope, fact, &result); err != nil && executeErr == nil {
		executeErr = err
	}
	return result, executeErr
}

// Shared by the execution envelope and its real-store failure regression. It
// publishes original bytes only; it does not judge evidence or grant repair.
func persistShellProjectStageRaw(ctx context.Context, store *shellProjectRawStore, scope shellProjectRawScope, fact shellProjectConjunctionFact, result *shellProjectExecutionResult) error {
	for _, stage := range fact.Stages {
		refs := shellProjectStageRawReferences{StageID: stage.ID}
		for _, stream := range []string{"stdout", "stderr"} {
			raw := stage.Stdout
			if stream == "stderr" {
				raw = stage.Stderr
			}
			streamScope := scope
			streamScope.StageID, streamScope.Stream = stage.ID, stream
			ref, err := store.Store(ctx, streamScope, raw)
			if err != nil {
				refs.OmittedReason = "shell_stage_raw_storage_failed"
				if ctx.Err() != nil {
					refs.OmittedReason = "shell_stage_raw_operation_context_expired"
				}
				result.Raw = append(result.Raw, refs)
				for _, pending := range fact.Stages[len(result.Raw):] {
					result.Raw = append(result.Raw, shellProjectStageRawReferences{StageID: pending.ID, OmittedReason: refs.OmittedReason})
				}
				return err
			}
			if stream == "stdout" {
				refs.Stdout = ref
			} else {
				refs.Stderr = ref
			}
		}
		result.Raw = append(result.Raw, refs)
	}
	return nil
}

func shellProjectOutputStorage(ref, omittedReason string) *runtimecontract.RuntimeValidationOutputStorage {
	if ref != "" {
		return &runtimecontract.RuntimeValidationOutputStorage{Reference: ref}
	}
	if omittedReason == "" {
		omittedReason = "shell_stage_raw_storage_unavailable"
	}
	return &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: omittedReason}
}

func finishShellProjectExecution(result *shellProjectExecutionResult, executeErr, cleanupErr, contextErr error) error {
	if cleanupErr != nil || contextErr != nil {
		// A reproduced failure is not usable repair attribution if the original
		// operation failed its cleanup/deadline boundary.
		result.Result.ValidationFailureDiagnostic = nil
		result.Result.RuntimeDiagnostics.ValidationFailureDiagnostic = nil
		result.Result.workspaceBehaviorTargetRefs = nil
	}
	if result.Conjunction != nil {
		for index := range result.Conjunction.Stages {
			stage := &result.Conjunction.Stages[index]
			refs := shellProjectStageRawReferences{OmittedReason: "shell_stage_raw_storage_unavailable"}
			if index < len(result.Raw) && result.Raw[index].StageID == stage.Identity.ID {
				refs = result.Raw[index]
			}
			stage.Stdout.Storage = shellProjectOutputStorage(refs.Stdout, refs.OmittedReason)
			stage.Stderr.Storage = shellProjectOutputStorage(refs.Stderr, refs.OmittedReason)
			if refs.Stdout == "" || refs.Stderr == "" || refs.OmittedReason != "" {
				result.Result.ValidationFailureDiagnostic = nil
				result.Result.RuntimeDiagnostics.ValidationFailureDiagnostic = nil
				result.Result.workspaceBehaviorTargetRefs = nil
			}
		}
	}
	if executeErr == nil {
		executeErr = cleanupErr
	}
	if executeErr == nil {
		executeErr = contextErr
	}
	if executeErr == nil && (result.Conjunction == nil || result.Conjunction.Validate() != nil || !result.Conjunction.Complete || len(result.Raw) != len(result.Conjunction.Stages)) {
		executeErr = fmt.Errorf("复合验证缺少完整合取或原文持久引用")
	}
	if executeErr == nil {
		for index, refs := range result.Raw {
			if refs.StageID != result.Conjunction.Stages[index].Identity.ID || refs.Stdout == "" || refs.Stderr == "" || refs.OmittedReason != "" {
				executeErr = fmt.Errorf("复合验证原文引用不完整或阶段漂移")
				break
			}
		}
	}
	if executeErr != nil {
		if result.Conjunction != nil {
			result.Conjunction.Complete = false
		}
		result.Result.EvidenceQuality = string(common.EvidenceQualityInvalid)
		if result.Result.ValidationObservation != nil {
			result.Result.ValidationObservation.ProofClass = ""
		}
		if result.Result.FailureCode == "" {
			result.Result.FailureCode = "validation_project_execution_incomplete"
		}
		result.Result.Status = "failed"
		if contextErr == context.DeadlineExceeded {
			result.Result.Status = "timeout"
		}
	} else {
		result.Result.Status = "completed"
	}
	if result.Conjunction != nil && result.Result.ValidationObservation != nil && shellProjectObservationProtocol(result.Result.ValidationObservation.ObservationProtocolRef) {
		conjunction := result.Conjunction.Clone()
		result.Result.ValidationObservation.ExecutionConjunction = &conjunction
	}
	result.Result.DurationMillis = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	result.Result, executeErr = finalizeShellResult(result.Result, executeErr)
	return executeErr
}
