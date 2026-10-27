package repository_test

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestPreparationEvaluationRepositoryOriginalCASAndImmutableReadback(t *testing.T) {
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
			w := &workspace.Workspace{ID: "ws", PreparationFence: &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32)}}
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			a := workspace.WorkspacePreparationAttempt{RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1, CreationEpoch: w.PreparationFence.CreationEpoch, EvaluationVersion: workspace.PreparationCheckEvaluationVersion, State: workspace.WorkspacePreparationAccepted, StartedAt: now,
				Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"},
				Checks:  []workspace.WorkspacePreparationCheckResult{{ID: "directory_boundary", Owner: "file", Required: true, State: "pending", Reason: "not_observed", SubjectHash: workspace.PreparationFileSubjectHash("directory_boundary", "/internal", "/deliverable", "/deliverable", "")}, {ID: "exact_seal", Owner: "workspace", Required: true, State: "pending", Reason: "not_observed"}},
			}
			w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{a}}
			w.StartupPreparation.Attempts[0].ObservationSnapshotHash = "observed-snapshot"
			if err := repo.CompareAndSwap(ctx, w, w.Revision); err != nil {
				t.Fatal(err)
			}
			accepted, _ := repo.Get(ctx, w.ID)
			settle := func() *workspace.Workspace {
				next, err := repo.Get(ctx, w.ID)
				if err != nil {
					t.Fatal(err)
				}
				a := &next.StartupPreparation.Attempts[0]
				a.State, a.CompletedAt, a.CommittedWorkspaceRevision = workspace.WorkspacePreparationBlocked, now.Add(time.Second), next.Revision+1
				for i := range a.Checks {
					a.Checks[i].State, a.Checks[i].Reason = "blocked", "check_pipeline_incomplete"
				}
				dir := func(path string) fileobservation.Directory {
					return fileobservation.Directory{Path: path, Complete: true, Inode: 1, Mode: 0040700, Readable: true, Writable: true, Searchable: true, DiagnosticSHA256: fileobservation.Digest(nil)}
				}
				e := workspace.WorkspacePreparationDirectoryEvidence{Version: workspace.PreparationDirectoryEvidenceVersion, WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: a.ExpectedWorkspaceRevision + 1, StartedAt: now, CompletedAt: a.CompletedAt, Internal: dir("/internal"), Deliverable: dir("/deliverable")}
				e.ContentHash = e.Hash()
				a.DirectoryEvidenceHash = e.ContentHash
				a.Checks = workspace.EvaluatePreparationFileChecks(*a, &e, nil, nil)
				next.PreparationDirectoryEvidence = []workspace.WorkspacePreparationDirectoryEvidence{e}
				return next
			}
			for _, mutation := range []string{"subject", "version", "accepted_snapshot", "no_material", "sibling", "other_pass", "ordinary"} {
				next := settle()
				switch mutation {
				case "subject":
					next.StartupPreparation.Attempts[0].Checks[0].SubjectHash = strings.Repeat("b", 64)
				case "version":
					next.StartupPreparation.Attempts[0].EvaluationVersion = ""
				case "accepted_snapshot":
					next.StartupPreparation.Attempts[0].ObservationSnapshotHash = "substituted-snapshot"
				case "no_material":
					next.PreparationDirectoryEvidence = nil
				case "sibling":
					e := &next.PreparationDirectoryEvidence[0]
					e.Deliverable.Path = "/sibling"
					e.ContentHash = e.Hash()
					next.StartupPreparation.Attempts[0].DirectoryEvidenceHash = e.ContentHash
				case "other_pass":
					next.StartupPreparation.Attempts[0].Checks[1].State = "passed"
				}
				var err error
				if mutation == "ordinary" {
					err = repo.Update(ctx, next)
				} else {
					err = repo.CompareAndSwap(ctx, next, next.Revision)
				}
				if err == nil {
					t.Fatal("unproved/unauthorized settlement accepted", mutation)
				}
				got, _ := repo.Get(ctx, w.ID)
				if !reflect.DeepEqual(got, accepted) {
					t.Fatal("rejected CAS partially persisted", mutation)
				}
			}
			next := settle()
			if err := repo.CompareAndSwap(ctx, next, next.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, _ := repo.Get(ctx, w.ID)
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) || read.StartupPreparation.Attempts[0].Checks[0].State != "passed" || read.StartupPreparation.Attempts[0].State != workspace.WorkspacePreparationBlocked {
				t.Fatal("reopen lost exact per-check history", err)
			}
			read.StartupPreparation.Attempts[0].Checks[0].State = "blocked"
			if err := repo.CompareAndSwap(ctx, read, read.Revision); err == nil {
				t.Fatal("completed judgments overwritten")
			}
			fresh, _ := repo.Get(ctx, w.ID)
			if !reflect.DeepEqual(fresh, baseline) {
				t.Fatal("mutation aliases persisted checks")
			}
		})
	}
}
