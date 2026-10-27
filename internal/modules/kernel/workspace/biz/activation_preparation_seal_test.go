package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestPreparationSealOriginalOwnerMaterialCASAndReopen(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	ready := seedPreparationEligible(t, owners, "ws-seal-source")
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: ready.ID, RequestID: "seal-original", ExpectedWorkspaceRevision: ready.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true); err != nil {
		t.Fatal(err)
	}
	accepted, err := owners.repositories.Workspaces.Get(ctx, ready.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := owners.workspace.observeOriginalPreparationSeal(ctx, c)
	if e == nil {
		t.Fatal("real exact source was not observed")
	}
	fact := workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", SealEvidence: e, At: time.Now().UTC()}
	transition, err := reduceWorkspacePreparationBlocked(accepted, fact)
	if err != nil {
		t.Fatal("private seal consumer", err)
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
			base := *ready
			if err := repo.Create(ctx, &base); err != nil {
				t.Fatal(err)
			}
			before := *accepted
			before.Revision = base.Revision
			if err := repo.CompareAndSwap(ctx, &before, base.Revision); err != nil {
				t.Fatal("accept", err)
			}
			next := *transition.Next
			if err := repo.CompareAndSwap(ctx, &next, before.Revision); err != nil {
				t.Fatal("settle", err)
			}
			baseline, err := repo.Get(ctx, ready.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(baseline) != nil || len(baseline.PreparationSealEvidence) != 1 {
				t.Fatal("readback", err)
			}
			a := baseline.StartupPreparation.Attempts[0]
			passed := 0
			for _, check := range a.Checks {
				if check.ID == "exact_seal" || check.ID == "persistence_integrity" {
					if check.State != "passed" {
						t.Fatal("missing precise judgment", check)
					}
					passed++
				}
			}
			if passed != 2 || a.State != workspace.WorkspacePreparationBlocked || baseline.ActiveRunID != "" {
				t.Fatal("seal proof became readiness")
			}
			raw, _ := json.Marshal(baseline)
			if bytes.Contains(raw, []byte("PreparationSealEvidence")) || bytes.Contains(raw, []byte("ReobservedSeal")) {
				t.Fatal("private observation leaked")
			}
			for _, change := range []string{"erase", "row", "order", "review", "admission", "snapshot", "request", "owner", "mixed_stale", "version", "epoch", "downgrade"} {
				bad, _ := repo.Get(ctx, ready.ID)
				archive := &bad.PreparationSealEvidence[0]
				switch change {
				case "erase":
					bad.PreparationSealEvidence = nil
				case "row":
					archive.Before.Goal.Text += " changed"
					archive.After.Goal.Text = archive.Before.Goal.Text
				case "order":
					archive.Before.FormalDoDItemRefs = nil
					archive.After.FormalDoDItemRefs = nil
				case "review":
					bad.DefinitionAuthoring.SpecificationRequests = nil
				case "admission":
					archive.Before.ActivationAdmission.CandidateHash = "neighbor"
					archive.After.ActivationAdmission.CandidateHash = "neighbor"
				case "snapshot":
					bad.StartupPreparation.Attempts[0].ObservationSnapshotHash = "neighbor"
				case "request":
					archive.RequestID = "neighbor"
				case "owner":
					for i := range bad.StartupPreparation.Attempts[0].Checks {
						if bad.StartupPreparation.Attempts[0].Checks[i].ID == "exact_seal" {
							bad.StartupPreparation.Attempts[0].Checks[i].Owner = "caller"
						}
					}
				case "mixed_stale":
					for i := range bad.StartupPreparation.Attempts[0].Checks {
						check := &bad.StartupPreparation.Attempts[0].Checks[i]
						if check.ID == "exact_seal" {
							check.State, check.Reason = "blocked", "binding_changed"
						}
					}
				case "version":
					archive.Version = "future"
				case "epoch":
					bad.StartupPreparation.Attempts[0].PreparationEpoch++
				case "downgrade":
					bad.PreparationSealEvidence = nil
					bad.StartupPreparation.Attempts[0].SealEvidenceRef = ""
					for i := range bad.StartupPreparation.Attempts[0].Checks {
						check := &bad.StartupPreparation.Attempts[0].Checks[i]
						if check.ID == "exact_seal" || check.ID == "persistence_integrity" {
							check.State, check.Reason = "blocked", "check_pipeline_incomplete"
						}
					}
				}
				if change != "erase" && change != "downgrade" {
					archive.Before.SnapshotHash = archive.Before.Hash()
					archive.After.SnapshotHash = archive.After.Hash()
					if change == "row" || change == "order" {
						// Reach the full provenance gate even after the caller
						// makes every local content pin self-consistent.
						attempt := &bad.StartupPreparation.Attempts[0]
						attempt.ObservationSnapshotHash = archive.Before.SnapshotHash
						prior := archive.Before
						prior.WorkspaceRevision--
						attempt.Binding.SnapshotHash = prior.Hash()
						archive.Binding = attempt.Binding
					}
					archive.ContentHash = archive.Hash()
				}
				if change != "downgrade" && change != "epoch" && workspace.ValidatePreparationEvidencePersistence(bad) == nil {
					t.Fatal("independent readback trusted substituted source", change)
				}
				if repo.CompareAndSwap(ctx, bad, bad.Revision) == nil {
					t.Fatal("rehash/overwrite accepted", change)
				}
			}
			mutated, _ := repo.Get(ctx, ready.ID)
			mutated.PreparationSealEvidence[0].Before.DoD.Items[0] = "caller"
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, ready.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) {
				t.Fatal("clone/reopen changed original", err)
			}
			// A later authoring slot cannot replace the archived proof's exact
			// original ledger. This checks lookup, not successor admission.
			historical, _ := repo.Get(ctx, ready.ID)
			historical.DefinitionAuthoringHistory = append(historical.DefinitionAuthoringHistory, workspace.DefinitionAuthoringConfirmedRevision{Record: historical.DefinitionAuthoring.Clone(), Confirmation: historical.DefinitionAuthoringConfirmation.Clone()})
			historical.DefinitionAuthoring, historical.DefinitionAuthoringConfirmation = nil, nil
			if workspace.ValidatePreparationEvidencePersistence(historical) != nil {
				t.Fatal("original historical provenance was replaced with current source")
			}
		})
	}
}

