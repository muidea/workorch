package workspace

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Deterministic history fixture for reducer/CAS fault tests, not live readiness.
// The separate actual-Run-owner test verifies the production observation path.
func noRunFinalOriginal(t *testing.T, owners *fileDefinitionAdmissionOwners) (*workspace.Workspace, workspacePreparationBlocked) {
	t.Helper()
	ctx := context.Background()
	w := seedPreparationEligible(t, owners, "ws-no-run-final")
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-final", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true); err != nil {
		t.Fatal(err)
	}
	seal := owners.workspace.observeOriginalPreparationSeal(ctx, c)
	if seal == nil {
		t.Fatal("original complete seal missing")
	}
	accepted, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	f := workspace.PreparationRunHistoryFacts{Version: workspace.PreparationRunHistoryEvidenceVersion, WorkspaceRevision: accepted.Revision,
		Observation: runevents.ObserveWorkspacePreparationHistoryResult{RequestID: c.RequestID, ObservedAt: time.Now().UTC(), History: runmodel.WorkspacePreparationHistory{WorkspaceID: w.ID}}}
	e := owners.workspace.bindOriginalPreparationRunHistory(ctx, workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: c, RunHistoryFacts: &f, RunHistoryReason: f.BlockedReason()})
	if e == nil {
		t.Fatal("original history binding missing")
	}
	return accepted, workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", SealEvidence: seal, ProviderCacheReason: "provider_cache_miss", RunHistoryReason: f.BlockedReason(), RunHistoryEvidence: e, At: time.Now().UTC()}
}

func TestPreparationNoRunFinalFenceAllRepositoriesPreservePrivateValue(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	accepted, fact := noRunFinalOriginal(t, owners)
	r, err := reduceWorkspacePreparationBlocked(accepted, fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			open := func() repository.WorkspaceRepository {
				switch backend {
				case "file":
					return file.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
				case "scoped":
					return workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Workspaces
				default:
					return memory.NewWorkspaceRepository()
				}
			}
			repo := open()
			base := *accepted
			base.StartupPreparation, base.PreparationSkillHubEffects = nil, nil
			base.Revision = fact.Command.ExpectedWorkspaceRevision
			if err := repo.Create(ctx, &base); err != nil {
				t.Fatal("create", err)
			}
			before := *accepted
			before.Revision = base.Revision
			if err := repo.CompareAndSwap(ctx, &before, base.Revision); err != nil {
				t.Fatal("accept", err)
			}
			next := *r.Next
			if err := repo.CompareAndSwap(ctx, &next, before.Revision); err != nil {
				t.Fatal("settle", err)
			}
			baseline, err := repo.Get(ctx, accepted.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(baseline) != nil || baseline.PreparationRunHistoryEvidence[0].FinalFence == nil {
				t.Fatal("readback", err)
			}
			mutated, _ := repo.Get(ctx, accepted.ID)
			mutated.PreparationRunHistoryEvidence[0].FinalFence.Fence.CreationEpoch = "caller"
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, accepted.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) {
				t.Fatal("clone/reopen lost original fence", err)
			}
		})
	}
}

func reserveNoRunTestIntent(t *testing.T, w *workspace.Workspace) *workspace.Workspace {
	t.Helper()
	// Inject only the lower-level fence transition to test the original no-Run
	// final-CAS race. This partial fixture cannot satisfy first-start readiness;
	// the execution ingress is tested separately and must reject it.
	next := *w
	next.PreparationFence = w.PreparationFence.Clone()
	next.PreparationFence.PreparationEpoch++
	next.PreparationFence.Pending = &workspace.WorkspaceFreshIntentReservation{
		RequestID: "later-fresh", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		StartupInputHash: w.StartupInput.StartupInputHash,
		PayloadHash:      strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64),
	}
	if err := workspace.ValidatePreparationFenceTransition(w, &next); err != nil {
		t.Fatal("lower-level reservation transition", err)
	}
	return &next
}

