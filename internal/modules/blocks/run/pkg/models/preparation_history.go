package run

import (
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ValidatePreparationHistoryWorkspaceID(workspaceID common.WorkspaceID) error {
	value := string(workspaceID)
	if value == "" || len(value) > 256 || value == "." || value == ".." || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsAny(value, "/\\\x00\r\n") {
		return common.ErrInvalidArgument
	}
	return nil
}

// WorkspacePreparationHistory is a complete, unfiltered owner-local read at
// one generation-head boundary. Counts include terminal and retired records;
// the durable watermark survives removal of individual Runs. This is only a
// point-in-time fact, never a first-activation admission or execution permit.
type WorkspacePreparationHistory struct {
	WorkspaceID            common.WorkspaceID
	GenerationHead         uint64
	RunCount               uint64
	PendingActivationCount uint64
	ReplacementCount       uint64
	Retirement             *WorkspaceRetirementFence
}

func (h WorkspacePreparationHistory) Clone() WorkspacePreparationHistory {
	if h.Retirement != nil {
		value := *h.Retirement
		h.Retirement = &value
	}
	return h
}

func (h WorkspacePreparationHistory) Validate() error {
	if ValidatePreparationHistoryWorkspaceID(h.WorkspaceID) != nil || h.RunCount > h.GenerationHead || h.PendingActivationCount > h.RunCount || h.ReplacementCount > h.RunCount {
		return common.ErrConflict
	}
	if h.Retirement != nil && (h.Retirement.Validate() != nil || h.Retirement.WorkspaceID != h.WorkspaceID || h.Retirement.WorkspaceRunGeneration != h.GenerationHead) {
		return common.ErrConflict
	}
	return nil
}

// CompileWorkspacePreparationHistory must receive the complete inventory and
// watermark from the same repository read lock. It rejects inconsistent or
// duplicate coordinates rather than silently filtering malformed records.
func CompileWorkspacePreparationHistory(workspaceID common.WorkspaceID, head uint64, retirement *WorkspaceRetirementFence, items []*Run) (WorkspacePreparationHistory, error) {
	h := WorkspacePreparationHistory{WorkspaceID: workspaceID, GenerationHead: head, Retirement: retirement}
	ids, generations := map[common.RunID]bool{}, map[uint64]bool{}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || item.ID == "" || item.Revision == 0 || item.WorkspaceRunGeneration == 0 || item.WorkspaceRunGeneration > head || ids[item.ID] || generations[item.WorkspaceRunGeneration] || ValidateRunRuntimeBootstrapsRaw(item) != nil {
			return WorkspacePreparationHistory{}, common.ErrConflict
		}
		ids[item.ID], generations[item.WorkspaceRunGeneration] = true, true
		switch item.State {
		case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		default:
			return WorkspacePreparationHistory{}, common.ErrConflict
		}
		h.RunCount++
		if item.State == common.RunStatePending {
			h.PendingActivationCount++
		}
		if item.ReplacementIntent != nil {
			if !item.ReplacementIntent.ValidFor(item) {
				return WorkspacePreparationHistory{}, common.ErrConflict
			}
			h.ReplacementCount++
		}
	}
	if err := h.Validate(); err != nil {
		return WorkspacePreparationHistory{}, err
	}
	return h.Clone(), nil
}
