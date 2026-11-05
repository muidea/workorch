package orchestrator

import (
	"context"
	"encoding/json"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"os"
	"path/filepath"
	"testing"

	event "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type runtimeIntegrityReadRepository struct {
	repository.RunRepository
	fullReads int
}

func (r *runtimeIntegrityReadRepository) GetByWorkspaceID(ctx context.Context, w common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	r.fullReads++
	value, err := r.RunRepository.GetByWorkspaceID(ctx, w, id)
	return value, err
}

func TestRuntimeOwnerReadIntegrityAndNarrowLifecycle(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	reads := &runtimeIntegrityReadRepository{RunRepository: runs}
	hub := event.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := runmodule.New()
	if err := owner.Configure(runOwnerTestDependencies(reads, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Stop(ctx) })
	repo := runtimeStateRepository{hub: hub}
	a := runtimeAggregate(t, 1)
	if err := repo.Save(ctx, a, 0); err != nil {
		t.Fatal(err)
	}
	original, err := runs.GetByWorkspaceID(ctx, a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*runmodel.Run){
		"missing integrity":  func(r *runmodel.Run) { r.RuntimeStateHash = "" },
		"different original": func(r *runmodel.Run) { r.RuntimeState.ResultFeedbackVersion++ },
	} {
		t.Run(name, func(t *testing.T) {
			corrupted := runmodel.CloneRun(original)
			mutate(corrupted)
			body, err := json.Marshal(map[common.RunID]*runmodel.Run{corrupted.ID: corrupted})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "unaccepted.json")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := filerepository.NewRunRepository(path).GetByWorkspaceID(ctx, a.Facts.WorkspaceID, a.Facts.RunID); err == nil {
				t.Fatal("corrupt physical original was consumed")
			}
		})
	}
	if _, err := repo.Load(ctx, a.Facts.WorkspaceID, a.Facts.RunID); err != nil {
		t.Fatal(err)
	}
	before := reads.fullReads
	view, err := (runSnapshotReader{hub: hub}).ReadLifecycle(ctx, a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil || view.RuntimeStateRevision != a.StateRevision || reads.fullReads != before {
		t.Fatal("narrow lifecycle loaded full Runtime", err)
	}
	if _, err := (runSnapshotReader{hub: hub}).ReadLifecycle(ctx, "different-workspace", a.Facts.RunID); err == nil {
		t.Fatal("cross workspace lifecycle")
	}
}