func TestPreparationNoRunFinalFenceRehashedTamperingRejected(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	accepted, fact := noRunFinalOriginal(t, owners)
	r, err := reduceWorkspacePreparationBlocked(accepted, fact)
	if err != nil || r.Next.StartupPreparation.Attempts[0].NoRunEvaluationVersion != workspace.PreparationNoRunFinalVersion {
		t.Fatal("exact original did not compile", err)
	}
	for _, change := range []string{"active_run", "creation", "revision", "next_run", "unknown"} {
		before, _ := owners.repositories.Workspaces.Get(ctx, accepted.ID)
		after := *r.Next
		switch change {
		case "active_run":
			before.ActiveRunID = "existing"
		case "creation":
			before.PreparationFence.CreationEpoch = strings.Repeat("d", 32)
		case "revision":
			before.Revision++
		case "next_run":
			after.ActiveRunID = "concurrent"
		case "unknown":
			before.StartupPreparation.Attempts[0].State = workspace.WorkspacePreparationUnknown
		}
		if workspace.ValidatePreparationRecordTransition(before, &after, true) == nil {
			t.Fatal("saved proof was trusted instead of actual CAS preimage", change)
		}
	}
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, r.Next, accepted.Revision); err != nil {
		t.Fatal("settlement", err)
	}
	for _, scenario := range []string{"missing_fence", "version", "scope", "revision", "active_run", "creation", "epoch", "pending", "settled", "history", "payload", "request", "seal", "missing_version", "orphan_version", "future_version", "optional", "owner", "reason", "blocked", "seal_blocked", "unknown", "legacy", "commit_revision", "downgrade"} {
		t.Run(scenario, func(t *testing.T) {
			w, err := owners.repositories.Workspaces.Get(ctx, accepted.ID)
			if err != nil {
				t.Fatal(err)
			}
			a, e := &w.StartupPreparation.Attempts[0], &w.PreparationRunHistoryEvidence[0]
			for i := range a.Checks {
				c := &a.Checks[i]
				if c.ID == "first_activation_no_run" {
					switch scenario {
					case "optional":
						c.Required = false
					case "owner":
						c.Owner = "caller"
					case "reason":
						c.Reason = "run_history_empty_fence_not_final"
					case "blocked":
						c.State = "blocked"
					case "downgrade":
						c.State, c.Reason = "blocked", "run_history_empty_fence_not_final"
					}
				}
				if c.ID == "exact_seal" && scenario == "seal_blocked" {
					c.State, c.Reason = "blocked", "binding_changed"
				}
			}
			switch scenario {
			case "missing_fence":
				e.FinalFence = nil
			case "version":
				e.FinalFence.Version = "future"
			case "scope":
				e.FinalFence.Fence.WorkspaceID = "other"
			case "revision":
				e.FinalFence.WorkspaceRevision++
			case "active_run":
				e.FinalFence.ActiveRunID = "run"
			case "creation":
				e.FinalFence.Fence.CreationEpoch = strings.Repeat("e", 32)
			case "epoch":
				e.FinalFence.Fence.PreparationEpoch++
			case "pending":
				e.FinalFence.Fence.Pending = &workspace.WorkspaceFreshIntentReservation{}
			case "settled":
				e.FinalFence.Fence.Settled = []workspace.WorkspaceFreshIntentSettlement{{}}
			case "history":
				e.Facts.Observation.History.GenerationHead = 1
			case "payload":
				e.PayloadHash = "other"
			case "request":
				e.RequestID, e.Facts.Observation.RequestID = "other", "other"
			case "seal":
				w.PreparationSealEvidence = nil
			case "missing_version":
				a.NoRunEvaluationVersion = ""
			case "orphan_version":
				a.RunHistoryEvidenceRef = ""
				w.PreparationRunHistoryEvidence = nil
			case "future_version":
				a.NoRunEvaluationVersion = "future"
			case "unknown":
				a.State = workspace.WorkspacePreparationUnknown
			case "legacy":
				w.StartupPreparation.Version = "unsupported-record-contract"
			case "commit_revision":
				a.CommittedWorkspaceRevision++
			case "downgrade":
				a.NoRunEvaluationVersion = ""
				e.FinalFence = nil
			}
			e.ContentHash = e.Hash()
			if scenario != "downgrade" && workspace.ValidatePreparationEvidencePersistence(w) == nil {
				t.Fatal("self-consistent rehash bypassed full proof")
			}
			if owners.repositories.Workspaces.CompareAndSwap(ctx, w, w.Revision) == nil {
				t.Fatal("immutable final observation overwritten")
			}
		})
	}
	// A caller cannot change its detached proof and thereby mutate the stored one.
	w, _ := owners.repositories.Workspaces.Get(ctx, accepted.ID)
	w.PreparationRunHistoryEvidence[0].FinalFence.Fence.CreationEpoch = "caller"
	read, _ := owners.repositories.Workspaces.Get(ctx, accepted.ID)
	if read.PreparationRunHistoryEvidence[0].FinalFence.Fence.CreationEpoch != accepted.PreparationFence.CreationEpoch {
		t.Fatal("private final fence clone aliases storage")
	}
}

