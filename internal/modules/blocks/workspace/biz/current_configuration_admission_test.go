package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCurrentConfigurationAdmissionPreservesFileArchivesAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	old := testAdmissionCandidate()
	old.CandidateID = "workspace-definition"
	old.SchemaVersion = 1
	old.AdmissionCompilerVersion = "historical-compiler"
	// Deliberately noncanonical old declarations prove that repository reads,
	// EventHub copies and admission CAS do not normalize archived payloads.
	old.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: " z "}, {ID: "a"}}
	old.CandidateHash = "historical-candidate-hash"
	archive, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{ID: old.WorkspaceID, Title: "keep title", State: common.WorkspaceStateActive, TemplateID: "template", ProfileID: "template",
		DeliverableDirectory: filepath.Join(root, "project"), InternalDirectory: filepath.Join(root, "internal-project"), DefaultSoulID: "soul-old",
		DefinitionAdmissionCandidates: []workspacemodel.DefinitionAdmissionCandidate{old},
		StartupInput:                  &workspacemodel.WorkspaceStartupInputContract{WorkspaceID: old.WorkspaceID, DefinitionRevision: "historical-definition", StartupInputHash: "historical-startup", TemplateSnapshotHash: "historical-template"},
		ActivationAdmission: &workspacemodel.WorkspaceActivationAdmission{WorkspaceID: old.WorkspaceID, CandidateID: old.CandidateID, CandidateRevision: old.CandidateRevision, CandidateHash: old.CandidateHash,
			DefinitionRevision: "historical-definition", StartupInputHash: "historical-startup", State: workspacemodel.WorkspaceActivationEligible}}
	if err := owners.repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
	if err := owners.repositories.Runs.Create(bound, &runmodel.Run{ID: "run-001", WorkspaceID: workspace.ID, State: common.RunStateFailed, StartupInputHash: "historical-startup"}); err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(root, "internal", "workspaces", string(workspace.ID), "runs", "run-001", "run.json")
	runArchive, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	hints, _ := workorcheventpayload.FromMap(map[string]any{"configVersion": "v2"})
	template := &workspacemodel.WorkspaceTemplate{ID: "template", Name: "current template", GoalText: "template goal must not replace user goal", DoDItems: []string{"template default must not replace user DoD"}, DefaultSoulID: "soul-current", SoulBindings: map[string]string{"execution": "soul-current"}, ContextHints: hints}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, template); err != nil {
		t.Fatal(err)
	}
	goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: workspace.ID, Text: "user goal", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: workspace.ID, Items: []string{"user DoD"}, DefinitionRevision: "dod-r1"}
	var admittedHash string
	var admittedRevision uint64
	for attempt := 0; attempt < 2; attempt++ {
		owners.close()
		owners = openFileDefinitionAdmissionOwners(t, root)
		if err := owners.workspace.Start(); err != nil {
			t.Fatal(err)
		}
		current, err := owners.repositories.Workspaces.Get(ctx, workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, owners.hub, current, goal, dod)
		if err != nil {
			t.Fatalf("automatic admission after reopen %d: %v", attempt, err)
		}
		current = snapshot.Workspace
		if current.ID != workspace.ID || current.Title != workspace.Title || current.DeliverableDirectory != workspace.DeliverableDirectory || current.InternalDirectory != workspace.InternalDirectory || current.State != workspace.State || current.ActiveRunID != "" {
			t.Fatalf("new admission changed workspace identity/lifecycle: %+v", current)
		}
		if len(current.DefinitionAdmissionCandidates) != 2 || snapshot.Candidate.CandidateRevision != 2 || snapshot.Candidate.GoalInput.Text != goal.Text || snapshot.Candidate.DoDInputs[0].Text != dod.Items[0] || current.DefaultSoulID != "soul-current" || current.SoulBindings["execution"] != "soul-current" || !current.ContextHints.Equal(hints) {
			t.Fatalf("new admission did not load current configuration: %+v", current)
		}
		if current.StartupInput.TemplateSnapshotHash != workspacemodel.WorkspaceTemplateSnapshotHash(template) || current.StartupInput.StartupInputHash == workspace.StartupInput.StartupInputHash || snapshot.Candidate.BaseStartupInputHash != "historical-startup" {
			t.Fatal("new admission lost current template or historical lineage")
		}
		if _, present, err := current.ValidateActivationAdmissionPersistence(); err != nil || !present {
			t.Fatalf("new admission invalid: %v", err)
		}
		encoded, _ := json.Marshal(current.DefinitionAdmissionCandidates[0])
		if !bytes.Equal(encoded, archive) {
			t.Fatalf("archive changed across read/CAS/reopen:\n%s\n%s", archive, encoded)
		}
		afterRun, err := os.ReadFile(runPath)
		if err != nil || !bytes.Equal(afterRun, runArchive) {
			t.Fatalf("configuration activation rewrote old Run: %v", err)
		}
		if attempt == 0 {
			admittedHash, admittedRevision = current.StartupInput.StartupInputHash, current.Revision
		} else if current.StartupInput.StartupInputHash != admittedHash || current.Revision != admittedRevision {
			t.Fatal("same configuration was not an idempotent admission replay")
		}
	}
	// An invalid new profile cannot fall back to the formerly eligible seal.
	// It remains an ordinary blocking clarification, without a new Run.
	template.ContextHints, _ = workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"template": map[string]any{
		"acceptanceDimensions": []any{map[string]any{"id": "unprovable", "required": true}},
	}}})
	if err := owners.repositories.WorkspaceTemplates.Update(ctx, template); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, owners.hub, current, goal, dod); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("invalid new configuration did not block: %v", err)
	}
	blocked, err := owners.repositories.Workspaces.Get(ctx, workspace.ID)
	if err != nil || len(blocked.DefinitionAdmissionCandidates) != 3 || blocked.StartupInput.StartupInputHash != admittedHash {
		t.Fatalf("invalid new configuration replaced the eligible seal: %+v %v", blocked, err)
	}
	encoded, _ := json.Marshal(blocked.DefinitionAdmissionCandidates[0])
	afterRun, runErr := os.ReadFile(runPath)
	if !bytes.Equal(encoded, archive) || runErr != nil || !bytes.Equal(afterRun, runArchive) {
		t.Fatal("blocking new configuration modified old archives")
	}
	blockedArchive, _ := json.Marshal(blocked.DefinitionAdmissionCandidates)
	template.ContextHints = hints
	if err := owners.repositories.WorkspaceTemplates.Update(ctx, template); err != nil {
		t.Fatal(err)
	}
	corrected, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, owners.hub, blocked, goal, dod)
	if err != nil || corrected.Candidate.CandidateRevision != 4 || corrected.Workspace.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible || corrected.Workspace.ActivationAdmission.ClarificationResolutionID == "" {
		t.Fatalf("corrected template did not resolve clarification through owners: %+v %v", corrected, err)
	}
	retained, _ := json.Marshal(corrected.Workspace.DefinitionAdmissionCandidates[:3])
	if !bytes.Equal(blockedArchive, retained) {
		t.Fatal("clarification resolution rewrote previous authoring attempts")
	}
}

