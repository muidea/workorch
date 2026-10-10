package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuthoringSpecificationPersistsProposalAndSixIndependentReviews(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "test"}
	configuration := llmevents.GetActiveConfigurationResult{ConfigurationRevision: "revision", DefaultChat: target}
	bindingHash := definitionProviderBindingHash(configuration)
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	var err error
	var derivation workspace.DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("derive/"+record.InputHash, bindingHash, nil)
	if err != nil {
		t.Fatal(err)
	}
	acceptance := &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	record, err = record.CompleteDerivation(derivation.PermitID, acceptance, "")
	if err != nil {
		t.Fatal(err)
	}
	candidateItems, err := acceptance.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	mapping, _, proposal, err := testkit.Proposal(record.DerivationRequests[0], workspace.DefinitionAuthoringVerificationMapping{Status: workspace.GoalVerificationUnverified, Declaration: "待评估"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.ValidateAuthoringSpecification(input, &template, *acceptance, nil, proposal); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	owner := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	for _, topic := range []string{workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, workspaceevents.TopicWorkspaceAuthoringSpecificationComplete} {
		if err := owner.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
			mu.Lock()
			defer mu.Unlock()
			switch command := ev.Data().(type) {
			case workspaceevents.QueryDefinitionAuthoringCommand:
				copy := record.Clone()
				result.Set(workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 1, Record: authoringTestRecord(copy), CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: copy.TemplateSnapshotHash}, nil)
			case workspaceevents.ReserveAuthoringSpecificationCommand:
				next, request, fresh, err := record.ReserveSpecification(command.Request())
				if err != nil {
					t.Error(err)
					return
				}
				record = next
				response := workspaceevents.AuthoringSpecificationRequestResult{Request: request.Metadata(), Consumed: uint32(len(record.SpecificationRequests)), Remaining: uint32(workspace.DefinitionAuthoringSpecificationRequestLimit - len(record.SpecificationRequests))}
				if fresh {
					response.FreshPermitID = request.PermitID
				}
				result.Set(response, nil)
			case workspaceevents.CompleteAuthoringSpecificationCommand:
				next, err := record.CompleteSpecification(command.PermitID, command.Proposal, command.Review, command.FailureCode, command.Diagnostic)
				if command.Mapping != nil {
					next, err = record.CompleteSpecificationMapping(command.PermitID, *command.Mapping, *command.SuccessAudit)
				}
				if err != nil {
					t.Error(err)
					return
				}
				record = next
				for _, request := range record.SpecificationRequests {
					if request.PermitID == command.PermitID {
						result.Set(workspaceevents.AuthoringSpecificationRequestResult{Request: request.Metadata(), Consumed: uint32(len(record.SpecificationRequests)), Remaining: uint32(workspace.DefinitionAuthoringSpecificationRequestLimit - len(record.SpecificationRequests))}, nil)
						return
					}
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
		defer owner.Unsubscribe(topic)
	}
	llm := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	if err := llm.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(configuration, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer llm.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
	calls := 0
	physicalCalls := 0
	persistedPrefix := 0
	if err := llm.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(ev magicevent.Event, result magicevent.Result) {
		mu.Lock()
		defer mu.Unlock()
		command := ev.Data().(llmevents.CompleteProviderDocumentCommand)
		if command.ExpectedConfigurationRevision != "revision" || command.Target != target || len(record.SpecificationRequests) != persistedPrefix+calls+1 {
			t.Errorf("provider called without persisted permit and frozen target: %+v", command)
		}
		var payload []byte
		var err error
		if command.Request.Kind == "workspace/specification/proposal" {
			payload, err = json.Marshal(struct {
				Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
			}{mapping})
		} else {
			kind := strings.TrimPrefix(command.Request.Kind, "workspace/specification/review/")
			if strings.Contains(command.Request.PromptContext.Sections[1].Content, `"Reviews":[{`) {
				t.Error("critic received a previous critic's conclusion")
			}
			review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "覆盖", EvidenceRefs: []string{candidateItems[0].CandidateItemRef}, RequirementIDs: authoringHTTPRequiredReviewRequirementIDs(t, proposal, kind)}
			payload, err = json.Marshal(struct {
				Review workspace.GoalSpecificationCompletenessReview `json:"review"`
			}{review})
		}
		if err != nil {
			t.Error(err)
		}
		calls++
		physicalCalls++
		result.Set(authoringTestDocumentResult(t, command, payload), nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer llm.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument)
	for range 2 {
		b := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
		spec, err := b.CompileAuthoringSpecification(ctx, input.WorkspaceID)
		if err != nil || len(spec.Reviews) != 6 {
			t.Fatalf("compile: %+v %v", spec, err)
		}
	}
	if calls != 7 || physicalCalls != 7 || len(record.SpecificationRequests) != 7 {
		t.Fatalf("physical calls=%d, durable records=%d", physicalCalls, len(record.SpecificationRequests))
	}
	complete := record.Clone()
	blockedView, err := definitionAuthoringView(workspaceevents.QueryDefinitionAuthoringResult{
		WorkspaceID: input.WorkspaceID, WorkspaceRevision: 1, Record: authoringTestRecord(complete),
		CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: complete.TemplateSnapshotHash,
	})
	if err != nil || blockedView.State != "review_blocked" || blockedView.Proposal == nil || len(blockedView.Proposal.Reviews) != 6 {
		t.Fatalf("six PASS reviews with UNVERIFIED became ready: %+v %v", blockedView, err)
	}
	// 协调修改完整 ledger 身份后仍然缺少实际 Profile，不能仅凭自洽 DECLARED 展示 ready。
	selfDeclared := complete.Clone()
	for index := range selfDeclared.SpecificationRequests {
		request := &selfDeclared.SpecificationRequests[index]
		if request.Proposal == nil {
			continue
		}
		for verificationIndex := range request.Proposal.Verifications {
			verification := &request.Proposal.Verifications[verificationIndex]
			verification.Status = workspace.GoalVerificationDeclared
			verification.ProfileID, verification.EvidenceRequirementID, verification.Scope = "invented", "*", "dodItem"
		}
		for i := range request.Mapping.Obligations {
			verification := &request.Mapping.Obligations[i].Verification
			verification.Status = workspace.GoalVerificationDeclared
			verification.ProfileID, verification.EvidenceRequirementID, verification.Scope = "invented", "*", "dodItem"
		}
		audit, err := testkit.Audit(*request.Mapping)
		if err != nil {
			t.Fatal(err)
		}
		request.SuccessAudit = &audit
		request.CompletionIdentity = workspace.MappingCompletionIdentity(request.PermitID, *request.Mapping, audit)
		changedHash := request.Proposal.ContentHash()
		for reviewIndex := range selfDeclared.SpecificationRequests {
			reviewRequest := &selfDeclared.SpecificationRequests[reviewIndex]
			if reviewRequest.Review == nil {
				continue
			}
			reviewRequest.ProposalHash = changedHash
			reviewRequest.Review.SpecificationHash = changedHash
			reviewRequest.PermitID = reviewRequest.ExpectedPermitID()
		}
	}
	if err := selfDeclared.Validate(); err != nil {
		t.Fatalf("self-declared fixture must remain model-valid: %v", err)
	}
	selfDeclaredView, err := definitionAuthoringView(workspaceevents.QueryDefinitionAuthoringResult{
		WorkspaceID: input.WorkspaceID, WorkspaceRevision: 1, Record: authoringTestRecord(selfDeclared),
		CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: selfDeclared.TemplateSnapshotHash,
	})
	if err != nil || selfDeclaredView.State != "review_blocked" {
		t.Fatalf("self-declared Profile became ready: %+v %v", selfDeclaredView, err)
	}
	configuration.ConfigurationRevision = "changed-after-completion"
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil), authoringTasks: newAuthoringTaskLifecycle()}
	if replay, err := b.CompileAuthoringSpecification(ctx, input.WorkspaceID); err != nil || len(replay.Reviews) != 6 || physicalCalls != 7 {
		t.Fatalf("sealed result was not replayed independent of provider configuration: %+v %v", replay, err)
	}
	partial := complete.Clone()
	partial.SpecificationRequests = partial.SpecificationRequests[:3]
	record = partial
	if _, err := b.CompileAuthoringSpecification(ctx, input.WorkspaceID); err == nil || physicalCalls != 7 || len(record.SpecificationRequests) != 3 {
		t.Fatalf("partial reviews crossed provider configuration drift: calls=%d err=%v", physicalCalls, err)
	}
	configuration.ConfigurationRevision = "revision"
	unresolved := partial.Clone()
	unresolved.SpecificationRequests[2].Review = nil
	record = unresolved
	if _, err := b.CompileAuthoringSpecification(ctx, input.WorkspaceID); err == nil || physicalCalls != 7 || len(record.SpecificationRequests) != 3 {
		t.Fatalf("unresolved critic was resubmitted: calls=%d err=%v", physicalCalls, err)
	}
	record = partial.Clone()
	// The stored prefix replays; only the four missing kinds can issue new permits.
	persistedPrefix = 3
	calls = 0
	if resumed, err := b.CompileAuthoringSpecification(ctx, input.WorkspaceID); err != nil || len(resumed.Reviews) != 6 || calls != 4 || physicalCalls != 11 || len(record.SpecificationRequests) != 7 {
		t.Fatalf("partial reviews were not resumed: calls=%d record=%d err=%v", physicalCalls, len(record.SpecificationRequests), err)
	}
	failed := partial.Clone()
	failed.SpecificationRequests[2].Review = nil
	failed.SpecificationRequests[2].FailureCode = "provider_failed"
	record = failed
	if _, err := b.CompileAuthoringSpecification(ctx, input.WorkspaceID); err == nil || physicalCalls != 11 || len(record.SpecificationRequests) != 3 {
		t.Fatalf("failed critic was resubmitted: calls=%d err=%v", physicalCalls, err)
	}
}
