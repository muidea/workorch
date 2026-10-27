package workspace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func reservationTestInput(t *testing.T) (*workspacemodel.Workspace, workspaceevents.WorkspaceExecutionIntentCommand) {
	t.Helper()
	f := newWorkspaceExecutionIntentFixture(t, func(runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		t.Error("reservation forwarded an execution intent")
		return runevents.WorkspaceExecutionIntentResult{}
	})
	bootstrap := f.seedWorkspace(t, "reservation-ws", "", true)
	current, err := f.workspaces.Get(context.Background(), "reservation-ws")
	if err != nil {
		t.Fatal(err)
	}
	current.PreparationFence = &workspacemodel.WorkspacePreparationFence{
		Version: workspacemodel.WorkspacePreparationFenceVersion, WorkspaceID: current.ID, CreationEpoch: strings.Repeat("a", 32),
	}
	return current, workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: current.ID, RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap}
}

func TestFreshReservationConcurrentCASGrantsExactlyOneNewIdentity(t *testing.T) {
	current, cmd := reservationTestInput(t)
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	fact, err := workspaceFreshIntentReservationFact(cmd, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	owner := &Biz{workspaces: repo}
	var winners atomic.Int32
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			read, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact)
			if err != nil || read == nil || read.PreparationFence.Pending == nil {
				t.Errorf("reserve: %+v %v", read, err)
			}
			if first {
				winners.Add(1)
			}
		})
	}
	group.Wait()
	stored, _ := repo.Get(context.Background(), current.ID)
	if winners.Load() != 1 || stored.Revision != current.Revision+1 || stored.PreparationFence.PreparationEpoch != 1 {
		t.Fatalf("winners=%d stored=%+v", winners.Load(), stored)
	}
	// Admission and definition drift must not destroy the original transaction
	// identity. Reconstructed owner replays it without another CAS/permit.
	stored.ActivationAdmission = nil
	stored.StartupInput = nil
	if err := repo.CompareAndSwap(context.Background(), stored, stored.Revision); err != nil {
		t.Fatal(err)
	}
	restarted := &Biz{workspaces: repo}
	read, first, err := restarted.submitWorkspaceFreshIntentReserved(context.Background(), fact)
	if err != nil || first || read.Revision != stored.Revision {
		t.Fatalf("restarted replay: first=%v err=%v", first, err)
	}
	for _, altered := range []workspaceFreshIntentReserved{
		{WorkspaceID: fact.WorkspaceID, ExpectedRevision: stored.Revision, Reservation: workspacemodel.WorkspaceFreshIntentReservation{RequestID: "new", Kind: fact.Reservation.Kind, StartupInputHash: fact.Reservation.StartupInputHash, PayloadHash: fact.Reservation.PayloadHash, RunPayloadHash: fact.Reservation.RunPayloadHash}},
		{WorkspaceID: fact.WorkspaceID, ExpectedRevision: stored.Revision, Reservation: workspacemodel.WorkspaceFreshIntentReservation{RequestID: fact.Reservation.RequestID, Kind: fact.Reservation.Kind, StartupInputHash: fact.Reservation.StartupInputHash, PayloadHash: strings.Repeat("b", 64), RunPayloadHash: fact.Reservation.RunPayloadHash}},
	} {
		if _, first, err := restarted.submitWorkspaceFreshIntentReserved(context.Background(), altered); err == nil || first {
			t.Fatal("pending was bypassed or replaced")
		}
	}
}

type reservationFaultRepository struct {
	repository.WorkspaceRepository
	afterCommit bool
}

var errReservationLostReply = errors.New("reservation reply lost")

func (r *reservationFaultRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, revision uint64) error {
	if r.afterCommit {
		if err := r.WorkspaceRepository.CompareAndSwap(ctx, item, revision); err != nil {
			return err
		}
	}
	return errReservationLostReply
}

