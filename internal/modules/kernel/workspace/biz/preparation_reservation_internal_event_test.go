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
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	bootstrap := f.seedWorkspace(t, "reservation-ws", "prior-run", true)
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
	// This case exercises invalidation by an existing-Run Restart. A first
	// execution must reject an unknown preparation, covered separately below.
	w.ActiveRunID = "existing-run"
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	seedPreparationRecord(t, w, workspacemodel.WorkspacePreparationAccepted)
	if err := repo.CompareAndSwap(context.Background(), w, w.Revision); err != nil {
		t.Fatal(err)
	}
	before, err := preparationStateFromPersistence(w, time.Now())
	if err != nil || before.CurrentValidity != "not_evaluated" {
		t.Fatalf("fixture: %+v %v", before, err)
	}
	fact := workspaceFreshIntentReserved{WorkspaceID: w.ID, ExpectedRevision: w.Revision,
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}}, Reservation: workspacemodel.WorkspaceFreshIntentReservation{
			RequestID: "fresh", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: w.StartupInput.StartupInputHash, PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64),
		}}
	owner := &Biz{workspaces: repo}
	after, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact)
	if err != nil || !first {
		t.Fatalf("reserve: %v", err)
	}
	state, err := preparationStateFromPersistence(after, time.Now())
	if err != nil || state.CurrentValidity != "stale" || state.State != "accepted" || !reflect.DeepEqual(state.Record, before.Record) {
		t.Fatalf("reservation rewrote preparation history or retained currentness: %+v %v", state, err)
	}
}

func TestFirstExecutionRejectsUnresolvedPreparationWithoutWrites(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "first-preparation-required")
	w.PreparationFence = &workspacemodel.WorkspacePreparationFence{Version: workspacemodel.WorkspacePreparationFenceVersion, WorkspaceID: w.ID, CreationEpoch: strings.Repeat("a", 32)}
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	seedPreparationRecord(t, w, workspacemodel.WorkspacePreparationAccepted)
	if err := repo.CompareAndSwap(context.Background(), w, w.Revision); err != nil {
		t.Fatal(err)
	}

	fact := workspaceFreshIntentReserved{WorkspaceID: w.ID, ExpectedRevision: w.Revision,
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}},
		Reservation:       workspacemodel.WorkspaceFreshIntentReservation{RequestID: "first", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: w.StartupInput.StartupInputHash, PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64)}}
	if _, first, err := (&Biz{workspaces: repo}).submitWorkspaceFreshIntentReserved(context.Background(), fact); err == nil || first {
		t.Fatal("unknown preparation granted first execution")
	}
	after, err := repo.Get(context.Background(), w.ID)
	if err != nil || !reflect.DeepEqual(w, after) {
		t.Fatal("rejected first execution changed workspace", err)
	}
}

func TestFreshReservationProviderMetadataSharesExactCASPreimage(t *testing.T) {
	for _, arm := range []string{"success", "stale", "invalid_selection", "before_cas", "after_cas", "existing_default"} {
		t.Run(arm, func(t *testing.T) {
			current, command := reservationTestInput(t)
			command.Bootstrap.Activation.ProviderSelection.ReasoningEffort = "high"
			if arm == "existing_default" {
				current.DefaultLLMSelection = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "preferred", ModelID: "retained"}}
			}
			beforeSelection := current.DefaultLLMSelection
			fact, err := workspaceFreshIntentReservationFact(command, current.Revision)
			if err != nil {
				t.Fatal(err)
			}
			switch arm {
			case "stale":
				fact.ExpectedRevision++
			case "invalid_selection":
				fact.ProviderSelection.Target.ModelID = ""
			}
			repo := memoryrepo.NewWorkspaceRepository()
			if err := repo.Create(context.Background(), current); err != nil {
				t.Fatal(err)
			}
			owner := &Biz{workspaces: repo}
			if arm == "before_cas" || arm == "after_cas" {
				owner.workspaces = &reservationFaultRepository{WorkspaceRepository: repo, afterCommit: arm == "after_cas"}
			}
			_, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact)
			stored, readErr := repo.Get(context.Background(), current.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			committed := arm == "success" || arm == "existing_default" || arm == "after_cas"
			if !committed {
				if err == nil || first || !reflect.DeepEqual(current, stored) {
					t.Fatal("rejected/failed reservation left partial provider metadata", err)
				}
				return
			}
			if stored.Revision != current.Revision+1 || stored.PreparationFence.Pending == nil || stored.DefaultLLMSelection == nil || stored.DefaultLLMSelection.ReasoningEffort != "" {
				t.Fatal("metadata/reservation did not share one exact CAS preimage")
			}
			if arm == "existing_default" && !stored.DefaultLLMSelection.Same(*beforeSelection) {
				t.Fatal("reservation replaced existing Workspace preference")
			}
			if arm == "after_cas" && (!errors.Is(err, errReservationLostReply) || first) {
				t.Fatal("lost combined CAS reply granted forward permission")
			}
			if arm != "after_cas" && (err != nil || !first) {
				t.Fatal("initial combined CAS did not grant exactly one forward", err)
			}
			// Original replay remains first even when current metadata/inputs differ.
			fact.ProviderSelection = runtimecontract.ProviderInferenceSelection{}
			restarted := &Biz{workspaces: repo}
			if _, first, err := restarted.submitWorkspaceFreshIntentReserved(context.Background(), fact); err != nil || first {
				t.Fatal("original replay revalidated mutable metadata or forwarded", err)
			}
		})
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

func TestFirstExecutionPreparationCannotBeBypassedByMissingConfirmationOrAdvancedEpoch(t *testing.T) {
	w := &workspacemodel.Workspace{ID: "ws", PreparationFence: &workspacemodel.WorkspacePreparationFence{PreparationEpoch: 0}}
	if !requiresFirstExecutionPreparation(w) {
		t.Fatal("missing confirmation bypassed preparation")
	}
	w.PreparationFence.PreparationEpoch = 1
	if !requiresFirstExecutionPreparation(w) {
		t.Fatal("an earlier reservation proves no Run")
	}
	w.PreparationFence.Settled = []workspacemodel.WorkspaceFreshIntentSettlement{{Receipt: runmodel.WorkspaceExecutionIntentReceipt{Admission: runtimecontract.RuntimeExecutionIntentAdmission{Reason: runtimecontract.RuntimeExecutionIntentReasonDefinitionAdmissionMissing}}}}
	if !requiresFirstExecutionPreparation(w) {
		t.Fatal("rejection bypassed first preparation")
	}
	w.PreparationFence.Settled[0].Receipt.Admission.Reason = runtimecontract.RuntimeExecutionIntentReasonNewRunActivated
	if !requiresFirstExecutionPreparation(w) {
		t.Fatal("missing accepted Run identity bypassed preparation")
	}
	w.PreparationFence.Settled[0].Receipt.Admission.RunID = "original-run"
	if requiresFirstExecutionPreparation(w) {
		t.Fatal("Restart of an accepted original must not consume first no-Run completion")
	}
	w.PreparationFence = nil
	if !requiresFirstExecutionPreparation(w) {
		t.Fatal("legacy absence is not earlier execution")
	}
	w.ActiveRunID = "original-run"
	if requiresFirstExecutionPreparation(w) {
		t.Fatal("existing Run must retain Restart semantics")
	}
}
