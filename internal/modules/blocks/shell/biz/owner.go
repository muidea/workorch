package shell

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Biz struct {
	basebiz.Base
	lifecycle        basebiz.Lifecycle
	runtimeAdapter   *RuntimeAdapter
	started          bool
	subscribedTopics []string
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
	// RuntimeAdapter is an explicit owner-local dependency used by focused
	// runtimes and tests. Production leaves it nil so configuration remains the
	// sole construction source.
	RuntimeAdapter *RuntimeAdapter
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps ...Dependencies) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	m := &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine)}
	if len(deps) > 0 && deps[0].RuntimeAdapter != nil {
		m.runtimeAdapter = deps[0].RuntimeAdapter
		return m, nil
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return nil, err
	}
	adapter, err := NewRuntimeAdapterFromTimeoutString(cfg.Runtime.ShellCommandTimeout)
	if err != nil {
		return nil, err
	}
	m.runtimeAdapter = adapter
	return m, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	leaveStart, err := m.lifecycle.Enter()
	if err != nil {
		return err
	}
	defer leaveStart()
	if m.started {
		return nil
	}
	subscriptions := []struct {
		topic   string
		handler magicevent.ObserverFunc
	}{
		{shellevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities},
		{shellevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility},
		{shellevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility},
		{shellevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility},
		{shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction, m.handleRuntimeOwnerFactProspectiveSatisfaction},
		{shellevents.TopicQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery},
		{shellevents.TopicQueryRuntimeValidationRawOutput, m.handleRuntimeValidationRawOutput},
		{shellevents.TopicQueryRuntimeValidationReadDependencies, m.handleRuntimeValidationReadDependencies},
		{shellevents.TopicExecuteOperation, m.handleShellOperation},
	}
	// A failed Start keeps every completed subscription reachable. Retrying
	// resumes at the first unfinished topic rather than registering duplicates.
	for _, subscription := range subscriptions[len(m.subscribedTopics):] {
		if err := m.SubscribeFuncChecked(subscription.topic, m.lifecycleHandler(subscription.handler, subscription.topic == shellevents.TopicExecuteOperation)); err != nil {
			return err
		}
		m.subscribedTopics = append(m.subscribedTopics, subscription.topic)
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 最终释放仍由应用共享 Hub/任务屏障后的调用方授权；本地等待只是防御。
	if err := m.BeginShutdown(ctx); err != nil {
		return err
	}
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	// Lifecycle calls are serialized by their caller. Do not hold an owner
	// lock across Hub control actions. Unsubscribe completion is a resource
	// receipt, not proof of handler quiescence; the caller must drain first.
	for len(m.subscribedTopics) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.subscribedTopics) - 1
		if err := m.UnsubscribeFuncChecked(m.subscribedTopics[last]); err != nil {
			return err
		}
		m.subscribedTopics = m.subscribedTopics[:last]
		m.started = false
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.started = false
	m.runtimeAdapter = nil
	return ctx.Err()
}

