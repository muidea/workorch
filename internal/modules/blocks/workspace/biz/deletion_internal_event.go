package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const workspaceDeletionCASMaxAttempts = 8

type workspaceDeletionInternalEventKind string

const (
	workspaceDeletionInternalEventRequested                 workspaceDeletionInternalEventKind = "deletion_requested"
	workspaceDeletionInternalEventRunBusyObserved           workspaceDeletionInternalEventKind = "run_busy_observed"
	workspaceDeletionInternalEventRunDrainingObserved       workspaceDeletionInternalEventKind = "run_draining_observed"
	workspaceDeletionInternalEventRuntimeQuiescenceObserved workspaceDeletionInternalEventKind = "runtime_quiescence_observed"
	workspaceDeletionInternalEventRunRetiredObserved        workspaceDeletionInternalEventKind = "run_retired_observed"
	workspaceDeletionInternalEventOwnerSealingStarted       workspaceDeletionInternalEventKind = "owner_sealing_started"
	workspaceDeletionInternalEventOwnerSealObserved         workspaceDeletionInternalEventKind = "owner_seal_observed"
	workspaceDeletionInternalEventOwnerPurgeObserved        workspaceDeletionInternalEventKind = "owner_purge_observed"
	workspaceDeletionInternalEventDirectoryCleanupRequested workspaceDeletionInternalEventKind = "directory_cleanup_requested"
	workspaceDeletionInternalEventDirectoryCleanupObserved  workspaceDeletionInternalEventKind = "directory_cleanup_observed"
)

var workspaceDeletionSealOwners = workspacemodel.WorkspaceDeletionRequiredSealOwners()

var workspaceDeletionPreCleanupSealOwners = workspaceDeletionSealOwners[:len(workspaceDeletionSealOwners)-1]

var workspaceDeletionPurgeOwners = workspacemodel.WorkspaceDeletionRequiredPurgeOwners()

var workspaceDeletionPreEventStreamPurgeOwners = workspaceDeletionPurgeOwners[:len(workspaceDeletionPurgeOwners)-1]

type workspaceDeletionRequestedFact struct {
	Command    workspaceevents.DeleteWorkspaceCommand
	OccurredAt time.Time
}

type workspaceDeletionRunBusyFact struct {
	WorkspaceID common.WorkspaceID
	DeletionID  string
	OccurredAt  time.Time
}

type workspaceDeletionRunDrainingFact struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	OccurredAt             time.Time
}

type workspaceDeletionRuntimeQuiescenceFact struct {
	WorkspaceID common.WorkspaceID
	DeletionID  string
	Token       deletioncontract.WorkspaceRuntimeQuiescenceToken
	OccurredAt  time.Time
}

type workspaceDeletionRunRetiredFact struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	QuiescenceID           string
	RuntimeInventoryHash   string
	OccurredAt             time.Time
}

type workspaceDeletionOwnerSealingFact struct {
	WorkspaceID common.WorkspaceID
	DeletionID  string
	OccurredAt  time.Time
}

type workspaceDeletionOwnerSealFact struct {
	WorkspaceID   common.WorkspaceID
	DeletionID    string
	OwnerModuleID string
	FenceID       string
	Revision      uint64
	AckHash       string
	OccurredAt    time.Time
}

type workspaceDeletionOwnerPurgeFact struct {
	WorkspaceID    common.WorkspaceID
	DeletionID     string
	OwnerModuleID  string
	PurgeID        string
	Revision       uint64
	AckHash        string
	PurgeProofHash string
	OccurredAt     time.Time
}

type workspaceDeletionDirectoryCleanupFact struct {
	WorkspaceID common.WorkspaceID
	DeletionID  string
	CleanupID   string
	OccurredAt  time.Time
}

type workspaceDeletionInternalEvent struct {
	Kind              workspaceDeletionInternalEventKind
	Requested         *workspaceDeletionRequestedFact
	RunBusy           *workspaceDeletionRunBusyFact
	RunDraining       *workspaceDeletionRunDrainingFact
	RuntimeQuiescence *workspaceDeletionRuntimeQuiescenceFact
	RunRetired        *workspaceDeletionRunRetiredFact
	OwnerSealing      *workspaceDeletionOwnerSealingFact
	OwnerSeal         *workspaceDeletionOwnerSealFact
	OwnerPurge        *workspaceDeletionOwnerPurgeFact
	DirectoryCleanup  *workspaceDeletionDirectoryCleanupFact
}

