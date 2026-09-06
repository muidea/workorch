package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func historicalSoftwareTemplate(t *testing.T) *workspacemodel.WorkspaceTemplate {
	t.Helper()
	defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range defaults {
		if raw.ID != "software-dev" {
			continue
		}
		raw.DoDItems[2] = "代码和配置变更保持最小且与目标直接相关，并提供 unified_diff 证据"
		profile := raw.ContextHints["profileContracts"].(map[string]any)["software-dev"].(map[string]any)
		for _, item := range profile["evidenceRequirements"].([]any) {
			requirement := item.(map[string]any)
			if requirement["id"] == "source_change" {
				requirement["acceptedOperationVariants"] = []any{map[string]any{"kind": "patch/apply", "variantId": "unified_diff"}}
			}
		}
		value, err := builtinWorkspaceTemplate(raw)
		if err != nil {
			t.Fatal(err)
		}
		if hash := workspacemodel.StableWorkspaceTemplateContentHash(value); hash != "workspace-template-content-sha256:328c0f442fa57ba4f0808948c474e755d1ec72b5743c5e4bf5252491cc2ed9d8" {
			t.Fatalf("historical fixture changed: %s", hash)
		}
		return value
	}
	t.Fatal("missing builtin software-dev template")
	return nil
}

type lostTemplateUpgradeResponse struct {
	repository.WorkspaceTemplateRepository
	updates int
}

func (r *lostTemplateUpgradeResponse) Update(ctx context.Context, item *workspacemodel.WorkspaceTemplate) error {
	r.updates++
	if err := r.WorkspaceTemplateRepository.Update(ctx, item); err != nil {
		return err
	}
	return errors.New("response lost after durable update")
}

func TestBuiltinTemplateUpgradeIsExplicitGuardedAndReplayable(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var repo repository.WorkspaceTemplateRepository = memoryrepo.NewWorkspaceTemplateRepository()
			path := filepath.Join(t.TempDir(), "templates.json")
			if backend == "file" {
				repo = filerepo.NewWorkspaceTemplateRepository(path)
			}
			old := historicalSoftwareTemplate(t)
			if err := repo.Create(ctx, old); err != nil {
				t.Fatal(err)
			}
			old, err := repo.Get(ctx, old.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := ensureBuiltinWorkspaceTemplateDefaults(ctx, repo); err != nil {
				t.Fatal(err)
			}
			unchanged, err := repo.Get(ctx, old.ID)
			if err != nil || !reflect.DeepEqual(unchanged, old) {
				t.Fatalf("startup upgraded template: %v", err)
			}
			owner := &Biz{templates: repo}
			preview, err := owner.queryBuiltinTemplateDefaults(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var request workspaceTemplateUpgradeEvent
			for _, item := range preview {
				if item.TemplateID == old.ID {
					if !item.UpgradeAvailable {
						t.Fatalf("historical template not recognized: %+v", item)
					}
					request = workspaceTemplateUpgradeEvent{TemplateID: old.ID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash}
				}
			}
			lost := &lostTemplateUpgradeResponse{WorkspaceTemplateRepository: repo}
			owner.templates = lost
			if _, err := owner.submitBuiltinTemplateUpgrade(ctx, request); err == nil {
				t.Fatal("expected lost response")
			}
			if lost.updates != 1 {
				t.Fatalf("writes = %d", lost.updates)
			}
			if backend == "file" {
				repo = filerepo.NewWorkspaceTemplateRepository(path)
			}
			owner = &Biz{templates: repo}
			for attempt := 0; attempt < 2; attempt++ {
				result, err := owner.submitBuiltinTemplateUpgrade(ctx, request)
				if err != nil || result.Upgraded || result.Default.UpgradeAvailable || result.Default.CurrentContentHash != request.TargetContentHash {
					t.Fatalf("replay after reload: %+v %v", result, err)
				}
			}
			custom := historicalSoftwareTemplate(t)
			custom.Description += " user customization"
			cmd := workspaceevents.UpdateWorkspaceTemplateCommand{TemplateID: custom.ID, Name: custom.Name, Description: custom.Description, GoalText: custom.GoalText, DoDItems: custom.DoDItems, DefaultSoulID: custom.DefaultSoulID, ContextHints: custom.ContextHints, EvidencePolicies: custom.EvidencePolicies}
			event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpdate, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
			if _, err := owner.executeWorkspaceTemplateCommand(ctx, event); err != nil {
				t.Fatal(err)
			}
			before, err := repo.Get(ctx, old.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owner.submitBuiltinTemplateUpgrade(ctx, request); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("customized template upgraded: %v", err)
			}
			after, err := repo.Get(ctx, old.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("customization overwritten: %v", err)
			}
		})
	}
}

