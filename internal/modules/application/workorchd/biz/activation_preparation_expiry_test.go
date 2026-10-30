package biz

import (
	"reflect"
	"testing"
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationResponseCompilationCannotReturnElapsedCompletionAsObserved(t *testing.T) {
	at := time.Now().UTC()
	w := &workspace.Workspace{StartupPreparation: &workspace.WorkspacePreparationRecord{
		Attempts:    []workspace.WorkspacePreparationAttempt{{RequestID: "original"}},
		Completions: []workspace.WorkspacePreparationCompletion{{RequestID: "original", CompletedAt: at, ExpiresAt: at.Add(30 * time.Second)}},
	}}
	original := w.StartupPreparation.Clone()
	for _, validity := range []string{"observed", "not_evaluated", "stale"} {
		view := management.StartupPreparationView{CurrentValidity: validity}
		if validity == "not_evaluated" {
			view.CurrentValidityReason = "file_epoch_unavailable"
		}
		if validity == "stale" {
			view.CurrentValidityReason = "binding_changed"
		}
		if err := projectPreparationResponseExpiry(&view, w, at.Add(30*time.Second)); err != nil {
			t.Fatal(err)
		}
		if view.CurrentValidity != "stale" || validity != "stale" && view.CurrentValidityReason != "completion_outside_window" || validity == "stale" && view.CurrentValidityReason != "binding_changed" {
			t.Fatal("response renewed expiry, hid known drift or preserved expired observed projection", view)
		}
	}
	if !reflect.DeepEqual(original, *w.StartupPreparation) {
		t.Fatal("response changed original history")
	}
}
