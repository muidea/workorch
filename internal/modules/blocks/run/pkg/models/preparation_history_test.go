package run

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationHistoryCompilerRejectsIncompleteCoordinates(t *testing.T) {
	base := &Run{ID: "r", WorkspaceID: "ws", Revision: 1, WorkspaceRunGeneration: 1, State: common.RunStatePending}
	if h, err := CompileWorkspacePreparationHistory("ws", 1, nil, []*Run{base}); err != nil || h.PendingActivationCount != 1 {
		t.Fatalf("pending observation: %+v %v", h, err)
	}
	for name, mutate := range map[string]func(*Run){
		"scope":         func(r *Run) { r.WorkspaceID = "other" },
		"id":            func(r *Run) { r.ID = "" },
		"revision":      func(r *Run) { r.Revision = 0 },
		"generation":    func(r *Run) { r.WorkspaceRunGeneration = 0 },
		"ahead_of_head": func(r *Run) { r.WorkspaceRunGeneration = 2 },
		"unknown":       func(r *Run) { r.State = "unknown" },
		"replacement":   func(r *Run) { r.ReplacementIntent = &ReplacementIntent{} },
	} {
		t.Run(name, func(t *testing.T) {
			item := *base
			mutate(&item)
			if _, err := CompileWorkspacePreparationHistory("ws", 1, nil, []*Run{&item}); err == nil {
				t.Fatal("incomplete inventory accepted")
			}
		})
	}
	for _, items := range [][]*Run{{nil}, {base, base}} {
		if _, err := CompileWorkspacePreparationHistory("ws", 1, nil, items); err == nil {
			t.Fatal("nil/duplicate inventory accepted")
		}
	}
	if h, err := CompileWorkspacePreparationHistory("ws", 4, nil, nil); err != nil || h.GenerationHead != 4 || h.RunCount != 0 {
		t.Fatal("watermark gap must remain historical, not unknown or zero")
	}
}
