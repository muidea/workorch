package workspace

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
)

func seedPreparationRecord(t *testing.T, w *workspace.Workspace, state workspace.WorkspacePreparationAttemptState) {
	t.Helper()
	snapshot, err := startuppreparation.FromOwnerPersistence(w, &w.DefinitionAuthoringConfirmation.Goal, &w.DefinitionAuthoringConfirmation.DoD)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := startuppreparation.CompilePreparationManifest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	attempt := workspace.WorkspacePreparationAttempt{
		RequestID: "preparation-request", PayloadHash: "preparation-payload", ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1,
		Binding: workspace.WorkspacePreparationBinding{
			AuthoringRef: snapshot.AuthoringRef, CandidateID: snapshot.Candidate.CandidateID, CandidateRevision: snapshot.Candidate.CandidateRevision, CandidateHash: snapshot.Candidate.CandidateHash,
			DefinitionRevision: snapshot.StartupInput.DefinitionRevision, StartupInputHash: snapshot.StartupInput.StartupInputHash,
			AuthoringConfigurationHash: snapshot.AuthoringConfigurationHash, SnapshotHash: snapshot.SnapshotHash, ManifestHash: manifest.ManifestHash,
			CheckPolicyVersion: startuppreparation.CheckPolicyVersion, CompilerVersion: startuppreparation.BootstrapCompilerVersion,
		},
		StartedAt: time.Now().Add(-time.Minute).UTC(), State: state,
	}
	if state == workspace.WorkspacePreparationBlocked || state == workspace.WorkspacePreparationFailed {
		attempt.CompletedAt = attempt.StartedAt.Add(time.Second)
	}
	w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationRecordVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{attempt}}
}

func TestPreparationStateQueryIsReadOnlyAcrossFileReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	w := seedPreparationEligible(t, owners, "ws-preparation-state")
	command := workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID}
	state, err := owners.workspace.queryWorkspacePreparationState(ctx, command)
	if err != nil || state.Record != nil || state.State != "not_checked" || state.CurrentValidity != "not_checked" {
		t.Fatalf("unchecked projection: %+v %v", state, err)
	}
	w.ActiveRunID = "actual-run"
	seedPreparationRecord(t, w, workspace.WorkspacePreparationUnknown)
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, w, w.Revision); err != nil {
		t.Fatal(err)
	}
	stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	before, _ := json.Marshal(stored)
	state, err = owners.workspace.queryWorkspacePreparationState(ctx, command)
	if err != nil || state.State != "unknown" || state.CurrentValidity != "not_evaluated" || state.ActiveRunID != "actual-run" {
		t.Fatalf("unknown projection: %+v %v", state, err)
	}
	mutated := *state.Record
	mutated.Attempts[0].RequestID = "caller-mutation"
	state, err = owners.workspace.queryWorkspacePreparationState(ctx, command)
	if err != nil || state.Record.Attempts[0].RequestID != "preparation-request" {
		t.Fatalf("query aliases owner: %v", err)
	}
	after, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
	afterBytes, _ := json.Marshal(after)
	if string(before) != string(afterBytes) {
		t.Fatal("GET settled unknown or wrote owner")
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	reopened, err := owners.workspace.queryWorkspacePreparationState(ctx, command)
	if err != nil || !reflect.DeepEqual(reopened, state) {
		t.Fatalf("record not recovered by read: %v", err)
	}
	// No Run, provider, MCP, File, or Memory observers are installed. These
	// reads cannot fall back to a probe or implicitly recover an operation.
}