// handleRuntimeCWIReconciliationQuery executes only the recipes sealed for
// this binding.  It deliberately does not persist an operation receipt or
// mutate Runtime coverage: the result is an owner-local current-workspace
// observation consumed by the Orchestrator's SM-2 reducer.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.QueryRuntimeCWIReconciliationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if !runtimeCWIReconciliationBindingHasShellProof(command.Binding) {
		observation := shellReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, nil, "binding has no Shell validation recipe", "validation_recipe_unavailable")
		result.Set(shellevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
		return
	}
	recipes := runtimeCWIReconciliationRecipes(command)
	if len(recipes) != len(command.Binding.ValidationRecipeIDs) {
		observation := shellReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, nil, "sealed Shell validation recipe is unavailable", "validation_recipe_unavailable")
		result.Set(shellevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
		return
	}
	var observedRevision = command.WorkspaceBaselineRevision
	allPassed := true
	var failureCode, failureMessage string
	var checkRefs []string
	var targetRefs []string
	for _, recipe := range recipes {
		request := RuntimeCommandRequest{
			WorkspaceID:                   command.WorkspaceID,
			RunID:                         command.RunID,
			StepID:                        common.StepID("reconciliation/" + command.RequestID + "/" + command.Binding.BindingID + "/" + recipe.ID),
			WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
			WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
			ValidationRecipeID:            recipe.ID,
			ManagedValidation: runtimecontract.RuntimeManagedValidation{
				RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
				OriginalNodeKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			},
			TargetRefs:                  append([]string(nil), command.Binding.TargetRefs...),
			ValidationScopeTargetRefs:   append([]string(nil), command.ValidationScopeTargetRefs...),
			ValidationRecipes:           []runtimecontract.RuntimeValidationRecipe{recipe},
			ValidationCheckAttestations: runtimecontract.CloneRuntimeValidationCheckAttestationBindings(command.Binding.ValidationCheckAttestations),
		}
		// Reconciliation uses the same owner-local recipe resolution boundary as
		// ordinary managed-validation dispatch. The provider supplies only the
		// sealed recipe identity; Shell must materialize the command/args before
		// invoking its adapter.
		if resolveErr := resolveManagedValidationRecipe(&request); resolveErr != nil {
			allPassed = false
			if failureCode == "" {
				failureCode, failureMessage = "validation_recipe_resolution_failed", "sealed validation recipe could not be prepared"
			}
			continue
		}
		output := RuntimeCommandResult{}
		var executeErr error
		if selected, selectErr := shellProjectRequestSelected(request); selectErr != nil {
			output, executeErr = validationRecipeUnavailableResult(request, selectErr), selectErr
		} else if selected {
			produced, err := m.runtimeAdapter.executeShellProjectScope(event.Context(), request, shellProjectReconciliationRawScope(command, recipe.ID))
			output, executeErr = produced.Result, err
		} else {
			output, executeErr = m.runtimeAdapter.ExecuteOperation(event.Context(), request)
		}
		if executeErr != nil || output.Status != "completed" || output.ExitCode != 0 || output.ZeroWork || output.ValidationObservation == nil || output.Evidence.Quality != common.EvidenceQualityValid {
			allPassed = false
			if failureCode == "" && output.FailureCode != "" {
				failureCode, failureMessage = output.FailureCode, output.FailureMessage
			}
		}
		if output.ValidationObservation != nil {
			value := output.ValidationObservation.Normalized()
			if value.RecipeID != recipe.ID {
				allPassed = false
			}
			if len(runtimecontract.RuntimeValidationConflictingChecks(value)) != 0 {
				allPassed = false
			}
			for _, attestation := range command.Binding.ValidationCheckAttestations {
				attestation = attestation.Normalized()
				if runtimecontract.RuntimeValidationAttestationPassed(value, attestation) {
					checkRefs = append(checkRefs, attestation.RequiredCheckRef)
				}
			}
			targetRefs = append(targetRefs, output.TargetRefs...)
		}
	}
	// Required checks/metrics/artifacts are binding-specific. A successful
	// process alone is not proof for any CWI; each declared check must be
	// explicitly reported by the owner, and a metric/artifact constraint that
	// this owner cannot observe remains unverifiable.
	if len(command.Binding.RequiredCheckRefs) != 0 {
		for _, required := range command.Binding.RequiredCheckRefs {
			if !shellStringIncluded(checkRefs, required) {
				allPassed = false
			}
		}
	} else {
		// A successful generic recipe/exit code is intentionally insufficient
		// for an heterogeneous CWI.  Every Shell-backed binding must declare at
		// least one owner-verifiable check so a shared `go test ./...` (or an
		// empty managed validation session) cannot fan out credit to unrelated
		// artifacts, tools, configuration, or documentation work items.
		allPassed = false
	}
	if len(command.Binding.MetricConstraints) != 0 || len(command.Binding.ArtifactRefs) != 0 {
		allPassed = false
	}
	status := runtimecontract.RuntimeCWIReconciliationObservationSatisfied
	message, code := "", ""
	if !allPassed {
		status = runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
		message, code = "Shell recipe did not produce all binding-specific validation facts", "validation_proof_incomplete"
		if failureCode != "" {
			message, code = failureMessage, failureCode
		}
		if len(command.Binding.RequiredCheckRefs) == 0 {
			message, code = "Shell binding declares no CWI-specific required checks", "validation_check_declaration_unavailable"
		}
	}
	if len(targetRefs) == 0 {
		targetRefs = append([]string(nil), command.Binding.TargetRefs...)
	}
	observation := shellReconciliationObservation(command, status, observedRevision, targetRefs, checkRefs, message, code)
	result.Set(shellevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
}

func runtimeCWIReconciliationBindingHasShellProof(binding runtimecontract.RuntimeCWIReconciliationBindingRef) bool {
	for _, kind := range binding.AllowedOperationKinds {
		if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
			return true
		}
	}
	return len(binding.ValidationRecipeIDs) != 0
}

func runtimeCWIReconciliationRecipes(command shellevents.QueryRuntimeCWIReconciliationCommand) []runtimecontract.RuntimeValidationRecipe {
	ret := make([]runtimecontract.RuntimeValidationRecipe, 0, len(command.Binding.ValidationRecipeIDs))
	for _, id := range command.Binding.ValidationRecipeIDs {
		recipe, found := runtimecontract.RuntimeValidationRecipeForID(command.ValidationRecipes, id)
		if !found {
			continue
		}
		ret = append(ret, recipe)
	}
	return runtimecontract.NormalizeRuntimeValidationRecipes(ret)
}

