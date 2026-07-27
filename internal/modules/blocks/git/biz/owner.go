package git

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindRead = modulecommon.StepKindRead
const StepKindWrite = modulecommon.StepKindWrite

type Biz struct {
	basebiz.Base
	runtimeAdapter     *RuntimeAdapter
	capabilityReceipts *CapabilityStateReceiptStore
}

type Dependencies struct {
	EventHub                magicevent.Hub
	BackgroundRoutine       task.BackgroundRoutine
	RuntimeAdapter          *RuntimeAdapter
	CapabilityStateReceipts *CapabilityStateReceiptStore
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps ...Dependencies) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	m := &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine)}
	if len(deps) > 0 {
		m.runtimeAdapter = deps[0].RuntimeAdapter
		m.capabilityReceipts = deps[0].CapabilityStateReceipts
	}
	if m.runtimeAdapter != nil && m.capabilityReceipts != nil {
		return m, nil
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return nil, err
	}
	if m.runtimeAdapter == nil {
		adapter, err := NewRuntimeAdapterFromTimeoutString(
			true,
			cfg.Runtime.Git.DefaultTimeout,
		)
		if err != nil {
			return nil, err
		}
		m.runtimeAdapter = adapter
	}
	if m.capabilityReceipts == nil {
		receipts, err := NewCapabilityStateReceiptStore(cfg.Repository.DataDir)
		if err != nil {
			return nil, err
		}
		m.capabilityReceipts = receipts
	}
	return m, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil {
		return common.ErrInvalidArgument
	}
	if m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireGitCapabilityState, m.handleGitCapabilityState)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteGitOperation, m.handleGitOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteGitOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireGitCapabilityState)
	m.runtimeAdapter = nil
	m.capabilityReceipts = nil
}

func (m *Biz) handleGitCapabilityState(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireGitCapabilityStateCommand)
	if !ok || m.runtimeAdapter == nil || m.capabilityReceipts == nil || command.WorkspaceID == "" || command.PrerequisiteID == "" || command.AcquisitionKey == "" || command.WorkspaceDeliverableDirectory == "" || command.CapabilityStateKind != "repository_state" || command.SubjectRef == "" {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	workspaceTreeRevision, err := m.acquireFileWorkspaceTreeRevision(event.Context(), command)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if snapshot, found, err := m.capabilityReceipts.Load(command.WorkspaceID, command.AcquisitionKey); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		if snapshot.WorkspaceTreeRevision != workspaceTreeRevision {
			setCapabilityError(result, fmt.Errorf("%w: git capability acquisition %q has a different File workspace-tree revision", common.ErrConflict, command.AcquisitionKey))
			return
		}
		observation := gitCapabilityStateObservation(command.PrerequisiteID, snapshot)
		result.Set(orchestratorevents.AcquireGitCapabilityStateResult{PrerequisiteID: command.PrerequisiteID, Snapshot: snapshot, Observation: &observation}, nil)
		return
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(event.Context(), command.PrerequisiteID, command.SubjectRef, command.WorkspaceDeliverableDirectory)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	snapshot.AcquisitionKey = command.AcquisitionKey
	// The receipt key is accepted by Git only once it names a durable owner
	// snapshot.  Downstream components compare this opaque binding but never
	// derive repository semantics from it.
	snapshot.AcquisitionBinding = command.AcquisitionKey
	snapshot.WorkspaceTreeRevision = workspaceTreeRevision
	snapshot.QueryReceiptRef = "capability-state-receipt/" + command.AcquisitionKey
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	snapshot, err = m.capabilityReceipts.StoreIfAbsent(command.WorkspaceID, snapshot)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := gitCapabilityStateObservation(command.PrerequisiteID, snapshot)
	result.Set(orchestratorevents.AcquireGitCapabilityStateResult{PrerequisiteID: command.PrerequisiteID, Snapshot: snapshot, Observation: &observation}, nil)
}

