package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/application/workorchd/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type entryTestRuntime struct {
	ensureWorkspaceErr error
	ensureWorkspaceCnt int
	ensureRunCnt       int
}

func (r *entryTestRuntime) GetWorkspaceByID(context.Context, common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return nil, common.ErrNotFound
}

func (r *entryTestRuntime) GetRun(context.Context, common.RunID) (*runmodel.Run, error) {
	return nil, common.ErrNotFound
}

func (r *entryTestRuntime) GetStep(context.Context, common.StepID) (*runmodel.Step, error) {
	return nil, common.ErrNotFound
}

func (r *entryTestRuntime) GetTicket(context.Context, common.TicketID) (*decisionmodel.Ticket, error) {
	return nil, common.ErrNotFound
}

func (r *entryTestRuntime) GetResolutionByTicketID(context.Context, common.TicketID) (*decisionmodel.Resolution, error) {
	return nil, common.ErrNotFound
}

func (r *entryTestRuntime) GetApproval(context.Context, common.ApprovalID) (*approvalmodel.Approval, error) {
	return nil, common.ErrNotFound
}

func (r *entryTestRuntime) EnsureWorkspace(context.Context, common.WorkspaceID, string) error {
	r.ensureWorkspaceCnt++
	return r.ensureWorkspaceErr
}

func (r *entryTestRuntime) EnsureRun(context.Context, common.RunID, common.WorkspaceID) error {
	r.ensureRunCnt++
	return nil
}

func (r *entryTestRuntime) EnsureRunStarted(context.Context, common.RunID) error {
	return nil
}

func (r *entryTestRuntime) EnsureRunBlockedOnDecision(context.Context, common.RunID, common.TicketID) error {
	return nil
}

func (r *entryTestRuntime) EnsureRunBlockedOnApproval(context.Context, common.RunID, common.ApprovalID) error {
	return nil
}

func (r *entryTestRuntime) EnsureRunResumed(context.Context, common.RunID) error {
	return nil
}

func (r *entryTestRuntime) EnsureTicket(context.Context, common.TicketID, common.WorkspaceID, common.RunID, common.StepID, string) error {
	return nil
}

func (r *entryTestRuntime) EnsureResolvedTicket(context.Context, common.ResolutionID, common.TicketID, string, map[string]any) error {
	return nil
}

func (r *entryTestRuntime) EnsureApproval(context.Context, common.ApprovalID, common.WorkspaceID, common.RunID, common.StepID, string) error {
	return nil
}

func (r *entryTestRuntime) EnsureApprovalApproved(context.Context, common.ApprovalID) error {
	return nil
}

func (r *entryTestRuntime) EnsureApprovalRejected(context.Context, common.ApprovalID) error {
	return nil
}

func TestRunAllWorkorchWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := RunAllWorkorchWithSession(context.Background(), nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil session err = %v, want invalid argument", err)
	}
}

func TestRunAllWorkorchWithSessionReturnsInvalidArgumentWhenRuntimeMissing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session *RuntimeSession
	}{
		{name: "empty session", session: &RuntimeSession{}},
		{name: "nil managed runtime", session: &RuntimeSession{Runtime: nil}},
		{name: "managed runtime without inner runtime", session: &RuntimeSession{Runtime: &ManagedRuntime{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RunAllWorkorchWithSession(context.Background(), tc.session); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("session err = %v, want invalid argument", err)
			}
		})
	}
}

func TestRunAllWorkorchWithSessionClosesSessionAfterSuccess(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}

	slices, err := RunAllWorkorchWithSession(context.Background(), session)
	if err != nil {
		t.Fatalf("RunAllWorkorchWithSession: %v", err)
	}
	if len(slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(slices))
	}
	if session.Runtime != nil {
		t.Fatalf("session runtime not released: %#v", session.Runtime)
	}
	if session.Bootstrap != nil {
		t.Fatalf("session bootstrap not released: %#v", session.Bootstrap)
	}
}

