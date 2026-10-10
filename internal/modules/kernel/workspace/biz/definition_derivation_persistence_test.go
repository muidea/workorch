package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestDerivationReserveRejectsUnsealedResourceBeforeProviderPermit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	directory := t.TempDir()
	content := "验收资源正文"
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-derive-resource", Revision: 1, GoalText: "目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: "t", Name: "模板"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DeliverableDirectory: directory, DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, RequestID: "derive/" + record.InputHash, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve}); err == nil {
		t.Fatal("unsealed resource obtained a derivation provider permit")
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
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
	defer func() {
		file.Teardown(ctx)
		owners.close()
	}()
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: input.WorkspaceID})
	if err != nil || view.Record == nil || len(view.Record.DerivationRequests) != 0 {
		t.Fatalf("resource reserve mutated persistent budget: %+v %v", view, err)
	}
	checksum := sha256.Sum256([]byte(content))
	reserve.Resources = []workspace.DefinitionAuthoringResourceSnapshot{{Ref: input.ResourceRefs[0], Content: content, ContentHash: "sha256:" + hex.EncodeToString(checksum[:])}}
	reserve.ResourceSnapshotHash = workspace.DefinitionAuthoringResourcesHash(reserve.Resources)
	reserve.WorkspaceRevision = view.WorkspaceRevision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve}); err == nil {
		t.Fatal("unverified caller snapshot obtained a permit directly")
	}
	send := func(command workspaceevents.ReserveDefinitionDerivationCommand) (workspaceevents.DefinitionDerivationRequestResult, error) {
		response := owners.hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDerivationReserve, definitionSpecificationCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command))
		if response == nil {
			t.Fatal("missing derivation reserve response")
		}
		if err := response.Error(); err != nil {
			return workspaceevents.DefinitionDerivationRequestResult{}, err
		}
		value, err := response.Get()
		if err != nil {
			return workspaceevents.DefinitionDerivationRequestResult{}, err
		}
		result, ok := value.(workspaceevents.DefinitionDerivationRequestResult)
		if !ok {
			t.Fatalf("unexpected derivation result: %T", value)
		}
		return result, nil
	}
	forged := reserve
	forged.Resources = workspace.CloneDefinitionAuthoringResources(reserve.Resources)
	forged.Resources[0].Content = "伪造正文"
	changed := sha256.Sum256([]byte(forged.Resources[0].Content))
	forged.Resources[0].ContentHash = "sha256:" + hex.EncodeToString(changed[:])
	forged.ResourceSnapshotHash = workspace.DefinitionAuthoringResourcesHash(forged.Resources)
	if _, err := send(forged); err == nil {
		t.Fatal("caller-forged consistent content obtained a File verification")
	}
	first, err := send(reserve)
	if err != nil || first.FreshPermitID == "" {
		t.Fatalf("File-verified snapshot reservation: %+v %v", first, err)
	}
	file.Teardown(ctx)
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	view, err = owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: input.WorkspaceID})
	if err != nil || view.Record == nil || len(view.Record.DerivationRequests) != 1 || view.Record.DerivationRequests[0].ResourceSnapshotHash != reserve.ResourceSnapshotHash || view.Record.DerivationRequests[0].Resources[0].Content != content {
		t.Fatalf("resource snapshot did not survive owner reopen: %+v %v", view, err)
	}
	reserve.WorkspaceRevision = view.WorkspaceRevision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve}); err == nil {
		t.Fatal("direct replay bypassed File verification")
	}
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
	replay, err := send(reserve)
	if err != nil || replay.FreshPermitID != "" || replay.Request.PermitID != first.Request.PermitID {
		t.Fatalf("replay reissued physical permit: %+v %v", replay, err)
	}
	if _, err := send(forged); err == nil {
		t.Fatal("changed resource reused the original derivation request identity")
	}
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("修改后的正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := send(reserve); err == nil {
		t.Fatal("file drift replayed a resource-bound reserve")
	}
	current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	current.Description = "same authoring input, newer workspace revision"
	if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := send(reserve); err == nil {
		t.Fatal("stale workspace revision replayed a resource-bound reserve")
	}
}

func TestDerivationHistoricalCompletionAfterTemplateRemoval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-derive-removed-template", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t", Name: "模板"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DefinitionAuthoring: &record}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, RequestID: "derive/" + record.InputHash, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	first, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve})
	if err != nil || first.Derivation == nil || first.Derivation.FreshPermitID == "" {
		owners.close()
		t.Fatalf("reserve: %+v %v", first, err)
	}
	if err := owners.repositories.WorkspaceTemplates.Delete(ctx, template.ID); err != nil {
		owners.close()
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: input.WorkspaceID})
	if err != nil || view.Record == nil || view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != "" {
		t.Fatalf("historical query: %+v %v", view, err)
	}
	proposal := &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"验收条件"}}
	complete := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, PermitID: first.Derivation.FreshPermitID, Proposal: proposal}
	done, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &complete})
	if err != nil || done.Derivation == nil || done.Derivation.Request.Proposal == nil {
		t.Fatalf("complete historical permit: %+v %v", done, err)
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve}); err == nil {
		t.Fatal("new reserve accepted after template removal")
	}
	view, err = owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: input.WorkspaceID})
	if err != nil || view.Record == nil || view.Record.DerivationRequests[0].Proposal == nil {
		t.Fatalf("historical completion not queryable: %+v %v", view, err)
	}
}

func TestDerivationPermitSurvivesFileReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-derive", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t", Name: "模板"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: "t", DefinitionAuthoring: &record}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	command := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, InputHash: input.InputHash(), RequestID: "derive-1", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil), ProviderBindingHash: "provider"}
	first, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &command})
	owners.close()
	if err != nil {
		t.Fatal(err)
	}
	if first.Derivation == nil || first.Derivation.FreshPermitID == "" {
		t.Fatal("no durable permit")
	}
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	replay, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &command})
	if err != nil || replay.Derivation.FreshPermitID != "" || replay.Derivation.Consumed != 1 {
		t.Fatalf("restart reissued permit: %+v %v", replay, err)
	}
	proposal := &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"验收条件"}}
	complete := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, PermitID: first.Derivation.FreshPermitID, Proposal: proposal}
	done, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &complete})
	if err != nil || done.Derivation.Request.Proposal == nil || done.Derivation.FreshPermitID != "" {
		t.Fatalf("complete: %+v %v", done, err)
	}
	duplicate, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &complete})
	if err != nil || duplicate.Next != nil {
		t.Fatalf("duplicate completion wrote: %v", err)
	}
	complete.Proposal = nil
	complete.FailureCode = "provider_failed"
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &complete}); err == nil {
		t.Fatal("completion overwritten")
	}
	// 通知调用没有同步 result，不得解引用 nil 或执行写入。
	owners.workspace.handleDefinitionSpecificationCommand(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDerivationReserve, definitionSpecificationCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command), nil)
}