func shellReconciliationObservation(command shellevents.QueryRuntimeCWIReconciliationCommand, status runtimecontract.RuntimeCWIReconciliationObservationStatus, revision string, targetRefs, checkRefs []string, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
	if strings.TrimSpace(revision) == "" {
		revision = command.WorkspaceBaselineRevision
	}
	value := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: command.RequestID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Generation: command.Generation, CoverageWorkItemID: command.Binding.CoverageWorkItemID,
		BindingID: command.Binding.BindingID, BindingHash: command.Binding.BindingHash,
		OwnerModuleID: ModuleID, ObservedRevision: revision, Status: status,
		TargetRefs: append([]string(nil), targetRefs...), CheckRefs: append([]string(nil), checkRefs...),
	}
	if status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		value.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ModuleID,
			OriginalClass: "reconciliation", OriginalCode: code, OriginalMessage: message,
			Signature: "shell-reconciliation/" + code, TargetRefs: append([]string(nil), targetRefs...), ObservedRevision: revision,
		}
	}
	return value.Normalized()
}

func (m *Biz) handleShellOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.ExecuteOperationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := command.Execution.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation, authorization, err := command.SealedOperation.ValidateForExecution(command.RuntimeOperationExecutionContext)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	data, err := shellevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := validateRuntimeShellOperationData(command.RuntimeOperationExecutionContext, data, authorization); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		result.Set(shellevents.ExecuteOperationResult{Receipt: receipt}, nil)
		return
	}
	// 未命中的回执查询不能借 drain 准入产生新 effect 或新 receipt。
	leaveFresh, admissionErr := m.lifecycle.Enter()
	if admissionErr != nil {
		setCapabilityError(result, admissionErr)
		return
	}
	defer leaveFresh()

	if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession && (command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision)) {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	currentRevision := m.runtimeAdapter.runtimeAdmissibilityPolicyRevision()
	if currentRevision != authorization.OwnerRevision {
		message := "Shell owner policy changed after operation seal"
		failure := &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "operation_admissibility_stale",
			Signature: "shell-owner-catalog-stale:" + authorization.CatalogObservation.ObservationID, Message: message,
			TargetRefs: append([]string(nil), command.TargetRefs...),
		}
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs,
			runtimecontract.RuntimeOperationEvidence{Summary: message}, failure, nil,
			orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone})
		receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: authorization.PrerequisiteID, ObservationID: authorization.CatalogObservation.ObservationID,
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ModuleID, SubjectRef: "shell_runtime_policy",
			ExpectedRevision: authorization.OwnerRevision, ObservedRevision: currentRevision, Reason: message,
		}
		committed, commitErr := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
		if commitErr != nil {
			setCapabilityError(result, commitErr)
			return
		}
		result.Set(shellevents.ExecuteOperationResult{Receipt: committed}, nil)
		return
	}
	runtimeRequest := RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID:                        common.StepID(command.DispatchID + "/" + command.OperationID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		ValidationRecipeID:            data.ValidationRecipeID,
		Command:                       data.Command, Args: data.Args, TimeoutMillis: data.TimeoutMillis,
		StateChanging: data.StateChanging, TargetRef: data.TargetRef, TargetRefs: data.TargetRefs,
		ValidationScopeTargetRefs: authorization.ValidationScopeTargetRefs,
		VerificationScope:         data.VerificationScope,
		BoundedReason:             data.BoundedReason, ValidationScratchDirectory: data.ValidationScratchDirectory,
		ValidationScratchAutoCleanup:    data.ValidationScratchAutoCleanup,
		ValidationScratchAutoCleanupSet: data.ValidationScratchAutoCleanupSet,
		ValidationEvidence:              data.ValidationEvidence, ManagedValidation: data.ManagedValidation,
		EvidencePolicies:            authorization.EvidencePolicies,
		ValidationRecipes:           authorization.ValidationRecipes,
		ValidationCheckAttestations: runtimecontract.CloneRuntimeValidationCheckAttestationBindings(authorization.ValidationProofBinding.CheckAttestations),
		validationProofBinding:      authorization.ValidationProofBinding.Clone(),
	}
	operationResult := RuntimeCommandResult{}
	var executeErr error
	if executeErr = resolveManagedValidationRecipe(&runtimeRequest); executeErr != nil {
		operationResult = validationRecipeUnavailableResult(runtimeRequest, executeErr)
	} else if selected, selectErr := shellProjectRequestSelected(runtimeRequest); selectErr != nil {
		operationResult, executeErr = validationRecipeUnavailableResult(runtimeRequest, selectErr), selectErr
	} else if selected {
		produced, err := m.runtimeAdapter.executeShellProjectConjunction(event.Context(), runtimeRequest, command.RuntimeOperationExecutionContext)
		operationResult, executeErr = produced.Result, err
	} else {
		operationResult, executeErr = m.runtimeAdapter.ExecuteOperation(event.Context(), runtimeRequest)
	}
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	if operationResult.validationCopiedInputs != nil {
		dependencies, dependencyErr := shellValidationOwnerDependencies(command.Transaction.DefinitionRevision, runtimeRequest, operationResult.validationCopiedInputs)
		if dependencyErr != nil {
			setCapabilityError(result, dependencyErr)
			return
		}
		evidence.OwnerRevisionSet = &dependencies
		if len(dependencies.Entries) != 0 {
			evidence.ApplicabilityDependencies = &runtimecontract.RuntimeEvidenceApplicabilityDependencies{
				ProtocolRef:      runtimecontract.RuntimeEvidenceApplicabilityDependenciesProtocol,
				OwnerRevisionSet: *runtimecontract.CloneRuntimeOwnerRevisionSet(&dependencies),
			}
		}
	}
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		// Managed validation has one failure-surface protocol. Even the closed
		// WORKORCH_FAILURE_TARGET marker is only a bounded diagnostic candidate;
		// it cannot create RuntimeFailureTargetObservation authority directly.
		// The candidate must fit the SM-3 sealed validation scope, then pass the
		// RuntimeValidationFailureObservation + File confirmation path below.
		failure = orchestratorevents.RuntimeOperationFailureWithTargetObservation(command.Transaction, operation.Kind, operationResult.FailureCode, operationResult.FailureMessage, nil, nil)
		if operationResult.ValidationFailureDiagnostic != nil {
			diagnostic := operationResult.ValidationFailureDiagnostic.Clone()
			candidateTargetRefs, candidateValid := shellFailureTargetCandidateUnion(operationResult, authorization.TargetRole, authorization.ValidationScopeTargetRefs)
			// Diagnostic text/class remains factual even when target extraction
			// fails closed.  Candidate refs do not: a malformed, out-of-scope or
			// over-bound marker invalidates the complete marker+tool union and
			// cannot silently fall back to only parser-derived locations.
			diagnostic.TargetRefs = nil
			if candidateValid {
				diagnostic.TargetRefs = candidateTargetRefs
			}
			failure.ValidationDiagnostic = &diagnostic
			if candidateValid {
				if observation := shellValidationFailureObservation(command.RuntimeOperationExecutionContext, authorization, diagnostic); observation != nil {
					observation.FailureSignature = failure.Signature
					failure.ValidationFailureObservation = observation
				}
			}
		}
	} else if undeclared := shellUndeclaredWorkspaceTargets(operationResult.WorkspaceObservedTargetChanges, data.TargetRefs); len(undeclared) > 0 {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassEffectMismatch, Code: "undeclared_workspace_effect",
			Signature: "undeclared-workspace-effect/" + command.OperationID,
			Message:   "state-changing shell command changed workspace target(s) outside sealed TargetRefs",
		}
	}
	observation := shellMutationReceiptObservation(data.StateChanging, shellInputHasManagedValidation(runtimeRequest), status, operationResult)
	receiptTargets := operationResult.TargetRefs
	if status == runtimecontract.RuntimeOperationExecutionStatusAccepted && data.StateChanging {
		receiptTargets = shellObservedChangedWorkspaceTargets(operationResult.WorkspaceObservedTargetChanges)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, receiptTargets, evidence, failure, nil, observation)
	if receipt.Feedback != nil && receipt.Feedback.OriginalDiagnostic != nil {
		stdoutBytes := operationResult.StdoutBytes
		if stdoutBytes == 0 {
			stdoutBytes = int64(len(operationResult.Stdout))
		}
		stderrBytes := operationResult.StderrBytes
		if stderrBytes == 0 {
			stderrBytes = int64(len(operationResult.Stderr))
		}
		if stdoutBytes+stderrBytes > 0 {
			receipt.Feedback.OriginalDiagnostic.DetailsBytes = stdoutBytes + stderrBytes
			receipt.Feedback.OriginalDiagnostic.ContentType = "text/plain"
			receipt.Feedback.OriginalDiagnostic.OmittedReason = "raw_shell_command_output"
			receipt.Feedback.OriginalDiagnostic.Truncated = operationResult.OutputTruncated
			receipt.Feedback.OriginalDiagnostic.Redacted = true
		}
	}
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(shellevents.ExecuteOperationResult{Receipt: committed}, nil)
}

