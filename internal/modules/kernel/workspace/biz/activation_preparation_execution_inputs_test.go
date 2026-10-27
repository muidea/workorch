package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func executionInputsOriginal(t *testing.T, owners *fileDefinitionAdmissionOwners) (*workspace.Workspace, workspacePreparationBlocked) {
	t.Helper()
	ctx := context.Background()
	w := seedPreparationEligibleWithConfiguration(t, owners, "ws-execution-inputs", t.TempDir(), func(w *workspace.Workspace) {
		w.DefaultSoulID = "declared-soul"
		w.ContextHints = payload.New(payload.StringField("custom", "original"))
	})
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-inputs", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true); err != nil {
		t.Fatal(err)
	}
	accepted, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	seal := owners.workspace.observeOriginalPreparationSeal(ctx, c)
	if seal == nil {
		t.Fatal("original seal unavailable")
	}
	fact := workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", SealEvidence: seal}
	p := workspace.PreparationProcessPolicy{ConfiguredDialogLanguage: "  zh_CN.UTF-8  ", LCAll: "en_US.UTF-8", MaxRuntimeRepairAttempts: 7, MaxDocumentRecoveryAttempts: 2}
	fact.ProcessPolicyFacts = &workspace.PreparationProcessPolicyFacts{Version: workspace.PreparationExecutionInputsVersion, WorkspaceID: accepted.ID, WorkspaceRevision: accepted.Revision, StartedAt: time.Now().UTC(), Before: p, After: p}
	fact.ProcessPolicyFacts.CompletedAt = time.Now().UTC()
	fact.At = time.Now().UTC()
	return accepted, fact
}

func TestPreparationExecutionInputsOriginalCASPrivateCloneAndReopen(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	accepted, fact := executionInputsOriginal(t, owners)
	r, err := reduceWorkspacePreparationBlocked(accepted, fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("LC_ALL", "en_US.UTF-8")
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
				t.Fatal(err)
			}
			before := *accepted
			before.Revision = base.Revision
			if err := repo.CompareAndSwap(ctx, &before, base.Revision); err != nil {
				t.Fatal(err)
			}
			next := *r.Next
			if err := repo.CompareAndSwap(ctx, &next, before.Revision); err != nil {
				t.Fatal(err)
			}
			saved, err := repo.Get(ctx, base.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := saved.StartupPreparation.Attempts[0]
			if a.ExecutionInputVersion != workspace.PreparationExecutionInputsVersion || a.State != workspace.WorkspacePreparationBlocked || saved.ActiveRunID != "" || saved.PreparationSealEvidence[0].ExecutionInputs == nil {
				t.Fatal("missing archive or new authority")
			}
			policy, err := activationports.CompileOriginalPreparedExecutionPolicy(saved, a)
			if err != nil || policy.DialogLanguage != "zh-CN" || policy.ExecutionContract.MaxRepairAttemptsPerFocus != 7 || policy.ExecutionContract.CandidateDocumentRecovery.MaxAttempts != 2 {
				t.Fatal("original policy compilation", err)
			}
			raw, _ := json.Marshal(saved)
			if bytes.Contains(raw, []byte(`"ConfiguredDialogLanguage":`)) || bytes.Contains(raw, []byte(`"ExecutionInputs":`)) {
				t.Fatal("private inputs leaked")
			}
			// Clone mutation must not alter persisted facts; reopening must consume
			// the original values, independent of the current process environment.
			saved.PreparationSealEvidence[0].ExecutionInputs.Process.Before.MaxRuntimeRepairAttempts = 99
			saved.PreparationSealEvidence[0].ExecutionInputs.Template.DoDItems[0] = "caller"
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, base.ID)
			if err != nil {
				t.Fatal(err)
			}
			again, err := activationports.CompileOriginalPreparedExecutionPolicy(read, read.StartupPreparation.Attempts[0])
			if err != nil || !reflect.DeepEqual(policy, again) {
				t.Fatal("reopen borrowed current inputs", err)
			}
			for _, change := range []string{"remove", "limit", "template", "workspace", "marker", "time"} {
				bad, _ := repo.Get(ctx, base.ID)
				e := &bad.PreparationSealEvidence[0]
				switch change {
				case "remove":
					e.ExecutionInputs = nil
					bad.StartupPreparation.Attempts[0].ExecutionInputVersion = ""
				case "limit":
					e.ExecutionInputs.Process.Before.MaxRuntimeRepairAttempts++
					e.ExecutionInputs.Process.After = e.ExecutionInputs.Process.Before
				case "template":
					e.ExecutionInputs.Template.Name += "changed"
				case "workspace":
					e.ExecutionInputs.Configuration.ContextHints = payload.New(payload.StringField("unexpected", "caller"))
				case "marker":
					bad.StartupPreparation.Attempts[0].ExecutionInputVersion = "other"
				case "time":
					e.ExecutionInputs.Process.StartedAt = a.StartedAt.Add(-time.Second)
				}
				e.ContentHash = e.Hash()
				if err := repo.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
					t.Fatalf("rewrote completed %s", change)
				}
			}
		})
	}
}

