package workspace

import (
	"context"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	"os"
	"reflect"
	"testing"
	"time"
)

// Opt-in to the isolated production verifier's source archive. This never
// reads live configuration, rewrites an archive or starts an external owner.
func TestPreparationCompletionArchivedProductionSource(t *testing.T) {
	root := os.Getenv("WORKORCH_PREPARATION_ARCHIVE_ROOT")
	if root == "" {
		t.Skip("requires the isolated production HTTP verifier archive")
	}
	repo := workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Workspaces
	items, err := repo.List(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatal("expected isolated single Workspace", err)
	}
	w := items[0]
	if w.ActiveRunID != "" || w.StartupPreparation == nil || len(w.StartupPreparation.Completions) != 2 || preparationcompiler.ValidatePreparedPersistence(w) != nil {
		t.Fatal("complete original success history absent")
	}
	for _, c := range w.StartupPreparation.Completions {
		var a workspace.WorkspacePreparationAttempt
		for _, v := range w.StartupPreparation.Attempts {
			if v.RequestID == c.RequestID {
				a = v
			}
		}
		expected, err := preparationcompiler.CompileCompletion(w, a)
		if err != nil || !reflect.DeepEqual(c, expected) {
			t.Fatal("independent original compilation differs", err)
		}
	}
	// Negative verification uses detached clones only. No forged after-image is
	// written to a repository and no input in the archive is changed.
	for _, kind := range []string{"hash", "manifest", "owner_source", "raw_protocol", "no_run", "assessment", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			bad := *w
			r := w.StartupPreparation.Clone()
			bad.StartupPreparation = &r
			switch kind {
			case "hash":
				r.Completions[0].BootstrapHash = "prepared-bootstrap-sha256:" + string(make([]byte, 64))
			case "manifest":
				r.Completions[0].Binding.ManifestHash = "other"
			case "owner_source":
				bad.PreparationSoulEvidence = nil
			case "raw_protocol":
				bad.PreparationMCPRegistryEvidence = workspace.ClonePreparationMCPRegistryEvidence(w.PreparationMCPRegistryEvidence)
				bad.PreparationMCPRegistryEvidence[0].Connection.Observations[0].Frames[1].Response = []byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`)
				bad.PreparationMCPRegistryEvidence[0].ContentHash = bad.PreparationMCPRegistryEvidence[0].Hash()
			case "no_run":
				r.Attempts[0].NoRunEvaluationVersion = ""
			case "assessment":
				r.Attempts[0].Assessment.Outcome = "blocked"
			case "duplicate":
				r.Completions = append(r.Completions, r.Completions[0])
			}
			bad.StartupPreparation = &r
			if preparationcompiler.ValidatePreparedPersistence(&bad) == nil {
				t.Fatal("forged completion accepted")
			}
		})
	}
	original := w.StartupPreparation.Clone()
	latest := original.Attempts[len(original.Attempts)-1]
	view, err := preparationStateFromPersistence(w, latest.CompletedAt.Add(time.Minute))
	if err != nil || view.State != "completed" || view.CurrentValidity != "stale" || !reflect.DeepEqual(*view.Record, original) {
		t.Fatal("expiry rewrote history or removed M3", err)
	}
}
