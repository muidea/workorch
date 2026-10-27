package events

import (
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationHistoryContractRequiresExactScopeIdentityAndCompleteShape(t *testing.T) {
	c := ObserveWorkspacePreparationHistoryCommand{WorkspaceID: "ws", RequestID: "check"}
	r := ObserveWorkspacePreparationHistoryResult{RequestID: c.RequestID, ObservedAt: time.Now().UTC(), History: runmodel.WorkspacePreparationHistory{WorkspaceID: c.WorkspaceID}}
	if r.ValidateFor(c) != nil {
		t.Fatal("empty complete observation rejected")
	}
	for _, scope := range []string{"", ".", "..", "ws/other", "ws\\other", " ws", "ws\n", "ws\x00", string([]byte{0xff})} {
		bad := c
		bad.WorkspaceID = common.WorkspaceID(scope)
		if bad.Validate() == nil {
			t.Fatalf("invalid scope accepted: %q", scope)
		}
	}
	for _, request := range []string{"", " check", "check\r", string([]byte{0xff})} {
		bad := c
		bad.RequestID = request
		if bad.Validate() == nil {
			t.Fatal("invalid observation identity accepted")
		}
	}
	for name, mutate := range map[string]func(*ObserveWorkspacePreparationHistoryResult){
		"scope":       func(v *ObserveWorkspacePreparationHistoryResult) { v.History.WorkspaceID = "other" },
		"request":     func(v *ObserveWorkspacePreparationHistoryResult) { v.RequestID = "other" },
		"time":        func(v *ObserveWorkspacePreparationHistoryResult) { v.ObservedAt = time.Time{} },
		"head":        func(v *ObserveWorkspacePreparationHistoryResult) { v.History.RunCount = 1 },
		"pending":     func(v *ObserveWorkspacePreparationHistoryResult) { v.History.PendingActivationCount = 1 },
		"replacement": func(v *ObserveWorkspacePreparationHistoryResult) { v.History.ReplacementCount = 1 },
		"retirement": func(v *ObserveWorkspacePreparationHistoryResult) {
			v.History.Retirement = &runmodel.WorkspaceRetirementFence{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := r.Clone()
			mutate(&bad)
			if bad.ValidateFor(c) == nil {
				t.Fatal("incomplete observation accepted")
			}
		})
	}
	// A nonzero head survives deletion and may never be collapsed to empty
	// history merely because there are no currently visible Run records.
	r.History.GenerationHead = 7
	if r.ValidateFor(c) != nil || r.Clone().History.GenerationHead != 7 {
		t.Fatal("historical watermark lost")
	}
}