func TestPreparationSealStaleInterruptedAndMissingNeverGrantCredit(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-seal-stale")
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true); err != nil {
		t.Fatal(err)
	}
	e := owners.workspace.observeOriginalPreparationSeal(ctx, c)
	accepted, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	if e == nil {
		t.Fatal("missing original source")
	}
	for _, scenario := range []string{"stale", "interrupted", "missing", "changed_second_read"} {
		t.Run(scenario, func(t *testing.T) {
			current := *accepted
			fact := workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", At: time.Now().UTC()}
			switch scenario {
			case "stale":
				current.Revision++
				fact.SealEvidence = e
			case "interrupted":
				fact.Reason = "original_check_interrupted"
			case "changed_second_read":
				bad := e.Clone()
				bad.After.Goal.Text += " drift"
				bad.After.SnapshotHash = bad.After.Hash()
				bad.ContentHash = bad.Hash()
				fact.SealEvidence = &bad
			}
			result, err := reduceWorkspacePreparationBlocked(&current, fact)
			if scenario == "changed_second_read" {
				if err == nil {
					t.Fatal("mixed exact observations accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range result.Next.StartupPreparation.Attempts[0].Checks {
				if check.State != "blocked" {
					t.Fatal("false stale/recovery credit", check)
				}
			}
			if scenario == "stale" && len(result.Next.PreparationSealEvidence) != 1 {
				t.Fatal("stale observation history discarded")
			}
		})
	}
}
