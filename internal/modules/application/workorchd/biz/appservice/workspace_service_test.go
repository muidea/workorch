package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateWorkspaceWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateWorkspaceWithSession(context.Background(), nil, "workspace 1", "", "workspaces/workspace 1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateWorkspaceWithTemplateKeepsGitInitializationDeclarative(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:       "template-git-init",
		Name:     "git init template",
		DoDItems: []string{"git repo available"},
		ContextHints: map[string]any{
			"versionControl": map[string]any{
				"git": map[string]any{
					"initializeOnCreate": true,
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}

	item, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "git initialized workspace",
		TemplateID: "template-git-init",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	if _, err := os.Stat(filepath.Join(item.DeliverableDirectory, ".git")); !os.IsNotExist(err) {
		t.Fatalf("workspace git directory stat err = %v, want no eager repository initialization", err)
	}
	if _, ok := item.ContextHints["versionControl"]; !ok {
		t.Fatalf("workspace context hints lost declarative git setup: %v", item.ContextHints)
	}
}

func TestCreateWorkspaceWithTemplateDoesNotCopyManagedWorkspaceConfig(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:       "template-managed-config",
		Name:     "managed config template",
		DoDItems: []string{"managed config available"},
		ContextHints: map[string]any{
			"managedWorkspaceConfigs": []any{
				map[string]any{
					"path":         "tools/config.json",
					"format":       "json",
					"mergeMissing": true,
					"content": map[string]any{
						"languages": map[string]any{
							"go": map[string]any{
								"command": []any{"gopls", "serve"},
							},
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}

	item, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "managed config workspace",
		TemplateID: "template-managed-config",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	if _, ok := item.ContextHints["managedWorkspaceConfigs"]; ok {
		t.Fatalf("workspace context hints copied managedWorkspaceConfigs: %v", item.ContextHints)
	}
	configPath := filepath.Join(item.InternalDirectory, "tools", "config.json")
	if _, err := os.Stat(configPath); err == nil {
		t.Fatalf("managed config was created during workspace creation: %s", configPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat managed config: %v", err)
	}
}

func TestCreateWorkspaceWithTemplateCopiesTemplateStagePolicies(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:       "template-stage-policy",
		Name:     "stage policy template",
		DoDItems: []string{"stage policy available"},
		ContextHints: map[string]any{
			"templateStagePolicies": map[string]any{
				"schemaVersion": "workorch.template_stage_policy.v1",
				"stages": []any{
					map[string]any{
						"id":          "execution",
						"soulBinding": "execution",
						"entry": map[string]any{
							"nextAction":                 "state_change",
							"requiresDefinitionRevision": true,
							"requiresGapBaseline":        true,
						},
						"requiredOutputs": []any{
							map[string]any{
								"kind":          "state_change",
								"evidenceKinds": []any{"state_change"},
							},
						},
						"completion": map[string]any{
							"policy": "required_outputs_accepted",
						},
					},
				},
			},
			"semanticCodeNavigation": map[string]any{
				"defaultMCPServerIds": []any{"mcp-shared-default"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}

	item, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:      "stage policy workspace",
		TemplateID: "template-stage-policy",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	policy, ok := item.ContextHints["templateStagePolicies"].(map[string]any)
	if !ok {
		t.Fatalf("workspace contextHints.templateStagePolicies = %+v, want map", item.ContextHints["templateStagePolicies"])
	}
	if policy["schemaVersion"] != "workorch.template_stage_policy.v1" {
		t.Fatalf("templateStagePolicies schemaVersion = %v, want workorch.template_stage_policy.v1", policy["schemaVersion"])
	}
	if _, ok := item.ContextHints["defaultMCPServerIds"]; ok {
		t.Fatalf("workspace contextHints copied common tool default at top level: %+v", item.ContextHints)
	}
	semantic, _ := item.ContextHints["semanticCodeNavigation"].(map[string]any)
	if _, ok := semantic["defaultMCPServerIds"]; ok {
		t.Fatalf("workspace contextHints copied nested common tool default: %+v", semantic)
	}

	readBack, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, item.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if _, ok := readBack.ContextHints["templateStagePolicies"].(map[string]any); !ok {
		t.Fatalf("persisted workspace contextHints.templateStagePolicies = %+v, want map", readBack.ContextHints["templateStagePolicies"])
	}
}

func TestCreateWorkspaceWithBackendReturnsCreatedWorkspace(t *testing.T) {
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "appservice created workspace", "appservice description", "workspaces/appservice created workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if item.ID == "" || item.Title != "appservice created workspace" || item.Description != "appservice description" || item.DeliverableDirectory != "workspaces/appservice created workspace" {
		t.Fatalf("workspace = %+v, want created workspace", item)
	}
	if !strings.HasPrefix(item.ID, "ws-") {
		t.Fatalf("workspace id = %q, want generated ws- id", item.ID)
	}
	if item.State != "active" {
		t.Fatalf("state = %q, want active", item.State)
	}
}

func TestCreateWorkspaceWithTemplateBindsWorkspaceSouls(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	for _, id := range []string{"soul-template-default", "soul-template-planning", "soul-template-execution"} {
		if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
			ID:     id,
			Name:   id,
			SoulMD: "test soul",
		}); err != nil {
			t.Fatalf("CreateSoulWithBackend %s: %v", id, err)
		}
	}
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:            "template-code-agent",
		Name:          "code agent",
		DefaultSoulID: "soul-template-default",
		SoulBindings: map[string]string{
			"planning":  "soul-template-planning",
			"execution": "soul-template-execution",
		},
		DoDItems: []string{"tests pass"},
		ContextHints: map[string]any{
			"stateChangePolicy": "must_produce_state_change",
		},
		EvidencePolicies: []map[string]any{
			{
				"id":              "template-evidence-policy",
				"zeroWorkMarkers": []any{"no work"},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}

	item, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:                "software workspace",
		DeliverableDirectory: "workspaces/software",
		TemplateID:           "template-code-agent",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}

	if item.DefaultSoulID != "soul-template-default" {
		t.Fatalf("defaultSoulId = %q, want soul-template-default", item.DefaultSoulID)
	}
	for _, stage := range []string{"planning", "execution"} {
		if item.SoulBindings[stage] == "" {
			t.Fatalf("soul binding %q missing in %+v", stage, item.SoulBindings)
		}
	}

	readBack, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, item.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if readBack.DefaultSoulID != item.DefaultSoulID || readBack.SoulBindings["execution"] != item.SoulBindings["execution"] {
		t.Fatalf("read workspace soul bindings = %+v, want %+v", readBack, item)
	}
	if readBack.ContextHints["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("read workspace contextHints = %+v, want must_produce_state_change", readBack.ContextHints)
	}
	if len(readBack.EvidencePolicies) != 1 || readBack.EvidencePolicies[0]["id"] != "template-evidence-policy" {
		t.Fatalf("read workspace evidence policies = %+v, want template policy", readBack.EvidencePolicies)
	}
}

func TestRefreshWorkspaceTemplateBindingUpdatesTemplateBoundPolicy(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:       "template-policy-refresh",
		Name:     "policy refresh template",
		DoDItems: []string{"evidence is available"},
		ContextHints: map[string]any{
			"qualityGate": "v1",
		},
		EvidencePolicies: []map[string]any{
			{
				"id":   "policy-v1",
				"kind": "initial",
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	workspace, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:                "template policy refresh workspace",
		DeliverableDirectory: "workspaces/template-policy-refresh",
		TemplateID:           "template-policy-refresh",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}
	if workspace.ContextHints["qualityGate"] != "v1" {
		t.Fatalf("created workspace contextHints = %+v, want v1", workspace.ContextHints)
	}

	if _, err := UpdateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, "template-policy-refresh", WorkspaceTemplate{
		Name:     "policy refresh template updated",
		DoDItems: []string{"evidence is available"},
		ContextHints: map[string]any{
			"qualityGate": "v2",
		},
		EvidencePolicies: []map[string]any{
			{
				"id":                 "policy-v2",
				"acceptedStepKinds":  []any{"artifact/write"},
				"requiredConfidence": "high",
			},
		},
	}); err != nil {
		t.Fatalf("UpdateWorkspaceTemplateWithBackend: %v", err)
	}

	refreshed, err := RefreshWorkspaceTemplateBindingWithBackend(ctx, "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("RefreshWorkspaceTemplateBindingWithBackend: %v", err)
	}
	if refreshed.ID != workspace.ID || refreshed.Title != workspace.Title || refreshed.DeliverableDirectory != workspace.DeliverableDirectory || refreshed.State != workspace.State {
		t.Fatalf("refreshed workspace identity/state = %+v, want preserved from %+v", refreshed, workspace)
	}
	if refreshed.ContextHints["qualityGate"] != "v2" {
		t.Fatalf("refreshed contextHints = %+v, want v2", refreshed.ContextHints)
	}
	if len(refreshed.EvidencePolicies) != 1 || refreshed.EvidencePolicies[0]["id"] != "policy-v2" {
		t.Fatalf("refreshed evidence policies = %+v, want policy-v2", refreshed.EvidencePolicies)
	}

	readBack, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if readBack.ContextHints["qualityGate"] != "v2" || len(readBack.EvidencePolicies) != 1 || readBack.EvidencePolicies[0]["id"] != "policy-v2" {
		t.Fatalf("persisted workspace = %+v, want refreshed template-bound policy", readBack)
	}
}

func TestReadWorkspaceWithBackendReturnsPersistedWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "appservice read workspace", "read description", "workspaces/appservice read workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := ReadWorkspaceWithBackend(context.Background(), "file", dataRoot, created.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if item.ID != created.ID || item.Title != "appservice read workspace" || item.Description != "read description" || item.DeliverableDirectory != "workspaces/appservice read workspace" {
		t.Fatalf("workspace = %+v, want persisted workspace", item)
	}
}

func TestReadAndListWorkspacesProjectActiveRunState(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "projected workspace", "", "workspaces/projected")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	run, err := CreateRunWithBackend(ctx, "file", dataRoot, created.ID, "run-project-workspace", "", "")
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(created.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.State = common.WorkspaceStateActive
	workspaceItem.ActiveRunID = common.RunID(run.ID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace: %v", err)
	}
	runItem, err := exports.Repositories.Runs.Get(ctx, common.RunID(run.ID))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	runItem.State = common.RunStateBlockedOnDecision
	runItem.BlockingTicketID = common.TicketID("ticket-project-workspace")
	if err := exports.Repositories.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	readBack, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, created.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if readBack.State != string(common.WorkspaceStateActive) || readBack.ActiveRunID != run.ID || readBack.BlockingTicketID != "ticket-project-workspace" {
		t.Fatalf("read workspace = %+v, want active projection with decision ticket", readBack)
	}
	items, err := ListWorkspacesWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("ListWorkspacesWithBackend: %v", err)
	}
	if len(items) != 1 || items[0].State != string(common.WorkspaceStateActive) || items[0].ActiveRunID != run.ID || items[0].BlockingTicketID != "ticket-project-workspace" {
		t.Fatalf("listed workspaces = %+v, want active projection with decision ticket", items)
	}
}

func TestCreateWorkspaceWithBackendDefaultsDeliverableDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "appservice default directory workspace", "", "")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	want := filepath.Join(filepath.Dir(dataRoot), ".workorch-workspaces", item.ID)
	if item.DeliverableDirectory != want {
		t.Fatalf("deliverableDirectory = %q, want %q", item.DeliverableDirectory, want)
	}
	for _, dir := range []string{
		filepath.Join(filepath.Dir(dataRoot), ".workorch-workspaces", item.ID),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "artifacts"),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "runs"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("workspace directory %q = %v/%v, want existing directory", dir, info, err)
		}
	}
}