func validateRuntimeShellOperationData(context runtimecontract.RuntimeOperationExecutionContext, data shellevents.OperationData, authorization shellevents.RuntimeExecutionAuthorization) error {
	managed := data.ManagedValidation.RuntimeManaged ||
		strings.TrimSpace(data.ManagedValidation.Kind) == runtimecontract.RuntimeBlockKindManagedValidationSession ||
		strings.TrimSpace(data.ManagedValidation.OriginalNodeKind) == runtimecontract.RuntimeBlockKindManagedValidationSession
	if !sameShellTargetRefs(context.TargetRefs, data.TargetRefs) {
		return fmt.Errorf("runtime Shell operation data target refs do not match sealed execution context")
	}
	if managed {
		if data.StateChanging || strings.TrimSpace(data.ValidationRecipeID) == "" ||
			strings.TrimSpace(data.Command) != "" || len(data.Args) != 0 ||
			strings.TrimSpace(data.ValidationScratchDirectory) != "" || data.ValidationScratchAutoCleanupSet ||
			context.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectEphemeralSession {
			return fmt.Errorf("runtime managed validation requires recipe-only ephemeral execution data")
		}
		if _, found := runtimecontract.RuntimeValidationRecipeForID(authorization.ValidationRecipes, data.ValidationRecipeID); !found {
			return fmt.Errorf("runtime managed validation recipe is absent from sealed owner catalog")
		}
		return nil
	}
	if len(authorization.ValidationScopeTargetRefs) != 0 {
		return fmt.Errorf("runtime shell/command must not carry managed-validation target scope")
	}
	if !data.StateChanging || strings.TrimSpace(data.ValidationRecipeID) != "" ||
		strings.TrimSpace(data.Command) == "" || len(context.TargetRefs) == 0 ||
		strings.TrimSpace(data.ValidationScratchDirectory) != "" || data.ValidationScratchAutoCleanupSet ||
		context.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		return fmt.Errorf("runtime shell/command requires durable stateChanging execution data")
	}
	if strings.TrimSpace(data.TargetRef) != "" && !shellStringIncluded(context.TargetRefs, data.TargetRef) {
		return fmt.Errorf("runtime Shell targetRef is outside sealed target refs")
	}
	return nil
}