func TestPreparationExecutionInputsFirstCASRechecksActualWorkspacePreimage(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	accepted, fact := executionInputsOriginal(t, owners)
	r, err := reduceWorkspacePreparationBlocked(accepted, fact)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"workspace", "template", "revision", "process_time", "version"} {
		bad := *r.Next
		bad.PreparationSealEvidence = workspace.ClonePreparationSealEvidence(r.Next.PreparationSealEvidence)
		e := &bad.PreparationSealEvidence[0]
		switch change {
		case "workspace":
			e.ExecutionInputs.Configuration.ContextHints = payload.New(payload.StringField("unexpected", "caller"))
		case "template":
			e.ExecutionInputs.Template.Name += "changed"
		case "revision":
			e.ExecutionInputs.Process.WorkspaceRevision++
		case "process_time":
			e.ExecutionInputs.Process.CompletedAt = fact.At.Add(time.Second)
		case "version":
			e.ExecutionInputs.Version = "other"
		}
		e.ContentHash = e.Hash()
		if workspace.ValidatePreparationRecordTransition(accepted, &bad, true) == nil {
			t.Fatalf("self-rehashed %s passed first CAS", change)
		}
	}
	cleared := *accepted
	cleared.DefaultSoulID = ""
	if workspace.ValidatePreparationExecutionInputsPreimage(&cleared, r.Next) == nil {
		t.Fatal("complete source check accepted a missing real sidecar binding")
	}
}

func TestPreparationExecutionInputsNeverBackfillInterruptedStaleOrOldHistory(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	accepted, fact := executionInputsOriginal(t, owners)
	for _, scenario := range []string{"missing", "stale", "interrupted"} {
		before := *accepted
		f := fact
		if scenario == "missing" {
			f.ProcessPolicyFacts = nil
		}
		if scenario == "stale" {
			before.Revision++
		}
		if scenario == "interrupted" {
			f = workspacePreparationBlocked{Command: fact.Command, Reason: "original_check_interrupted", At: time.Now().UTC()}
		}
		r, err := reduceWorkspacePreparationBlocked(&before, f)
		if err != nil {
			t.Fatal(scenario, err)
		}
		a := r.Next.StartupPreparation.Attempts[0]
		if a.ExecutionInputVersion != "" {
			t.Fatal("backfilled", scenario)
		}
		if _, err := activationports.CompileOriginalPreparedExecutionPolicy(r.Next, a); err == nil {
			t.Fatal("compiled absent source", scenario)
		}
		// A duplicate carrying fresh settings cannot upgrade the completed attempt.
		f = fact
		again, err := reduceWorkspacePreparationBlocked(r.Next, f)
		if err != nil || !again.Noop || !reflect.DeepEqual(r.Next, again.Next) {
			t.Fatal("duplicate backfilled source", scenario, err)
		}
	}
}
