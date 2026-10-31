package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDerivationApplicationAcquiresFileFactsBeforeProviderAndReplays(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	storage, directory := t.TempDir(), t.TempDir()
	content := "可验证的验收资源\n"
	path := filepath.Join(directory, "requirements.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-derive-with-resource", Revision: 1, GoalText: "交付目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID, Name: "模板"}
	if err := repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: template.ID, DeliverableDirectory: directory}); err != nil {
		t.Fatal(err)
	}
	owner, err := workspaceowner.New(hub, nil, workspaceowner.Dependencies{OwnerRepositories: repositories.WorkspaceOwner, EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	file, err := fileowner.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	defer file.Teardown(ctx)
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
	stored, err := repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDefinitionAuthoring(ctx, workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: input, ExpectedWorkspaceRevision: stored.Revision}); err != nil {
		t.Fatal(err)
	}
	provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}
	if err := provider.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetActiveConfigurationResult{ConfigurationRevision: "revision", DefaultChat: target}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
	calls := 0
	if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(llmevents.CompleteProviderDocumentCommand)
		if !ok {
			t.Error("unexpected provider command")
			return
		}
		latest, err := repositories.Workspaces.Get(ctx, input.WorkspaceID)
		if err != nil || latest.DefinitionAuthoring == nil || len(latest.DefinitionAuthoring.DerivationRequests) != 1 {
			t.Errorf("provider called before owner permit: %v", err)
			return
		}
		sum := sha256.Sum256([]byte(content))
		sealed := latest.DefinitionAuthoring.DerivationRequests[0]
		if len(sealed.Resources) != 1 || sealed.Resources[0].ContentHash != "sha256:"+hex.EncodeToString(sum[:]) || sealed.Resources[0].Content != content || command.Request.Kind != "workspace/specification/derive" {
			t.Errorf("provider lost exact File snapshot: %+v", sealed)
			return
		}
		calls++
		payload, err := json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"补充验收"}})
		if err != nil {
			t.Error(err)
			return
		}
		result.Set(authoringTestDocumentResult(t, command, payload), nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument)
	proposal, err := b.DeriveDefinitionAcceptance(ctx, input.WorkspaceID)
	if err != nil || len(proposal.AdditionalItems) != 1 || calls != 1 {
		t.Fatalf("File-derived acceptance: %+v calls=%d err=%v", proposal, calls, err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	replay, err := b.DeriveDefinitionAcceptance(ctx, input.WorkspaceID)
	if err != nil || replay.AuthoringInputHash != proposal.AuthoringInputHash || calls != 1 {
		t.Fatalf("historical request repeated physical provider call: %+v calls=%d err=%v", replay, calls, err)
	}
}

func TestDerivationApplicationReplaysHistoricalRequestWithoutCurrentTemplate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var request workspace.DefinitionDerivationRequest
	var err error
	record, request, _, err = record.ReserveDerivation("derive/"+record.InputHash, "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = completeTestDerivation(record, request.PermitID, &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"验收"}}, "")
	if err != nil {
		t.Fatal(err)
	}

	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(ev magicevent.Event, result magicevent.Result) {
		copy := record.Clone()
		result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 2, Record: authoringTestRecord(copy)}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery)
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
	proposal, err := b.DeriveDefinitionAcceptance(ctx, input.WorkspaceID)
	if err != nil || len(proposal.AdditionalItems) != 1 {
		t.Fatalf("historical replay required current template or provider: %+v %v", proposal, err)
	}
}

func TestDerivationApplicationCallsProviderOnceAndReplays(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var mu sync.Mutex
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	for _, topic := range []string{workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, workspaceevents.TopicWorkspaceDerivationReserve, workspaceevents.TopicWorkspaceDerivationComplete} {
		if err := owner.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
			mu.Lock()
			defer mu.Unlock()
			switch cmd := ev.Data().(type) {
			case workspaceevents.QueryDefinitionAuthoringCommand:
				copy := record.Clone()
				result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 1, Record: authoringTestRecord(copy), CurrentTemplateSnapshotHash: record.TemplateSnapshotHash, CurrentTemplateAvailable: true}, nil)
			case workspaceevents.ReserveDefinitionDerivationCommand:
				next, request, fresh, err := record.ReserveDerivation(cmd.RequestID, cmd.ProviderBindingHash, cmd.Resources)
				if err != nil {
					t.Error(err)
					return
				}
				record = next
				reply := workspaceevents.DefinitionDerivationRequestResult{Request: workspaceeventmapping.DefinitionDerivationRequestDTO(request), Consumed: 1, Remaining: 15}
				if fresh {
					reply.FreshPermitID = request.PermitID
				}
				result.Set(reply, nil)
			case workspaceevents.CompleteDefinitionDerivationCommand:
				next, err := record.CompleteDerivationDocument(cmd.PermitID, cmd.Proposal, cmd.FailureCode, cmd.SuccessAudit, cmd.Diagnostic)
				if err != nil {
					t.Error(err)
					return
				}
				record = next
				result.Set(workspaceevents.DefinitionDerivationRequestResult{Request: workspaceeventmapping.DefinitionDerivationRequestDTO(record.DerivationRequests[0]), Consumed: 1, Remaining: 15}, nil)
			}
		}); err != nil {
			t.Fatal(err)
		}
		defer owner.Unsubscribe(topic)
	}
	provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	target := runtimecontract.ProviderModelTarget{ProviderID: "test", ModelID: "test"}
	if err := provider.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetActiveConfigurationResult{ConfigurationRevision: "revision", DefaultChat: target}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
	calls := 0
	if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(ev magicevent.Event, result magicevent.Result) {
		mu.Lock()
		defer mu.Unlock()
		command, ok := ev.Data().(llmevents.CompleteProviderDocumentCommand)
		if !ok || command.ExpectedConfigurationRevision != "revision" {
			t.Error("provider request lost reserved configuration identity")
		}
		if len(record.DerivationRequests) != 1 {
			t.Error("provider called before durable reserve")
		}
		calls++
		payload, err := json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"验收"}})
		if err != nil {
			t.Error(err)
			return
		}
		result.Set(authoringTestDocumentResult(t, command, payload), nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument)
	for range 2 {
		b := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
		proposal, err := b.DeriveDefinitionAcceptance(ctx, input.WorkspaceID)
		if err != nil || len(proposal.AdditionalItems) != 1 {
			t.Fatalf("derivation: %+v %v", proposal, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("physical calls=%d", calls)
	}
}