type workspaceDeletionTransition struct {
	Next      *workspacemodel.Workspace
	Noop      bool
	Duplicate bool
	Completed bool
}

func normalizeWorkspaceDeletionInternalEvent(event workspaceDeletionInternalEvent) (workspaceDeletionInternalEvent, error) {
	payloads := 0
	for _, present := range []bool{event.Requested != nil, event.RunBusy != nil, event.RunDraining != nil, event.RuntimeQuiescence != nil, event.RunRetired != nil, event.OwnerSealing != nil, event.OwnerSeal != nil, event.OwnerPurge != nil, event.DirectoryCleanup != nil} {
		if present {
			payloads++
		}
	}
	if payloads != 1 {
		return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceDeletionInternalEventRequested:
		if event.Requested == nil {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
		event.Requested.Command = event.Requested.Command.Normalized()
		if err := event.Requested.Command.Validate(); err != nil || event.Requested.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventRunBusyObserved:
		if event.RunBusy == nil || event.RunBusy.WorkspaceID == "" || strings.TrimSpace(event.RunBusy.DeletionID) == "" || event.RunBusy.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventRunDrainingObserved:
		if event.RunDraining == nil || event.RunDraining.WorkspaceID == "" || strings.TrimSpace(event.RunDraining.DeletionID) == "" || event.RunDraining.FenceRevision == 0 || event.RunDraining.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventRuntimeQuiescenceObserved:
		if event.RuntimeQuiescence == nil || event.RuntimeQuiescence.WorkspaceID == "" || strings.TrimSpace(event.RuntimeQuiescence.DeletionID) == "" || event.RuntimeQuiescence.Token.Validate() != nil || event.RuntimeQuiescence.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
		event.RuntimeQuiescence.Token = deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(event.RuntimeQuiescence.Token)
	case workspaceDeletionInternalEventRunRetiredObserved:
		if event.RunRetired == nil || event.RunRetired.WorkspaceID == "" || strings.TrimSpace(event.RunRetired.DeletionID) == "" || event.RunRetired.FenceRevision == 0 || strings.TrimSpace(event.RunRetired.QuiescenceID) == "" || strings.TrimSpace(event.RunRetired.RuntimeInventoryHash) == "" || event.RunRetired.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventOwnerSealingStarted:
		if event.OwnerSealing == nil || event.OwnerSealing.WorkspaceID == "" || strings.TrimSpace(event.OwnerSealing.DeletionID) == "" || event.OwnerSealing.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventOwnerSealObserved:
		if event.OwnerSeal == nil || event.OwnerSeal.WorkspaceID == "" || strings.TrimSpace(event.OwnerSeal.DeletionID) == "" || !workspaceDeletionOwnerAllowed(event.OwnerSeal.OwnerModuleID, workspaceDeletionSealOwners) || strings.TrimSpace(event.OwnerSeal.FenceID) == "" || event.OwnerSeal.Revision == 0 || strings.TrimSpace(event.OwnerSeal.AckHash) == "" || event.OwnerSeal.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventOwnerPurgeObserved:
		if event.OwnerPurge == nil || event.OwnerPurge.WorkspaceID == "" || strings.TrimSpace(event.OwnerPurge.DeletionID) == "" || !workspaceDeletionOwnerAllowed(event.OwnerPurge.OwnerModuleID, workspaceDeletionPurgeOwners) || strings.TrimSpace(event.OwnerPurge.PurgeID) == "" || event.OwnerPurge.Revision == 0 || strings.TrimSpace(event.OwnerPurge.AckHash) == "" || strings.TrimSpace(event.OwnerPurge.PurgeProofHash) == "" || event.OwnerPurge.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	case workspaceDeletionInternalEventDirectoryCleanupRequested, workspaceDeletionInternalEventDirectoryCleanupObserved:
		if event.DirectoryCleanup == nil || event.DirectoryCleanup.WorkspaceID == "" || strings.TrimSpace(event.DirectoryCleanup.DeletionID) == "" || strings.TrimSpace(event.DirectoryCleanup.CleanupID) == "" || event.DirectoryCleanup.OccurredAt.IsZero() {
			return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
		}
	default:
		return workspaceDeletionInternalEvent{}, common.ErrInvalidArgument
	}
	return event, nil
}

func reduceWorkspaceDeletionInternalEvent(current *workspacemodel.Workspace, event workspaceDeletionInternalEvent) (workspaceDeletionTransition, error) {
	if current == nil || current.ID == "" {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	if current.Deletion != nil {
		if err := current.Deletion.Validate(current.ID); err != nil {
			return workspaceDeletionTransition{}, fmt.Errorf("%w: invalid persisted Workspace deletion: %v", common.ErrConflict, err)
		}
	}
	switch event.Kind {
	case workspaceDeletionInternalEventRequested:
		return reduceWorkspaceDeletionRequested(current, *event.Requested)
	case workspaceDeletionInternalEventRunBusyObserved:
		return reduceWorkspaceDeletionRunBusy(current, *event.RunBusy)
	case workspaceDeletionInternalEventRunDrainingObserved:
		return reduceWorkspaceDeletionRunDraining(current, *event.RunDraining)
	case workspaceDeletionInternalEventRuntimeQuiescenceObserved:
		return reduceWorkspaceDeletionRuntimeQuiescence(current, *event.RuntimeQuiescence)
	case workspaceDeletionInternalEventRunRetiredObserved:
		return reduceWorkspaceDeletionRunRetired(current, *event.RunRetired)
	case workspaceDeletionInternalEventOwnerSealingStarted:
		return reduceWorkspaceDeletionOwnerSealing(current, *event.OwnerSealing)
	case workspaceDeletionInternalEventOwnerSealObserved:
		return reduceWorkspaceDeletionOwnerSeal(current, *event.OwnerSeal)
	case workspaceDeletionInternalEventOwnerPurgeObserved:
		return reduceWorkspaceDeletionOwnerPurge(current, *event.OwnerPurge)
	case workspaceDeletionInternalEventDirectoryCleanupObserved:
		return reduceWorkspaceDeletionDirectoryCleanup(current, *event.DirectoryCleanup)
	default:
		return workspaceDeletionTransition{}, common.ErrInvalidArgument
	}
}

func reduceWorkspaceDeletionRequested(current *workspacemodel.Workspace, fact workspaceDeletionRequestedFact) (workspaceDeletionTransition, error) {
	command := fact.Command.Normalized()
	deletionID := workspaceevents.StableWorkspaceDeletionID(command.WorkspaceID, command.RequestID)
	if current.ID != command.WorkspaceID {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	if current.Deletion == nil {
		if current.ActiveRunID != "" || (command.ExpectedRevision != 0 && current.Revision != command.ExpectedRevision) {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		record := workspacemodel.WorkspaceDeletionRecord{
			DeletionID: deletionID, DeletionGeneration: 1, RequestID: command.RequestID, Mode: command.Mode(),
			ExpectedRevision: command.ExpectedRevision, Phase: workspacemodel.WorkspaceDeletionPhaseRequested,
			SealedDeliverableDirectory: strings.TrimSpace(current.DeliverableDirectory), SealedInternalDirectory: strings.TrimSpace(current.InternalDirectory),
			Revision: 1, RequestedAt: fact.OccurredAt.UTC(), UpdatedAt: fact.OccurredAt.UTC(),
		}
		record.IntentHash = workspacemodel.StableWorkspaceDeletionIntentHash(current.ID, record)
		next := *current
		next.Deletion = workspaceDeletionRecordPtr(record)
		return workspaceDeletionTransition{Next: &next}, nil
	}
	record := current.Deletion.Clone()
	if record.DeletionID != deletionID || record.RequestID != command.RequestID || record.Mode != command.Mode() || record.ExpectedRevision != command.ExpectedRevision || record.IntentHash != workspacemodel.StableWorkspaceDeletionIntentHash(current.ID, record) {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	if record.Phase == workspacemodel.WorkspaceDeletionPhaseWaitingForIdle {
		// waiting_for_idle is a stable, non-active intent state. Exact retries
		// re-drive the authoritative Run fence without burning Workspace
		// revisions when the Run is still busy.
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true}, nil
	}
	return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true, Completed: record.Completed}, nil
}

func reduceWorkspaceDeletionRunBusy(current *workspacemodel.Workspace, fact workspaceDeletionRunBusyFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	if record.Phase == workspacemodel.WorkspaceDeletionPhaseWaitingForIdle {
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true}, nil
	}
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseRequested || record.RunDrainingAck != nil {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.Phase = workspacemodel.WorkspaceDeletionPhaseWaitingForIdle
	record.Revision++
	record.UpdatedAt = workspaceDeletionTransitionTime(record, fact.OccurredAt)
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next, Completed: record.Completed}, nil
}

func reduceWorkspaceDeletionRunDraining(current *workspacemodel.Workspace, fact workspaceDeletionRunDrainingFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	ack := workspacemodel.WorkspaceDeletionRunDrainingAck{FenceRevision: fact.FenceRevision, WorkspaceRunGeneration: fact.WorkspaceRunGeneration}
	if record.RunDrainingAck != nil {
		if *record.RunDrainingAck != ack {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true, Completed: record.Completed}, nil
	}
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseRequested && record.Phase != workspacemodel.WorkspaceDeletionPhaseWaitingForIdle {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.RunDrainingAck = &ack
	record.Phase = workspacemodel.WorkspaceDeletionPhaseRunDraining
	record.Revision++
	record.UpdatedAt = workspaceDeletionTransitionTime(record, fact.OccurredAt)
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next}, nil
}

func reduceWorkspaceDeletionRuntimeQuiescence(current *workspacemodel.Workspace, fact workspaceDeletionRuntimeQuiescenceFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	ack := workspacemodel.WorkspaceDeletionRuntimeQuiescenceAck{Token: deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(fact.Token)}
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: current.ID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
		DataDispositionIntent: record.Mode, WorkspaceIntentHash: record.IntentHash,
		RunDrainingRevision: record.RunDrainingAck.FenceRevision, WorkspaceRunGeneration: record.RunDrainingAck.WorkspaceRunGeneration,
	}
	if record.RunDrainingAck == nil || ack.Token.ValidateFor(intent) != nil {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	if record.RuntimeQuiescenceAck != nil {
		if reflect.DeepEqual(record.RuntimeQuiescenceAck.Token.Normalized(), ack.Token.Normalized()) {
			return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true, Completed: record.Completed}, nil
		}
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseRunDraining {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.RuntimeQuiescenceAck = &ack
	record.Revision++
	record.UpdatedAt = workspaceDeletionTransitionTime(record, fact.OccurredAt)
	record.Phase = workspacemodel.WorkspaceDeletionPhaseRuntimeQuiescent
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next, Completed: record.Completed}, nil
}

func reduceWorkspaceDeletionRunRetired(current *workspacemodel.Workspace, fact workspaceDeletionRunRetiredFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	ack := workspacemodel.WorkspaceDeletionRunRetiredAck{
		FenceRevision: fact.FenceRevision, WorkspaceRunGeneration: fact.WorkspaceRunGeneration,
		QuiescenceID: strings.TrimSpace(fact.QuiescenceID), RuntimeInventoryHash: strings.TrimSpace(fact.RuntimeInventoryHash),
	}
	if record.RunRetiredAck != nil {
		if *record.RunRetiredAck != ack {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true, Completed: record.Completed}, nil
	}
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseRuntimeQuiescent || record.RunDrainingAck == nil || record.RuntimeQuiescenceAck == nil ||
		ack.FenceRevision != record.RunDrainingAck.FenceRevision+1 || ack.WorkspaceRunGeneration != record.RunDrainingAck.WorkspaceRunGeneration ||
		ack.QuiescenceID != record.RuntimeQuiescenceAck.Token.QuiescenceID || ack.RuntimeInventoryHash != record.RuntimeQuiescenceAck.Token.RuntimeInventoryHash {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.RunRetiredAck = &ack
	record.Phase = workspacemodel.WorkspaceDeletionPhaseRunRetired
	record.Revision++
	record.UpdatedAt = workspaceDeletionTransitionTime(record, fact.OccurredAt)
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next}, nil
}

func reduceWorkspaceDeletionOwnerSealing(current *workspacemodel.Workspace, fact workspaceDeletionOwnerSealingFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	if record.Phase == workspacemodel.WorkspaceDeletionPhaseOwnerSealing {
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true}, nil
	}
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseRunRetired || record.RunRetiredAck == nil {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.Phase = workspacemodel.WorkspaceDeletionPhaseOwnerSealing
	record.Revision++
	record.UpdatedAt = workspaceDeletionTransitionTime(record, fact.OccurredAt)
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next}, nil
}

func reduceWorkspaceDeletionOwnerSeal(current *workspacemodel.Workspace, fact workspaceDeletionOwnerSealFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	observedAt := workspaceDeletionTransitionTime(record, fact.OccurredAt)
	ack := workspacemodel.WorkspaceDeletionOwnerSealAck{OwnerModuleID: fact.OwnerModuleID, FenceID: fact.FenceID, Revision: fact.Revision, AckHash: fact.AckHash, SealedAt: observedAt}
	if existing, ok := workspaceDeletionOwnerSeal(record.OwnerSealAcks, fact.OwnerModuleID); ok {
		if existing.OwnerModuleID != ack.OwnerModuleID || existing.FenceID != ack.FenceID || existing.Revision != ack.Revision || existing.AckHash != ack.AckHash {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true}, nil
	}
	allowed := false
	if record.Phase == workspacemodel.WorkspaceDeletionPhaseOwnerSealing {
		allowed = len(record.OwnerSealAcks) < len(workspaceDeletionPreCleanupSealOwners) && fact.OwnerModuleID == workspaceDeletionPreCleanupSealOwners[len(record.OwnerSealAcks)]
	} else if record.Phase == workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing {
		allowed = len(record.OwnerSealAcks) == len(workspaceDeletionPreCleanupSealOwners) && fact.OwnerModuleID == workspaceDeletionSealOwners[len(workspaceDeletionSealOwners)-1]
	}
	if !allowed {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.OwnerSealAcks = append(record.OwnerSealAcks, ack)
	sortWorkspaceDeletionOwnerSeals(record.OwnerSealAcks)
	if workspaceDeletionOwnersComplete(workspaceDeletionPreCleanupSealOwners, workspaceDeletionSealOwnerIDs(record.OwnerSealAcks)) && record.Phase == workspacemodel.WorkspaceDeletionPhaseOwnerSealing {
		if record.Mode == deletioncontract.ModeRetain {
			record.Phase = workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing
		} else {
			record.Phase = workspacemodel.WorkspaceDeletionPhaseOwnerPurging
		}
	}
	if workspaceDeletionOwnersComplete(workspaceDeletionSealOwners, workspaceDeletionSealOwnerIDs(record.OwnerSealAcks)) && record.Phase == workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing && record.Mode == deletioncontract.ModeRetain {
		record.Phase = workspacemodel.WorkspaceDeletionPhaseCompleted
		record.Completed = true
		record.CompletedAt = observedAt
	}
	record.Revision++
	record.UpdatedAt = observedAt
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next, Completed: record.Completed}, nil
}

func reduceWorkspaceDeletionOwnerPurge(current *workspacemodel.Workspace, fact workspaceDeletionOwnerPurgeFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	observedAt := workspaceDeletionTransitionTime(record, fact.OccurredAt)
	ack := workspacemodel.WorkspaceDeletionOwnerPurgeAck{OwnerModuleID: fact.OwnerModuleID, PurgeID: fact.PurgeID, Revision: fact.Revision, AckHash: fact.AckHash, PurgeProofHash: fact.PurgeProofHash, PurgedAt: observedAt}
	if existing, ok := workspaceDeletionOwnerPurge(record.OwnerPurgeAcks, fact.OwnerModuleID); ok {
		if existing.OwnerModuleID != ack.OwnerModuleID || existing.PurgeID != ack.PurgeID || existing.Revision != ack.Revision || existing.AckHash != ack.AckHash || existing.PurgeProofHash != ack.PurgeProofHash {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true}, nil
	}
	allowed := false
	if record.Phase == workspacemodel.WorkspaceDeletionPhaseOwnerPurging {
		allowed = len(record.OwnerPurgeAcks) < len(workspaceDeletionPreEventStreamPurgeOwners) && fact.OwnerModuleID == workspaceDeletionPreEventStreamPurgeOwners[len(record.OwnerPurgeAcks)]
	} else if record.Phase == workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing {
		allowed = len(record.OwnerPurgeAcks) == len(workspaceDeletionPreEventStreamPurgeOwners) && fact.OwnerModuleID == workspaceDeletionPurgeOwners[len(workspaceDeletionPurgeOwners)-1]
	}
	if !allowed {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	record.OwnerPurgeAcks = append(record.OwnerPurgeAcks, ack)
	sortWorkspaceDeletionOwnerPurges(record.OwnerPurgeAcks)
	if workspaceDeletionOwnersComplete(workspaceDeletionPreEventStreamPurgeOwners, workspaceDeletionPurgeOwnerIDs(record.OwnerPurgeAcks)) && record.Phase == workspacemodel.WorkspaceDeletionPhaseOwnerPurging {
		record.Phase = workspacemodel.WorkspaceDeletionPhaseDirectoryCleanup
	}
	if workspaceDeletionOwnersComplete(workspaceDeletionPurgeOwners, workspaceDeletionPurgeOwnerIDs(record.OwnerPurgeAcks)) && record.Phase == workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing {
		record.Phase = workspacemodel.WorkspaceDeletionPhaseCompleted
		record.Completed = true
		record.CompletedAt = observedAt
	}
	record.Revision++
	record.UpdatedAt = observedAt
	next := *current
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next, Completed: record.Completed}, nil
}

func reduceWorkspaceDeletionDirectoryCleanup(current *workspacemodel.Workspace, fact workspaceDeletionDirectoryCleanupFact) (workspaceDeletionTransition, error) {
	record, err := currentWorkspaceDeletionForFact(current, fact.WorkspaceID, fact.DeletionID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	if record.DirectoryCleanupAck != nil {
		if record.DirectoryCleanupAck.CleanupID != fact.CleanupID {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		return workspaceDeletionTransition{Next: current, Noop: true, Duplicate: true, Completed: record.Completed}, nil
	}
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseDirectoryCleanup || !workspaceDeletionOwnersComplete(workspaceDeletionPreEventStreamPurgeOwners, workspaceDeletionPurgeOwnerIDs(record.OwnerPurgeAcks)) || fact.CleanupID != stableWorkspaceDeletionCleanupID(record) {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	observedAt := workspaceDeletionTransitionTime(record, fact.OccurredAt)
	record.DirectoryCleanupAck = &workspacemodel.WorkspaceDeletionDirectoryCleanupAck{CleanupID: fact.CleanupID, CleanedAt: observedAt}
	record.Phase = workspacemodel.WorkspaceDeletionPhaseEventStreamFinalizing
	record.Revision++
	record.UpdatedAt = observedAt
	next := *current
	// Purge removes ordinary Workspace configuration material, but retains the
	// deletion journal and every other Workspace-owned state-machine record in
	// the durable tombstone. Definition-admission and lifecycle state can only be
	// changed by their own owner-private reducers; completed deletion hides the
	// entire tombstone from public reads.
	next.Title = ""
	next.Description = ""
	next.DeliverableDirectory = ""
	next.InternalDirectory = ""
	next.TemplateID = ""
	next.ProfileID = ""
	next.DefaultLLMSelection = nil
	next.DefaultSoulID = ""
	next.SoulBindings = nil
	next.ContextHints = workorcheventpayload.Payload{}
	next.EvidencePolicies = nil
	next.Deletion = workspaceDeletionRecordPtr(record)
	return workspaceDeletionTransition{Next: &next}, nil
}

func (m *Biz) submitWorkspaceDeletionInternalEvent(ctx context.Context, event workspaceDeletionInternalEvent) (workspaceDeletionTransition, error) {
	if m == nil || m.workspaces == nil {
		return workspaceDeletionTransition{}, common.ErrInvalidArgument
	}
	event, err := normalizeWorkspaceDeletionInternalEvent(event)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	if event.Kind == workspaceDeletionInternalEventDirectoryCleanupObserved {
		return workspaceDeletionTransition{}, common.ErrConflict
	}
	workspaceID := workspaceDeletionEventWorkspaceID(event)
	boundCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return workspaceDeletionTransition{}, err
	}
	if event.Kind == workspaceDeletionInternalEventDirectoryCleanupRequested {
		current, err := m.workspaces.Get(boundCtx, workspaceID)
		if err != nil {
			return workspaceDeletionTransition{}, err
		}
		record, err := currentWorkspaceDeletionForFact(current, event.DirectoryCleanup.WorkspaceID, event.DirectoryCleanup.DeletionID)
		if err != nil || record.Phase != workspacemodel.WorkspaceDeletionPhaseDirectoryCleanup ||
			!workspaceDeletionOwnersComplete(workspaceDeletionPreEventStreamPurgeOwners, workspaceDeletionPurgeOwnerIDs(record.OwnerPurgeAcks)) ||
			event.DirectoryCleanup.CleanupID != stableWorkspaceDeletionCleanupID(record) {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		if err := m.cleanupWorkspaceOwnedDeletionMaterial(boundCtx, record, workspaceID); err != nil {
			return workspaceDeletionTransition{}, err
		}
		if m.cleaner == nil {
			return workspaceDeletionTransition{}, common.ErrInvalidArgument
		}
		if err := m.cleaner.CleanupDeletionMaterial(boundCtx, repository.WorkspaceDeletionMaterialCleanupScope{
			WorkspaceID: workspaceID, DeletionID: record.DeletionID, DeletionGeneration: record.DeletionGeneration,
			IntentHash: record.IntentHash, ExpectedWorkspaceRevision: current.Revision,
			ExpectedDeletionRevision: record.Revision, CleanupID: event.DirectoryCleanup.CleanupID,
		}); err != nil {
			return workspaceDeletionTransition{}, err
		}
		event = workspaceDeletionInternalEvent{
			Kind: workspaceDeletionInternalEventDirectoryCleanupObserved,
			DirectoryCleanup: &workspaceDeletionDirectoryCleanupFact{
				WorkspaceID: workspaceID, DeletionID: record.DeletionID,
				CleanupID: event.DirectoryCleanup.CleanupID, OccurredAt: time.Now().UTC(),
			},
		}
	}
	for attempt := 0; attempt < workspaceDeletionCASMaxAttempts; attempt++ {
		current, err := m.workspaces.Get(boundCtx, workspaceID)
		if err != nil {
			return workspaceDeletionTransition{}, err
		}
		transition, err := reduceWorkspaceDeletionInternalEvent(current, event)
		if err != nil {
			return workspaceDeletionTransition{}, err
		}
		if transition.Noop {
			return transition, nil
		}
		if transition.Next == nil {
			return workspaceDeletionTransition{}, common.ErrInvalidArgument
		}
		if transition.Next.Deletion == nil || transition.Next.Deletion.Validate(transition.Next.ID) != nil {
			return workspaceDeletionTransition{}, common.ErrConflict
		}
		if err := m.workspaces.CompareAndSwap(boundCtx, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceDeletionTransition{}, err
		}
		return transition, nil
	}
	return workspaceDeletionTransition{}, common.ErrConflict
}

func workspaceDeletionEventWorkspaceID(event workspaceDeletionInternalEvent) common.WorkspaceID {
	switch event.Kind {
	case workspaceDeletionInternalEventRequested:
		return event.Requested.Command.WorkspaceID
	case workspaceDeletionInternalEventRunBusyObserved:
		return event.RunBusy.WorkspaceID
	case workspaceDeletionInternalEventRunDrainingObserved:
		return event.RunDraining.WorkspaceID
	case workspaceDeletionInternalEventRuntimeQuiescenceObserved:
		return event.RuntimeQuiescence.WorkspaceID
	case workspaceDeletionInternalEventRunRetiredObserved:
		return event.RunRetired.WorkspaceID
	case workspaceDeletionInternalEventOwnerSealingStarted:
		return event.OwnerSealing.WorkspaceID
	case workspaceDeletionInternalEventOwnerSealObserved:
		return event.OwnerSeal.WorkspaceID
	case workspaceDeletionInternalEventOwnerPurgeObserved:
		return event.OwnerPurge.WorkspaceID
	case workspaceDeletionInternalEventDirectoryCleanupRequested, workspaceDeletionInternalEventDirectoryCleanupObserved:
		return event.DirectoryCleanup.WorkspaceID
	default:
		return ""
	}
}

func currentWorkspaceDeletionForFact(current *workspacemodel.Workspace, workspaceID common.WorkspaceID, deletionID string) (workspacemodel.WorkspaceDeletionRecord, error) {
	if current == nil || current.ID != workspaceID || current.Deletion == nil || current.Deletion.DeletionID != strings.TrimSpace(deletionID) {
		return workspacemodel.WorkspaceDeletionRecord{}, common.ErrConflict
	}
	return current.Deletion.Clone(), nil
}

func workspaceDeletionRecordPtr(value workspacemodel.WorkspaceDeletionRecord) *workspacemodel.WorkspaceDeletionRecord {
	cloned := value.Clone()
	return &cloned
}

func workspaceDeletionTransitionTime(record workspacemodel.WorkspaceDeletionRecord, occurredAt time.Time) time.Time {
	observedAt := occurredAt.UTC()
	if observedAt.Before(record.UpdatedAt) {
		return record.UpdatedAt
	}
	return observedAt
}

func workspaceDeletionOwnerAllowed(owner string, values []string) bool {
	owner = strings.TrimSpace(owner)
	for _, value := range values {
		if owner == value {
			return true
		}
	}
	return false
}

func workspaceDeletionOwnerSeal(values []workspacemodel.WorkspaceDeletionOwnerSealAck, owner string) (workspacemodel.WorkspaceDeletionOwnerSealAck, bool) {
	for _, item := range values {
		if item.OwnerModuleID == owner {
			return item, true
		}
	}
	return workspacemodel.WorkspaceDeletionOwnerSealAck{}, false
}

func workspaceDeletionOwnerPurge(values []workspacemodel.WorkspaceDeletionOwnerPurgeAck, owner string) (workspacemodel.WorkspaceDeletionOwnerPurgeAck, bool) {
	for _, item := range values {
		if item.OwnerModuleID == owner {
			return item, true
		}
	}
	return workspacemodel.WorkspaceDeletionOwnerPurgeAck{}, false
}

func workspaceDeletionSealOwnerIDs(values []workspacemodel.WorkspaceDeletionOwnerSealAck) []string {
	ret := make([]string, 0, len(values))
	for _, item := range values {
		ret = append(ret, item.OwnerModuleID)
	}
	return ret
}

func workspaceDeletionPurgeOwnerIDs(values []workspacemodel.WorkspaceDeletionOwnerPurgeAck) []string {
	ret := make([]string, 0, len(values))
	for _, item := range values {
		ret = append(ret, item.OwnerModuleID)
	}
	return ret
}

func workspaceDeletionOwnersComplete(required []string, observed []string) bool {
	set := map[string]struct{}{}
	for _, item := range observed {
		set[item] = struct{}{}
	}
	for _, item := range required {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func sortWorkspaceDeletionOwnerSeals(values []workspacemodel.WorkspaceDeletionOwnerSealAck) {
	sort.Slice(values, func(i, j int) bool {
		return workspaceDeletionOwnerOrder(values[i].OwnerModuleID, workspaceDeletionSealOwners) < workspaceDeletionOwnerOrder(values[j].OwnerModuleID, workspaceDeletionSealOwners)
	})
}

func sortWorkspaceDeletionOwnerPurges(values []workspacemodel.WorkspaceDeletionOwnerPurgeAck) {
	sort.Slice(values, func(i, j int) bool {
		return workspaceDeletionOwnerOrder(values[i].OwnerModuleID, workspaceDeletionPurgeOwners) < workspaceDeletionOwnerOrder(values[j].OwnerModuleID, workspaceDeletionPurgeOwners)
	})
}

func workspaceDeletionOwnerOrder(owner string, values []string) int {
	for idx, item := range values {
		if owner == item {
			return idx
		}
	}
	return len(values)
}

func stableWorkspaceDeletionCleanupID(record workspacemodel.WorkspaceDeletionRecord) string {
	return workspacemodel.StableWorkspaceDeletionCleanupID(record)
}
