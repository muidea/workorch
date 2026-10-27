package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestPreparationRunHistoryRepositoryPrivateCloneCASAndImmutableReopen(t *testing.T) {
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
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
			w := &workspace.Workspace{ID: "ws-history", PreparationFence: &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws-history", CreationEpoch: strings.Repeat("a", 32)}}
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{{RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1, CreationEpoch: w.PreparationFence.CreationEpoch, State: workspace.WorkspacePreparationAccepted, StartedAt: now, Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"}, Checks: []workspace.WorkspacePreparationCheckResult{{ID: "first_activation_no_run", Owner: "run", Required: true, State: "pending", Reason: "not_observed"}}}}}
			if err := repo.CompareAndSwap(ctx, w, w.Revision); err != nil {
				t.Fatal(err)
			}
			current, err := repo.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := &current.StartupPreparation.Attempts[0]
			a.State, a.CompletedAt, a.CommittedWorkspaceRevision = workspace.WorkspacePreparationBlocked, now.Add(time.Second), current.Revision+1
			a.Checks[0].State, a.Checks[0].Reason = "blocked", "run_history_retired"
			e := workspace.WorkspacePreparationRunHistoryEvidence{Version: workspace.PreparationRunHistoryEvidenceVersion, Ref: workspace.PreparationRunHistoryRefPrefix + strings.Repeat("b", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, CreationEpoch: a.CreationEpoch, PreparationEpoch: a.PreparationEpoch, Facts: workspace.PreparationRunHistoryFacts{Version: workspace.PreparationRunHistoryEvidenceVersion, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, Observation: runevents.ObserveWorkspacePreparationHistoryResult{RequestID: a.RequestID, ObservedAt: now.Add(time.Millisecond), History: runmodel.WorkspacePreparationHistory{WorkspaceID: w.ID, GenerationHead: 5, RunCount: 3, PendingActivationCount: 1, ReplacementCount: 1, Retirement: &runmodel.WorkspaceRetirementFence{WorkspaceID: w.ID, DeletionID: "original-deletion", DeletionGeneration: 1, DataDispositionIntent: "retain", WorkspaceIntentHash: "original-intent", Phase: "retired", FenceRevision: 2, WorkspaceRunGeneration: 5, QuiescenceID: "original-quiescence", RuntimeInventoryHash: "original-inventory"}}}}}
			e.ContentHash = e.Hash()
			current.PreparationRunHistoryEvidence, a.RunHistoryEvidenceRef = []workspace.WorkspacePreparationRunHistoryEvidence{e}, e.Ref
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, err := repo.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			current.PreparationRunHistoryEvidence[0].Facts.Observation.History.Retirement.QuiescenceID = "caller"
			current.PreparationRunHistoryEvidence[0].Facts.Observation.History.GenerationHead = 0
			if backend == "file" {
				info, err := os.Stat(filepath.Join(root, "workspaces.json"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("private history file is not 0600", err)
				}
			}
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) {
				t.Fatal("private history clone/reopen drift", err)
			}
			for _, change := range []string{"erase", "generation", "retirement", "scope", "binding", "epoch", "reason", "pass"} {
				bad, err := repo.Get(ctx, w.ID)
				if err != nil {
					t.Fatal(err)
				}
				switch change {
				case "erase":
					bad.PreparationRunHistoryEvidence = nil
					bad.StartupPreparation.Attempts[0].RunHistoryEvidenceRef = ""
				case "generation":
					bad.PreparationRunHistoryEvidence[0].Facts.Observation.History.GenerationHead++
					bad.PreparationRunHistoryEvidence[0].Facts.Observation.History.Retirement.WorkspaceRunGeneration++
				case "retirement":
					bad.PreparationRunHistoryEvidence[0].Facts.Observation.History.Retirement.QuiescenceID = "replacement"
				case "scope":
					bad.PreparationRunHistoryEvidence[0].Facts.Observation.History.WorkspaceID = "other"
				case "binding":
					bad.PreparationRunHistoryEvidence[0].Binding.StartupInputHash = "replacement"
				case "epoch":
					bad.PreparationRunHistoryEvidence[0].PreparationEpoch++
				case "reason":
					bad.StartupPreparation.Attempts[0].Checks[0].Reason = "run_history_empty_fence_not_final"
				case "pass":
					bad.StartupPreparation.Attempts[0].Checks[0].State = "passed"
				}
				if len(bad.PreparationRunHistoryEvidence) > 0 {
					bad.PreparationRunHistoryEvidence[0].ContentHash = bad.PreparationRunHistoryEvidence[0].Hash()
				}
				if repo.CompareAndSwap(ctx, bad, bad.Revision) == nil {
					t.Fatal("completed history rewritten after rehash", change)
				}
			}
		})
	}
}
