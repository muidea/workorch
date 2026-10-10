package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func seedPreparationWorkspace(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID) (*workspace.Workspace, workspaceevents.RecordPreparationDiagnosticCommand) {
	t.Helper()
	ctx := context.Background()
	template := workspace.WorkspaceTemplate{ID: "prep-template"}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: id, TemplateID: template.ID}); err != nil {
		t.Fatal(err)
	}
	w, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, GoalText: "目标", TemplateID: template.ID}
	save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-1", Input: input, ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	w, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	d := workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: id, InputHash: w.DefinitionAuthoring.InputHash, TemplateSnapshotHash: w.DefinitionAuthoring.TemplateSnapshotHash, ObservedWorkspaceRevision: w.Revision, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "derive-prep", PhaseCode: "render", ProviderBindingHash: "provider"}, []byte{0xff, 0, '\r', '\n', ' '})
	return w, workspaceevents.RecordPreparationDiagnosticCommand{Diagnostic: d}
}

func submitPreparation(t *testing.T, owners *fileDefinitionAdmissionOwners, cmd workspaceevents.RecordPreparationDiagnosticCommand) workspaceevents.RecordPreparationDiagnosticResult {
	t.Helper()
	transition, err := owners.workspace.submitDefinitionSpecificationInternalEvent(context.Background(), definitionSpecificationInternalEvent{WorkspaceID: cmd.Diagnostic.WorkspaceID, PreparationDiagnostic: &cmd})
	if err != nil || transition.PreparationDiagnostic == nil || transition.PreparationDiagnostic.ValidateFor(cmd) != nil {
		t.Fatalf("preparation command: %v", err)
	}
	return transition.PreparationDiagnostic.DeepClone()
}

func TestPreparationDiagnosticFileReopenResponseLossAndInputRevisionInheritance(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	before, cmd := seedPreparationWorkspace(t, owners, "ws-prep-reopen")
	first := submitPreparation(t, owners, cmd)
	if first.Duplicate || first.WorkspaceRevision != before.Revision+1 {
		t.Fatal("first write missing")
	}
	// Simulate committed response loss and process restart: exact replay must not CAS.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	replayed := submitPreparation(t, owners, cmd)
	if !replayed.Duplicate || replayed.WorkspaceRevision != first.WorkspaceRevision {
		t.Fatal("lost response consumed revision")
	}
	current, err := owners.repositories.Workspaces.Get(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	newerCommand := cmd.DeepClone()
	newerCommand.Diagnostic.ObservedWorkspaceRevision = current.Revision
	newer := submitPreparation(t, owners, newerCommand)
	if !newer.Duplicate || newer.Diagnostic.ObservedWorkspaceRevision != cmd.Diagnostic.ObservedWorkspaceRevision {
		t.Fatal("duplicate rewrote first observation")
	}
	oldHash := current.DefinitionAuthoring.InputHash
	input := current.DefinitionAuthoring.Input.Clone()
	input.Revision++
	save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-2", ExpectedInputHash: oldHash, ExpectedWorkspaceRevision: current.Revision, Input: input}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.DefinitionAuthoring.InputHash == oldHash || len(current.DefinitionAuthoring.PreparationDiagnostics) != 1 || current.DefinitionAuthoring.Validate() != nil {
		t.Fatal("input revision lost lineage or ABA identity")
	}
	replayed = submitPreparation(t, owners, cmd)
	if !replayed.Duplicate || replayed.WorkspaceRevision != current.Revision {
		t.Fatal("historical duplicate rebound")
	}
	stale := cmd.DeepClone()
	stale.Diagnostic.TargetRequestID = "new-stale"
	stale.Diagnostic.DiagnosticID = stale.Diagnostic.ExpectedDiagnosticID()
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: before.ID, PreparationDiagnostic: &stale}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("new stale diagnostic rebound: %v", err)
	}
	query := workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: before.ID, DiagnosticID: cmd.Diagnostic.DiagnosticID}
	result, err := owners.workspace.queryPreparationDiagnostic(ctx, query)
	if err != nil || !bytes.Equal(result.Diagnostic.Cause, cmd.Diagnostic.Cause) || result.ValidateFor(query) != nil {
		t.Fatalf("query full cause: %v", err)
	}
	result.Diagnostic.Cause[0]++
	fresh, err := owners.workspace.queryPreparationDiagnostic(ctx, query)
	if err != nil || !reflect.DeepEqual(fresh.Diagnostic, cmd.Diagnostic) {
		t.Fatal("query leaked owner alias")
	}
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: before.ID})
	if err != nil || view.ValidateFor(workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: before.ID}) != nil {
		t.Fatalf("metadata DTO invalidated ordinary query: %v", err)
	}
	if current.ActiveRunID != "" || current.StartupInput != nil || current.ActivationAdmission != nil || len(current.DefinitionAuthoring.DerivationRequests) != 0 {
		t.Fatal("diagnostic authorized provider or startup")
	}
}

