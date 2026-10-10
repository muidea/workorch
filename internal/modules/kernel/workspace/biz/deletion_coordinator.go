package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

const workspaceDeletionMaxDriveSteps = 64

// driveWorkspaceDeletion is a coordinator over persisted Workspace-owner
// phase authority. Cross-owner responses are facts only; every phase advance
// is revalidated and committed by submitWorkspaceDeletionInternalEvent.
func (m *Biz) driveWorkspaceDeletion(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if m == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	m.deletionWorkMu.Lock()
	defer m.deletionWorkMu.Unlock()
	return m.driveWorkspaceDeletionLocked(ctx, workspaceID)
}

func (m *Biz) driveWorkspaceDeletionLocked(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if m == nil || m.workspaces == nil || m.EventHub() == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	boundCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for step := 0; step < workspaceDeletionMaxDriveSteps; step++ {
		item, err := m.workspaces.Get(boundCtx, workspaceID)
		if err != nil {
			return nil, err
		}
		if item.Deletion == nil {
			return nil, common.ErrConflict
		}
		record := item.Deletion.Clone()
		if err := record.Validate(item.ID); err != nil {
			return nil, fmt.Errorf("%w: invalid persisted Workspace deletion: %v", common.ErrConflict, err)
		}
		switch record.Phase {
		case workspacemodel.WorkspaceDeletionPhaseRequested, workspacemodel.WorkspaceDeletionPhaseWaitingForIdle:
			if err := m.driveWorkspaceDeletionRunDraining(boundCtx, record, workspaceID); err != nil {
				if errors.Is(err, runevents.ErrWorkspaceRetirementBusy) && record.Phase == workspacemodel.WorkspaceDeletionPhaseRequested {
					_, reduceErr := m.submitWorkspaceDeletionInternalEvent(boundCtx, workspaceDeletionInternalEvent{
						Kind:    workspaceDeletionInternalEventRunBusyObserved,
						RunBusy: &workspaceDeletionRunBusyFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OccurredAt: time.Now().UTC()},
					})
					if reduceErr != nil {
						return nil, reduceErr
					}
				}
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseRunDraining:
			if err := m.driveWorkspaceDeletionQuiescence(boundCtx, record, workspaceID); err != nil {
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseRuntimeQuiescent:
			if err := m.driveWorkspaceDeletionRunRetired(boundCtx, record, workspaceID); err != nil {
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseRunRetired:
			_, err := m.submitWorkspaceDeletionInternalEvent(boundCtx, workspaceDeletionInternalEvent{
				Kind:         workspaceDeletionInternalEventOwnerSealingStarted,
				OwnerSealing: &workspaceDeletionOwnerSealingFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OccurredAt: time.Now().UTC()},
			})
			if err != nil {
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseOwnerSealing:
			if err := m.driveWorkspaceDeletionNextSeal(boundCtx, record, workspaceID, false); err != nil {
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseOwnerPurging:
			if err := m.driveWorkspaceDeletionNextPurge(boundCtx, record, workspaceID, false); err != nil {
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseDirectoryCleanup:
			if err := m.driveWorkspaceDeletionDirectoryCleanup(boundCtx, record, workspaceID); err != nil {
				return nil, err
			}
		case workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing:
			if len(record.OwnerSealAcks) < len(workspaceDeletionSealOwners) {
				if err := m.driveWorkspaceDeletionNextSeal(boundCtx, record, workspaceID, true); err != nil {
					return nil, err
				}
				continue
			}
			if record.Mode == deletioncontract.ModePurge && len(record.OwnerPurgeAcks) < len(workspaceDeletionPurgeOwners) {
				if err := m.driveWorkspaceDeletionNextPurge(boundCtx, record, workspaceID, true); err != nil {
					return nil, err
				}
				continue
			}
			return nil, common.ErrConflict
		case workspacemodel.WorkspaceDeletionPhaseCompleted:
			if err := m.publishWorkspaceDeleted(boundCtx, item); err != nil {
				return nil, err
			}
			return item, nil
		default:
			return nil, common.ErrConflict
		}
	}
	return nil, fmt.Errorf("%w: Workspace deletion drive exceeded bounded transitions", common.ErrConflict)
}

func (m *Biz) driveWorkspaceDeletionRunDraining(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID) error {
	command := runevents.PrepareWorkspaceRetirementCommand{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
	}.Normalized()
	result, err := sendWorkspaceDeletionOwnerCommand[runevents.PrepareWorkspaceRetirementResult](m, ctx, runevents.TopicRunCommandPrepareWorkspaceRetirement, runevents.CommandDestination, runevents.Contracts(), command)
	if err != nil {
		return err
	}
	if err := result.ValidateFor(command); err != nil {
		return common.ErrConflict
	}
	_, err = m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{
		Kind:        workspaceDeletionInternalEventRunDrainingObserved,
		RunDraining: &workspaceDeletionRunDrainingFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, FenceRevision: result.Fence.FenceRevision, WorkspaceRunGeneration: result.Fence.WorkspaceRunGeneration, OccurredAt: time.Now().UTC()},
	})
	return err
}

func (m *Biz) driveWorkspaceDeletionQuiescence(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID) error {
	if record.RunDrainingAck == nil {
		return common.ErrConflict
	}
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
		RunDrainingRevision: record.RunDrainingAck.FenceRevision, WorkspaceRunGeneration: record.RunDrainingAck.WorkspaceRunGeneration,
	}.Normalized()
	beginCommand := orchestratorevents.BeginWorkspaceRuntimeQuiescenceCommand{Intent: intent}
	beginResult, err := sendWorkspaceDeletionOwnerCommand[orchestratorevents.BeginWorkspaceRuntimeQuiescenceResult](m, ctx, orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence, orchestratorevents.CommandDestination, orchestratorevents.Contracts(), beginCommand)
	if err != nil {
		return err
	}
	if err := beginResult.ValidateFor(beginCommand); err != nil {
		return common.ErrConflict
	}
	command := orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceCommand{Intent: intent}
	result, err := sendWorkspaceDeletionOwnerCommand[orchestratorevents.ConfirmWorkspaceRuntimeQuiescenceResult](m, ctx, orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence, orchestratorevents.CommandDestination, orchestratorevents.Contracts(), command)
	if err != nil {
		return err
	}
	if err := result.ValidateFor(command); err != nil {
		return common.ErrConflict
	}
	_, err = m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventRuntimeQuiescenceObserved,
		RuntimeQuiescence: &workspaceDeletionRuntimeQuiescenceFact{
			WorkspaceID: workspaceID, DeletionID: record.DeletionID, Token: result.Token, OccurredAt: time.Now().UTC(),
		},
	})
	return err
}

