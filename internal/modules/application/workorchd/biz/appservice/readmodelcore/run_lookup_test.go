package readmodelcore

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type fakeRunReader struct {
	byID      map[common.RunID]*runmodel.Run
	byWS      map[common.WorkspaceID][]*runmodel.Run
	getCalls  int
	listCalls int
	getErr    error
	listWSErr error
}

func (r *fakeRunReader) Get(_ context.Context, id common.RunID) (*runmodel.Run, error) {
	r.getCalls++
	if r.getErr != nil {
		return nil, r.getErr
	}
	item, ok := r.byID[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func (r *fakeRunReader) List(_ context.Context) ([]*runmodel.Run, error) {
	return nil, nil
}

func (r *fakeRunReader) ListByWorkspaceID(_ context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	r.listCalls++
	if r.listWSErr != nil {
		return nil, r.listWSErr
	}
	return r.byWS[workspaceID], nil
}

func TestGetRunFromReaderUsesDirectGetWithoutRuntimeWorkspace(t *testing.T) {
	run := &runmodel.Run{ID: "run-001", WorkspaceID: "ws-001"}
	reader := &fakeRunReader{byID: map[common.RunID]*runmodel.Run{run.ID: run}}

	got, err := GetRunFromReader(context.Background(), reader, run.ID)
	if err != nil {
		t.Fatalf("GetRunFromReader returned error: %v", err)
	}
	if got != run {
		t.Fatalf("run mismatch")
	}
	if reader.getCalls != 1 || reader.listCalls != 0 {
		t.Fatalf("calls mismatch: get=%d list=%d", reader.getCalls, reader.listCalls)
	}
}

func TestGetRunFromReaderUsesRuntimeWorkspaceScope(t *testing.T) {
	run := &runmodel.Run{ID: "run-001", WorkspaceID: "ws-001"}
	reader := &fakeRunReader{
		byID: map[common.RunID]*runmodel.Run{
			run.ID: {ID: run.ID, WorkspaceID: "ws-other"},
		},
		byWS: map[common.WorkspaceID][]*runmodel.Run{
			"ws-001": {run},
		},
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-001")

	got, err := GetRunFromReader(ctx, reader, run.ID)
	if err != nil {
		t.Fatalf("GetRunFromReader returned error: %v", err)
	}
	if got != run {
		t.Fatalf("run mismatch")
	}
	if reader.getCalls != 0 || reader.listCalls != 1 {
		t.Fatalf("calls mismatch: get=%d list=%d", reader.getCalls, reader.listCalls)
	}
}

func TestGetRunFromReaderReturnsNotFoundWhenScopedRunMissing(t *testing.T) {
	reader := &fakeRunReader{
		byWS: map[common.WorkspaceID][]*runmodel.Run{
			"ws-001": {{ID: "run-other", WorkspaceID: "ws-001"}},
		},
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-001")

	_, err := GetRunFromReader(ctx, reader, "run-001")
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
