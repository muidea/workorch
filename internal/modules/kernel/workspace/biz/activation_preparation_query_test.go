package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Recompute the public content identity so these cases exercise semantic
// validation, rather than being rejected merely for an old checksum.
func rehashPreparationSnapshot(t *testing.T, snapshot *startuppreparation.ExactConfirmedStartupSnapshot) {
	t.Helper()
	snapshot.SnapshotHash = ""
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SnapshotHash = fmt.Sprintf("startup-snapshot-sha256:%x", sha256.Sum256(encoded))
}

type preparationRevisionReadRepository struct {
	repository.WorkspaceRepository
	reads int
}

func (r *preparationRevisionReadRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspace.Workspace, error) {
	w, err := r.WorkspaceRepository.Get(ctx, id)
	r.reads++
	if err == nil && r.reads == 2 {
		// Simulate a metadata CAS between the first read and the final fence.
		// Do not write any repository fixture as part of this query.
		w.Revision++
	}
	return w, err
}

func TestExactConfirmedStartupQueryRejectsMixedRevisionObservation(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-mixed-read")
	observed := &preparationRevisionReadRepository{WorkspaceRepository: owners.workspace.workspaces}
	owners.workspace.workspaces = observed
	result, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if !errors.Is(err, common.ErrConflict) || result.Snapshot.SnapshotHash != "" || observed.reads != 2 {
		t.Fatalf("mixed read returned an exact snapshot: err=%v reads=%d", err, observed.reads)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(stored, w) {
		t.Fatal("rejected query changed owner persistence")
	}
}

func TestExactConfirmedStartupSnapshotRejectsUnrepresentableDirectoryAndByteOverflow(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-byte-boundary")
	read, err := owners.workspace.queryExactConfirmedStartupSnapshot(context.Background(), workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		t.Fatal(err)
	}
	valid := read.Snapshot.Clone()
	valid.InternalDirectory = "内部目录/保留原文"
	rehashPreparationSnapshot(t, &valid)
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var restored startuppreparation.ExactConfirmedStartupSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Validate() != nil || !reflect.DeepEqual(valid, restored) {
		t.Fatalf("valid UTF-8 snapshot lost bytes on transport: %v", err)
	}
	for name, directory := range map[string]string{
		"invalid_utf8": "internal/" + string([]byte{0xff}),
		"nul":          "internal/\x00config",
		"byte_limit":   strings.Repeat("d", 2*1024*1024),
	} {
		t.Run(name, func(t *testing.T) {
			changed := read.Snapshot.Clone()
			changed.InternalDirectory = directory
			rehashPreparationSnapshot(t, &changed)
			if changed.Validate() == nil {
				t.Fatal("accepted unrepresentable or oversized exact directory")
			}
			if changed.InternalDirectory != directory {
				t.Fatal("validator rewrote or truncated the original input")
			}
		})
	}
	// Below the snapshot bound, but above the manifest's independent bound.
	changed := read.Snapshot.Clone()
	changed.InternalDirectory = strings.Repeat("d", 256*1024)
	rehashPreparationSnapshot(t, &changed)
	if err := changed.Validate(); err != nil {
		t.Fatalf("snapshot fixture should fit its own bound: %v", err)
	}
	manifest, err := startuppreparation.CompilePreparationManifest(changed)
	if err == nil || manifest.ManifestHash != "" || len(manifest.Checks) != 0 || changed.InternalDirectory != strings.Repeat("d", 256*1024) {
		t.Fatal("manifest overflow was truncated or returned as partial success")
	}
}

func seedPreparationEligible(t *testing.T, owners *fileDefinitionAdmissionOwners, id common.WorkspaceID) *workspace.Workspace {
	t.Helper()
	ctx := context.Background()
	confirmation := seedReviewedAuthoringConfirmationWithTemplate(t, owners, id, true, true, "*", "dodItem", func(template *workspace.WorkspaceTemplate) {
		hints := template.ContextHints.Map()
		declaration := hints["profileContracts"].(map[string]any)["generic"].(map[string]any)["evidenceRequirements"].([]any)[0].(map[string]any)
		declaration["requiredOwnerFacts"] = []any{map[string]any{"ownerModuleId": runtimecontract.RuntimeModuleShell, "factKind": string(runtimecontract.RuntimeOwnerFactSelectableValidationSurface), "operationKind": runtimecontract.RuntimeBlockKindManagedValidationSession}}
		declaration["ownerFactConvergences"] = []any{map[string]any{"ownerModuleId": runtimecontract.RuntimeModuleShell, "factKind": string(runtimecontract.RuntimeOwnerFactSelectableValidationSurface), "operationKind": runtimecontract.RuntimeBlockKindManagedValidationSession, "requirePreexisting": true}}
		var err error
		template.ContextHints, err = workorcheventpayload.FromMap(hints)
		if err != nil {
			t.Fatal(err)
		}
	})
	// The historical confirmation fixture omits an internal directory. Supply
	// it before confirmation, not by mutating the sealed configuration later.
	draft, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	draft.InternalDirectory = "internal"
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, draft, draft.Revision); err != nil {
		t.Fatal(err)
	}
	confirmation = authoringConfirmationCommandFromPersistedLedger(t, draft)
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	w, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, w)
	if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, workspaceevents.AdmitDefinitionAuthoringCommand{
		Candidate: candidate, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(w),
	}); err != nil {
		t.Fatal(err)
	}
	w, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || w.ActivationAdmission == nil || w.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
		t.Fatalf("seed eligible: %v", err)
	}
	return w
}