func TestRunWorkorchSliceWithSessionClosesSessionAfterSuccess(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}

	item, err := RunWorkorchSliceWithSession(context.Background(), session, "workorch-runnable")
	if err != nil {
		t.Fatalf("RunWorkorchSliceWithSession: %v", err)
	}
	if item.ID != "workorch-runnable" || item.Title != "workorch runnable slice" {
		t.Fatalf("slice = %+v, want workorch-runnable/workorch runnable slice", item)
	}
	if session.Runtime != nil {
		t.Fatalf("session runtime not released: %#v", session.Runtime)
	}
	if session.Bootstrap != nil {
		t.Fatalf("session bootstrap not released: %#v", session.Bootstrap)
	}
}

func TestListAvailableWorkorchSliceSnapshotsWithBackendReturnsPersistedSnapshots(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := RunWorkorchSliceWithBackend(context.Background(), "file", dataRoot, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSliceWithBackend: %v", err)
	}

	slices, err := ListAvailableWorkorchSliceSnapshotsWithBackend(context.Background(), "file", dataRoot)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceSnapshotsWithBackend: %v", err)
	}
	if len(slices) != 1 {
		t.Fatalf("len(slices) = %d, want 1", len(slices))
	}
	if slices[0].ID != "workorch-runnable" || slices[0].Snapshot == nil || slices[0].Snapshot.Slice != "runnable" {
		t.Fatalf("slice = %+v, want runnable snapshot", slices[0])
	}
}

func TestRunAllWorkorchWithSessionClosesSessionWhenRuntimeInvalid(t *testing.T) {
	session := NewRuntimeSession(bootstrap.NewWithBackend("memory", ""))
	session.Runtime = nil

	if _, err := RunAllWorkorchWithSession(context.Background(), session); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if session.Runtime != nil {
		t.Fatalf("session runtime not released after invalid runtime: %#v", session.Runtime)
	}
	if session.Bootstrap != nil {
		t.Fatalf("session bootstrap not released after invalid runtime: %#v", session.Bootstrap)
	}
}

func TestRunAllWorkorchWithSessionReturnsSlicesAndCloseErrorAfterSuccess(t *testing.T) {
	errShutdown := errors.New("shutdown failed")
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	prependBootstrapRuntimeModule(t, session.Bootstrap, runtimeSessionTestLifecycle{
		id:          "broken",
		teardownErr: errShutdown,
	})

	slices, err := RunAllWorkorchWithSession(context.Background(), session)
	if len(slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(slices))
	}
	if !errors.Is(err, errShutdown) {
		t.Fatalf("err = %v, want shutdown error", err)
	}
	if session.Runtime != nil {
		t.Fatalf("session runtime not released after shutdown error: %#v", session.Runtime)
	}
	if session.Bootstrap != nil {
		t.Fatalf("session bootstrap not released after shutdown error: %#v", session.Bootstrap)
	}
}

func TestRunAllWorkorchWithSessionPropagatesTopLevelBusinessErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "blocked", err: common.ErrBlocked},
		{name: "conflict", err: common.ErrConflict},
		{name: "not found", err: common.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &entryTestRuntime{ensureWorkspaceErr: tc.err}
			session := &RuntimeSession{
				Runtime: NewManagedRuntime(runtime, newTestHub()),
			}

			_, err := RunAllWorkorchWithSession(context.Background(), session)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if runtime.ensureWorkspaceCnt != 1 {
				t.Fatalf("ensureWorkspaceCnt = %d, want 1", runtime.ensureWorkspaceCnt)
			}
			if runtime.ensureRunCnt != 0 {
				t.Fatalf("ensureRunCnt = %d, want 0", runtime.ensureRunCnt)
			}
		})
	}
}