func TestBuiltinTemplateUpgradeRejectsUntrustedOrUnreviewedRequests(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceTemplateRepository()
	old := historicalSoftwareTemplate(t)
	if err := repo.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	owner := &Biz{templates: repo}
	preview, err := owner.queryBuiltinTemplateDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var valid workspaceTemplateUpgradeEvent
	for _, item := range preview {
		if item.TemplateID == old.ID {
			valid = workspaceTemplateUpgradeEvent{TemplateID: old.ID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash}
		}
	}
	for _, change := range []func(*workspaceTemplateUpgradeEvent){
		func(e *workspaceTemplateUpgradeEvent) { e.ExpectedContentHash = "" },
		func(e *workspaceTemplateUpgradeEvent) { e.ExpectedContentHash = "unknown" },
		func(e *workspaceTemplateUpgradeEvent) { e.TargetContentHash = "different-release" },
		func(e *workspaceTemplateUpgradeEvent) { e.TemplateID = "missing" },
	} {
		request := valid
		change(&request)
		if _, err := owner.submitBuiltinTemplateUpgrade(ctx, request); err == nil {
			t.Fatalf("invalid upgrade accepted: %+v", request)
		}
	}
	command := workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultCommand{TemplateID: valid.TemplateID, ExpectedContentHash: valid.ExpectedContentHash, TargetContentHash: valid.TargetContentHash}
	for _, source := range []string{"untrusted/module", " application/workorchd/appservice "} {
		event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault, source, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		result := magicevent.NewResult(event.ID(), source, workspaceevents.CommandDestination)
		owner.handleBuiltinTemplateDefaultsCommand(event, result)
		if _, err := result.Get(); err == nil {
			t.Fatalf("unauthorized source accepted: %q", source)
		}
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	owner.handleBuiltinTemplateDefaultsCommand(event, nil)
	got, err := repo.Get(ctx, old.ID)
	if err != nil || !reflect.DeepEqual(got, old) {
		t.Fatalf("rejected/Post requests wrote content: %v", err)
	}
	result, err := owner.submitBuiltinTemplateUpgrade(ctx, valid)
	if err != nil || !result.Upgraded {
		t.Fatalf("valid request: %+v %v", result, err)
	}
}

func TestBuiltinTemplateUpgradeSerializesWithOrdinaryEdit(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceTemplateRepository()
	old := historicalSoftwareTemplate(t)
	if err := repo.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	owner := &Biz{templates: repo}
	items, err := owner.queryBuiltinTemplateDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var request workspaceTemplateUpgradeEvent
	for _, item := range items {
		if item.TemplateID == old.ID {
			request = workspaceTemplateUpgradeEvent{TemplateID: old.ID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash}
		}
	}
	cmd := workspaceevents.UpdateWorkspaceTemplateCommand{TemplateID: old.ID, Name: old.Name, Description: "concurrent user edit", GoalText: old.GoalText, DoDItems: old.DoDItems, DefaultSoulID: old.DefaultSoulID, ContextHints: old.ContextHints, EvidencePolicies: old.EvidencePolicies}
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpdate, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
		if _, err := owner.executeWorkspaceTemplateCommand(ctx, event); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wait.Done()
		<-start
		if _, err := owner.submitBuiltinTemplateUpgrade(ctx, request); err != nil && !errors.Is(err, common.ErrConflict) {
			t.Error(err)
		}
	}()
	close(start)
	wait.Wait()
	got, err := repo.Get(ctx, old.ID)
	if err != nil || got.Description != cmd.Description {
		t.Fatalf("upgrade overwrote concurrent edit: %+v %v", got, err)
	}
}
