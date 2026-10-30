package workspace

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Frozen historical bytes are for restart/snapshot preservation, not upgrade authority.
//
//go:embed testdata/software-dev-template-0317a3f6.json
var previousSoftwareTemplateJSON []byte

func previousSoftwareTemplate(t *testing.T) workorchmanagementcontract.WorkspaceTemplate {
	t.Helper()
	var raw workorchmanagementcontract.WorkspaceTemplate
	if err := json.Unmarshal(previousSoftwareTemplateJSON, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func historicalSoftwareTemplate(t *testing.T) *workspacemodel.WorkspaceTemplate {
	t.Helper()
	// Historical authority is reconstructed only from frozen reviewed input,
	// never from mutable current defaults.
	defaults := []workorchmanagementcontract.WorkspaceTemplate{previousSoftwareTemplate(t)}
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

func TestBuiltinTemplateIngressIsReadOnlyAndRejectsStaleOrUntrustedRequests(t *testing.T) {
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
	var valid workspaceTemplateUpgradeEvent
	for _, item := range items {
		if !item.Present || item.UpgradeAvailable || item.CurrentContentHash != item.BuiltinContentHash {
			t.Fatalf("not current: %+v", item)
		}
		if item.TemplateID == old.ID {
			valid = workspaceTemplateUpgradeEvent{TemplateID: item.TemplateID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash}
		}
	}
	for _, change := range []func(*workspaceTemplateUpgradeEvent){
		func(e *workspaceTemplateUpgradeEvent) { e.ExpectedContentHash = "" },
		func(e *workspaceTemplateUpgradeEvent) {
			e.ExpectedContentHash = workspacemodel.StableWorkspaceTemplateContentHash(old)
		},
		func(e *workspaceTemplateUpgradeEvent) { e.TargetContentHash = "other-release" },
		func(e *workspaceTemplateUpgradeEvent) { e.TemplateID = "missing" },
	} {
		request := valid
		change(&request)
		if _, err := owner.submitBuiltinTemplateUpgrade(ctx, request); err == nil {
			t.Fatalf("stale request accepted: %+v", request)
		}
	}
	cmd := workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultCommand{TemplateID: valid.TemplateID, ExpectedContentHash: valid.ExpectedContentHash, TargetContentHash: valid.TargetContentHash}
	for _, source := range []string{"untrusted/module", " application/workorchd/appservice "} {
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault, source, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
		result := magicevent.NewResult(ev.ID(), source, workspaceevents.CommandDestination)
		owner.handleBuiltinTemplateDefaultsCommand(ev, result)
		if _, err := result.Get(); err == nil {
			t.Fatal("untrusted source accepted")
		}
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	owner.handleBuiltinTemplateDefaultsCommand(ev, nil)
	for i := 0; i < 2; i++ {
		result, err := owner.submitBuiltinTemplateUpgrade(ctx, valid)
		if err != nil || result.Upgraded || result.Default.CurrentContentHash != valid.TargetContentHash {
			t.Fatalf("read-only replay: %+v %v", result, err)
		}
	}
	after, err := repo.Get(ctx, old.ID)
	if err != nil || !reflect.DeepEqual(after, old) {
		t.Fatal("read/retired upgrade changed historical bytes", err)
	}
}

func TestBuiltinTemplateCannotBeCreatedEditedOrDeletedConcurrently(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceTemplateRepository()
	owner := &Biz{templates: repo}
	commands := []struct {
		topic string
		data  any
	}{
		{workspaceevents.TopicWorkspaceTemplateCommandCreate, workspaceevents.CreateWorkspaceTemplateCommand{TemplateID: "software-dev", Name: "override"}},
		{workspaceevents.TopicWorkspaceTemplateCommandUpdate, workspaceevents.UpdateWorkspaceTemplateCommand{TemplateID: "software-dev", Name: "override"}},
		{workspaceevents.TopicWorkspaceTemplateCommandDelete, workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: "software-dev"}},
	}
	var wait sync.WaitGroup
	for _, command := range commands {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ev := magicevent.NewEventWithContext(command.topic, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command.data)
			if _, err := owner.executeWorkspaceTemplateCommand(ctx, ev); !errors.Is(err, common.ErrConflict) {
				t.Errorf("builtin mutation: %v", err)
			}
		}()
	}
	wait.Wait()
	stored, err := repo.List(ctx)
	if err != nil || len(stored) != 0 {
		t.Fatal("builtin persisted", err)
	}
}
