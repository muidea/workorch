package shell

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeCWIReconciliationBatch(event magicevent.Event, result magicevent.Result) {
	c, ok := shellRuntimeIngressCommand[shellevents.QueryRuntimeCWIReconciliationBatchCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || c.ValidateForOwner(ModuleID) != nil {
		setCapabilityError(result, fmt.Errorf("invalid Shell reconciliation batch"))
		return
	}
	requests, eligible := shellProjectBatchRequests(c)
	response := shellevents.QueryRuntimeCWIReconciliationBatchResult{}
	if !eligible {
		// Applicability is an optimization decision, never another readiness
		// gate. Each member still executes its own original owner protocol.
		for _, member := range c.Members {
			o, err := m.queryRuntimeCWIReconciliation(event.Context(), shellevents.QueryRuntimeCWIReconciliationCommand(member))
			if err != nil {
				setCapabilityError(result, err)
				return
			}
			response.Observations = append(response.Observations, o)
		}
	} else {
		response.Observations = m.runtimeAdapter.executeShellProjectBatch(event.Context(), c, requests)
	}
	if err := c.ValidateResult(response); err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

// Compile only existing, exact methods and scopes. No model chooses consumer
// membership, changes a recipe or describes dependency closure. Sharing takes
// place inside one actual execution on one copy; it is not cross-time reuse.
func shellProjectBatchRequests(c shellevents.QueryRuntimeCWIReconciliationBatchCommand) ([]RuntimeCommandRequest, bool) {
	requests := make([]RuntimeCommandRequest, 0, len(c.Members))
	seenNative := make(map[string]bool)
	for _, member := range c.Members {
		if len(member.Binding.ValidationRecipeIDs) != 1 || len(member.Binding.RequiredCheckRefs) == 0 ||
			len(member.Binding.MetricConstraints) != 0 || len(member.Binding.ArtifactRefs) != 0 || len(member.ValidationScopeTargetRefs) == 0 || member.ValidationSourceReceiptHash == "" {
			return nil, false
		}
		recipes := runtimeCWIReconciliationRecipes(shellevents.QueryRuntimeCWIReconciliationCommand(member))
		if len(recipes) != 1 {
			return nil, false
		}
		r := RuntimeCommandRequest{WorkspaceID: member.WorkspaceID, RunID: member.RunID,
			StepID:                        common.StepID("reconciliation-batch/" + member.Binding.BindingID),
			WorkspaceDeliverableDirectory: member.WorkspaceDeliverableDirectory, WorkspaceInternalDirectory: member.WorkspaceInternalDirectory,
			ValidationRecipeID: recipes[0].ID, ValidationRecipes: recipes, TargetRefs: slices.Clone(member.Binding.TargetRefs),
			ValidationScopeTargetRefs:   slices.Clone(member.ValidationScopeTargetRefs),
			ValidationCheckAttestations: runtimecontract.CloneRuntimeValidationCheckAttestationBindings(member.Binding.ValidationCheckAttestations),
			ManagedValidation:           runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
		if resolveManagedValidationRecipe(&r) != nil {
			return nil, false
		}
		if selected, err := shellProjectRequestSelected(r); err != nil || !selected {
			return nil, false
		}
		for _, check := range r.ValidationCheckAttestations {
			for _, native := range check.NativeCheckRefs() {
				if seenNative[native] {
					return nil, false
				}
				seenNative[native] = true
			}
		}
		if len(requests) > 0 {
			first := requests[0]
			if r.WorkspaceDeliverableDirectory != first.WorkspaceDeliverableDirectory || r.WorkspaceInternalDirectory != first.WorkspaceInternalDirectory ||
				!reflect.DeepEqual(r.ValidationRecipes, first.ValidationRecipes) {
				return nil, false
			}
		}
		requests = append(requests, r)
	}
	return requests, len(requests) >= 2
}

func (a *RuntimeAdapter) executeShellProjectBatch(ctx context.Context, c shellevents.QueryRuntimeCWIReconciliationBatchCommand, requests []RuntimeCommandRequest) (observations []runtimecontract.RuntimeCWIReconciliationObservation) {
	started := time.Now().UTC()
	var completed []shellProjectExecutionResult
	var batchErr error
	failureCode := "validation_project_execution_incomplete"
	first := requests[0]
	timeout, err := shellCommandTimeout(first.TimeoutMillis, a.timeout)
	if err != nil {
		batchErr = err
	}
	runCtx, cancel := context.WithDeadline(ctx, started.Add(timeout))
	defer cancel()
	bindings := make([]runtimecontract.RuntimeCWIReconciliationBindingRef, 0, len(c.Members))
	for _, member := range c.Members {
		bindings = append(bindings, member.Binding.Clone())
	}
	slices.SortFunc(bindings, func(a, b runtimecontract.RuntimeCWIReconciliationBindingRef) int {
		if a.BindingID < b.BindingID {
			return -1
		}
		if a.BindingID > b.BindingID {
			return 1
		}
		return 0
	})
	refs := make([]string, 0, len(bindings))
	for _, b := range bindings {
		refs = append(refs, b.BindingID)
	}
	group := runtimecontract.RuntimeValidationExecutionGroupID(c.Request.WorkspaceID, c.Request.RunID, c.Request.RequestID, c.Request.Generation, c.Request.BindingSetHash, bindings)
	defer func() {
		cleanup := RuntimeCommandResult{}
		attachManagedValidationResources(&cleanup, first, 0, started, time.Now().UTC(), "released")
		contextErr := runCtx.Err()
		if deadline, ok := runCtx.Deadline(); contextErr == nil && ok && time.Now().After(deadline) {
			contextErr = context.DeadlineExceeded
		}
		if first.ValidationScratchDirectory != "" && !cleanup.ManagedValidation.ResourcesReleased {
			batchErr = errors.Join(batchErr, fmt.Errorf("shared validation cleanup incomplete: %v", cleanup.ValidationResources))
		}
		if contextErr != nil {
			batchErr = errors.Join(batchErr, contextErr)
		}
		// Complete every envelope before publishing any consumer. A later
		// storage/cleanup/behavior failure invalidates the whole fresh group.
		if batchErr == nil && len(completed) != len(c.Members) {
			batchErr = fmt.Errorf("shared validation execution has incomplete consumers")
		}
		for i := range completed {
			completed[i].FinishedAt = time.Now().UTC()
			completed[i].Deadline, _ = runCtx.Deadline()
			if err := finishShellProjectExecution(&completed[i], batchErr, nil, contextErr); err != nil && batchErr == nil {
				batchErr = err
				if completed[i].Result.FailureCode != "" {
					failureCode = completed[i].Result.FailureCode
				}
			}
		}
		for i, member := range c.Members {
			command := shellevents.QueryRuntimeCWIReconciliationCommand(member)
			if batchErr != nil {
				observations = append(observations, shellReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, member.WorkspaceBaselineRevision, member.Binding.TargetRefs, nil, batchErr.Error(), failureCode))
				continue
			}
			r := &completed[i]
			o := shellReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationSatisfied, member.WorkspaceBaselineRevision, r.Result.TargetRefs, member.Binding.RequiredCheckRefs, "", "")
			o.ValidationExecution = &runtimecontract.RuntimeValidationExecutionUse{GroupID: group, SourceReceiptHash: member.ValidationSourceReceiptHash, SourceScopeHash: runtimecontract.RuntimeValidationExecutionScopeHash(member.ValidationScopeTargetRefs, member.ValidationRecipes), ConsumerBindingRefs: slices.Clone(refs), SharedStageCount: 3, Validation: r.Result.ValidationObservation.Clone()}
			observations = append(observations, o)
		}
	}()
	if batchErr != nil {
		return
	}
	if err := runCtx.Err(); err != nil {
		batchErr = err
		return
	}
	directory, err := prepareManagedValidationSandbox(&first, first.WorkspaceDeliverableDirectory)
	if err != nil {
		batchErr = err
		return
	}
	before, err := shellProjectSourceSetHash(directory)
	if err != nil {
		batchErr = err
		return
	}
	// Preflight every consumer before any process: another package's readiness
	// cannot be inferred from the first one.
	for _, r := range requests {
		if inspection := inspectShellNativeValidationChecks(directory, r.ValidationScopeTargetRefs, r.ValidationCheckAttestations); inspection.Code != "" {
			failureCode = inspection.Code
			batchErr = fmt.Errorf("consumer %s: %s; check=%s targets=%v", r.StepID, inspection.Code, inspection.CheckRef, inspection.TargetRefs)
			return
		}
	}
	store, err := openShellProjectRawStore(first.WorkspaceInternalDirectory)
	if err != nil {
		batchErr = err
		return
	}
	defer func() {
		if err := store.Close(); err != nil {
			batchErr = errors.Join(batchErr, err)
		}
	}()
	shared := &shellProjectSharedStages{}
	for i, r := range requests {
		r.ValidationScratchDirectory = first.ValidationScratchDirectory
		r.TimeoutMillis = timeout.Milliseconds()
		output, fact, executeErr := runShellProjectConjunctionShared(runCtx, directory, managedValidationEnvironment(a.filteredEnv(), first.ValidationScratchDirectory), r, shared)
		batchErr = executeErr
		if executeErr != nil && output.FailureCode != "" {
			failureCode = output.FailureCode
		}
		value := shellProjectExecutionResult{Result: output, StartedAt: started}
		conjunction, observationErr := shellProjectConjunctionObservation(fact, output.projectStageSpecs, output.projectDiagnosticStageSpecs...)
		if observationErr != nil {
			batchErr = errors.Join(batchErr, observationErr)
			return
		}
		value.Conjunction = &conjunction
		for index, stage := range fact.Stages {
			scope := shellProjectRawScope{WorkspaceID: c.Request.WorkspaceID, RunID: c.Request.RunID, ExecutionKey: "reconciliation/" + group, StageID: stage.ID}
			if index >= 3 {
				scope.ExecutionKey += "/" + c.Members[i].Binding.BindingID
			}
			raw := shellProjectStageRawReferences{StageID: stage.ID}
			for _, stream := range []string{"stdout", "stderr"} {
				scope.Stream = stream
				body := stage.Stdout
				if stream == "stderr" {
					body = stage.Stderr
				}
				ref, err := store.Store(runCtx, scope, body)
				if err != nil {
					batchErr = errors.Join(batchErr, err)
					return
				}
				if stream == "stdout" {
					raw.Stdout = ref
				} else {
					raw.Stderr = ref
				}
			}
			value.Raw = append(value.Raw, raw)
		}
		completed = append(completed, value)
		if executeErr != nil {
			return
		}
	}
	// Both the exact copy and the original complete member set must still be
	// current. Added files and changed adjacent harnesses invalidate the batch.
	after, sourceErr := shellProjectSourceSetHash(directory)
	live, liveErr := shellProjectSourceSetHash(first.WorkspaceDeliverableDirectory)
	if sourceErr != nil || liveErr != nil || after != before || live != before {
		failureCode = "validation_project_source_changed"
		batchErr = errors.Join(fmt.Errorf("shared validation source/member set changed"), sourceErr, liveErr)
	}
	return
}