// acquireFileWorkspaceTreeRevision keeps Git's repository-state observation
// bound to the File owner's exact workspace-tree revision. Git does not hash
// or infer that File-owned value itself; the nested typed EventHub query is a
// read-only owner observation and never changes Runtime authority.
func (m *Biz) acquireFileWorkspaceTreeRevision(ctx context.Context, command orchestratorevents.AcquireGitCapabilityStateCommand) (string, error) {
	if m == nil || m.EventHub() == nil {
		return "", common.ErrInvalidArgument
	}
	result := m.EventHub().Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest,
		ModuleID,
		orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
			PrerequisiteID: "git-workspace-tree/" + command.AcquisitionKey, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		},
	))
	if result == nil {
		return "", common.ErrInvalidArgument
	}
	if eventErr := result.Error(); eventErr != nil {
		if eventErr.Cause != nil {
			return "", eventErr.Cause
		}
		return "", eventErr
	}
	data, err := result.Get()
	if err != nil {
		if err.Cause != nil {
			return "", err.Cause
		}
		return "", err
	}
	response, ok := data.(orchestratorevents.AcquireWorkspaceManifestResult)
	if !ok || response.Manifest.Revision == "" {
		return "", fmt.Errorf("%w: File owner returned an invalid workspace-tree observation", common.ErrConflict)
	}
	return response.Manifest.Revision, nil
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	// A capability-bound Git inventory is derived from the exact durable owner
	// receipt requested by Orchestrator.  It must not perform a second,
	// independently timed CapabilityState acquisition here.
	var snapshot runtimecontract.RuntimeCapabilityStateSnapshot
	var err error
	binding := strings.TrimSpace(command.CapabilityStateBinding)
	if binding != "" {
		if m.capabilityReceipts == nil {
			setCapabilityError(result, common.ErrInvalidArgument)
			return
		}
		var found bool
		snapshot, found, err = m.capabilityReceipts.Load(command.WorkspaceID, binding)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		if !found || snapshot.AcquisitionBinding != binding {
			setCapabilityError(result, fmt.Errorf("%w: git capability acquisition binding %q is unavailable", common.ErrConflict, binding))
			return
		}
	} else {
		// Owners/profiles without a declared capability-state prerequisite retain
		// the existing read-only inventory behavior.
		snapshot, err = m.runtimeAdapter.CapabilityState(event.Context(), command.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibilityWithBinding(
		command.PrerequisiteID,
		ModuleID,
		command.RequiredKinds,
		nil,
		nil,
		[]string{"repository-state-snapshot:" + snapshot.SnapshotID, "owner-execution-policy"},
		snapshot.ObservedRevision,
		binding,
	)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := gitAdmissibilityObservation(command.PrerequisiteID, inventory, snapshot.WorkspaceTreeRevision)
	result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: command.PrerequisiteID, Admissibility: inventory, Observation: &observation}, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	failure := m.runtimeOperationFeasibility(event.Context(), command)
	var stale *runtimecontract.RuntimePrerequisiteStale
	if failure != nil && failure.Code == "operation_admissibility_stale" && command.Observation != nil {
		if observed, err := m.runtimeAdapter.CapabilityState(event.Context(), command.Admissibility.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory); err == nil {
			stale = gitAdmissibilityStale(*command.Observation, command.Admissibility, observed.ObservedRevision)
			if stale != nil {
				failure = nil
			}
		}
	}
	observation, err := orchestratorevents.NewProbeRuntimeOperationFeasibilityResult(command, failure == nil && stale == nil, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

func gitCapabilityStateObservation(prerequisiteID string, snapshot runtimecontract.RuntimeCapabilityStateSnapshot) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: ModuleID, ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: snapshot.SubjectRef, Revision: snapshot.ObservedRevision},
			// CapabilityState reads git status or the root directory to derive the
			// required sequence. A File effect can therefore invalidate this
			// observation before Git executes its next sealed operation.
			{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: snapshot.WorkspaceTreeRevision},
		},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func gitAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility, workspaceTreeRevision string) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision},
			{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: strings.TrimSpace(workspaceTreeRevision)},
		},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func gitAdmissibilityStale(observation runtimecontract.RuntimePrerequisiteObservation, inventory runtimecontract.RuntimeOperationAdmissibility, observedRevision string) *runtimecontract.RuntimePrerequisiteStale {
	observation = observation.Normalized()
	for _, dependency := range observation.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceRepositoryState && dependency.OwnerModuleID == ModuleID && dependency.SubjectRef == "workspace_deliverable" && dependency.Revision == inventory.ObservedRevision {
			return &runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime git operation admissibility snapshot is stale"}
		}
	}
	return nil
}

