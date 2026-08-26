package events

import (
	"errors"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletedDataValidatesStableCompletedReceipt(t *testing.T) {
	data := WorkspaceDeletedData{
		WorkspaceID:       "ws-deleted-receipt",
		RequestID:         "request-delete",
		Mode:              deletioncontract.ModeRetain,
		Disposition:       DeleteWorkspaceCompleted,
		DeletionRevision:  9,
		WorkspaceRevision: 12,
		CompletedAt:       time.Date(2026, 8, 26, 1, 2, 3, 0, time.UTC),
	}
	data.DeletionID = StableWorkspaceDeletionID(data.WorkspaceID, data.RequestID)
	if err := data.Validate(); err != nil {
		t.Fatalf("valid completed receipt: %v", err)
	}

	for name, mutate := range map[string]func(*WorkspaceDeletedData){
		"forged-deletion-id": func(value *WorkspaceDeletedData) { value.DeletionID = "forged" },
		"duplicate-disposition": func(value *WorkspaceDeletedData) {
			value.Disposition = DeleteWorkspaceDuplicate
		},
		"missing-completed-at": func(value *WorkspaceDeletedData) { value.CompletedAt = time.Time{} },
		"unknown-mode":         func(value *WorkspaceDeletedData) { value.Mode = deletioncontract.Mode("archive") },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := data
			mutate(&invalid)
			if err := invalid.Validate(); err == nil || (!errors.Is(err, common.ErrConflict) && !errors.Is(err, common.ErrInvalidArgument)) {
				t.Fatalf("invalid receipt err=%v", err)
			}
		})
	}
}
