package models

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

// WorkspaceDeletionPhase is Orchestrator-owner private deletion authority.
// Cross-owner commands provide immutable facts only and cannot select it.
type WorkspaceDeletionPhase string

const (
	WorkspaceDeletionPhaseQuiescing WorkspaceDeletionPhase = "quiescing"
	WorkspaceDeletionPhaseQuiesced  WorkspaceDeletionPhase = "quiesced"
	WorkspaceDeletionPhaseFenced    WorkspaceDeletionPhase = "fenced"
	WorkspaceDeletionPhasePurged    WorkspaceDeletionPhase = "purged"
)

// WorkspaceDeletion is the permanent Orchestrator-owned evidence fence.  It is
// retained after purge so delayed SM-2/3/4 writes and fresh-process recovery
// can never recreate evidence for a retired Workspace.
type WorkspaceDeletion struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	DeletionGeneration     uint64
	Mode                   deletioncontract.Mode
	WorkspaceIntentHash    string
	RunDrainingRevision    uint64
	WorkspaceRunGeneration uint64
	QuiescingGateID        string
	QuiescenceID           string
	RequestHash            string
	QuiescenceToken        *deletioncontract.WorkspaceRuntimeQuiescenceToken
	FenceID                string
	PurgeID                string
	Phase                  WorkspaceDeletionPhase
	Revision               uint64
	SealAckHash            string
	PurgeAckHash           string
	PurgeProofHash         string
	RequestedAt            string
	UpdatedAt              string
	PurgedAt               string
}

func CloneWorkspaceDeletion(item *WorkspaceDeletion) *WorkspaceDeletion {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.QuiescenceToken != nil {
		token := deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(*item.QuiescenceToken)
		cloned.QuiescenceToken = &token
	}
	return &cloned
}

func (d WorkspaceDeletion) QuiescenceIntent() deletioncontract.WorkspaceRuntimeQuiescenceIntent {
	return deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: d.WorkspaceID, DeletionID: d.DeletionID, DeletionGeneration: d.DeletionGeneration,
		DataDispositionIntent: d.Mode, WorkspaceIntentHash: d.WorkspaceIntentHash,
		RunDrainingRevision: d.RunDrainingRevision, WorkspaceRunGeneration: d.WorkspaceRunGeneration,
	}.Normalized()
}

func (d WorkspaceDeletion) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{
		WorkspaceID: d.WorkspaceID, DeletionID: d.DeletionID, DeletionGeneration: d.DeletionGeneration,
		Mode: d.Mode, QuiescenceID: d.QuiescenceID, RequestHash: d.RequestHash,
	}.Normalize()
}

func (d WorkspaceDeletion) Validate() error {
	if err := d.QuiescenceIntent().Validate(); err != nil {
		return err
	}
	if d.Revision == 0 || strings.TrimSpace(d.RequestedAt) == "" || strings.TrimSpace(d.UpdatedAt) == "" {
		return common.ErrInvalidArgument
	}
	switch d.Phase {
	case WorkspaceDeletionPhaseQuiescing:
		if d.Revision != 1 || strings.TrimSpace(d.QuiescingGateID) == "" || d.QuiescenceToken != nil || d.QuiescenceID != "" || d.RequestHash != "" || d.FenceID != "" || d.SealAckHash != "" || d.PurgeID != "" || d.PurgeAckHash != "" || d.PurgeProofHash != "" || d.PurgedAt != "" {
			return common.ErrInvalidArgument
		}
	case WorkspaceDeletionPhaseQuiesced:
		if d.Revision != 2 || strings.TrimSpace(d.QuiescingGateID) == "" || d.QuiescenceToken == nil || d.QuiescenceToken.ValidateFor(d.QuiescenceIntent()) != nil || d.QuiescenceID != d.QuiescenceToken.QuiescenceID || d.Scope().Validate() != nil || d.FenceID != "" || d.SealAckHash != "" || d.PurgeID != "" || d.PurgeAckHash != "" || d.PurgeProofHash != "" || d.PurgedAt != "" {
			return common.ErrInvalidArgument
		}
	case WorkspaceDeletionPhaseFenced:
		if d.Revision != 3 || d.QuiescenceToken == nil || d.QuiescenceToken.ValidateFor(d.QuiescenceIntent()) != nil || d.Scope().Validate() != nil || strings.TrimSpace(d.FenceID) == "" || strings.TrimSpace(d.SealAckHash) == "" || d.PurgeID != "" || d.PurgeAckHash != "" || d.PurgeProofHash != "" || d.PurgedAt != "" {
			return common.ErrInvalidArgument
		}
	case WorkspaceDeletionPhasePurged:
		if d.Revision != 4 || d.QuiescenceToken == nil || d.QuiescenceToken.ValidateFor(d.QuiescenceIntent()) != nil || d.Scope().Validate() != nil || d.Mode != deletioncontract.ModePurge || strings.TrimSpace(d.FenceID) == "" || strings.TrimSpace(d.SealAckHash) == "" || strings.TrimSpace(d.PurgeID) == "" || strings.TrimSpace(d.PurgeAckHash) == "" || strings.TrimSpace(d.PurgeProofHash) == "" || strings.TrimSpace(d.PurgedAt) == "" {
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	return nil
}