func sameShellTargetRefs(left, right []string) bool {
	leftSet := make(map[string]struct{}, len(left))
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range left {
		if value = strings.TrimSpace(value); value != "" {
			leftSet[value] = struct{}{}
		}
	}
	for _, value := range right {
		if value = strings.TrimSpace(value); value != "" {
			rightSet[value] = struct{}{}
		}
	}
	if len(leftSet) != len(rightSet) {
		return false
	}
	for value := range leftSet {
		if _, found := rightSet[value]; !found {
			return false
		}
	}
	return true
}

func shellStringIncluded(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

// resolveManagedValidationRecipe is the Shell-owner boundary between a sealed
// selection ID and an executable command. It overwrites every executable
// command field from a Shell-owner-confirmed admissibility snapshot so no
// provider-provided command/args can
// reach the adapter.
func resolveManagedValidationRecipe(request *RuntimeCommandRequest) error {
	if request == nil || !shellInputHasManagedValidation(*request) {
		return nil
	}
	id := strings.TrimSpace(request.ValidationRecipeID)
	if id == "" {
		return fmt.Errorf("validation recipe is missing")
	}
	recipe, found := runtimecontract.RuntimeValidationRecipeForID(request.ValidationRecipes, id)
	if !found {
		return fmt.Errorf("validation recipe %q is unavailable", id)
	}
	request.Command = recipe.Command
	request.Args = append([]string(nil), recipe.Args...)
	if recipe.TimeoutMillis > 0 {
		request.TimeoutMillis = recipe.TimeoutMillis
	}
	return instrumentManagedValidationNativeChecks(request)
}

func validationRecipeUnavailableResult(request RuntimeCommandRequest, err error) RuntimeCommandResult {
	message := "sealed validation recipe is unavailable"
	if err != nil {
		message = err.Error()
	}
	return RuntimeCommandResult{
		Status: "failed", Stage: "validation_recipe_unavailable", FailureCode: "validation_recipe_unavailable",
		FailureMessage: message, Reason: message, ValidationRecipeID: strings.TrimSpace(request.ValidationRecipeID),
		ManagedValidation: request.ManagedValidation, TargetRefs: normalizeShellStrings(request.TargetRefs),
	}
}

// shellValidationFailureObservation exports only the sealed validation scope
// plus bounded diagnostic candidates.  It is not a repair target and cannot
// carry a Runtime action; the File owner must independently confirm it.
func shellValidationFailureObservation(context runtimecontract.RuntimeOperationExecutionContext, authorization shellevents.RuntimeExecutionAuthorization, diagnostic runtimecontract.ValidationFailureDiagnostic) *runtimecontract.RuntimeValidationFailureObservation {
	diagnostic = diagnostic.Normalized()
	if !shellValidationDiagnosticMayIdentifyImplementationEffect(diagnostic.Class) || context.WorkspaceID == "" || context.RunID == "" || context.Transaction.AuthoritySnapshotID == "" || strings.TrimSpace(context.DispatchID) == "" || strings.TrimSpace(context.OperationID) == "" || strings.TrimSpace(authorization.TargetRole) == "" || strings.TrimSpace(diagnostic.Class) == "" || len(diagnostic.TargetRefs) == 0 || len(diagnostic.TargetRefs) > shellValidationDiagnosticMaxTargetRefs || len(authorization.ValidationScopeTargetRefs) == 0 {
		return nil
	}
	if !shellTargetRefsSubset(diagnostic.TargetRefs, authorization.ValidationScopeTargetRefs) {
		return nil
	}
	// The deterministic bounded digest binds every typed field used by exact
	// repair-route selection. It permits the resource owner and Orchestrator to
	// verify causation without passing raw stdout/stderr across owner boundaries.
	value := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: context.WorkspaceID, RunID: context.RunID, AuthoritySnapshotID: context.Transaction.AuthoritySnapshotID,
		DispatchID: context.DispatchID, OperationID: context.OperationID, TargetRole: authorization.TargetRole,
		DiagnosticClass: diagnostic.Class, CandidateTargetRefs: diagnostic.TargetRefs, ValidationScopeTargetRefs: authorization.ValidationScopeTargetRefs,
		DiagnosticHash: runtimecontract.StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, authorization.ValidationScopeTargetRefs),
	}.Normalized()
	// The failure signature is added by RuntimeOperationFailure below; do not
	// validate this transient value before the caller binds that causation key.
	return &value
}