func TestFreshReservationCASFailureAndLostReplyNeverGrantPermit(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[afterCommit], func(t *testing.T) {
			current, cmd := reservationTestInput(t)
			repo := memoryrepo.NewWorkspaceRepository()
			if err := repo.Create(context.Background(), current); err != nil {
				t.Fatal(err)
			}
			fact, _ := workspaceFreshIntentReservationFact(cmd, current.Revision)
			owner := &Biz{workspaces: &reservationFaultRepository{WorkspaceRepository: repo, afterCommit: afterCommit}}
			if _, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact); !errors.Is(err, errReservationLostReply) || first {
				t.Fatalf("error granted permit: first=%v err=%v", first, err)
			}
			restarted := &Biz{workspaces: repo}
			read, first, err := restarted.submitWorkspaceFreshIntentReserved(context.Background(), fact)
			if err != nil || first == afterCommit || read.PreparationFence.PreparationEpoch != 1 {
				t.Fatalf("retry before/after cut: first=%v err=%v", first, err)
			}
		})
	}
}

func TestFreshReservationCompetingRequestsCannotBypassPending(t *testing.T) {
	current, cmd := reservationTestInput(t)
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	owner := &Biz{workspaces: repo}
	var winners, conflicts atomic.Int32
	var group sync.WaitGroup
	for _, requestID := range []string{"first", "second"} {
		command := cmd.Normalized()
		command.RequestID = requestID
		fact, err := workspaceFreshIntentReservationFact(command, current.Revision)
		if err != nil {
			t.Fatal(err)
		}
		group.Go(func() {
			_, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact)
			if first {
				winners.Add(1)
			} else if errors.Is(err, common.ErrConflict) {
				conflicts.Add(1)
			} else {
				t.Errorf("unexpected competing result: first=%v err=%v", first, err)
			}
		})
	}
	group.Wait()
	read, _ := repo.Get(context.Background(), current.ID)
	if winners.Load() != 1 || conflicts.Load() != 1 || read.Revision != current.Revision+1 {
		t.Fatalf("winners=%d conflicts=%d revision=%d", winners.Load(), conflicts.Load(), read.Revision)
	}
}

func TestFreshReservationInvalidatesCurrentPreparationWithoutRewritingHistory(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "reservation-preparation-history")
	// The historical fixture may be legacy. Seed a distinct test repository at
	// creation rather than upgrading that persisted Workspace in place.
	w.PreparationFence = &workspacemodel.WorkspacePreparationFence{Version: workspacemodel.WorkspacePreparationFenceVersion, WorkspaceID: w.ID, CreationEpoch: strings.Repeat("a", 32)}
	seedPreparationRecord(t, w, workspacemodel.WorkspacePreparationUnknown)
	w.Revision++
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	before, err := preparationStateFromPersistence(w, time.Now())
	if err != nil || before.CurrentValidity != "not_evaluated" {
		t.Fatalf("fixture: %+v %v", before, err)
	}
	fact := workspaceFreshIntentReserved{WorkspaceID: w.ID, ExpectedRevision: w.Revision, Reservation: workspacemodel.WorkspaceFreshIntentReservation{
		RequestID: "fresh", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: w.StartupInput.StartupInputHash, PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64),
	}}
	owner := &Biz{workspaces: repo}
	after, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact)
	if err != nil || !first {
		t.Fatalf("reserve: %v", err)
	}
	state, err := preparationStateFromPersistence(after, time.Now())
	if err != nil || state.CurrentValidity != "stale" || state.State != "unknown" || !reflect.DeepEqual(state.Record, before.Record) {
		t.Fatalf("reservation rewrote preparation history or retained currentness: %+v %v", state, err)
	}
}