// runtimeOperationFeasibility is a read-only candidate-admission check.  It
// never initializes a repository, runs a write operation, or commits an
// owner receipt.  The actual effect still occurs only after sealed dispatch.
func (m *Biz) runtimeOperationFeasibility(ctx context.Context, command orchestratorevents.ProbeRuntimeOperationFeasibilityCommand) *runtimecontract.FailureFact {
	operation := command.Operation.Normalized()
	inventory := command.Admissibility.Normalized()
	if binding := strings.TrimSpace(command.CapabilityStateBinding); binding != "" {
		if binding != inventory.AcquisitionBinding {
			return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime git operation admissibility acquisition binding is stale")
		}
		if m.capabilityReceipts == nil {
			return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_binding_unavailable", "runtime git capability acquisition binding is unavailable")
		}
		snapshot, found, err := m.capabilityReceipts.Load(command.WorkspaceID, binding)
		if err != nil || !found || snapshot.AcquisitionBinding != binding || snapshot.ObservedRevision != inventory.ObservedRevision {
			return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime git operation admissibility acquisition binding is stale")
		}
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(ctx, inventory.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory)
	if err != nil {
		return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "git_repository_unavailable", err.Error())
	}
	if snapshot.ObservedRevision != inventory.ObservedRevision {
		return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime git operation admissibility snapshot is stale")
	}
	// A repository snapshot can require a specific write sequence (for example
	// init -> add -> commit).  The generic candidate validator verifies the
	// complete dependency graph; the owner gate ensures this concrete operation
	// is at least one currently declared Git variant before it can be sealed.
	if operation.Kind == StepKindWrite && len(snapshot.RequiredOperationSequence) > 0 {
		contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
		if !found {
			return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "git_operation_contract_missing", "runtime git operation has no contract")
		}
		variant, matchErr := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
		if matchErr != nil {
			return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "git_operation_variant_invalid", matchErr.Error())
		}
		for _, required := range snapshot.RequiredOperationSequence {
			if required.Kind == operation.Kind && required.VariantID == variant.ID {
				return nil
			}
		}
		return orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "git_operation_variant_not_admissible", "runtime git operation is absent from the current repository-state sequence")
	}
	return nil
}