func TestRunAllWorkorchWithSessionJoinsTopLevelBusinessAndCloseErrors(t *testing.T) {
	errShutdown := errors.New("shutdown failed")
	runtime := &entryTestRuntime{ensureWorkspaceErr: common.ErrBlocked}
	session := &RuntimeSession{
		Bootstrap: &bootstrap.Runtime{},
		Runtime:   NewManagedRuntime(runtime, newTestHub()),
	}
	setBootstrapRuntimeModules(t, session.Bootstrap, runtimeSessionTestLifecycle{
		id:          "broken",
		teardownErr: errShutdown,
	})

	_, err := RunAllWorkorchWithSession(context.Background(), session)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("err = %v, want blocked", err)
	}
	if !errors.Is(err, errShutdown) {
		t.Fatalf("err = %v, want shutdown error", err)
	}
	if runtime.ensureWorkspaceCnt != 1 {
		t.Fatalf("ensureWorkspaceCnt = %d, want 1", runtime.ensureWorkspaceCnt)
	}
	if runtime.ensureRunCnt != 0 {
		t.Fatalf("ensureRunCnt = %d, want 0", runtime.ensureRunCnt)
	}
	if session.Runtime != nil {
		t.Fatalf("session runtime not released after joined error: %#v", session.Runtime)
	}
	if session.Bootstrap != nil {
		t.Fatalf("session bootstrap not released after joined error: %#v", session.Bootstrap)
	}
}

func TestRunAllWorkorchWithBackendReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	if _, err := RunAllWorkorchWithBackend(context.Background(), "unsupported", ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsupported backend err = %v, want invalid argument", err)
	}
}

func TestRunWorkorchSliceWithBackendReturnsNotFoundForUnknownSlice(t *testing.T) {
	if _, err := RunWorkorchSliceWithBackend(context.Background(), "memory", "", "workorch-missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("unknown slice err = %v, want not found", err)
	}
}

func TestReadWorkorchSliceSnapshotWithBackendReadsPersistedSlice(t *testing.T) {
	dataRoot := t.TempDir()

	if _, err := RunWorkorchSliceWithBackend(context.Background(), "file", dataRoot, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSliceWithBackend: %v", err)
	}

	item, err := ReadWorkorchSliceSnapshotWithBackend(context.Background(), "file", dataRoot, "workorch-runnable")
	if err != nil {
		t.Fatalf("ReadWorkorchSliceSnapshotWithBackend: %v", err)
	}
	if item.ID != "workorch-runnable" || item.Title != "workorch runnable slice" {
		t.Fatalf("slice = %+v, want workorch-runnable/workorch runnable slice", item)
	}
	if item.Snapshot == nil || item.Snapshot.Slice != "runnable" {
		t.Fatalf("snapshot = %+v, want runnable snapshot", item.Snapshot)
	}
}

func TestSummarizeWorkorchSliceStatusWithBackendReadsPersistedSummary(t *testing.T) {
	dataRoot := t.TempDir()

	if _, err := RunWorkorchSliceWithBackend(context.Background(), "file", dataRoot, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSliceWithBackend: %v", err)
	}

	status, err := SummarizeWorkorchSliceStatusWithBackend(context.Background(), "file", dataRoot, "workorch-runnable")
	if err != nil {
		t.Fatalf("SummarizeWorkorchSliceStatusWithBackend: %v", err)
	}
	if status.ID != "workorch-runnable" || status.RunState != "completed" || status.StepState != "completed" {
		t.Fatalf("status = %+v, want completed runnable summary", status)
	}
}

func TestListAvailableWorkorchSliceStatusesWithBackendReturnsPersistedStatuses(t *testing.T) {
	dataRoot := t.TempDir()

	statuses, err := ListAvailableWorkorchSliceStatusesWithBackend(context.Background(), "file", dataRoot)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceStatusesWithBackend before run: %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("len(statuses) = %d, want 0 before run", len(statuses))
	}

	if _, err := RunWorkorchSliceWithBackend(context.Background(), "file", dataRoot, "workorch-runnable"); err != nil {
		t.Fatalf("RunWorkorchSliceWithBackend: %v", err)
	}

	statuses, err = ListAvailableWorkorchSliceStatusesWithBackend(context.Background(), "file", dataRoot)
	if err != nil {
		t.Fatalf("ListAvailableWorkorchSliceStatusesWithBackend after run: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("len(statuses) = %d, want 1 after single run", len(statuses))
	}
	if statuses[0].ID != "workorch-runnable" || statuses[0].RunState != "completed" {
		t.Fatalf("status = %+v, want completed runnable summary", statuses[0])
	}
}

func TestRunAllWorkorchWithBackendFileUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataRoot := filepath.Join(tempRoot, "workorchd-data")
	t.Setenv("TMPDIR", tempRoot)

	slices, err := RunAllWorkorchWithBackend(context.Background(), "file", "")
	if err != nil {
		t.Fatalf("RunAllWorkorchWithBackend: %v", err)
	}
	if len(slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(slices))
	}

	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(defaultDataRoot, name)); err != nil {
			t.Fatalf("stat %s in default data root: %v", name, err)
		}
	}
}

func TestRunAllWorkorchWithBackendFileRemainsStableAcrossReloadsWhenDefaultDataRootUnset(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	first, err := RunAllWorkorchWithBackend(context.Background(), "file", "")
	if err != nil {
		t.Fatalf("first RunAllWorkorchWithBackend: %v", err)
	}
	second, err := RunAllWorkorchWithBackend(context.Background(), "file", "")
	if err != nil {
		t.Fatalf("second RunAllWorkorchWithBackend: %v", err)
	}

	assertWorkorchSlicesStableAfterReload(t, first, second)
}

func TestRunAllWorkorchFromEnvReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "unsupported")
	t.Setenv("WORKORCH_DATA_DIR", t.TempDir())

	if _, err := RunAllWorkorchFromEnv(context.Background()); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unsupported env backend err = %v, want invalid argument", err)
	}
}

func TestRunAllWorkorchFromEnvWithFileBackend(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "file")
	t.Setenv("WORKORCH_DATA_DIR", dataRoot)

	slices, err := RunAllWorkorchFromEnv(context.Background())
	if err != nil {
		t.Fatalf("RunAllWorkorchFromEnv: %v", err)
	}
	if len(slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(slices))
	}

	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(dataRoot, name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestRunAllWorkorchFromEnvWithFileBackendUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataRoot := filepath.Join(tempRoot, "workorchd-data")
	t.Setenv("TMPDIR", tempRoot)
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "file")
	t.Setenv("WORKORCH_DATA_DIR", "")

	slices, err := RunAllWorkorchFromEnv(context.Background())
	if err != nil {
		t.Fatalf("RunAllWorkorchFromEnv: %v", err)
	}
	if len(slices) != 17 {
		t.Fatalf("len(slices) = %d, want 17", len(slices))
	}

	for _, name := range []string{
		"workspace.json",
		"run.json",
		"step.json",
		"ticket.json",
		"resolution.json",
		"approval.json",
	} {
		if _, err := os.Stat(filepath.Join(defaultDataRoot, name)); err != nil {
			t.Fatalf("stat %s in default data root: %v", name, err)
		}
	}
}

func TestRunAllWorkorchFromEnvWithFileBackendRemainsStableAcrossReloads(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "file")
	t.Setenv("WORKORCH_DATA_DIR", dataRoot)

	first, err := RunAllWorkorchFromEnv(context.Background())
	if err != nil {
		t.Fatalf("first RunAllWorkorchFromEnv: %v", err)
	}
	second, err := RunAllWorkorchFromEnv(context.Background())
	if err != nil {
		t.Fatalf("second RunAllWorkorchFromEnv: %v", err)
	}

	assertWorkorchSlicesStableAfterReload(t, first, second)
}

func assertWorkorchSlicesStableAfterReload(t *testing.T, first []Slice, second []Slice) {
	t.Helper()

	if len(first) != len(second) {
		t.Fatalf("slice count mismatch after reload: first=%d second=%d", len(first), len(second))
	}

	for idx := range first {
		assertWorkorchSliceStableAfterReload(t, idx, first[idx], second[idx])
	}
}

