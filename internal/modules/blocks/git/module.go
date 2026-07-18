package git

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindRead = modulecommon.StepKindRead
const StepKindWrite = modulecommon.StepKindWrite

type Module struct {
	runtimeAdapter     *gitbiz.RuntimeAdapter
	capabilityReceipts *gitbiz.CapabilityStateReceiptStore
	capabilityObserver magicevent.SimpleObserver
	eventHub           magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub                magicevent.Hub
	RuntimeAdapter          *gitbiz.RuntimeAdapter
	CapabilityStateReceipts *gitbiz.CapabilityStateReceiptStore
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
		m.runtimeAdapter = deps[0].RuntimeAdapter
		m.capabilityReceipts = deps[0].CapabilityStateReceipts
	}
	if m.runtimeAdapter != nil && m.capabilityReceipts != nil {
		return nil
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return err
	}
	if m.runtimeAdapter == nil {
		adapter, err := gitbiz.NewRuntimeAdapterFromTimeoutString(
			true,
			cfg.Runtime.Git.DefaultTimeout,
		)
		if err != nil {
			return err
		}
		m.runtimeAdapter = adapter
	}
	if m.capabilityReceipts == nil {
		receipts, err := gitbiz.NewCapabilityStateReceiptStore(cfg.Repository.DataDir)
		if err != nil {
			return err
		}
		m.capabilityReceipts = receipts
	}
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil {
		return common.ErrInvalidArgument
	}
	if m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.capabilityObserver != nil {
		return nil
	}
	m.capabilityObserver = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireGitCapabilityState, m.handleGitCapabilityState)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteGitOperation, m.handleGitOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteGitOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireGitCapabilityState)
	}
	m.capabilityObserver = nil
	m.runtimeAdapter = nil
	m.capabilityReceipts = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleGitCapabilityState(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireGitCapabilityStateCommand)
	if !ok || m.runtimeAdapter == nil || m.capabilityReceipts == nil || command.PrerequisiteID == "" || command.AcquisitionKey == "" || command.WorkspaceDeliverableDirectory == "" || command.CapabilityStateKind != "repository_state" || command.SubjectRef == "" {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if snapshot, found, err := m.capabilityReceipts.Load(command.AcquisitionKey); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		result.Set(orchestratorevents.AcquireGitCapabilityStateResult{PrerequisiteID: command.PrerequisiteID, Snapshot: snapshot}, nil)
		return
	}
	snapshot, err := m.runtimeAdapter.CapabilityState(event.Context(), command.PrerequisiteID, command.SubjectRef, command.WorkspaceDeliverableDirectory)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	snapshot.AcquisitionKey = command.AcquisitionKey
	snapshot.QueryReceiptRef = "capability-state-receipt/" + command.AcquisitionKey
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	snapshot, err = m.capabilityReceipts.StoreIfAbsent(snapshot)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(orchestratorevents.AcquireGitCapabilityStateResult{PrerequisiteID: command.PrerequisiteID, Snapshot: snapshot}, nil)
}

func (m *Module) handleGitOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteGitOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := orchestratorevents.QueryPersistedRuntimeOperationReceipt(event.Context(), m.eventHub, ModuleID, command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
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
		committed, err := orchestratorevents.CommitPersistedRuntimeOperationReceipt(event.Context(), m.eventHub, ModuleID, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, *stale)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(committed)
		result.Set(orchestratorevents.ExecuteGitOperationResult{Receipt: committed, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), gitbiz.RuntimeOperationRequest{
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
			}
		}
	}
	receipt := orchestratorevents.NewRuntimeOperationReceiptWithObservation(command.Execution, command.OperationID, response.Status, command.TargetRefs, response.Evidence, response.Failure, nil, observation)
	committed, err := orchestratorevents.CommitPersistedRuntimeOperationReceipt(event.Context(), m.eventHub, ModuleID, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
}

func (m *Module) runtimeCapabilityStateStaleReceipt(ctx context.Context, command orchestratorevents.ExecuteGitOperationCommand) (*runtimecontract.RuntimeOperationReceipt, error) {
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
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "capability_state_stale", Signature: "capability-state-stale:" + snapshot.SnapshotID, Message: "owner capability state changed after sealed snapshot", TargetRefs: append([]string(nil), command.TargetRefs...)}
		receipt := orchestratorevents.NewRuntimeOperationReceiptWithObservation(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{Summary: failure.Message}, failure, nil, orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone})
		receipt.CapabilityStateStale = &runtimecontract.RuntimeCapabilityStateStaleDetail{
			StateRef:   runtimecontract.RuntimeCapabilityStateRef{OwnerModuleID: snapshot.OwnerModuleID, CapabilityStateKind: snapshot.CapabilityStateKind, SubjectRef: snapshot.SubjectRef},
			SnapshotID: snapshot.SnapshotID, ExpectedHash: snapshot.ContentHash,
			ObservedRevision: observed.ObservedRevision, ObservedHash: observed.ContentHash, Reason: failure.Message,
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

func gitFileChangesToEventData(values []gitbiz.GitFileChange) []orchestratorevents.GitFileChangeData {
	ret := make([]orchestratorevents.GitFileChangeData, 0, len(values))
	for _, value := range values {
		ret = append(ret, orchestratorevents.GitFileChangeData{Path: value.Path, OldPath: value.OldPath, Status: value.Status, Staged: value.Staged, Unstaged: value.Unstaged, Untracked: value.Untracked})
	}
	return ret
}

func gitCommitsToEventData(values []gitbiz.GitCommit) []orchestratorevents.GitCommitData {
	ret := make([]orchestratorevents.GitCommitData, 0, len(values))
	for _, value := range values {
		ret = append(ret, orchestratorevents.GitCommitData{Hash: value.Hash, ShortHash: value.ShortHash, Author: value.Author, AuthorTime: value.AuthorTime, Subject: value.Subject})
	}
	return ret
}

func gitBranchesToEventData(values []gitbiz.GitBranch) []orchestratorevents.GitBranchData {
	ret := make([]orchestratorevents.GitBranchData, 0, len(values))
	for _, value := range values {
		ret = append(ret, orchestratorevents.GitBranchData{Name: value.Name, Commit: value.Commit, Current: value.Current, Upstream: value.Upstream})
	}
	return ret
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindRead, StepKindWrite)
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
