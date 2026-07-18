package git

import (
	"context"

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
	capabilityObserver magicevent.SimpleObserver
	eventHub           magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub magicevent.Hub
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return err
	}
	adapter, err := gitbiz.NewRuntimeAdapterFromTimeoutString(
		true,
		cfg.Runtime.Git.DefaultTimeout,
	)
	if err != nil {
		return err
	}
	m.runtimeAdapter = adapter
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
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteGitOperation, m.handleGitOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteGitOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	}
	m.capabilityObserver = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
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