func TestPreparationNoRunFinalFenceCASOrdersAndRecovery(t *testing.T) {
	for _, scenario := range []string{"reservation_first", "metadata_first", "preparation_first", "write_before", "write_after", "missing_seal", "missing_history", "unknown", "active_run", "historical"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			accepted, fact := noRunFinalOriginal(t, owners)
			original, err := reduceWorkspacePreparationBlocked(accepted, fact)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "reservation_first", "metadata_first":
				competing, _ := owners.repositories.Workspaces.Get(ctx, accepted.ID)
				if scenario == "reservation_first" {
					competing = reserveNoRunTestIntent(t, competing)
				} else {
					competing.Title += " updated"
				}
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, competing, competing.Revision); err != nil {
					t.Fatal(err)
				}
				if owners.repositories.Workspaces.CompareAndSwap(ctx, original.Next, accepted.Revision) == nil {
					t.Fatal("stale original CAS won")
				}
			case "preparation_first":
				if _, err := owners.workspace.finishBlockedPreparation(ctx, fact); err != nil {
					t.Fatal(err)
				}
				before, _ := owners.repositories.Workspaces.Get(ctx, accepted.ID)
				next := reserveNoRunTestIntent(t, before)
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, next, before.Revision); err != nil {
					t.Fatal(err)
				}
				read, _ := owners.repositories.Workspaces.Get(ctx, accepted.ID)
				if !reflect.DeepEqual(before.PreparationRunHistoryEvidence, read.PreparationRunHistoryEvidence) {
					t.Fatal("new reservation rewrote old proof")
				}
			case "write_before", "write_after":
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.repositories.Workspaces, failAt: 1, persistFirst: scenario == "write_after"}
				if _, err := owners.workspace.finishBlockedPreparation(ctx, fact); err == nil {
					t.Fatal("write cut was not observed")
				}
				owners.workspace.workspaces = owners.repositories.Workspaces
			case "missing_seal":
				fact.SealEvidence = nil
			case "missing_history":
				fact.RunHistoryEvidence = nil
				fact.RunHistoryReason = "run_history_unavailable"
			case "unknown":
				// A reducer fixture, not a permitted production mutation. It must
				// not turn an interrupted original into an observed final fence.
				accepted.StartupPreparation.Attempts[0].State = workspace.WorkspacePreparationUnknown
				r, err := reduceWorkspacePreparationBlocked(accepted, fact)
				if err != nil || r.Next.StartupPreparation.Attempts[0].NoRunEvaluationVersion != "" {
					t.Fatal("unknown upgraded", err)
				}
				return
			case "active_run":
				accepted.ActiveRunID = "existing"
				r, err := reduceWorkspacePreparationBlocked(accepted, fact)
				if err != nil || r.Next.StartupPreparation.Attempts[0].NoRunEvaluationVersion != "" {
					t.Fatal("active Run ignored", err)
				}
				return
			case "historical":
				fact.RunHistoryEvidence.Facts.Observation.History.GenerationHead = 1
				fact.RunHistoryEvidence.ContentHash = fact.RunHistoryEvidence.Hash()
				fact.RunHistoryReason = "run_history_present"
			}
			if scenario != "write_before" && scenario != "write_after" {
				if _, err := owners.workspace.finishBlockedPreparation(ctx, fact); err != nil {
					t.Fatal(err)
				}
			}
			// Explicit original recovery never repeats the observation or grants
			// the proof after a pre-write failure. A persisted reply loss replays.
			if _, err := owners.workspace.beginWorkspacePreparation(ctx, fact.Command, true, true); err != nil {
				t.Fatal("recover original", err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, accepted.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("stored proof", err)
			}
			a := stored.StartupPreparation.Attempts[0]
			wantFinal := scenario == "preparation_first" || scenario == "write_after"
			if (a.NoRunEvaluationVersion == workspace.PreparationNoRunFinalVersion) != wantFinal || a.State != workspace.WorkspacePreparationBlocked {
				t.Fatal("incorrect historical final judgment", scenario, a.NoRunEvaluationVersion)
			}
			if scenario == "reservation_first" || scenario == "metadata_first" || scenario == "preparation_first" {
				state, err := preparationStateFromPersistence(stored, time.Now().UTC())
				if err != nil || state.CurrentValidity != "stale" {
					t.Fatal("later mutation granted currentness", err)
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, accepted.ID)
			if err != nil || !reflect.DeepEqual(stored, read) {
				t.Fatal("owner reopen changed original", err)
			}
			inventory, err := reopened.repositories.Runs.List(ctx)
			if err != nil || len(inventory) != 0 {
				t.Fatal("CAS test created Run", err)
			}
		})
	}
}