func (m *Biz) driveWorkspaceDeletionRunRetired(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID) error {
	if record.RuntimeQuiescenceAck == nil {
		return common.ErrConflict
	}
	command := runevents.FinalizeWorkspaceRetirementCommand{QuiescenceToken: record.RuntimeQuiescenceAck.Token}.Normalized()
	result, err := sendWorkspaceDeletionOwnerCommand[runevents.FinalizeWorkspaceRetirementResult](m, ctx, runevents.TopicRunCommandFinalizeWorkspaceRetirement, runevents.CommandDestination, runevents.Contracts(), command)
	if err != nil {
		return err
	}
	if err := result.ValidateFor(command); err != nil {
		return common.ErrConflict
	}
	_, err = m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventRunRetiredObserved,
		RunRetired: &workspaceDeletionRunRetiredFact{
			WorkspaceID: workspaceID, DeletionID: record.DeletionID, FenceRevision: result.Fence.FenceRevision,
			WorkspaceRunGeneration: result.Fence.WorkspaceRunGeneration, QuiescenceID: result.Fence.QuiescenceID,
			RuntimeInventoryHash: result.Fence.RuntimeInventoryHash, OccurredAt: time.Now().UTC(),
		},
	})
	return err
}

func workspaceDeletionScope(record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID) (deletioncontract.Scope, error) {
	if record.RuntimeQuiescenceAck == nil || record.RunRetiredAck == nil {
		return deletioncontract.Scope{}, common.ErrConflict
	}
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		Mode: record.Mode, QuiescenceID: record.RuntimeQuiescenceAck.Token.QuiescenceID,
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	if err := scope.Validate(); err != nil {
		return deletioncontract.Scope{}, err
	}
	return scope, nil
}