func TestFreshReservationRejectsLegacyStaleScopeCancellationAndAdmissionDrift(t *testing.T) {
	current, cmd := reservationTestInput(t)
	fact, err := workspaceFreshIntentReservationFact(cmd, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"legacy", "stale", "scope", "cancelled", "admission_drift", "startup_drift"} {
		t.Run(scenario, func(t *testing.T) {
			seed := *current
			seed.PreparationFence = current.PreparationFence.Clone()
			ctx := context.Background()
			other := fact
			switch scenario {
			case "legacy":
				seed.PreparationFence = nil
			case "stale":
				other.ExpectedRevision++
			case "scope":
				ctx = common.ContextWithRuntimeWorkspaceID(ctx, "other")
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "admission_drift":
				seed.ActivationAdmission = nil
			case "startup_drift":
				other.Reservation.StartupInputHash = "different"
			}
			repo := memoryrepo.NewWorkspaceRepository()
			if err := repo.Create(context.Background(), &seed); err != nil {
				t.Fatal(err)
			}
			owner := &Biz{workspaces: repo}
			if _, first, err := owner.submitWorkspaceFreshIntentReserved(ctx, other); err == nil || first {
				t.Fatalf("%s reserved", scenario)
			}
			read, _ := repo.Get(context.Background(), seed.ID)
			if !reflect.DeepEqual(read, &seed) {
				t.Fatalf("%s mutated on rejection", scenario)
			}
		})
	}
}

func TestFreshReservationIdentityBindsCompleteTypedInput(t *testing.T) {
	current, cmd := reservationTestInput(t)
	fact, err := workspaceFreshIntentReservationFact(cmd, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := workspaceFreshIntentReservationFact(cmd.Normalized(), current.Revision+1)
	if err != nil || duplicate.Reservation != fact.Reservation {
		t.Fatal("revision was incorrectly part of immutable payload identity")
	}
	spaced := cmd.Normalized()
	spaced.RequestID = " " + spaced.RequestID + " "
	spaced.Kind = " restart "
	canonical, err := workspaceFreshIntentReservationFact(spaced, current.Revision)
	if err != nil || canonical.Reservation != fact.Reservation {
		t.Fatalf("normalization changed full payload identity: %v", err)
	}
	for _, field := range []string{"request", "scope", "model"} {
		changed := cmd.Normalized()
		switch field {
		case "request":
			changed.RequestID = "other"
		case "scope":
			changed.WorkspaceID = "other"
		case "model":
			changed.Bootstrap.Activation.ProviderSelection.Target.ModelID = "other-model"
		}
		identity, err := workspaceFreshIntentReservationFact(changed, current.Revision)
		if err != nil || identity.Reservation.PayloadHash == fact.Reservation.PayloadHash {
			t.Fatalf("%s not included: %v", field, err)
		}
	}
	cmd.Kind = runtimecontract.RuntimeExecutionIntentResume
	cmd.Bootstrap = nil
	if _, err := workspaceFreshIntentReservationFact(cmd, current.Revision); err == nil {
		t.Fatal("resume treated as fresh")
	}
}

func TestWorkspaceCreationMintsDistinctPreparationEpochsOnlyAtCreation(t *testing.T) {
	repo := memoryrepo.NewWorkspaceRepository()
	manager := &goalDoDManager{workspaces: repo}
	for _, id := range []common.WorkspaceID{"a", "b"} {
		fact := workspaceCreationInternalEvent{Kind: workspaceCreationInternalEventRequested, Requested: &workspaceCreationRequestedFact{WorkspaceID: id, Title: "title", DeliverableDirectory: "/tmp/test"}}
		created, err := manager.submitWorkspaceCreationInternalEvent(context.Background(), fact)
		if err != nil || created.PreparationFence == nil || created.PreparationFence.Validate(id) != nil || created.PreparationFence.Pending != nil {
			t.Fatalf("creation: %+v %v", created, err)
		}
		before := created.PreparationFence.Clone()
		if _, err := manager.submitWorkspaceCreationInternalEvent(context.Background(), fact); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("duplicate creation: %v", err)
		}
		read, _ := repo.Get(context.Background(), id)
		if !reflect.DeepEqual(before, read.PreparationFence) {
			t.Fatal("duplicate creation replaced marker")
		}
	}
	a, _ := repo.Get(context.Background(), "a")
	b, _ := repo.Get(context.Background(), "b")
	if a.PreparationFence.CreationEpoch == b.PreparationFence.CreationEpoch {
		t.Fatal("creation epochs reused")
	}
}