func shellTargetRefsSubset(values, scope []string) bool {
	allowed := make(map[string]struct{}, len(scope))
	for _, value := range scope {
		if value = strings.TrimSpace(value); value != "" {
			allowed[value] = struct{}{}
		}
	}
	for _, value := range values {
		if _, found := allowed[strings.TrimSpace(value)]; !found {
			return false
		}
	}
	return len(values) != 0
}

func shellValidationDiagnosticMayIdentifyImplementationEffect(class string) bool {
	switch strings.TrimSpace(class) {
	case runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationDiagnosticImplementationGap:
		return true
	default:
		return false
	}
}

// shellMutationReceiptObservation turns only the Shell owner's bounded,
// before/after observations into resource changes. The resource identity uses
// File as the canonical owner of workspace targets/tree: Shell observes the
// mutation, while Orchestrator performs the neutral intersection against File
// and Shell prerequisites. No File service/repository is consulted here.
func shellMutationReceiptObservation(stateChanging, managedValidation bool, status runtimecontract.RuntimeOperationExecutionStatus, result RuntimeCommandResult) orchestratorevents.RuntimeOperationReceiptObservation {
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
	if managedValidation {
		observation.ObservedEffectScope = runtimecontract.RuntimeEffectEphemeralSession
		if status != runtimecontract.RuntimeOperationExecutionStatusAccepted {
			observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		}
		return observation
	}
	if !stateChanging {
		observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		return observation
	}
	observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
	changes := make([]runtimecontract.RuntimeResourceChange, 0, len(result.WorkspaceObservedTargetChanges)+1)
	for _, change := range result.WorkspaceObservedTargetChanges {
		targetRef := strings.TrimSpace(change.TargetRef)
		if targetRef == "" || change.BeforeRevision == change.AfterRevision || strings.TrimSpace(change.AfterRevision) == "" {
			continue
		}
		changes = append(changes, runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: targetRef, BeforeRevision: change.BeforeRevision, AfterRevision: change.AfterRevision,
		})
	}
	if fingerprint := result.WorkspaceStateChangeFingerprint; strings.TrimSpace(fingerprint.Before) != "" &&
		strings.TrimSpace(fingerprint.After) != "" && fingerprint.Before != fingerprint.After {
		changes = append(changes, runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: fingerprint.Before, AfterRevision: fingerprint.After,
		})
	}
	if len(changes) == 0 {
		// A rejected operation with no observed mutation remains an unknown-effect
		// failure. Only an accepted state-changing delivery can claim a no-op.
		observation.NoOp = status == runtimecontract.RuntimeOperationExecutionStatusAccepted
		return observation
	}
	// ResourceChanges are immutable owner audit facts, including changes observed
	// on a rejected command. They never authorize those subjects as repair
	// targets; failure TargetRefs remain empty unless the sealed failure-target
	// protocol independently admits them.
	observation.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
	observation.Changed = true
	observation.ResourceChanges = changes
	return observation
}