func (m *Biz) driveWorkspaceDeletionNextSeal(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID, eventStream bool) error {
	scope, err := workspaceDeletionScope(record, workspaceID)
	if err != nil {
		return err
	}
	owners := workspaceDeletionPreCleanupSealOwners
	if eventStream {
		owners = workspaceDeletionSealOwners
	}
	if len(record.OwnerSealAcks) >= len(owners) {
		return common.ErrConflict
	}
	owner := owners[len(record.OwnerSealAcks)]
	var fact workspaceDeletionOwnerSealFact
	switch owner {
	case "kernel/orchestrator":
		command := orchestratorevents.SealWorkspaceOrchestratorDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[orchestratorevents.SealWorkspaceOrchestratorDeletionResult](m, ctx, orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion, orchestratorevents.CommandDestination, orchestratorevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerSealFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, FenceID: result.FenceID, Revision: result.Revision, AckHash: result.AckHash}
	case "blocks/result":
		command := resultevents.SealWorkspaceResultDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[resultevents.SealWorkspaceResultDeletionResult](m, ctx, resultevents.TopicResultCommandSealWorkspaceDeletion, resultevents.CommandDestination, resultevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerSealFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, FenceID: result.FenceID, Revision: result.Revision, AckHash: result.AckHash}
	case "blocks/memory":
		command := memoryevents.SealWorkspaceMemoryDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[memoryevents.SealWorkspaceMemoryDeletionResult](m, ctx, memoryevents.TopicMemoryCommandSealWorkspaceDeletion, memoryevents.CommandDestination, memoryevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerSealFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, FenceID: result.FenceID, Revision: result.Revision, AckHash: result.AckHash}
	case "blocks/decision":
		command := decisionevents.SealWorkspaceDecisionDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[decisionevents.SealWorkspaceDecisionDeletionResult](m, ctx, decisionevents.TopicDecisionCommandSealWorkspaceDeletion, decisionevents.CommandDestination, decisionevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerSealFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, FenceID: result.FenceID, Revision: result.Revision, AckHash: result.AckHash}
	case "blocks/approval":
		command := approvalevents.SealWorkspaceApprovalDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[approvalevents.SealWorkspaceApprovalDeletionResult](m, ctx, approvalevents.TopicApprovalCommandSealWorkspaceDeletion, approvalevents.CommandDestination, approvalevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerSealFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, FenceID: result.FenceID, Revision: result.Revision, AckHash: result.AckHash}
	case "blocks/eventstream":
		command := eventstreamevents.SealWorkspaceEventStreamDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[eventstreamevents.SealWorkspaceEventStreamDeletionResult](m, ctx, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, eventstreamevents.CommandDestination, eventstreamevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerSealFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, FenceID: result.FenceID, Revision: result.Revision, AckHash: result.AckHash}
	default:
		return fmt.Errorf("%w: Workspace deletion seal owner %s is not ready", common.ErrConflict, owner)
	}
	fact.OccurredAt = time.Now().UTC()
	_, err = m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerSealObserved, OwnerSeal: &fact})
	return err
}

