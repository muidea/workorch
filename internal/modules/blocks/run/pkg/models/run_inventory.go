package run

import (
	"github.com/muidea/workorch/internal/pkg/common"
	memorycontract "github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RunInventoryItem is detached SM-1 metadata, projected under the repository
// lock before any Bootstrap or opaque Runtime graph is copied. It is neither
// an aggregate nor a source from which execution authority can be rebuilt.
type RunInventoryItem struct {
	Revision                  uint64
	WorkspaceRunGeneration    uint64
	TerminalResultID          common.ResultID
	ID                        common.RunID
	WorkspaceID               common.WorkspaceID
	GoalID                    common.GoalID
	DoDID                     common.DoDID
	Lineage                   RunLineage
	DefinitionRevision        string
	StartupInputHash          string
	InitialStepKind           string
	State                     common.RunState
	BlockingTicketID          common.TicketID
	BlockingApprovalID        common.ApprovalID
	FailureReason             string
	CreatedAt                 string
	StartedAt                 string
	EndedAt                   string
	ActivationRequestID       string
	ReconciliationRequestID   string
	ReconciliationScope       rc.RuntimeCWIReconciliationScope
	HasRuntimeBootstrap       bool
	HasRuntimeState           bool
	RuntimeStateRevision      uint64
	ReplacementState          ReplacementIntentState
	ReplacementRecoveryState  ReplacementRecoveryState
	ActivationMemorySelection *memorycontract.RecallSelectionEvidence
	SealedLLMSelection        *rc.ProviderInferenceSelection
}

func InventoryFromRun(r *Run) RunInventoryItem {
	v := RunInventoryItem{
		Revision: r.Revision, WorkspaceRunGeneration: r.WorkspaceRunGeneration,
		ID: r.ID, WorkspaceID: r.WorkspaceID, GoalID: r.GoalID, DoDID: r.DoDID, TerminalResultID: r.TerminalResultID,
		Lineage: r.Lineage, DefinitionRevision: r.DefinitionRevision, StartupInputHash: r.StartupInputHash,
		InitialStepKind: r.InitialStepKind, State: r.State, BlockingTicketID: r.BlockingTicketID,
		BlockingApprovalID: r.BlockingApprovalID, FailureReason: r.FailureReason, CreatedAt: r.CreatedAt,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt, ActivationRequestID: r.ActivationRequestID,
		ReconciliationRequestID: r.ReconciliationRequestID, ReconciliationScope: r.ReconciliationScope,
		HasRuntimeBootstrap: r.RuntimeBootstrap != nil, HasRuntimeState: r.RuntimeState != nil,
	}
	if r.RuntimeBootstrap != nil {
		v.ActivationMemorySelection = memorycontract.CloneRecallSelectionEvidence(r.RuntimeBootstrap.Activation.MemorySelection)
		selection := r.RuntimeBootstrap.Activation.ProviderSelection.Normalized()
		if selection.Validate() == nil {
			v.SealedLLMSelection = &selection
		}
	}
	if r.RuntimeState != nil {
		v.RuntimeStateRevision = r.RuntimeState.StateRevision
	}
	if r.ReplacementIntent != nil {
		v.ReplacementState = r.ReplacementIntent.State
		v.ReplacementRecoveryState = r.ReplacementIntent.Recovery.State
	}
	return v
}

// ListItem returns only fields promised by the compact list contract. It must
// never be accepted as an execution/recovery aggregate.
func (v RunInventoryItem) ListItem() *Run {
	return &Run{ID: v.ID, WorkspaceID: v.WorkspaceID, GoalID: v.GoalID, DoDID: v.DoDID,
		Revision: v.Revision, WorkspaceRunGeneration: v.WorkspaceRunGeneration, TerminalResultID: v.TerminalResultID,
		Lineage: v.Lineage, DefinitionRevision: v.DefinitionRevision, StartupInputHash: v.StartupInputHash,
		InitialStepKind: v.InitialStepKind, State: v.State, BlockingTicketID: v.BlockingTicketID,
		BlockingApprovalID: v.BlockingApprovalID, FailureReason: v.FailureReason, CreatedAt: v.CreatedAt,
		StartedAt: v.StartedAt, EndedAt: v.EndedAt}
}

// Clone detaches pointer fields before crossing an owner read boundary.
func (v RunInventoryItem) Clone() RunInventoryItem {
	v.ActivationMemorySelection = memorycontract.CloneRecallSelectionEvidence(v.ActivationMemorySelection)
	if v.SealedLLMSelection != nil {
		s := *v.SealedLLMSelection
		v.SealedLLMSelection = &s
	}
	return v
}
func CloneRunInventory(items []RunInventoryItem) []RunInventoryItem {
	if items == nil {
		return nil
	}
	ret := make([]RunInventoryItem, len(items))
	for i, v := range items {
		ret[i] = v.Clone()
	}
	return ret
}

// InventoryListItems projects display metadata only. Never use these values for
// execution or recovery; selected aggregates must be read from their owner.
func InventoryListItems(items []RunInventoryItem) []*Run {
	ret := make([]*Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ListItem())
	}
	return ret
}