func shellObservedChangedWorkspaceTargets(changes []RuntimeWorkspaceTargetChange) []string {
	ret := make([]string, 0, len(changes))
	for _, change := range changes {
		if target := strings.TrimSpace(change.TargetRef); target != "" && change.BeforeRevision != change.AfterRevision {
			ret = append(ret, target)
		}
	}
	return normalizeShellStrings(ret)
}

func shellUndeclaredWorkspaceTargets(changes []RuntimeWorkspaceTargetChange, declared []string) []string {
	allowed := make(map[string]struct{}, len(declared))
	for _, target := range declared {
		if target = strings.TrimSpace(target); target != "" {
			allowed[target] = struct{}{}
		}
	}
	undeclared := make([]string, 0, len(changes))
	for _, target := range shellObservedChangedWorkspaceTargets(changes) {
		if _, found := allowed[target]; !found {
			undeclared = append(undeclared, target)
		}
	}
	return normalizeShellStrings(undeclared)
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.AcquireRuntimeCapabilitiesCommand](event, result)
	if !ok {
		return
	}
	response, err := shellevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.AcquireRuntimeOperationAdmissibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := shellevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := m.runtimeAdapter.AcquireRuntimeOperationAdmissibility(command.WorkspaceDeliverableDirectory, command.PrerequisiteID, command.RequiredKinds)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if len(command.RequiredTargetRefs) > command.MaxItems {
		setCapabilityError(result, fmt.Errorf("runtime Shell admissibility target inventory exceeds bounded item count"))
		return
	}
	if err := validateDeclaredValidationTargetScope(command.WorkspaceDeliverableDirectory, command.RequiredTargetRefs); err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory.TargetRefs = normalizeShellStrings(command.RequiredTargetRefs)
	if runtimeShellManagedValidationRequested(command.RequiredKinds) && len(command.ValidationRecipes) != 0 {
		resolved := resolveRuntimeValidationRecipeDeclarations(command.ValidationRecipes, command.WorkspaceManifestBinding.Manifest)
		staticallyReady, preflightErr := m.runtimeAdapter.PreflightRuntimeValidationRecipes(command.WorkspaceDeliverableDirectory, resolved)
		if preflightErr != nil {
			setCapabilityError(result, preflightErr)
			return
		}
		confirmed := runtimeValidationRecipesApplicableToManifest(staticallyReady, command.WorkspaceManifestBinding.Manifest.TargetRefs)
		inventory.ValidationSurfaces = shellValidationSurfaces(command.ValidationRecipes, staticallyReady, confirmed, command.ValidationProofRequirements, command.WorkspaceManifestBinding, inventory.ObservedRevision, command.WorkspaceDeliverableDirectory)
		selectableRefs := runtimecontract.RuntimeSelectableValidationRecipeRefs(inventory.ValidationSurfaces)
		inventory.ValidationRecipeRefs = selectableRefs
		inventory.ValidationRecipes = shellValidationRecipesForRefs(confirmed, selectableRefs)
	}
	inventory = inventory.Normalized()
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := m.runtimeAdapter.runtimeAdmissibilityObservationWithManifest(
		command.WorkspaceDeliverableDirectory, command.PrerequisiteID, inventory, command.RequiredTargetRefs, command.WorkspaceManifestBinding,
	)
	result.Set(shellevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Admissibility: inventory, Observation: observation}}, nil)
}

func shellValidationRecipesForRefs(recipes []runtimecontract.RuntimeValidationRecipe, refs []string) []runtimecontract.RuntimeValidationRecipe {
	allowed := make(map[string]struct{}, len(refs))
	for _, ref := range normalizeShellStrings(refs) {
		allowed[ref] = struct{}{}
	}
	ret := make([]runtimecontract.RuntimeValidationRecipe, 0, len(allowed))
	for _, recipe := range runtimecontract.NormalizeRuntimeValidationRecipes(recipes) {
		if _, found := allowed[recipe.ID]; found {
			ret = append(ret, recipe)
		}
	}
	return runtimecontract.NormalizeRuntimeValidationRecipes(ret)
}