func (m *Biz) handleGitOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteGitOperationCommand)
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
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteGitOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	if stale, staleErr := m.runtimeCapabilityStateStaleReceipt(event.Context(), command); staleErr != nil {
		setCapabilityError(result, staleErr)
		return
	} else if stale != nil {
		committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, *stale)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(committed)
		result.Set(orchestratorevents.ExecuteGitOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: command.OperationKind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory,
		Operation:                  command.Data.Operation, TimeoutMillis: command.Data.TimeoutMillis, Pathspecs: command.Data.Pathspecs,
		Ref: command.Data.Ref, BaseRef: command.Data.BaseRef, Limit: command.Data.Limit,
		Message: command.Data.Message, ExpectedHead: command.Data.ExpectedHead, EvidencePolicies: command.EvidencePolicies,
	})
	response := orchestratorevents.ExecuteGitOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.GitOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, Operation: operationResult.Operation,
			RepoRoot: operationResult.RepoRoot, WorkingDirectory: operationResult.WorkingDirectory, Branch: operationResult.Branch,
			HeadCommit: operationResult.HeadCommit, HeadCommitBefore: operationResult.HeadCommitBefore, HeadCommitAfter: operationResult.HeadCommitAfter,
			DetachedHead: operationResult.DetachedHead, UnbornHead: operationResult.UnbornHead,
			Stdout: operationResult.Stdout, Stderr: operationResult.Stderr, ExitCode: operationResult.ExitCode,
			DurationMillis: operationResult.DurationMillis, TimeoutMillis: operationResult.TimeoutMillis, Truncated: operationResult.Truncated,
			ErrorCode: operationResult.ErrorCode, ErrorMessage: operationResult.ErrorMessage, Summary: operationResult.Summary,
			ZeroWork: operationResult.ZeroWork, Initialized: operationResult.Initialized, ExistingRepository: operationResult.ExistingRepository,
			Pathspecs: operationResult.Pathspecs, Ref: operationResult.Ref, Limit: operationResult.Limit,
			LimitClamped: operationResult.LimitClamped, Message: operationResult.Message,
			CommitHash: operationResult.CommitHash, CommitSubject: operationResult.CommitSubject,
			ChangedFiles:   gitFileChangesToEventData(operationResult.ChangedFiles),
			StagedFiles:    gitFileChangesToEventData(operationResult.StagedFiles),
			CommittedFiles: gitFileChangesToEventData(operationResult.CommittedFiles),
			Commits:        gitCommitsToEventData(operationResult.Commits),
			Branches:       gitBranchesToEventData(operationResult.Branches),
			AheadBehind:    orchestratorevents.GitAheadBehindData{Ahead: operationResult.AheadBehind.Ahead, Behind: operationResult.AheadBehind.Behind},
		},
	}
	if executeErr != nil {
		response.Status = runtimecontract.RuntimeOperationExecutionStatusRejected
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, command.OperationKind, operationResult.ErrorCode, operationResult.ErrorMessage, command.TargetRefs)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
	if command.OperationKind == runtimecontract.StepKindGitWrite {
		observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		if response.Status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
			observation.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
			observation.Changed = !operationResult.ZeroWork
			observation.NoOp = operationResult.ZeroWork
			if observation.NoOp {
				observation.ObservedEffectScope = runtimecontract.RuntimeEffectNone
			} else if after, afterErr := m.runtimeAdapter.CapabilityState(event.Context(), "runtime-receipt/"+command.OperationID, "workspace_deliverable", command.WorkspaceDeliverableDirectory); afterErr == nil {
				observation.ResourceChanges = gitReceiptResourceChanges(command.CapabilityStates, after)
			}
		}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, command.TargetRefs, response.Evidence, response.Failure, nil, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
}

// gitReceiptResourceChanges reports only revisions directly observed by the
// Git owner after a durable Git operation. Orchestrator performs the neutral
// intersection; it never derives repository state from operation kind.
func gitReceiptResourceChanges(before []runtimecontract.RuntimeCapabilityStateSnapshot, after runtimecontract.RuntimeCapabilityStateSnapshot) []runtimecontract.RuntimeResourceChange {
	if after.OwnerModuleID != ModuleID || after.CapabilityStateKind != "repository_state" || after.SubjectRef == "" || after.ObservedRevision == "" {
		return nil
	}
	beforeRevision := ""
	for _, snapshot := range before {
		if snapshot.OwnerModuleID == ModuleID && snapshot.CapabilityStateKind == "repository_state" && snapshot.SubjectRef == after.SubjectRef {
			beforeRevision = snapshot.ObservedRevision
			break
		}
	}
	return []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: after.SubjectRef,
		BeforeRevision: beforeRevision, AfterRevision: after.ObservedRevision,
	}}
}

