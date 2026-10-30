package file

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Opt-in, read-only representative-input regression. Historical timestamps are
// used only for deterministic counterchecks, never for a live execution or a
// renewed owner observation. The archive must include real formal rows and the
// actual Soul sidecar, not reconstructed journal after-images.
func TestPreparationReadOnlyArchiveCompilation(t *testing.T) {
	root := os.Getenv("WORKORCH_PREPARATION_READONLY_ARCHIVE")
	if root == "" {
		t.Skip("requires restricted complete Workspace archive")
	}
	started := time.Now()
	path := filepath.Join(root, "workspace.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := loadWorkspacePersistenceMap(path)
	if err != nil || len(rows) != 1 {
		t.Fatal("complete archive unavailable", err)
	}
	var w workspace.Workspace
	for _, row := range rows {
		w = row
	}
	var bindings struct {
		DefaultSoulID common.SoulID            `json:"defaultSoulId,omitempty"`
		Bindings      map[string]common.SoulID `json:"bindings,omitempty"`
	}
	sidecar, err := os.ReadFile(filepath.Join(root, "config/soul_bindings.json"))
	if err != nil || decodeCurrentJSON(sidecar, &bindings) != nil {
		t.Fatal("actual Soul sidecar missing", err)
	}
	w.DefaultSoulID, w.SoulBindings = bindings.DefaultSoulID, bindings.Bindings
	goals, err := loadMap[common.GoalID, workspace.Goal](filepath.Join(root, "goals.json"))
	if err != nil || len(goals) != 1 {
		t.Fatal("actual Goal missing", err)
	}
	dods, err := loadMap[common.DoDID, workspace.DoD](filepath.Join(root, "dods.json"))
	if err != nil || len(dods) != 1 {
		t.Fatal("actual DoD missing", err)
	}
	var goal workspace.Goal
	var dod workspace.DoD
	for _, v := range goals {
		goal = v
	}
	for _, v := range dods {
		dod = v
	}
	if w.StartupPreparation == nil || len(w.StartupPreparation.Attempts) == 0 {
		t.Fatal("preparation history missing")
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	original, err := preparationcompiler.CompileOriginalPreparedBootstrap(&w, a)
	if err != nil || original.Compilation == nil {
		t.Fatal("original bootstrap unavailable", err)
	}
	before := cloneWorkspace(&w)
	phase := time.Now()
	compiled, err := preparationcompiler.CompileFirstExecutionPreparation(&w, &goal, &dod, original.Compilation.Bootstrap, a.CompletedAt, workspace.PreparationFileObservationEpoch(&w))
	if err != nil || compiled.ExpectedWorkspaceRevision != w.Revision {
		t.Fatal("complete original preimage rejected", err)
	}
	t.Logf("DoD=%d attempts=%d completions=%d first_execution=%s total_read_and_compile=%s", len(dod.Items), len(w.StartupPreparation.Attempts), len(w.StartupPreparation.Completions), time.Since(phase), time.Since(started))
	if time.Since(started) >= 30*time.Second {
		t.Error("read/compile alone exhausted the unchanged original 30s window")
	}
	if err := preparationcompiler.CheckFirstExecutionPreparationWindow(&w, a.CompletedAt.Add(30*time.Second), workspace.PreparationFileObservationEpoch(&w)); err == nil {
		t.Fatal("expired completion passed early rejection")
	}
	// Recheck the clock at persistence after the expensive complete gates.
	next := cloneWorkspace(&w)
	next.PreparationFence.Pending = &workspace.WorkspaceFreshIntentReservation{PreparationConsumedAt: a.CompletedAt}
	files := w.PreparationDirectoryEvidence[len(w.PreparationDirectoryEvidence)-1].Revalidation.Clone()
	next.FirstExecutionPreparationEvidence = &workspace.WorkspaceFirstExecutionPreparationEvidence{Files: files}
	if err := preparationcompiler.ValidateFirstExecutionPreparationCommitTime(&w, next, a.CompletedAt.Add(30*time.Second)); err == nil {
		t.Fatal("late CAS consumed an expired original window")
	}
	// Once committed, historical replay never revalidates against today's age.
	if err := preparationcompiler.ValidateFirstExecutionPreparationCommitTime(next, next, a.CompletedAt.Add(24*time.Hour)); err != nil {
		t.Fatal("historical consumption lost replay", err)
	}
	// A necessary-condition precheck cannot authenticate complete history.
	// Recomputed current hashes must not hide a changed older source.
	bad := cloneWorkspace(&w)
	bad.PreparationSoulEvidence[0].Facts.Souls[0].SoulMD += "changed historical source"
	bad.PreparationSoulEvidence[0].ContentHash = bad.PreparationSoulEvidence[0].Hash()
	if _, err := preparationcompiler.CompileFirstExecutionPreparation(bad, &goal, &dod, original.Compilation.Bootstrap, a.CompletedAt, workspace.PreparationFileObservationEpoch(bad)); err == nil {
		t.Fatal("changed historical raw source accepted")
	}
	// Reusing the admission gate's local source product must still bind the
	// original reviewed DoD order. Self-consistent archive hashes do not prove it.
	bad = cloneWorkspace(&w)
	seal := &bad.PreparationSealEvidence[0]
	seal.Before.FormalDoDItemRefs[0] = "substituted-original-item"
	seal.Before.SnapshotHash = seal.Before.Hash()
	seal.After = seal.Before.Clone()
	seal.ContentHash = seal.Hash()
	if err := workspace.ValidatePreparationEvidencePersistence(bad); err == nil {
		t.Fatal("rehash hid substituted formal DoD lineage")
	}
	other := goal
	other.WorkspaceID = "another-workspace"
	if _, err := preparationcompiler.CompileFirstExecutionPreparation(&w, &other, &dod, original.Compilation.Bootstrap, a.CompletedAt, workspace.PreparationFileObservationEpoch(&w)); err == nil {
		t.Fatal("cross-Workspace formal rows accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(raw, after) || !reflect.DeepEqual(before, &w) {
		t.Fatal("archive/input changed", err)
	}
}