func TestCreateWorkspaceWithBackendCreatesExplicitDeliverableDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	deliverableDirectory := filepath.Join(t.TempDir(), "custom-project")
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "appservice explicit directory workspace", "", deliverableDirectory)
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if item.DeliverableDirectory != deliverableDirectory {
		t.Fatalf("deliverableDirectory = %q, want %q", item.DeliverableDirectory, deliverableDirectory)
	}
	if info, err := os.Stat(deliverableDirectory); err != nil || !info.IsDir() {
		t.Fatalf("explicit deliverable directory = %v/%v, want existing directory", info, err)
	}
	for _, dir := range []string{
		filepath.Join(dataRoot, "internal", "workspaces", item.ID),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "config"),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "runs"),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "artifacts"),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "events"),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "llm-calls"),
		filepath.Join(dataRoot, "internal", "workspaces", item.ID, "indexes"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("workspace internal directory %q = %v/%v, want existing directory", dir, info, err)
		}
	}
	if _, err := os.Stat(filepath.Join(deliverableDirectory, "working")); !os.IsNotExist(err) {
		t.Fatalf("legacy working directory stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "internal", "workspaces", item.ID, "working")); !os.IsNotExist(err) {
		t.Fatalf("legacy internal working directory stat err = %v, want not exist", err)
	}
}