func TestExactConfirmedStartupQueryReadsOriginalSealWithoutEffects(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	w := seedPreparationEligible(t, owners, "ws-preparation-exact")
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	before, _ := json.Marshal(w)
	first, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command)
	if err != nil || first.ValidateFor(command) != nil {
		t.Fatalf("exact sealed read: %v", err)
	}
	if !reflect.DeepEqual(first.Snapshot.DoD, w.DefinitionAuthoringConfirmation.DoD) || first.Snapshot.AuthoringRef != *first.Snapshot.Candidate.AuthoringRef {
		t.Fatal("read changed original DoD or confirmation identity")
	}
	// Returned nested graphs cannot alias the repository or a later query.
	changed := first.Snapshot.Clone()
	changed.DoD.Items[0] = "caller mutation"
	changed.Candidate.DoDInputs[0].Text = "caller mutation"
	changed.StartupInput.CandidateItemToItemRefMappings[0].ItemRef = "caller mutation"
	changed.FormalDoDItemRefs[0] = "caller mutation"
	again, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("query changed after caller mutation: %v", err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	after, _ := json.Marshal(stored)
	if err != nil || string(before) != string(after) {
		t.Fatalf("query wrote Workspace: %v", err)
	}
	if tickets, err := owners.repositories.Tickets.ListByWorkspaceID(common.ContextWithRuntimeWorkspaceID(ctx, w.ID), w.ID); err != nil || len(tickets) != 0 {
		t.Fatalf("read opened a ticket: %v", err)
	}
	// The owner can answer after a real file repository reopen without invoking
	// any Run/provider/resource owner; no such observers are installed here.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	reopened, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command)
	if err != nil || !reflect.DeepEqual(first, reopened) {
		t.Fatalf("read after reopen: %v", err)
	}
}