func (m *Biz) runtimeCapabilityStateStaleReceipt(ctx context.Context, command orchestratorevents.ExecuteGitOperationCommand) (*runtimecontract.RuntimeOperationReceipt, error) {
	if strings.TrimSpace(command.RequiredOperationRef) == "" {
		return nil, nil
	}
	for _, snapshot := range command.CapabilityStates {
		if snapshot.OwnerModuleID != ModuleID || snapshot.CapabilityStateKind != "repository_state" || snapshot.SnapshotID == "" || snapshot.ContentHash == "" {
			continue
		}
		var required *runtimecontract.RuntimeRequiredOperationVariant
		for idx := range snapshot.RequiredOperationSequence {
			if snapshot.RequiredOperationSequence[idx].RequirementID == command.RequiredOperationRef {
				value := snapshot.RequiredOperationSequence[idx]
				required = &value
				break
			}
		}
		if required == nil {
			continue
		}
		// A predecessor only suppresses re-observation when the command carries
		// the exact completed receipt binding for every declared predecessor.
		// Dependency presence by itself is not authority: a missing, mismatched,
		// rejected, or no-op predecessor must never hide an external state drift.
		if len(required.DependsOnRequirementIDs) != 0 {
			if err := validateGitCapabilityStatePredecessorReceipts(snapshot, *required, command.CompletedRequiredOperationReceipts); err != nil {
				return nil, err
			}
			continue
		}
		observed, err := m.runtimeAdapter.CapabilityState(ctx, snapshot.RequirementID, snapshot.SubjectRef, command.WorkspaceDeliverableDirectory)
		if err != nil {
			return nil, err
		}
		if observed.ContentHash == snapshot.ContentHash {
			continue
		}
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "prerequisite_stale", Signature: "capability-state-stale:" + snapshot.SnapshotID, Message: "owner capability state changed after sealed snapshot", TargetRefs: append([]string(nil), command.TargetRefs...)}
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: failure.Message}, failure, nil, orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone})
		observation := gitCapabilityStateObservation(snapshot.RequirementID, snapshot)
		receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
			Domain: runtimecontract.RuntimeResourceRepositoryState, OwnerModuleID: ModuleID, SubjectRef: snapshot.SubjectRef,
			ExpectedRevision: snapshot.ObservedRevision, ObservedRevision: observed.ObservedRevision, Reason: failure.Message,
		}
		return &receipt, nil
	}
	return nil, nil
}

func validateGitCapabilityStatePredecessorReceipts(snapshot runtimecontract.RuntimeCapabilityStateSnapshot, required runtimecontract.RuntimeRequiredOperationVariant, bindings []runtimecontract.RuntimeRequiredOperationReceiptBinding) error {
	requirements := map[string]runtimecontract.RuntimeRequiredOperationVariant{}
	for _, item := range snapshot.RequiredOperationSequence {
		requirements[item.RequirementID] = item
	}
	byRequirement := map[string]runtimecontract.RuntimeRequiredOperationReceiptBinding{}
	for _, binding := range runtimecontract.NormalizeRuntimeRequiredOperationReceiptBindings(bindings) {
		byRequirement[binding.RequirementID] = binding
	}
	for _, dependencyID := range required.DependsOnRequirementIDs {
		dependency, known := requirements[dependencyID]
		binding, found := byRequirement[dependencyID]
		if !known || !found || binding.Validate() != nil || binding.Kind != dependency.Kind || binding.VariantID != dependency.VariantID ||
			binding.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || binding.ObservedScope != runtimecontract.RuntimeEffectWorkspaceDurable || !binding.Changed {
			return fmt.Errorf("capability-state predecessor receipt %q is not an accepted durable binding", dependencyID)
		}
	}
	return nil
}

func gitFileChangesToEventData(values []GitFileChange) []orchestratorevents.GitFileChangeData {
	ret := make([]orchestratorevents.GitFileChangeData, 0, len(values))
	for _, value := range values {
		ret = append(ret, orchestratorevents.GitFileChangeData{Path: value.Path, OldPath: value.OldPath, Status: value.Status, Staged: value.Staged, Unstaged: value.Unstaged, Untracked: value.Untracked})
	}
	return ret
}

func gitCommitsToEventData(values []GitCommit) []orchestratorevents.GitCommitData {
	ret := make([]orchestratorevents.GitCommitData, 0, len(values))
	for _, value := range values {
		ret = append(ret, orchestratorevents.GitCommitData{Hash: value.Hash, ShortHash: value.ShortHash, Author: value.Author, AuthorTime: value.AuthorTime, Subject: value.Subject})
	}
	return ret
}

func gitBranchesToEventData(values []GitBranch) []orchestratorevents.GitBranchData {
	ret := make([]orchestratorevents.GitBranchData, 0, len(values))
	for _, value := range values {
		ret = append(ret, orchestratorevents.GitBranchData{Name: value.Name, Commit: value.Commit, Current: value.Current, Upstream: value.Upstream})
	}
	return ret
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
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