func TestResolveWorkspaceDirectoryLayoutUsesFileBackendRoots(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	defer func() {
		if err := session.Close(ctx); err != nil {
			t.Fatalf("close session: %v", err)
		}
	}()

	workspaceID := common.WorkspaceID("ws-layout-contract")
	layout, err := workspacecore.ResolveDirectoryLayout(session.RepositoryBackend(), session.RepositoryDataDir(), string(workspaceID), "")
	if err != nil {
		t.Fatalf("resolveWorkspaceDirectoryLayout default: %v", err)
	}
	if !layout.Managed {
		t.Fatalf("layout.Managed = false, want true for file backend")
	}
	wantDeliverableRoot := filepath.Join(filepath.Dir(dataRoot), ".workorch-workspaces")
	wantDeliverable := filepath.Join(wantDeliverableRoot, string(workspaceID))
	wantInternal := filepath.Join(dataRoot, "internal", "workspaces", string(workspaceID))
	if layout.DeliverableRoot != wantDeliverableRoot || layout.DeliverableDirectory != wantDeliverable || layout.InternalDirectory != wantInternal {
		t.Fatalf("layout = %+v, want deliverableRoot=%q deliverable=%q internal=%q", layout, wantDeliverableRoot, wantDeliverable, wantInternal)
	}
	if strings.Contains(filepath.ToSlash(layout.DeliverableDirectory), "/working") {
		t.Fatalf("deliverableDirectory = %q, must not include legacy working level", layout.DeliverableDirectory)
	}

	explicitDeliverable := filepath.Join(t.TempDir(), "explicit-project-root")
	explicit, err := workspacecore.ResolveDirectoryLayout(session.RepositoryBackend(), session.RepositoryDataDir(), string(workspaceID), explicitDeliverable)
	if err != nil {
		t.Fatalf("resolveWorkspaceDirectoryLayout explicit: %v", err)
	}
	if !explicit.Managed || explicit.DeliverableDirectory != explicitDeliverable || explicit.InternalDirectory != wantInternal {
		t.Fatalf("explicit layout = %+v, want managed explicit deliverable and stable internal root", explicit)
	}
}