func TestExactConfirmedStartupQueryRejectsStaleAndMissingSealWithoutFallback(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-rejections")
	for name, mutate := range map[string]func(*workspace.Workspace){
		"missing_startup":      func(w *workspace.Workspace) { w.StartupInput = nil },
		"missing_admission":    func(w *workspace.Workspace) { w.ActivationAdmission = nil },
		"missing_confirmation": func(w *workspace.Workspace) { w.DefinitionAuthoringConfirmation = nil },
		"missing_raw_source":   func(w *workspace.Workspace) { w.DefinitionAuthoring = nil },
		"prepared_not_materialized": func(w *workspace.Workspace) {
			w.DefinitionAuthoringConfirmation.Phase = workspace.DefinitionAuthoringConfirmationPrepared
		},
		"pending_clarification": func(w *workspace.Workspace) {
			w.ActivationAdmission.State = workspace.WorkspaceActivationPendingClarificationResolution
		},
		"changed_configuration": func(w *workspace.Workspace) { w.InternalDirectory += "-changed" },
		"changed_candidate":     func(w *workspace.Workspace) { w.DefinitionAdmissionCandidates[0].CandidateHash = "changed" },
		"changed_six_review_source": func(w *workspace.Workspace) {
			w.DefinitionAuthoring.SpecificationRequests[1].Review.Reason += "changed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Repository reads are detached; do not write the rejected fixture.
			changed, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			mutate(changed)
			if _, err := startuppreparation.FromOwnerPersistence(changed, &w.DefinitionAuthoringConfirmation.Goal, &w.DefinitionAuthoringConfirmation.DoD); err == nil {
				t.Fatal("invalid seal became exact startup")
			}
		})
	}
	for _, command := range []workspaceevents.QueryExactConfirmedStartupSnapshotCommand{
		{WorkspaceID: w.ID}, {WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision + 1},
		{WorkspaceID: "other-workspace", ExpectedWorkspaceRevision: w.Revision},
	} {
		if _, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, command); err == nil {
			t.Fatal("accepted stale or cross-scoped read")
		}
	}
	goal, dod, err := owners.workspace.currentExecutionDefinitions(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	goal.Text += "replacement row"
	if _, err := startuppreparation.FromOwnerPersistence(w, goal, dod); err == nil {
		t.Fatal("expected journal substituted for actual changed row")
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(w, stored) {
		t.Fatalf("rejections changed owner state: %v", err)
	}
}

func TestExactConfirmedStartupSnapshotResultRejectsTampering(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-result")
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	result, err := owners.workspace.queryExactConfirmedStartupSnapshot(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*startuppreparation.ExactConfirmedStartupSnapshot){
		"workspace":      func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.WorkspaceID = "other" },
		"revision":       func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.WorkspaceRevision++ },
		"confirmation":   func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.AuthoringRef.ProposalHash = "changed" },
		"candidate":      func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.ActivationAdmission.CandidateRevision++ },
		"startup":        func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.StartupInput.StartupInputHash = "changed" },
		"dod_content":    func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.DoD.Items[0] += "changed" },
		"dod_order_refs": func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.FormalDoDItemRefs[0] = "other" },
		"goal_lineage":   func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.Goal.LineageState = "retired" },
		"dod_state":      func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.DoD.State = "not_ready" },
		"goal_source": func(s *startuppreparation.ExactConfirmedStartupSnapshot) {
			s.Candidate.GoalInput.SourceIdentity = "goal:other@revision"
		},
		"template": func(s *startuppreparation.ExactConfirmedStartupSnapshot) {
			s.Candidate.WorkspaceTemplateSnapshot.Revision = "other"
		},
		"hash": func(s *startuppreparation.ExactConfirmedStartupSnapshot) { s.SnapshotHash = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := workspaceevents.QueryExactConfirmedStartupSnapshotResult{Snapshot: result.Snapshot.Clone()}
			mutate(&changed.Snapshot)
			if name != "hash" {
				rehashPreparationSnapshot(t, &changed.Snapshot)
			}
			if changed.ValidateFor(command) == nil {
				t.Fatal("accepted tampered result")
			}
		})
	}
}

func TestExactConfirmedStartupTypedOwnerIngress(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-typed")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	command := workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}
	for _, source := range []string{workspaceevents.CommandSource, "blocks/file", "blocks/run"} {
		event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, source, workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
		result := owners.hub.Send(event)
		if result == nil {
			t.Fatal("missing reply")
		}
		if source != workspaceevents.CommandSource {
			if result.Error() == nil {
				t.Fatal("untrusted sender accepted")
			}
			continue
		}
		data, err := result.Get()
		if err != nil {
			t.Fatal(err)
		}
		read, ok := data.(workspaceevents.QueryExactConfirmedStartupSnapshotResult)
		if !ok || read.ValidateFor(command) != nil {
			t.Fatal("invalid typed reply")
		}
	}
	owners.workspace.handleExactConfirmedStartupQuery(nil, nil)
}