func TestPreparationDiagnosticReducerGuardsAndTargetBeforeCapacity(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w, cmd := seedPreparationWorkspace(t, owners, "ws-prep-guards")
	o := definitionSpecificationObservation{workspace: w, template: &w.DefinitionAuthoring.TemplateSnapshot}
	for name, mutate := range map[string]func(*workspace.Workspace){
		"revision":   func(w *workspace.Workspace) { w.Revision++ },
		"startup":    func(w *workspace.Workspace) { w.StartupInput = &workspace.WorkspaceStartupInputContract{} },
		"activation": func(w *workspace.Workspace) { w.ActivationAdmission = &workspace.WorkspaceActivationAdmission{} },
		"active_run": func(w *workspace.Workspace) { w.ActiveRunID = "run" },
		"confirmation": func(w *workspace.Workspace) {
			w.DefinitionAuthoringConfirmation = &workspace.DefinitionAuthoringConfirmation{}
		},
		"admission": func(w *workspace.Workspace) {
			w.DefinitionAdmissionCandidates = []workspace.DefinitionAdmissionCandidate{{}}
		},
		"missing_current": func(w *workspace.Workspace) { w.DefinitionAuthoring = nil },
	} {
		t.Run(name, func(t *testing.T) {
			altered := *w
			mutate(&altered)
			guard := o
			guard.workspace = &altered
			if _, err := reducePreparationDiagnostic(guard, cmd); err == nil {
				t.Fatal("authority guard accepted")
			}
		})
	}
	changed := w.DefinitionAuthoring.TemplateSnapshot.Clone()
	changed.Description = "changed"
	o.template = &changed
	if _, err := reducePreparationDiagnostic(o, cmd); !errors.Is(err, common.ErrConflict) {
		t.Fatal("mutable template drift accepted")
	}
	for n := 0; n < workspace.DefinitionAuthoringPreparationDiagnosticLimit; n++ {
		current, _ := owners.repositories.Workspaces.Get(context.Background(), w.ID)
		entry := cmd.DeepClone()
		entry.Diagnostic.TargetRequestID = fmt.Sprintf("target-%d", n)
		entry.Diagnostic.ObservedWorkspaceRevision = current.Revision
		entry.Diagnostic.DiagnosticID = entry.Diagnostic.ExpectedDiagnosticID()
		submitPreparation(t, owners, entry)
	}
	current, _ := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	record := current.DefinitionAuthoring.Clone()
	var request workspace.DefinitionDerivationRequest
	var err error
	record, request, _, err = record.ReserveDerivation("already-reserved", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	current.DefinitionAuthoring = &record
	o = definitionSpecificationObservation{workspace: current, template: &record.TemplateSnapshot}
	entry := cmd.DeepClone()
	entry.Diagnostic.TargetRequestID = request.RequestID
	entry.Diagnostic.ObservedWorkspaceRevision = current.Revision
	entry.Diagnostic.DiagnosticID = entry.Diagnostic.ExpectedDiagnosticID()
	_, err = reducePreparationDiagnostic(o, entry)
	var limit *workspace.PreparationDiagnosticLimitError
	if !errors.Is(err, common.ErrConflict) || errors.As(err, &limit) {
		t.Fatalf("reserved target became capacity refusal: %v", err)
	}
	entry.Diagnostic.TargetRequestID = "capacity-only"
	entry.Diagnostic.DiagnosticID = entry.Diagnostic.ExpectedDiagnosticID()
	_, err = reducePreparationDiagnostic(o, entry)
	if !errors.As(err, &limit) {
		t.Fatalf("capacity error not typed: %v", err)
	}
}

type preparationCountingRepository struct {
	repository.WorkspaceRepository
	mu    sync.Mutex
	count int
}

func (r *preparationCountingRepository) CompareAndSwap(ctx context.Context, w *workspace.Workspace, revision uint64) error {
	r.mu.Lock()
	r.count++
	r.mu.Unlock()
	return r.WorkspaceRepository.CompareAndSwap(ctx, w, revision)
}

func TestPreparationDiagnosticConcurrentDuplicatesHaveSingleCASZeroNewProviderPermit(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w, cmd := seedPreparationWorkspace(t, owners, "ws-prep-concurrency")
	counting := &preparationCountingRepository{WorkspaceRepository: owners.workspace.workspaces}
	owners.workspace.workspaces = counting
	var group sync.WaitGroup
	errorsCh := make(chan error, 24)
	for n := 0; n < 24; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			transition, err := owners.workspace.submitDefinitionSpecificationInternalEvent(context.Background(), definitionSpecificationInternalEvent{WorkspaceID: w.ID, PreparationDiagnostic: &cmd})
			if err == nil && transition.PreparationDiagnostic.ValidateFor(cmd) != nil {
				err = common.ErrConflict
			}
			errorsCh <- err
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if counting.count != 1 {
		t.Fatalf("duplicate CAS count=%d", counting.count)
	}
	current, err := owners.repositories.Workspaces.Get(context.Background(), w.ID)
	if err != nil || current.Revision != w.Revision+1 || len(current.DefinitionAuthoring.PreparationDiagnostics) != 1 || len(current.DefinitionAuthoring.DerivationRequests) != 0 || len(current.DefinitionAuthoring.SpecificationRequests) != 0 || current.ActiveRunID != "" {
		t.Fatal("concurrent diagnostic consumed provider budget or authority")
	}
}

func TestPreparationDiagnosticHandlerRejectsPayloadScopeAndQueryWithoutCurrent(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w, cmd := seedPreparationWorkspace(t, owners, "ws-prep-payload")
	for _, topic := range []string{workspaceevents.TopicWorkspacePreparationDiagnosticRecord, workspaceevents.TopicWorkspacePreparationDiagnosticQuery} {
		ev := magicevent.NewEvent(topic, definitionSpecificationCommandSource, workspaceevents.CommandDestination, nil, map[string]any{"workspace": w.ID})
		result := magicevent.NewResult(topic, workspaceevents.CommandDestination, "test")
		owners.workspace.handleDefinitionSpecificationCommand(ev, result)
		if _, err := result.Get(); err == nil {
			t.Error("untyped payload accepted")
		}
	}
	bound, _ := common.BindRuntimeWorkspaceContext(context.Background(), "wrong-scope")
	if _, err := owners.workspace.queryPreparationDiagnostic(bound, workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: w.ID, DiagnosticID: cmd.Diagnostic.DiagnosticID}); err == nil {
		t.Fatal("wrong context scope accepted")
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(bound, definitionSpecificationInternalEvent{WorkspaceID: w.ID, PreparationDiagnostic: &cmd}); err == nil {
		t.Fatal("wrong record scope accepted")
	}
	if err := owners.repositories.Workspaces.Create(context.Background(), &workspace.Workspace{ID: "legacy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := owners.workspace.queryPreparationDiagnostic(context.Background(), workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: "legacy", DiagnosticID: cmd.Diagnostic.DiagnosticID}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing current used compatibility: %v", err)
	}
}

func TestPreparationDiagnosticDeletionFenceRejectsDuplicateAndQuery(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w, cmd := seedPreparationWorkspace(t, owners, "ws-prep-deleting")
	submitPreparation(t, owners, cmd)
	current, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	current.Deletion = &workspace.WorkspaceDeletionRecord{DeletionID: "deletion-prep", DeletionGeneration: 1, RequestID: "delete-prep", Mode: deletioncontract.ModeRetain, Phase: workspace.WorkspaceDeletionPhaseRequested, Revision: 1, RequestedAt: now, UpdatedAt: now, SealedDeliverableDirectory: "/tmp/prep-deletion", SealedInternalDirectory: "/tmp/prep-internal"}
	current.Deletion.IntentHash = workspace.StableWorkspaceDeletionIntentHash(current.ID, *current.Deletion)
	if err := current.Deletion.Validate(current.ID); err != nil {
		t.Fatal(err)
	}
	if _, matched, err := replayPreparationDiagnostic(current, cmd); err == nil || matched {
		t.Fatal("duplicate bypassed deletion fence")
	}
	if _, err := reducePreparationDiagnostic(definitionSpecificationObservation{workspace: current, template: &current.DefinitionAuthoring.TemplateSnapshot}, cmd); err == nil {
		t.Fatal("reducer duplicate bypassed deletion fence")
	}
	// 测试用只读 wrapper 注入已落地删除前像，验证查询入口的 fence。
	owners.workspace.workspaces = preparationObservedRepository{WorkspaceRepository: owners.workspace.workspaces, observed: current}
	query := workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: w.ID, DiagnosticID: cmd.Diagnostic.DiagnosticID}
	if _, err := owners.workspace.queryPreparationDiagnostic(ctx, query); err == nil {
		t.Fatal("raw query bypassed deletion fence")
	}
}