func runtimeShellManagedValidationRequested(kinds []string) bool {
	for _, kind := range kinds {
		if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
			return true
		}
	}
	return false
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.ProbeRuntimeOperationFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := shellevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	failure, stale := m.runtimeAdapter.ProbeRuntimeOperationFeasibilityWithObservationAndValidationRecipes(command.WorkspaceDeliverableDirectory, command.Admissibility, operation, command.Observation, command.OperationAuthorization.ValidationRecipes)
	category := ownerFeasibilityCategory(failure)
	observation, err := shellevents.NewProbeRuntimeOperationFeasibilityResult(command, category, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.ProbeRuntimeSemanticProofFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := shellevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	current, err := m.runtimeAdapter.AcquireRuntimeOperationAdmissibility(command.WorkspaceDeliverableDirectory, command.OwnerCatalog.PrerequisiteID, []string{command.Proposal.OperationKind})
	if err != nil {
		m.setRuntimeSemanticProofFeasibility(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, command.OwnerCatalog.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "shell_policy_unavailable", err.Error(), command.OwnerCatalog.ObservedRevision, "", nil))
		return
	}
	if current.ObservedRevision != command.OwnerCatalog.ObservedRevision {
		m.setRuntimeSemanticProofFeasibility(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, current.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime Shell semantic catalog is stale", command.OwnerCatalog.ObservedRevision, current.ObservedRevision, nil))
		return
	}
	targetRefs := runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)
	if command.Proposal.ValidationRecipeID != "" {
		recipe, found := runtimecontract.RuntimeValidationRecipeForID(command.OwnerCatalog.ValidationRecipes, command.Proposal.ValidationRecipeID)
		if !found {
			m.setRuntimeSemanticProofFeasibility(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, current.ObservedRevision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "validation_recipe_not_sealed", "runtime semantic validation recipe is absent from the Shell catalog", "", current.ObservedRevision, nil))
			return
		}
		preflightErr := m.runtimeAdapter.PreflightRuntimeSemanticValidationRecipe(command.WorkspaceDeliverableDirectory, recipe, targetRefs)
		if preflightErr != nil {
			message := "runtime semantic validation recipe is not executable"
			if preflightErr != nil {
				message = preflightErr.Error()
			}
			m.setRuntimeSemanticProofFeasibility(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, current.ObservedRevision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "validation_recipe_unavailable", message, "", current.ObservedRevision, nil))
			return
		}
	}
	if err := validateDeclaredValidationTargets(command.WorkspaceDeliverableDirectory, targetRefs); err != nil {
		m.setRuntimeSemanticProofFeasibility(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, current.ObservedRevision, targetRefs,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_target_unavailable", "runtime Shell semantic target is outside the current validation scope", "", current.ObservedRevision, targetRefs))
		return
	}
	m.setRuntimeSemanticProofFeasibility(result, command, runtimecontract.RuntimeSemanticFeasibilityFeasible, current.ObservedRevision, targetRefs, nil)
}

func (m *Biz) setRuntimeSemanticProofFeasibility(result magicevent.Result, command shellevents.ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, revision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	response, err := shellevents.NewRuntimeSemanticProofFeasibilityResult(command, status, revision, targetRefs, diagnostic)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

// shellRuntimeIngressCommand is the owner-local strong-call gate for every
// Runtime topic subscribed by Shell. Post is rejected before DTO inspection;
// only the exact Orchestrator source may reach process, receipt, or workspace
// observation paths.
func shellRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) {
	var zero T
	if result == nil {
		return zero, false
	}
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setShellRuntimeIngressInvalid(result, "runtime Shell ingress requires exact Orchestrator source")
		return zero, false
	}
	command, ok := event.Data().(T)
	if !ok {
		setShellRuntimeIngressInvalid(result, "runtime Shell ingress payload has an invalid concrete type")
		return zero, false
	}
	return command, true
}

func setShellRuntimeIngressInvalid(result magicevent.Result, message string) {
	if result == nil {
		return
	}
	err := fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
	result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

// BeginShutdown 关停新 root，不取消已接纳 handler；其 nested receipt/query
// 仍保留真实调用 context，只有实际返回时才释放在途记账。
func (m *Biz) BeginShutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.lifecycle.BeginShutdown()

	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if err := m.lifecycle.Quiesce(ctx); err != nil {
		return err
	}

	return nil
}

// Execute 的 drain 入口仅执行 sealed command 校验与 exact persisted receipt
// 查询；receipt 未命中必须另取 fresh 票，不能回退扩大停机期间的执行权限。
func (m *Biz) lifecycleHandler(handler magicevent.ObserverFunc, receiptDrain bool) magicevent.ObserverFunc {
	return func(ev magicevent.Event, result magicevent.Result) {
		if result == nil || ev == nil {
			return
		}
		if ev.Source() != runtimecontract.RuntimeModuleOrchestrator {
			setShellRuntimeIngressInvalid(result, "runtime command source must be kernel/orchestrator")
			return
		}
		var leave func()
		var err error
		if receiptDrain {
			leave, err = m.lifecycle.EnterDrain()
		} else {
			leave, err = m.lifecycle.Enter()
		}
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		defer leave()
		handler(ev, result)
	}
}
