package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringResourceScopeUsesRealWorkspaceOwnerAfterFileReopen(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	directory := t.TempDir()
	otherDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("验收要求\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	owners := openFileDefinitionAdmissionOwners(t, storage)
	defer func() { owners.close() }()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-authoring-file-scope", Revision: 1, GoalText: "目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: input.TemplateID}); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: input.TemplateID, DeliverableDirectory: directory}); err != nil {
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	save := workspaceevents.SaveDefinitionAuthoringCommand{Input: input, ExpectedWorkspaceRevision: stored.Revision}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, storage)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	file, err := fileowner.New(owners.hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { file.Teardown(ctx) }()
	command := fileevents.AcquireAuthoringResourceCommand{WorkspaceID: input.WorkspaceID, AuthoringInputHash: input.InputHash(), ResourceRef: input.ResourceRefs[0]}
	query := workspaceevents.QueryAuthoringResourceScopeCommand{WorkspaceID: input.WorkspaceID, InputHash: input.InputHash(), ResourceRef: input.ResourceRefs[0]}
	for _, source := range []string{"application/workorchd", "kernel/orchestrator", " blocks/file "} {
		response := owners.hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, source, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, query))
		if response == nil || response.Error() == nil {
			t.Fatalf("untrusted source %q obtained Workspace directory", source)
		}
	}
	response := owners.hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, fileowner.ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, query))
	if response == nil || response.Error() != nil {
		t.Fatalf("File scope query failed: %#v", response)
	}
	value, getErr := response.Get()
	if getErr != nil {
		t.Fatalf("File scope result unavailable: %v", getErr)
	}
	scope, ok := value.(workspaceevents.QueryAuthoringResourceScopeResult)
	if !ok || scope.ValidateFor(query) != nil || scope.DeliverableDirectory != directory {
		t.Fatalf("owner did not return its current directory: %#v", value)
	}
	read := func(c fileevents.AcquireAuthoringResourceCommand) (fileevents.AcquireAuthoringResourceResult, error) {
		response := owners.hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireAuthoringResources, "application/workorchd", fileowner.ModuleID, magicevent.NewHeader(), ctx, c))
		if response == nil {
			return fileevents.AcquireAuthoringResourceResult{}, nil
		}
		if err := response.Error(); err != nil {
			return fileevents.AcquireAuthoringResourceResult{}, err
		}
		value, err := response.Get()
		if err != nil {
			return fileevents.AcquireAuthoringResourceResult{}, err
		}
		result, _ := value.(fileevents.AcquireAuthoringResourceResult)
		return result, nil
	}
	fact, err := read(command)
	if err != nil || fact.ValidateFor(command) != nil || fact.Content != "验收要求\n" {
		t.Fatalf("file-backed owner query: %+v %v", fact, err)
	}
	if err := os.WriteFile(filepath.Join(otherDirectory, "requirements.txt"), []byte("另一目录\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	current.DeliverableDirectory = otherDirectory
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	fact, err = read(command)
	if err != nil || fact.ValidateFor(command) != nil || fact.Content != "另一目录\n" {
		t.Fatalf("File used stale caller directory after Workspace CAS: %+v %v", fact, err)
	}
	command.AuthoringInputHash = "stale"
	if fact, err := read(command); err == nil || fact.Content != "" {
		t.Fatalf("stale draft requested current directory: %+v %v", fact, err)
	}
	file.Teardown(ctx)
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, storage)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	file, err = fileowner.New(owners.hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	command.AuthoringInputHash = input.InputHash()
	fact, err = read(command)
	if err != nil || fact.ValidateFor(command) != nil || fact.Content != "另一目录\n" {
		t.Fatalf("owner reopened with stale resource directory: %+v %v", fact, err)
	}
}
