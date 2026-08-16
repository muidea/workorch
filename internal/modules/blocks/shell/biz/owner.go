package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	runtimeAdapter *RuntimeAdapter
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
	m.SubscribeFunc(shellevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(shellevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(shellevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(shellevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility)
	m.SubscribeFunc(shellevents.TopicQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery)
	m.SubscribeFunc(shellevents.TopicExecuteOperation, m.handleShellOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(shellevents.TopicExecuteOperation)
	m.UnsubscribeFunc(shellevents.TopicAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(shellevents.TopicAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(shellevents.TopicProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(shellevents.TopicProbeRuntimeSemanticProofFeasibility)
	m.UnsubscribeFunc(shellevents.TopicQueryRuntimeCWIReconciliation)
	m.runtimeAdapter = nil
}

// handleRuntimeCWIReconciliationQuery executes only the recipes sealed for
// this binding.  It deliberately does not persist an operation receipt or
// mutate Runtime coverage: the result is an owner-local current-workspace
// observation consumed by the Orchestrator's SM-2 reducer.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(shellevents.QueryRuntimeCWIReconciliationCommand)
	if !ok || m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
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
			TargetRefs:        append([]string(nil), command.Binding.TargetRefs...),
			ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		}
		// Reconciliation uses the same owner-local recipe resolution boundary as
		// ordinary managed-validation dispatch. The provider supplies only the
		// sealed recipe identity; Shell must materialize the command/args before
		// invoking its adapter.
		if resolveErr := resolveManagedValidationRecipe(&request); resolveErr != nil {
			allPassed = false
			continue
		}
		output, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), request)
		if executeErr != nil || output.Status != "completed" || output.ExitCode != 0 || output.ZeroWork || output.ValidationObservation == nil {
			allPassed = false
		}
		if output.ValidationObservation != nil {
			value := output.ValidationObservation.Normalized()
			if value.RecipeID != recipe.ID {
				allPassed = false
			}
			checkRefs = append(checkRefs, value.PassedChecks...)
			targetRefs = append(targetRefs, output.TargetRefs...)
		}
	}
	// Required checks/metrics/artifacts are binding-specific. A successful
	// process alone is not proof for any CWI; each declared check must be
	// explicitly reported by the owner, and a metric/artifact constraint that
	// this owner cannot observe remains unverifiable.
	if len(command.Binding.RequiredCheckRefs) != 0 {
		passed := make(map[string]struct{}, len(checkRefs))
		for _, check := range checkRefs {
			passed[check] = struct{}{}
		}
		for _, required := range command.Binding.RequiredCheckRefs {
			if _, ok := passed[required]; !ok {
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
	command, ok := event.Data().(shellevents.ExecuteOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
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
	runtimeRequest := RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID:                        common.StepID(command.DispatchID + "/" + command.OperationID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		ValidationRecipeID:            command.Data.ValidationRecipeID,
		Command:                       command.Data.Command, Args: command.Data.Args, TimeoutMillis: command.Data.TimeoutMillis,
		StateChanging: command.Data.StateChanging, TargetRef: command.Data.TargetRef, TargetRefs: command.Data.TargetRefs,
		VerificationScope: command.Data.VerificationScope,
		BoundedReason:     command.Data.BoundedReason, ValidationScratchDirectory: command.Data.ValidationScratchDirectory,
		ValidationScratchAutoCleanup:    command.Data.ValidationScratchAutoCleanup,
		ValidationScratchAutoCleanupSet: command.Data.ValidationScratchAutoCleanupSet,
		ValidationEvidence:              command.Data.ValidationEvidence, ManagedValidation: command.Data.ManagedValidation,
		EvidencePolicies:  command.EvidencePolicies,
		ValidationRecipes: command.ValidationRecipes,
	}
	operationResult := RuntimeCommandResult{}
	var executeErr error
	if executeErr = resolveManagedValidationRecipe(&runtimeRequest); executeErr != nil {
		operationResult = validationRecipeUnavailableResult(runtimeRequest, executeErr)
	} else {
		operationResult, executeErr = m.runtimeAdapter.ExecuteOperation(event.Context(), runtimeRequest)
	}
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		observation := shellFailureTargetObservation(operationResult, command.TargetRole, m.runtimeAdapter.runtimeAdmissibilityPolicyRevision())
		targetRefs := []string(nil)
		if observation != nil {
			targetRefs = append([]string(nil), observation.TargetRefs...)
		}
		// Only the closed stdout record above can create a related target. The
		// command target, validation entry, stderr and LLM summary stay receipt
		// lineage/diagnostics and cannot become repair authority.
		failure = orchestratorevents.RuntimeOperationFailureWithTargetObservation(command.Transaction, runtimecontract.StepKindShellCommand, operationResult.FailureCode, operationResult.FailureMessage, targetRefs, observation)
		if operationResult.ValidationFailureDiagnostic != nil {
			diagnostic := operationResult.ValidationFailureDiagnostic.Clone()
			failure.ValidationDiagnostic = &diagnostic
			if observation := shellValidationFailureObservation(command, diagnostic); observation != nil {
				observation.FailureSignature = failure.Signature
				failure.ValidationFailureObservation = observation
			}
		}
	} else if undeclared := shellUndeclaredWorkspaceTargets(operationResult.WorkspaceObservedTargetChanges, command.Data.TargetRefs); len(undeclared) > 0 {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassEffectMismatch, Code: "undeclared_workspace_effect",
			Signature:  "undeclared-workspace-effect/" + command.OperationID,
			Message:    "state-changing shell command changed workspace target(s) outside sealed TargetRefs",
			TargetRefs: undeclared,
		}
	}
	observation := shellMutationReceiptObservation(command.Data.StateChanging, status, operationResult)
	receiptTargets := operationResult.TargetRefs
	if status == runtimecontract.RuntimeOperationExecutionStatusAccepted && command.Data.StateChanging {
		receiptTargets = shellObservedChangedWorkspaceTargets(operationResult.WorkspaceObservedTargetChanges)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, receiptTargets, evidence, failure, nil, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(shellevents.ExecuteOperationResult{Receipt: committed}, nil)
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
	return nil
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
func shellValidationFailureObservation(command shellevents.ExecuteOperationCommand, diagnostic runtimecontract.ValidationFailureDiagnostic) *runtimecontract.RuntimeValidationFailureObservation {
	diagnostic = diagnostic.Normalized()
	if !shellValidationDiagnosticMayIdentifyImplementationEffect(diagnostic.Class) || command.WorkspaceID == "" || command.RunID == "" || command.Transaction.AuthoritySnapshotID == "" || strings.TrimSpace(command.DispatchID) == "" || strings.TrimSpace(command.OperationID) == "" || strings.TrimSpace(command.TargetRole) == "" || strings.TrimSpace(diagnostic.Class) == "" || len(diagnostic.TargetRefs) == 0 || len(command.TargetRefs) == 0 {
		return nil
	}
	// The deterministic bounded digest permits File/Orchestrator causation
	// checks without passing raw stdout/stderr across owner boundaries.
	hash := sha256.Sum256([]byte(strings.Join([]string{diagnostic.Class, diagnostic.Stage, diagnostic.Operation, strings.Join(diagnostic.TargetRefs, ","), strings.Join(command.TargetRefs, ",")}, "\x00")))
	value := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.Transaction.AuthoritySnapshotID,
		DispatchID: command.DispatchID, OperationID: command.OperationID, TargetRole: command.TargetRole,
		DiagnosticClass: diagnostic.Class, CandidateTargetRefs: diagnostic.TargetRefs, ValidationScopeTargetRefs: command.TargetRefs,
		DiagnosticHash: hex.EncodeToString(hash[:]),
	}.Normalized()
	// The failure signature is added by RuntimeOperationFailure below; do not
	// validate this transient value before the caller binds that causation key.
	return &value
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
func shellMutationReceiptObservation(stateChanging bool, status runtimecontract.RuntimeOperationExecutionStatus, result RuntimeCommandResult) orchestratorevents.RuntimeOperationReceiptObservation {
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
	if !stateChanging {
		return observation
	}
	observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
	if status != runtimecontract.RuntimeOperationExecutionStatusAccepted {
		return observation
	}
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
		observation.NoOp = true
		return observation
	}
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
	command, ok := event.Data().(shellevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
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
	command, ok := event.Data().(shellevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok || m.runtimeAdapter == nil {
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
		confirmed, preflightErr := m.runtimeAdapter.PreflightRuntimeValidationRecipesForManifest(
			command.WorkspaceDeliverableDirectory, command.ValidationRecipes, command.WorkspaceManifestBinding.Manifest,
		)
		if preflightErr != nil {
			setCapabilityError(result, preflightErr)
			return
		}
		inventory.ValidationRecipes = confirmed
		inventory.ValidationSurfaces = shellReadyValidationSurfaces(confirmed, command.WorkspaceManifestBinding)
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

func shellReadyValidationSurfaces(recipes []runtimecontract.RuntimeValidationRecipe, binding runtimecontract.RuntimeWorkspaceManifestBinding) []runtimecontract.RuntimeValidationSurface {
	binding = binding.Normalized()
	ret := make([]runtimecontract.RuntimeValidationSurface, 0, len(recipes))
	for _, recipe := range runtimecontract.NormalizeRuntimeValidationRecipes(recipes) {
		digest := sha256.New()
		_, _ = fmt.Fprintf(digest, "%s\x00%s\x00%d\x00", recipe.ID, recipe.Command, recipe.TimeoutMillis)
		for _, arg := range recipe.Args {
			_, _ = fmt.Fprintf(digest, "%s\x00", arg)
		}
		schemaHash := "shell-validation-recipe-" + hex.EncodeToString(digest.Sum(nil))[:20]
		inputBindingRef := "shell-validation-input/" + schemaHash
		if binding.Manifest.Revision != "" {
			inputBindingRef += "/" + binding.Manifest.Revision
		}
		ret = append(ret, runtimecontract.RuntimeValidationSurface{
			ID: recipe.ID, OwnerModuleID: ModuleID, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: schemaHash, InputBindingRef: inputBindingRef,
			CheckRefs: []string{recipe.ID}, EffectScope: runtimecontract.RuntimeEffectNone,
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		})
	}
	return runtimecontract.NormalizeRuntimeValidationSurfaces(ret)
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
	command, ok := event.Data().(shellevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
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
	command, ok := event.Data().(shellevents.ProbeRuntimeSemanticProofFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
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

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}