func assertWorkorchSliceStableAfterReload(t *testing.T, index int, first Slice, second Slice) {
	t.Helper()

	if first.Title != second.Title {
		t.Fatalf("slice %d title mismatch after reload: first=%q second=%q", index, first.Title, second.Title)
	}
	if first.Snapshot == nil || second.Snapshot == nil {
		t.Fatalf("slice %q snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot == nil, second.Snapshot == nil)
	}

	if first.Snapshot.Slice != second.Snapshot.Slice {
		t.Fatalf("slice %q snapshot slice mismatch after reload: first=%q second=%q", first.Title, first.Snapshot.Slice, second.Snapshot.Slice)
	}
	if first.Snapshot.Workspace == nil || second.Snapshot.Workspace == nil {
		t.Fatalf("slice %q workspace snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot.Workspace == nil, second.Snapshot.Workspace == nil)
	}
	if *first.Snapshot.Workspace != *second.Snapshot.Workspace {
		t.Fatalf("slice %q workspace mismatch after reload: first=%+v second=%+v", first.Title, *first.Snapshot.Workspace, *second.Snapshot.Workspace)
	}
	if first.Snapshot.Run == nil || second.Snapshot.Run == nil {
		t.Fatalf("slice %q run snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot.Run == nil, second.Snapshot.Run == nil)
	}
	if *first.Snapshot.Run != *second.Snapshot.Run {
		t.Fatalf("slice %q run mismatch after reload: first=%+v second=%+v", first.Title, *first.Snapshot.Run, *second.Snapshot.Run)
	}
	if len(first.Snapshot.Observed) != len(second.Snapshot.Observed) {
		t.Fatalf("slice %q observed count mismatch after reload: first=%d second=%d", first.Title, len(first.Snapshot.Observed), len(second.Snapshot.Observed))
	}
	for idx := range first.Snapshot.Observed {
		assertObservedViewStableAfterReload(t, first.Title, idx, first.Snapshot.Observed[idx], second.Snapshot.Observed[idx])
	}
	assertStepStableAfterReload(t, first.Title, first, second)
	assertBlockingStableAfterReload(t, first.Title, first, second)
}

func assertObservedViewStableAfterReload(t *testing.T, title string, idx int, first presenter.ObservedView, second presenter.ObservedView) {
	t.Helper()

	if first.ID != second.ID ||
		first.Version != second.Version ||
		first.Topic != second.Topic ||
		first.SourceModule != second.SourceModule ||
		first.SubjectType != second.SubjectType ||
		first.SubjectID != second.SubjectID ||
		first.WorkspaceID != second.WorkspaceID ||
		first.RunID != second.RunID ||
		first.StepID != second.StepID {
		t.Fatalf("slice %q observed[%d] mismatch after reload: first=%+v second=%+v", title, idx, first, second)
	}
	if !reflect.DeepEqual(first.Payload, second.Payload) {
		t.Fatalf("slice %q observed payload[%d] mismatch after reload: first=%+v second=%+v", title, idx, first.Payload, second.Payload)
	}
}

func assertStepStableAfterReload(t *testing.T, title string, first Slice, second Slice) {
	t.Helper()

	if (first.Snapshot.Step == nil) != (second.Snapshot.Step == nil) {
		t.Fatalf("slice %q step presence mismatch after reload: first=%v second=%v", title, first.Snapshot.Step == nil, second.Snapshot.Step == nil)
	}
	if first.Snapshot.Step == nil {
		return
	}
	if first.Snapshot.Step.ID != second.Snapshot.Step.ID ||
		first.Snapshot.Step.Kind != second.Snapshot.Step.Kind ||
		first.Snapshot.Step.State != second.Snapshot.Step.State {
		t.Fatalf("slice %q step mismatch after reload: first=%+v second=%+v", title, *first.Snapshot.Step, *second.Snapshot.Step)
	}
	if !reflect.DeepEqual(first.Snapshot.Step.Input, second.Snapshot.Step.Input) {
		t.Fatalf("slice %q step input mismatch after reload: first=%+v second=%+v", title, first.Snapshot.Step.Input, second.Snapshot.Step.Input)
	}
	if !reflect.DeepEqual(first.Snapshot.Step.Output, second.Snapshot.Step.Output) {
		t.Fatalf("slice %q step output mismatch after reload: first=%+v second=%+v", title, first.Snapshot.Step.Output, second.Snapshot.Step.Output)
	}
	if len(first.Snapshot.Steps) != len(second.Snapshot.Steps) {
		t.Fatalf("slice %q steps count mismatch after reload: first=%d second=%d", title, len(first.Snapshot.Steps), len(second.Snapshot.Steps))
	}
	for idx := range first.Snapshot.Steps {
		if first.Snapshot.Steps[idx].ID != second.Snapshot.Steps[idx].ID ||
			first.Snapshot.Steps[idx].Kind != second.Snapshot.Steps[idx].Kind ||
			first.Snapshot.Steps[idx].State != second.Snapshot.Steps[idx].State {
			t.Fatalf("slice %q steps[%d] mismatch after reload: first=%+v second=%+v", title, idx, first.Snapshot.Steps[idx], second.Snapshot.Steps[idx])
		}
		if !reflect.DeepEqual(first.Snapshot.Steps[idx].Input, second.Snapshot.Steps[idx].Input) {
			t.Fatalf("slice %q steps[%d] input mismatch after reload: first=%+v second=%+v", title, idx, first.Snapshot.Steps[idx].Input, second.Snapshot.Steps[idx].Input)
		}
		if !reflect.DeepEqual(first.Snapshot.Steps[idx].Output, second.Snapshot.Steps[idx].Output) {
			t.Fatalf("slice %q steps[%d] output mismatch after reload: first=%+v second=%+v", title, idx, first.Snapshot.Steps[idx].Output, second.Snapshot.Steps[idx].Output)
		}
	}
}

func assertBlockingStableAfterReload(t *testing.T, title string, first Slice, second Slice) {
	t.Helper()

	if (first.Snapshot.Blocking == nil) != (second.Snapshot.Blocking == nil) {
		t.Fatalf("slice %q blocking presence mismatch after reload: first=%v second=%v", title, first.Snapshot.Blocking == nil, second.Snapshot.Blocking == nil)
	}
	if first.Snapshot.Blocking == nil {
		return
	}
	if (first.Snapshot.Blocking.DecisionTicket == nil) != (second.Snapshot.Blocking.DecisionTicket == nil) {
		t.Fatalf("slice %q decision ticket presence mismatch after reload: first=%v second=%v", title, first.Snapshot.Blocking.DecisionTicket == nil, second.Snapshot.Blocking.DecisionTicket == nil)
	}
	if first.Snapshot.Blocking.DecisionTicket != nil && *first.Snapshot.Blocking.DecisionTicket != *second.Snapshot.Blocking.DecisionTicket {
		t.Fatalf("slice %q decision ticket mismatch after reload: first=%+v second=%+v", title, *first.Snapshot.Blocking.DecisionTicket, *second.Snapshot.Blocking.DecisionTicket)
	}
	if (first.Snapshot.Blocking.Approval == nil) != (second.Snapshot.Blocking.Approval == nil) {
		t.Fatalf("slice %q approval presence mismatch after reload: first=%v second=%v", title, first.Snapshot.Blocking.Approval == nil, second.Snapshot.Blocking.Approval == nil)
	}
	if first.Snapshot.Blocking.Approval != nil && *first.Snapshot.Blocking.Approval != *second.Snapshot.Blocking.Approval {
		t.Fatalf("slice %q approval mismatch after reload: first=%+v second=%+v", title, *first.Snapshot.Blocking.Approval, *second.Snapshot.Blocking.Approval)
	}
}