func (m *Biz) driveWorkspaceDeletionNextPurge(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID, eventStream bool) error {
	scope, err := workspaceDeletionScope(record, workspaceID)
	if err != nil {
		return err
	}
	owners := workspaceDeletionPreEventStreamPurgeOwners
	if eventStream {
		owners = workspaceDeletionPurgeOwners
	}
	if len(record.OwnerPurgeAcks) >= len(owners) {
		return common.ErrConflict
	}
	owner := owners[len(record.OwnerPurgeAcks)]
	var fact workspaceDeletionOwnerPurgeFact
	switch owner {
	case "kernel/orchestrator":
		command := orchestratorevents.PurgeWorkspaceOrchestratorEvidenceCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult](m, ctx, orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence, orchestratorevents.CommandDestination, orchestratorevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.Revision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	case "blocks/result":
		command := resultevents.PurgeWorkspaceResultsCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[resultevents.PurgeWorkspaceResultsResult](m, ctx, resultevents.TopicResultCommandPurgeWorkspace, resultevents.CommandDestination, resultevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.Revision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	case "blocks/memory":
		command := memoryevents.PurgeWorkspaceMemoryCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[memoryevents.PurgeWorkspaceMemoryResult](m, ctx, memoryevents.TopicMemoryCommandPurgeWorkspace, memoryevents.CommandDestination, memoryevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.Revision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	case "blocks/decision":
		command := decisionevents.PurgeWorkspaceDecisionsCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[decisionevents.PurgeWorkspaceDecisionsResult](m, ctx, decisionevents.TopicDecisionCommandPurgeWorkspace, decisionevents.CommandDestination, decisionevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.Revision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	case "blocks/approval":
		command := approvalevents.PurgeWorkspaceApprovalsCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[approvalevents.PurgeWorkspaceApprovalsResult](m, ctx, approvalevents.TopicApprovalCommandPurgeWorkspace, approvalevents.CommandDestination, approvalevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.Revision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	case "blocks/run":
		if record.RunRetiredAck == nil {
			return common.ErrConflict
		}
		command := runevents.PurgeWorkspaceRunsCommand{
			WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
			DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
			FenceRevision: record.RunRetiredAck.FenceRevision, WorkspaceRunGeneration: record.RunRetiredAck.WorkspaceRunGeneration,
			RuntimeInventoryHash: record.RunRetiredAck.RuntimeInventoryHash, QuiescenceID: record.RunRetiredAck.QuiescenceID,
		}.Normalized()
		result, sendErr := sendWorkspaceDeletionOwnerCommand[runevents.PurgeWorkspaceRunsResult](m, ctx, runevents.TopicRunCommandPurgeWorkspace, runevents.CommandDestination, runevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.FenceRevision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	case "blocks/eventstream":
		command := eventstreamevents.PurgeWorkspaceCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
		result, sendErr := sendWorkspaceDeletionOwnerCommand[eventstreamevents.PurgeWorkspaceResult](m, ctx, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, eventstreamevents.CommandDestination, eventstreamevents.Contracts(), command)
		if sendErr != nil {
			return sendErr
		}
		if result.ValidateFor(command) != nil {
			return common.ErrConflict
		}
		fact = workspaceDeletionOwnerPurgeFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, OwnerModuleID: owner, PurgeID: result.PurgeID, Revision: result.Revision, AckHash: result.AckHash, PurgeProofHash: result.PurgeProofHash}
	default:
		return fmt.Errorf("%w: Workspace deletion purge owner %s is not ready", common.ErrConflict, owner)
	}
	fact.OccurredAt = time.Now().UTC()
	_, err = m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventOwnerPurgeObserved, OwnerPurge: &fact})
	return err
}

func (m *Biz) driveWorkspaceDeletionDirectoryCleanup(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID) error {
	cleanupID := stableWorkspaceDeletionCleanupID(record)
	if cleanupID == "" {
		return common.ErrConflict
	}
	_, err := m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{
		Kind:             workspaceDeletionInternalEventDirectoryCleanupRequested,
		DirectoryCleanup: &workspaceDeletionDirectoryCleanupFact{WorkspaceID: workspaceID, DeletionID: record.DeletionID, CleanupID: cleanupID, OccurredAt: time.Now().UTC()},
	})
	return err
}

func (m *Biz) cleanupWorkspaceOwnedDeletionMaterial(ctx context.Context, record workspacemodel.WorkspaceDeletionRecord, workspaceID common.WorkspaceID) error {
	if m.manager == nil || m.goals == nil || m.dods == nil {
		return common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	goals, err := m.goals.ListByWorkspaceID(ctx, workspaceID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return err
	}
	for _, item := range goals {
		if item != nil {
			if err := m.goals.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, workspaceID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return err
	}
	for _, item := range dods {
		if item != nil {
			if err := m.dods.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
	}
	path, err := safeWorkspaceDeletionDeliverablePath(record.SealedDeliverableDirectory, record.SealedInternalDirectory)
	if err != nil {
		return err
	}
	if path != "" {
		return os.RemoveAll(path)
	}
	return nil
}

func safeWorkspaceDeletionDeliverablePath(deliverable, internal string) (string, error) {
	deliverable = strings.TrimSpace(deliverable)
	if deliverable == "" {
		return "", nil
	}
	abs, err := filepath.Abs(deliverable)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return "", common.ErrInvalidArgument
	}
	if filepath.Dir(abs) == string(filepath.Separator) {
		return "", common.ErrInvalidArgument
	}
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()
	for _, protected := range []string{home, wd, os.TempDir()} {
		if strings.TrimSpace(protected) == "" {
			continue
		}
		protectedAbs, err := filepath.Abs(protected)
		if err != nil {
			return "", err
		}
		if abs == filepath.Clean(protectedAbs) {
			return "", common.ErrInvalidArgument
		}
	}
	if strings.TrimSpace(internal) != "" {
		internalAbs, err := filepath.Abs(internal)
		if err != nil {
			return "", err
		}
		internalAbs = filepath.Clean(internalAbs)
		if abs == internalAbs || strings.HasPrefix(internalAbs, abs+string(filepath.Separator)) || strings.HasPrefix(abs, internalAbs+string(filepath.Separator)) {
			return "", common.ErrConflict
		}
	}
	return abs, nil
}

func sendWorkspaceDeletionOwnerCommand[Result any](m *Biz, ctx context.Context, topic, destination string, contracts []eventcontract.Contract, command any) (Result, error) {
	var zero Result
	if m == nil || m.EventHub() == nil {
		return zero, common.ErrInvalidArgument
	}
	contractPresent := false
	for _, contract := range contracts {
		if contract.Topic == topic {
			contractPresent = true
			break
		}
	}
	if !contractPresent {
		return zero, fmt.Errorf("%w: deletion dependency contract %s is not ready", common.ErrConflict, topic)
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, destination, magicevent.NewHeader(), ctx, command)
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if err := bindWorkspaceDeletionOwnerLane(event, topic, workspaceID); err != nil {
		return zero, err
	}
	response := m.SendEvent(event)
	if response == nil {
		return zero, common.ErrConflict
	}
	if eventErr := response.Error(); eventErr != nil {
		if eventErr.Cause != nil {
			return zero, eventErr.Cause
		}
		return zero, eventErr
	}
	value, err := response.Get()
	if err != nil {
		return zero, err
	}
	result, ok := value.(Result)
	if !ok {
		return zero, common.ErrConflict
	}
	return result, nil
}

// bindWorkspaceDeletionOwnerLane keeps every dependency inventory explicit.
// The generic transport helper cannot infer or substitute an owner's contract
// set; adding a new deletion participant therefore requires an adjudicated
// branch naming that receiving owner.
func bindWorkspaceDeletionOwnerLane(event magicevent.Event, topic string, workspaceID common.WorkspaceID) error {
	if event == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	scope := eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(workspaceID), WorkspaceID: string(workspaceID)}
	switch topic {
	case runevents.TopicRunCommandPrepareWorkspaceRetirement, runevents.TopicRunCommandFinalizeWorkspaceRetirement, runevents.TopicRunCommandPurgeWorkspace:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), topic, scope))
	case orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence, orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence, orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion, orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(orchestratorevents.Contracts(), topic, scope))
	case resultevents.TopicResultCommandSealWorkspaceDeletion, resultevents.TopicResultCommandPurgeWorkspace:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(resultevents.Contracts(), topic, scope))
	case memoryevents.TopicMemoryCommandSealWorkspaceDeletion, memoryevents.TopicMemoryCommandPurgeWorkspace:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), topic, scope))
	case decisionevents.TopicDecisionCommandSealWorkspaceDeletion, decisionevents.TopicDecisionCommandPurgeWorkspace:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), topic, scope))
	case approvalevents.TopicApprovalCommandSealWorkspaceDeletion, approvalevents.TopicApprovalCommandPurgeWorkspace:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(approvalevents.Contracts(), topic, scope))
	case eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, eventstreamevents.TopicEventStreamCommandPurgeWorkspace:
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(eventstreamevents.Contracts(), topic, scope))
	default:
		return fmt.Errorf("%w: deletion owner topic %s is not adjudicated", common.ErrConflict, topic)
	}
	return nil
}

func (m *Biz) publishWorkspaceDeleted(ctx context.Context, item *workspacemodel.Workspace) error {
	if m == nil || m.manager == nil || item == nil {
		return common.ErrInvalidArgument
	}
	return m.manager.publishWorkspaceDeleted(ctx, item)
}

func (m *Biz) reconcileWorkspaceDeletionInventory(ctx context.Context) {
	if m == nil || ctx == nil || ctx.Err() != nil {
		return
	}
	m.deletionWorkMu.Lock()
	defer m.deletionWorkMu.Unlock()
	if ctx.Err() != nil || m.workspaces == nil {
		return
	}
	items, err := m.workspaces.List(ctx)
	if err != nil {
		return
	}
	for _, item := range items {
		if item == nil || item.Deletion == nil || item.Deletion.Completed {
			continue
		}
		_, _ = m.driveWorkspaceDeletionLocked(ctx, item.ID)
	}
}