func TestPreparationStateKnownDriftNeverSettlesOperation(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-drift")
	seedPreparationRecord(t, w, workspace.WorkspacePreparationBlocked)
	w.Revision++ // Fixture of an already committed record, not a production write.
	base, err := preparationStateFromPersistence(w, time.Now())
	if err != nil || base.CurrentValidity != "not_evaluated" {
		t.Fatalf("fixture: %v", err)
	}
	for name, mutate := range map[string]func(*workspace.Workspace){
		"revision": func(w *workspace.Workspace) { w.Revision++ },
		"expired": func(w *workspace.Workspace) {
			a := &w.StartupPreparation.Attempts[0]
			a.ExpiresAt = a.CompletedAt.Add(time.Second)
		},
		"check_policy":    func(w *workspace.Workspace) { w.StartupPreparation.Attempts[0].Binding.CheckPolicyVersion += "-old" },
		"compiler":        func(w *workspace.Workspace) { w.StartupPreparation.Attempts[0].Binding.CompilerVersion += "-old" },
		"configuration":   func(w *workspace.Workspace) { w.InternalDirectory += "-changed" },
		"startup_missing": func(w *workspace.Workspace) { w.StartupInput = nil },
		"startup_hash":    func(w *workspace.Workspace) { w.StartupInput.StartupInputHash += "changed" },
		"definition":      func(w *workspace.Workspace) { w.StartupInput.DefinitionRevision += "changed" },
		"candidate":       func(w *workspace.Workspace) { w.ActivationAdmission.CandidateRevision++ },
		"candidate_id":    func(w *workspace.Workspace) { w.ActivationAdmission.CandidateID += "changed" },
		"candidate_hash":  func(w *workspace.Workspace) { w.ActivationAdmission.CandidateHash += "changed" },
		"not_eligible": func(w *workspace.Workspace) {
			w.ActivationAdmission.State = workspace.WorkspaceActivationPendingClarificationResolution
		},
		"confirmation_missing": func(w *workspace.Workspace) { w.DefinitionAuthoringConfirmation = nil },
		"original_input":       func(w *workspace.Workspace) { w.DefinitionAuthoringConfirmation.InputHash += "changed" },
		"original_permit":      func(w *workspace.Workspace) { w.DefinitionAuthoringConfirmation.DerivationPermitID += "changed" },
		"original_proposal":    func(w *workspace.Workspace) { w.DefinitionAuthoringConfirmation.ProposalHash += "changed" },
		"original_specification": func(w *workspace.Workspace) {
			w.DefinitionAuthoringConfirmation.SpecificationHash += "changed"
		},
		"confirmation_phase": func(w *workspace.Workspace) {
			w.DefinitionAuthoringConfirmation.Phase = workspace.DefinitionAuthoringConfirmationPrepared
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy, _ := owners.repositories.Workspaces.Get(context.Background(), w.ID)
			seedPreparationRecord(t, copy, workspace.WorkspacePreparationBlocked)
			copy.Revision++
			mutate(copy)
			before, _ := json.Marshal(copy)
			state, err := preparationStateFromPersistence(copy, time.Now())
			after, _ := json.Marshal(copy)
			if err != nil || state.State != "blocked" || state.CurrentValidity != "stale" || state.Record.Attempts[0].RequestID != "preparation-request" || string(before) != string(after) {
				t.Fatalf("drift changed operation or wrote: %+v %v", state, err)
			}
		})
	}
	w.StartupPreparation.Attempts[0].CommittedWorkspaceRevision++
	if _, err := preparationStateFromPersistence(w, time.Now()); err == nil {
		t.Fatal("future receipt accepted")
	}
}

func TestPreparationStateTypedOwnerIngress(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-state-ingress")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID}
	for _, source := range []string{workspaceevents.CommandSource, "blocks/run"} {
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspacePreparationStateQuery, source, workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
		result := owners.hub.Send(ev)
		if result == nil {
			t.Fatal("missing reply")
		}
		if source != workspaceevents.CommandSource {
			if result.Error() == nil {
				t.Fatal("untrusted source accepted")
			}
			continue
		}
		value, err := result.Get()
		if err != nil {
			t.Fatal(err)
		}
		state, ok := value.(workspaceevents.QueryWorkspacePreparationStateResult)
		if !ok || state.ValidateFor(command) != nil {
			t.Fatal("invalid typed reply")
		}
		state.WorkspaceID = "other"
		if state.ValidateFor(command) == nil {
			t.Fatal("cross-scope reply accepted")
		}
	}
	owners.workspace.handlePreparationStateQuery(nil, nil)
}