func TestReadWorkspaceWithBackendReturnsNotFoundWhenMissing(t *testing.T) {
	if _, err := ReadWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "ws-missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestDeleteWorkspaceWithBackendDeletesOnlyWorkspaceByDefault(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "delete workspace", "", "")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := DefineGoalWithBackend(ctx, "file", dataRoot, created.ID, "keep related goal"); err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}

	if err := DeleteWorkspaceWithBackend(ctx, "file", dataRoot, created.ID, DeleteWorkspaceCommand{}); err != nil {
		t.Fatalf("DeleteWorkspaceWithBackend: %v", err)
	}
	if _, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, created.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadWorkspaceWithBackend err = %v, want not found", err)
	}
	if info, err := os.Stat(created.DeliverableDirectory); err != nil || !info.IsDir() {
		t.Fatalf("deliverable directory = %v/%v, want preserved directory", info, err)
	}
}

func TestDeleteWorkspaceWithBackendPurgesWorkspaceData(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:   "template-purge-runtime-v2",
		Name: "purge Runtime V2 workspace",
		ContextHints: map[string]any{
			"acceptanceSlicePolicy": map[string]any{
				"schemaVersion": "workorch.acceptance_slice_policy.v2", "maxWorkItems": 1, "maxFocuses": 4, "maxCandidateContextItems": 4,
				"completionPolicy": "all_required_items_accepted", "resultPolicy": "slice_result_allowed",
				"groups": []any{map[string]any{"id": "purge-all", "maxWorkItems": 1, "match": map[string]any{"all": true}}},
			},
			"profileContracts": map[string]any{
				"purge-proof": map[string]any{"evidenceRequirements": []any{map[string]any{
					"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"},
					"acceptedStepKinds": []any{"validation/managed_session"}, "targetRole": "purge_target",
					"targetDiscoverySources": []any{"workspace_manifest"}, "targetSelectorRef": "file/purge_target",
					"targetSelectorOwnerRef": "purge-proof", "targetSelectorSubjectRef": "dodItem",
					"validationRecipeId": "purge-validation", "requiredCheckRefs": []any{"purge-check"},
				}}},
			},
		},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}
	created, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{Title: "purge workspace", TemplateID: "template-purge-runtime-v2"})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, created.ID, "purge related goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, created.ID, []string{"purge related dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, created.ID, "run-purge-workspace", goal.ID, dod.ID); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if err := DeleteWorkspaceWithBackend(ctx, "file", dataRoot, created.ID, DeleteWorkspaceCommand{Purge: true}); err != nil {
		t.Fatalf("DeleteWorkspaceWithBackend purge: %v", err)
	}
	if _, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, created.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadWorkspaceWithBackend err = %v, want not found", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "internal", "workspaces", created.ID)); !os.IsNotExist(err) {
		t.Fatalf("workspace data root stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(created.DeliverableDirectory); !os.IsNotExist(err) {
		t.Fatalf("workspace deliverable root stat err = %v, want not exist", err)
	}
}

func TestListWorkspacesWithBackendReturnsSortedWorkspaces(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-b", "workspace b", "", "workspaces/workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-b: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-a", "workspace a", "", "workspaces/workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-a: %v", err)
	}

	items, err := ListWorkspacesWithBackend(context.Background(), "file", dataRoot)
	if err != nil {
		t.Fatalf("ListWorkspacesWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "ws-a" || items[1].ID != "ws-b" {
		t.Fatalf("workspace ids = %q/%q, want ws-a/ws-b", items[0].ID, items[1].ID)
	}
}
