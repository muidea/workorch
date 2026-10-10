package biz

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestSpecificationCompletionRequiresExactPersistedOutcome(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "mismatch"}[mismatch], func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			defer hub.Terminate(ctx)
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			if err := owner.Subscribe(workspaceevents.TopicWorkspaceSpecificationComplete, func(ev magicevent.Event, result magicevent.Result) {
				command := ev.Data().(workspaceevents.CompleteDefinitionSpecificationRequestCommand)
				completion := command.Completion.Clone()
				if mismatch {
					completion.FailureCode = "other_failure"
				}
				result.Set(workspaceevents.DefinitionSpecificationRequestResult{Identity: command.Identity,
					Disposition: workspaceevents.DefinitionSpecificationAlreadyCompleted, Completed: &completion,
					Consumed: 1, Remaining: workspace.DefinitionSpecificationRequestLimit - 1}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer owner.Unsubscribe(workspaceevents.TopicWorkspaceSpecificationComplete)
			identity := workspace.DefinitionSpecificationRequestIdentity{WorkspaceID: "ws", CandidateID: "candidate",
				CandidateRevision: 1, CandidateHash: "hash", CompilationInputHash: "input",
				Phase: workspace.DefinitionSpecificationProposal, LogicalRequestID: "proposal-v1"}
			err := b.persistDefinitionSpecificationCompletion(ctx, workspaceevents.CompleteDefinitionSpecificationRequestCommand{
				Identity: identity, PermitID: identity.PermitID(), Completion: workspace.DefinitionSpecificationRequestCompletion{FailureCode: "provider_failed"}})
			if (err != nil) != mismatch {
				t.Fatalf("mismatch=%v err=%v", mismatch, err)
			}
		})
	}
}

func TestSpecificationPersistedFailureReplayNeverCallsProvider(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceSpecificationReserve, func(ev magicevent.Event, result magicevent.Result) {
		command := ev.Data().(workspaceevents.ReserveDefinitionSpecificationRequestCommand)
		result.Set(workspaceevents.DefinitionSpecificationRequestResult{Identity: command.Identity,
			Disposition: workspaceevents.DefinitionSpecificationAlreadyCompleted,
			Completed:   &workspace.DefinitionSpecificationRequestCompletion{FailureCode: "provider_failed"},
			Consumed:    1, Remaining: workspace.DefinitionSpecificationRequestLimit - 1}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceSpecificationReserve)
	provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	calls := 0
	if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProvider, func(ev magicevent.Event, result magicevent.Result) {
		calls++
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProvider)
	candidate := specificationCandidateFixture()
	identity := workspace.DefinitionSpecificationRequestIdentity{WorkspaceID: candidate.WorkspaceID,
		CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, CompilationInputHash: "input", Phase: workspace.DefinitionSpecificationProposal, LogicalRequestID: "proposal-v1"}
	for i := 0; i < 2; i++ {
		// 新 Application 实例也必须使用 owner 已保存的失败，不能重置调用额度。
		b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
		_, err := b.completePersistedSpecificationDocument(ctx, identity, llmevents.CompleteProviderCommand{}, candidate, nil, workspaceevents.BeginDefinitionSpecificationResult{})
		if err == nil {
			t.Fatal("persisted failure accepted as success")
		}
	}
	if calls != 0 {
		t.Fatalf("provider calls = %d", calls)
	}
}

func TestSpecificationPersistedReplayNeverRepeatsUnresolvedProviderCall(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := owner.Subscribe(workspaceevents.TopicWorkspaceSpecificationReserve, func(ev magicevent.Event, result magicevent.Result) {
		command := ev.Data().(workspaceevents.ReserveDefinitionSpecificationRequestCommand)
		result.Set(workspaceevents.DefinitionSpecificationRequestResult{Identity: command.Identity,
			Disposition: workspaceevents.DefinitionSpecificationAlreadyReserved, Consumed: 1,
			Remaining: workspace.DefinitionSpecificationRequestLimit - 1}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer owner.Unsubscribe(workspaceevents.TopicWorkspaceSpecificationReserve)
	calls := 0
	provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProvider, func(ev magicevent.Event, result magicevent.Result) {
		calls++
		t.Error("unresolved reservation repeated provider call")
	}); err != nil {
		t.Fatal(err)
	}
	defer provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProvider)
	candidate := specificationCandidateFixture()
	identity := workspace.DefinitionSpecificationRequestIdentity{WorkspaceID: candidate.WorkspaceID,
		CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, CompilationInputHash: "input", Phase: workspace.DefinitionSpecificationProposal,
		LogicalRequestID: "proposal-v1"}
	for i := 0; i < 2; i++ {
		_, err := b.completePersistedSpecificationDocument(ctx, identity, llmevents.CompleteProviderCommand{}, candidate, nil, workspaceevents.BeginDefinitionSpecificationResult{})
		if err == nil {
			t.Fatal("unresolved request accepted")
		}
	}
	if calls != 0 {
		t.Fatalf("provider calls = %d", calls)
	}
}