type preparationObservedRepository struct {
	repository.WorkspaceRepository
	observed *workspace.Workspace
}

func (r preparationObservedRepository) Get(context.Context, common.WorkspaceID) (*workspace.Workspace, error) {
	return r.observed, nil
}

func TestPreparationDiagnosticSuccessorArchiveAndCurrentScope(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	confirmation := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-prep-history")
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	d := workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: current.ID, InputHash: current.DefinitionAuthoring.InputHash, TemplateSnapshotHash: current.DefinitionAuthoring.TemplateSnapshotHash, ObservedWorkspaceRevision: current.Revision, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "unreserved-old", PhaseCode: "render"}, []byte("归档原始 cause\r\n"))
	cmd := workspaceevents.RecordPreparationDiagnosticCommand{Diagnostic: d}
	submitPreparation(t, owners, cmd)
	confirmation.ExpectedWorkspaceRevision++
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation); err != nil {
		t.Fatal(err)
	}
	reviewedAuthoringSuccessor(t, owners, current.ID, false)
	current, err = owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.DefinitionAuthoringHistory) != 1 || len(current.DefinitionAuthoringHistory[0].Record.PreparationDiagnostics) != 1 || current.ValidateDefinitionAuthoringHistory() != nil {
		t.Fatal("confirmed archive lost diagnosis")
	}
	query := workspaceevents.QueryPreparationDiagnosticCommand{WorkspaceID: current.ID, DiagnosticID: d.DiagnosticID}
	if result, err := owners.workspace.queryPreparationDiagnostic(ctx, query); err != nil || !bytes.Equal(result.Diagnostic.Cause, d.Cause) {
		t.Fatalf("archive scope query: %v", err)
	}
	fresh := workspace.NewDefinitionAuthoringPreparationDiagnostic(workspace.DefinitionAuthoringPreparationDiagnostic{WorkspaceID: current.ID, InputHash: current.DefinitionAuthoring.InputHash, TemplateSnapshotHash: current.DefinitionAuthoring.TemplateSnapshotHash, ObservedWorkspaceRevision: current.Revision, Stage: workspace.DefinitionAuthoringPreparationDerivation, TargetRequestID: "unreserved-successor", PhaseCode: "render"}, []byte("后继原始 cause"))
	submitPreparation(t, owners, workspaceevents.RecordPreparationDiagnosticCommand{Diagnostic: fresh})
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if result, err := owners.workspace.queryPreparationDiagnostic(ctx, query); err != nil || !bytes.Equal(result.Diagnostic.Cause, d.Cause) {
		t.Fatalf("history reopen: %v", err)
	}
	query.DiagnosticID = fresh.DiagnosticID
	if result, err := owners.workspace.queryPreparationDiagnostic(ctx, query); err != nil || !bytes.Equal(result.Diagnostic.Cause, fresh.Cause) {
		t.Fatalf("current reopen: %v", err)
	}
}
