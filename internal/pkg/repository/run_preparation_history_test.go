package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestRunPreparationHistoryPreservesDurableHeadAndRetirementAcrossReopen(t *testing.T) {
	for _, backend := range []string{"memory", "file", "workspace_scoped_file"} {
		t.Run(backend, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runs.json")
			var runs repository.RunRepository = memory.NewRunRepository()
			reopen := func() repository.RunRepository { return runs }
			if backend == "file" {
				reopen = func() repository.RunRepository { return filerepo.NewRunRepository(path) }
				runs = reopen()
			}
			if backend == "workspace_scoped_file" {
				reopen = func() repository.RunRepository {
					return workorchrepo.NewWorkspaceScopedFileRepositorySet(filepath.Dir(path)).Runs
				}
				runs = reopen()
			}
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "history")
			read := func() runmodel.WorkspacePreparationHistory {
				t.Helper()
				value, err := runs.(repository.RunPreparationHistoryReader).ObserveWorkspacePreparationHistory(ctx, "history")
				if err != nil || value.Validate() != nil {
					t.Fatalf("history: %+v %v", value, err)
				}
				return value
			}
			if value := read(); value.GenerationHead != 0 || value.RunCount != 0 || value.Retirement != nil {
				t.Fatal("fresh inventory fabricated history")
			}
			item := &runmodel.Run{WorkspaceID: "history", State: common.RunStateCompleted}
			if err := runs.CreateNextWorkspaceRun(ctx, item); err != nil {
				t.Fatal(err)
			}
			if value := read(); value.GenerationHead != 1 || value.RunCount != 1 {
				t.Fatalf("terminal Run filtered: %+v", value)
			}
			if err := runs.Delete(ctx, item.ID); err != nil {
				t.Fatal(err)
			}
			runs = reopen()
			if value := read(); value.GenerationHead != 1 || value.RunCount != 0 {
				t.Fatalf("deleted Run became no-history: %+v", value)
			}
			fence, err := runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{WorkspaceID: "history", DeletionID: "delete-history", DeletionGeneration: 1, DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "intent:history"})
			if err != nil {
				t.Fatal(err)
			}
			runs = reopen()
			value := read()
			if value.Retirement == nil || !reflect.DeepEqual(*value.Retirement, *fence) || value.GenerationHead != 1 {
				t.Fatalf("retirement hidden after reopen: %+v", value)
			}
			value.Retirement.DeletionID = "caller mutation"
			if read().Retirement.DeletionID != fence.DeletionID {
				t.Fatal("observation aliases persisted retirement")
			}
			wrong := common.ContextWithRuntimeWorkspaceID(context.Background(), "other")
			if _, err := runs.(repository.RunPreparationHistoryReader).ObserveWorkspacePreparationHistory(wrong, "history"); !errors.Is(err, common.ErrConflict) {
				t.Fatal("cross-scope history accepted")
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := runs.(repository.RunPreparationHistoryReader).ObserveWorkspacePreparationHistory(cancelled, "history"); !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled history accepted")
			}
		})
	}
}