func TestCurrentConfigurationAdmissionRejectsDriftAndReplaysExactly(t *testing.T) {
	template := &workspacemodel.WorkspaceTemplate{ID: "template", Name: "current"}
	current := &workspacemodel.Workspace{ID: "workspace-1", Revision: 5, TemplateID: template.ID, DeliverableDirectory: "/tmp/workspace-1"}
	candidate := testAdmissionCandidate()
	candidate.WorkspaceTemplateSnapshot.SnapshotHash = workspacemodel.WorkspaceTemplateSnapshotHash(template)
	candidate = candidate.WithHash()
	fresh, err := workspacemodel.WorkspaceForCurrentTemplate(current, template)
	if err != nil {
		t.Fatal(err)
	}
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate, CurrentTemplate: true, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(fresh)}
	event := definitionAdmissionInternalEvent{WorkspaceID: current.ID, Intent: &intent, Template: template}
	for _, drift := range []string{"template", "workspace revision", "configuration", "missing fence"} {
		t.Run(drift, func(t *testing.T) {
			changed, changedTemplate, changedIntent := *current, *template, intent
			switch drift {
			case "template":
				changedTemplate.Name += " changed"
			case "workspace revision":
				changed.Revision++
			case "configuration":
				changed.DeliverableDirectory += "/changed"
			case "missing fence":
				changedIntent.ExpectedConfigurationHash = ""
			}
			transition, err := reduceDefinitionAdmissionInternalEvent(&changed, definitionAdmissionInternalEvent{WorkspaceID: current.ID, Intent: &changedIntent, Template: &changedTemplate})
			if !errors.Is(err, common.ErrConflict) || transition.Next != nil || len(changed.DefinitionAdmissionCandidates) != 0 {
				t.Fatalf("stale input admitted: %+v %v", transition, err)
			}
		})
	}
	transition, err := reduceDefinitionAdmissionInternalEvent(current, event)
	if err != nil || transition.Next == nil || transition.Result.Disposition != workspaceevents.DefinitionAdmissionIntentEligible {
		t.Fatalf("fresh admission: %+v %v", transition, err)
	}
	committed := transition.Next
	committed.Revision++ // owner CAS committed, caller lost the reply
	for _, damage := range []string{"missing startup", "missing candidate", "duplicate candidate", "wrong workspace", "wrong admission hash", "unknown admission state"} {
		t.Run(damage, func(t *testing.T) {
			broken := *committed
			admission := *committed.ActivationAdmission
			broken.ActivationAdmission = &admission
			switch damage {
			case "missing startup":
				broken.StartupInput = nil
			case "missing candidate":
				broken.DefinitionAdmissionCandidates = nil
			case "duplicate candidate":
				broken.DefinitionAdmissionCandidates = []workspacemodel.DefinitionAdmissionCandidate{candidate, candidate}
			case "wrong workspace":
				admission.WorkspaceID = "other-workspace"
			case "wrong admission hash":
				admission.CandidateHash = "mismatched"
			case "unknown admission state":
				admission.State = "unknown"
			}
			result, err := reduceDefinitionAdmissionInternalEvent(&broken, event)
			if !errors.Is(err, common.ErrConflict) || result.Next != nil {
				t.Fatalf("damaged archive admitted: %+v %v", result, err)
			}
		})
	}
	replay, err := reduceDefinitionAdmissionInternalEvent(committed, event)
	if err != nil || !replay.Noop || replay.Result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate {
		t.Fatalf("response-loss replay: %+v %v", replay, err)
	}
	committed.SoulBindings = map[string]common.SoulID{"planning": "concurrent-user-selection"}
	if _, err := reduceDefinitionAdmissionInternalEvent(committed, event); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("exact-candidate replay accepted different current config: %v", err)
	}
}

