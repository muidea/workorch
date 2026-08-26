package result

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type WorkspaceDeletionPhase string

const (
	WorkspaceDeletionPhaseFenced WorkspaceDeletionPhase = "fenced"
	WorkspaceDeletionPhasePurged WorkspaceDeletionPhase = "purged"
)

type WorkspaceDeletion struct {
	WorkspaceID        common.WorkspaceID
	DeletionID         string
	DeletionGeneration uint64
	Mode               deletioncontract.Mode
	QuiescenceID       string
	RequestHash        string
	FenceID            string
	PurgeID            string
	Phase              WorkspaceDeletionPhase
	Revision           uint64
	SealAckHash        string
	PurgeAckHash       string
	PurgeProofHash     string
	RequestedAt        string
	UpdatedAt          string
	PurgedAt           string
}

func CloneWorkspaceDeletion(item *WorkspaceDeletion) *WorkspaceDeletion {
	if item == nil {
		return nil
	}
	cloned := *item
	return &cloned
}

func (d WorkspaceDeletion) Scope() deletioncontract.Scope {
	return deletioncontract.Scope{
		WorkspaceID: d.WorkspaceID, DeletionID: d.DeletionID, DeletionGeneration: d.DeletionGeneration,
		Mode: d.Mode, QuiescenceID: d.QuiescenceID, RequestHash: d.RequestHash,
	}.Normalize()
}

func (d WorkspaceDeletion) Validate() error {
	if err := d.Scope().Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(d.FenceID) == "" || d.Revision == 0 || strings.TrimSpace(d.SealAckHash) == "" || strings.TrimSpace(d.RequestedAt) == "" || strings.TrimSpace(d.UpdatedAt) == "" {
		return common.ErrInvalidArgument
	}
	switch d.Phase {
	case WorkspaceDeletionPhaseFenced:
		if d.Revision != 1 || d.PurgeID != "" || d.PurgeAckHash != "" || d.PurgeProofHash != "" || d.PurgedAt != "" {
			return common.ErrInvalidArgument
		}
	case WorkspaceDeletionPhasePurged:
		if d.Revision != 2 || d.Mode != deletioncontract.ModePurge || strings.TrimSpace(d.PurgeID) == "" || strings.TrimSpace(d.PurgeAckHash) == "" || strings.TrimSpace(d.PurgeProofHash) == "" || strings.TrimSpace(d.PurgedAt) == "" {
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
	return nil
}
