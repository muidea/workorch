package workorchrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *workspaceScopedRunRepository) ObserveWorkspacePreparationHistory(ctx context.Context, workspaceID common.WorkspaceID) (runmodel.WorkspacePreparationHistory, error) {
	return r.readWorkspacePreparation(ctx, workspaceID, "", nil)
}

func (r *workspaceScopedRunRepository) ReadWorkspaceExecutionIntentReceipt(ctx context.Context, workspaceID common.WorkspaceID, requestID string) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	var receipt *runmodel.WorkspaceExecutionIntentReceipt
	_, err := r.readWorkspacePreparation(ctx, workspaceID, requestID, &receipt)
	return receipt, err
}

func (r *workspaceScopedRunRepository) readWorkspacePreparation(ctx context.Context, workspaceID common.WorkspaceID, requestID string, receipt **runmodel.WorkspaceExecutionIntentReceipt) (runmodel.WorkspacePreparationHistory, error) {
	if ctx == nil || runmodel.ValidatePreparationHistoryWorkspaceID(workspaceID) != nil {
		return runmodel.WorkspacePreparationHistory{}, common.ErrInvalidArgument
	}
	if _, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID); err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	headLock := workspaceRunGenerationLock(r.store.workspaceRunGenerationPath(workspaceID))
	headLock.Lock()
	defer headLock.Unlock()
	if err := ctx.Err(); err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	// Observe only owner-local physical coordinates. In particular a dangling
	// symlink must not be mistaken for an absent generation head or inventory.
	for _, coordinate := range []struct {
		path      string
		directory bool
	}{
		{r.store.workspaceRoot(workspaceID), true},
		{filepath.Join(r.store.workspaceRoot(workspaceID), "runs"), true},
		{r.store.workspaceRunGenerationPath(workspaceID), false},
	} {
		info, err := os.Lstat(coordinate.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return runmodel.WorkspacePreparationHistory{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || coordinate.directory && !info.IsDir() || !coordinate.directory && !info.Mode().IsRegular() {
			return runmodel.WorkspacePreparationHistory{}, common.ErrConflict
		}
	}
	head, err := r.store.loadWorkspaceRunGenerationHead(workspaceID)
	if err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	// Do not use public Run projections: they can hide retired records. The
	// generation-head lock is also used by create/delete/retirement, and spans
	// every physical root in this Workspace's complete owner inventory.
	root := filepath.Join(r.store.workspaceRoot(workspaceID), "runs")
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	items := []*runmodel.Run{}
	// ReadDir is exact, sorted and error-aware. The ordinary helper uses Glob
	// and intentionally hides errors; it cannot establish complete history.
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return runmodel.WorkspacePreparationHistory{}, common.ErrConflict
		}
		if err := ctx.Err(); err != nil {
			return runmodel.WorkspacePreparationHistory{}, err
		}
		path := r.store.runPath(filepath.Join(root, entry.Name()), workspaceScopedRunFile)
		info, err := os.Lstat(path)
		if err != nil {
			return runmodel.WorkspacePreparationHistory{}, err
		}
		if !info.Mode().IsRegular() {
			return runmodel.WorkspacePreparationHistory{}, common.ErrConflict
		}
		values, err := r.store.runRepo(path).List(ctx)
		if err != nil {
			return runmodel.WorkspacePreparationHistory{}, err
		}
		// Ordinary exact deletion leaves an empty physical file. The durable
		// nonzero watermark still prevents it from becoming a no-history fact.
		if len(values) == 0 && head.Generation > 0 {
			continue
		}
		if len(values) != 1 || values[0] == nil || string(values[0].ID) != entry.Name() || values[0].WorkspaceID != workspaceID {
			return runmodel.WorkspacePreparationHistory{}, common.ErrConflict
		}
		items = append(items, values[0])
	}
	history, err := runmodel.CompileWorkspacePreparationHistory(workspaceID, head.Generation, head.Retirement, items)
	if err == nil && receipt != nil {
		*receipt, err = runmodel.CompileExecutionIntentReceipt(workspaceID, requestID, items)
	}
	return history, err
}
