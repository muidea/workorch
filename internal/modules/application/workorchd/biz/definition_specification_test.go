package biz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSpecificationCompilerSeparatesProposalAndSixCritics(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "pass", true: "unknown"}[unknown], func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			defer hub.Terminate(ctx)
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			candidate := specificationCandidateFixture()
			spec := workspace.GoalSpecification{
				Version: workspace.GoalSpecificationVersionCurrent, GoalItemRef: "goal", GoalTextHash: workspace.StableDefinitionTextHash(candidate.GoalInput.Text),
				Dimensions:    []workspace.GoalSpecificationDimension{{ID: "coverage", Required: true, Activated: true}},
				Requirements:  []workspace.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "coverage", Required: true, SourceKind: workspace.GoalRequirementUserExplicit, SourceItemRefs: []string{"goal", "dod"}, Text: "来源可追溯"}},
				Obligations:   []workspace.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: "文档引用原始来源"}},
				Verifications: []workspace.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Declaration: "检查来源", Status: workspace.GoalVerificationDeclared, ProfileID: "generic", EvidenceRequirementID: "proof", Scope: "dodItem"}},
			}
			calls := 0
			observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			if err := observer.Subscribe(llmevents.TopicLLMCommandCompleteProvider, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				command, ok := ev.Data().(llmevents.CompleteProviderCommand)
				if !ok {
					t.Error("wrong provider command")
					return
				}
				var payload []byte
				if calls == 1 {
					payload, _ = json.Marshal(struct {
						Specification workspace.GoalSpecification `json:"specification"`
					}{spec})
				} else {
					kind := strings.TrimPrefix(command.Request.Kind, "workspace/specification/review/")
					status := workspace.GoalReviewPass
					if unknown && kind == workspace.GoalReviewNegativePath {
						status = workspace.GoalReviewUnknown
					}
					review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: status, SpecificationHash: spec.ContentHash(), Reason: "逐项核对原约束", EvidenceRefs: []string{"goal", "dod"}, RequirementIDs: []string{"requirement"}}
					payload, _ = json.Marshal(struct {
						Review workspace.GoalSpecificationCompletenessReview `json:"review"`
					}{review})
					var request struct {
						Specification workspace.GoalSpecification `json:"specification"`
					}
					if err := json.Unmarshal([]byte(command.Request.PromptContext.Sections[1].Content), &request); err != nil || len(request.Specification.Reviews) != 0 {
						t.Error("critic was influenced by prior reviews")
					}
				}
				result.Set(llmevents.CompleteProviderResult{Target: runtimecontract.ProviderModelTarget{ProviderID: "test", ModelID: "test"}, Completion: llmevents.ProviderCompletionData{Content: string(payload)}}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(llmevents.TopicLLMCommandCompleteProvider)
			got, err := b.CompileDefinitionSpecification(ctx, candidate, nil, spec.Dimensions)
			if err != nil || calls != 7 || got.Specification == nil {
				t.Fatalf("compile: calls=%d candidate=%+v err=%v", calls, got, err)
			}
			admission, err := definitionadmission.Compile(got)
			want := workspace.DefinitionAdmissionAdmittedAllOf
			if unknown {
				want = workspace.DefinitionAdmissionClarificationRequired
			}
			if err != nil || admission.Disposition != want {
				t.Fatalf("admission=%+v err=%v", admission, err)
			}
			if candidate.Specification != nil {
				t.Fatal("input candidate mutated")
			}
		})
	}
}

func TestSpecificationCompilerRejectsInvalidProviderResultBeforeReview(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result llmevents.CompleteProviderResult
	}{
		{name: "missing target", result: llmevents.CompleteProviderResult{Completion: llmevents.ProviderCompletionData{Content: `{}`}}},
		{name: "empty content", result: llmevents.CompleteProviderResult{Target: runtimecontract.ProviderModelTarget{ProviderID: "test", ModelID: "test"}}},
		{name: "invalid document", result: llmevents.CompleteProviderResult{Target: runtimecontract.ProviderModelTarget{ProviderID: "test", ModelID: "test"}, Completion: llmevents.ProviderCompletionData{Content: `{"specification":null}`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			defer hub.Terminate(ctx)
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			calls := 0
			observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			if err := observer.Subscribe(llmevents.TopicLLMCommandCompleteProvider, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				result.Set(tc.result, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(llmevents.TopicLLMCommandCompleteProvider)
			candidate := specificationCandidateFixture()
			got, err := b.CompileDefinitionSpecification(ctx, candidate, nil, nil)
			if err == nil || calls != 1 || got.Specification != nil || candidate.Specification != nil {
				t.Fatalf("invalid completion advanced compilation: calls=%d result=%+v err=%v", calls, got, err)
			}
		})
	}
}

func TestSpecificationCompilerCancelledRequestDoesNotCallProvider(t *testing.T) {
	hub := magicevent.NewHub(16)
	defer hub.Terminate(context.Background())
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.CompileDefinitionSpecification(ctx, specificationCandidateFixture(), nil, nil)
	if err != context.Canceled {
		t.Fatalf("cancelled compilation: %v", err)
	}
}

func specificationCandidateFixture() workspace.DefinitionAdmissionCandidate {
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "generic", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "proof", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Description: "proof declaration"}}}}
	return (workspace.DefinitionAdmissionCandidate{
		SchemaVersion: workspace.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "ws", CandidateID: "candidate", CandidateRevision: 1,
		GoalInput:                 workspace.DefinitionCandidateItem{CandidateItemRef: "goal", Text: "生成可追溯文档", SourceIdentity: "goal@1"},
		DoDInputs:                 []workspace.DefinitionCandidateItem{{CandidateItemRef: "dod", Text: "来源可追溯", SourceIdentity: "dod@1"}},
		WorkspaceTemplateSnapshot: workspace.DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: "hash"},
		RuntimeProfileContracts:   profiles, RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(profiles),
		AdmissionCompilerVersion: workspace.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/workspace",
		ProposedRelationInputs: []workspace.DefinitionProposedRelationInput{{RelationRef: "all", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
}