func TestFreshSoulRoutingReadsUpdatedOwnerContentWithoutMutatingPriorSnapshot(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	souls, err := soulbiz.New(owners.hub, nil, soulbiz.Dependencies{Souls: filerepo.NewSoulRepository(filepath.Join(t.TempDir(), "souls")), EventHub: owners.hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := souls.Start(); err != nil {
		t.Fatal(err)
	}
	defer souls.Teardown(ctx)
	mutation := workorchmanagementcontract.SoulMutation{ID: "soul-live", Name: "live Soul", SoulMD: "instructions v1"}
	if _, err := contextregistryports.CreateSoul(ctx, owners.hub, mutation); err != nil {
		t.Fatal(err)
	}
	request := activationports.WorkspaceSoulRoutingRequest{Hub: owners.hub, Workspace: &workspacemodel.Workspace{ID: "workspace", DefaultSoulID: "soul-live"}}
	before, err := activationports.ResolveWorkspaceSoulRouting(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	prior, _ := json.Marshal(before)
	mutation.SoulMD = "instructions v2"
	if _, err := contextregistryports.UpdateSoul(ctx, owners.hub, mutation); err != nil {
		t.Fatal(err)
	}
	after, err := activationports.ResolveWorkspaceSoulRouting(ctx, request)
	if err != nil || after.DefaultSoul.Revision != before.DefaultSoul.Revision+1 || after.DefaultSoul.ContentHash == before.DefaultSoul.ContentHash || strings.TrimSpace(after.DefaultSoul.SoulMD) != mutation.SoulMD {
		t.Fatalf("new execution cached an old Soul: %+v %v", after, err)
	}
	stillPrior, _ := json.Marshal(before)
	if !reflect.DeepEqual(prior, stillPrior) {
		t.Fatal("fresh Soul resolution changed a previously loaded snapshot")
	}
}
