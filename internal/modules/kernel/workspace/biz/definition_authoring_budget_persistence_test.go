package workspace

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestAuthoringFileBudgetExhaustionSurvivesRevisions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-budget", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: input.WorkspaceID, TemplateID: "t", DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < workspace.DefinitionDerivationRequestLimit; n++ {
		current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		reserve := workspaceevents.ReserveDefinitionDerivationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "derive"), WorkspaceID: input.WorkspaceID, InputHash: current.DefinitionAuthoring.InputHash, RequestID: fmt.Sprintf("derive-%d", n), ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
		result, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve})
		if err != nil || result.Derivation.FreshPermitID == "" {
			t.Fatalf("预算内 reserve: %v", err)
		}
		complete := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: input.WorkspaceID, PermitID: result.Derivation.FreshPermitID, FailureCode: "provider_failed"}
		if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveComplete: &complete}); err != nil {
			t.Fatal(err)
		}
		current, err = owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		next := current.DefinitionAuthoring.Input.Clone()
		next.Revision++
		next.GoalText = fmt.Sprintf("修订 %d", n)
		save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: fmt.Sprintf("save-%d", n), Input: next, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
		if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, Authoring: &save}); err != nil {
			t.Fatal(err)
		}
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	current, err := owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	revision := current.Revision
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "derive"), WorkspaceID: input.WorkspaceID, InputHash: current.DefinitionAuthoring.InputHash, RequestID: "new-request-after-restart", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
	if transition, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: input.WorkspaceID, DeriveReserve: &reserve}); err == nil {
		t.Fatal("新 revision/requestID 重置了累计预算")
	} else {
		var refusal *workspace.DefinitionAuthoringBudgetRefusalError
		if !errors.As(err, &refusal) || refusal.Budget != "request" || refusal.Stage != workspace.DefinitionAuthoringPreparationDerivation || refusal.RequestID != reserve.RequestID || refusal.Limit != workspace.DefinitionDerivationRequestLimit || transition.Derivation != nil {
			t.Fatalf("预算拒绝没有具体 no-permit 事实: %v", err)
		}
	}
	current, err = owners.repositories.Workspaces.Get(ctx, input.WorkspaceID)
	if err != nil || current.Revision != revision || len(current.DefinitionAuthoring.DerivationRequests) != workspace.DefinitionDerivationRequestLimit || current.ActiveRunID != "" {
		t.Fatal("耗尽预算后仍持久写入或启动")
	}
}

func TestAuthoringFileSpecificationBudgetExhaustionSurvivesRevisions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	id := common.WorkspaceID("ws-specification-budget")
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "t", GoalText: "目标", AcceptanceRequirements: []string{"用户验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	if err := owners.repositories.Workspaces.Create(ctx, &workspace.Workspace{ID: id, TemplateID: "t", DefinitionAuthoring: &record}); err != nil {
		t.Fatal(err)
	}
	kinds := []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility}
	for revision := 0; revision < 8; revision++ {
		current, err := owners.repositories.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		reserve := workspaceevents.ReserveDefinitionDerivationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "derive"), WorkspaceID: id, InputHash: current.DefinitionAuthoring.InputHash, RequestID: fmt.Sprintf("derive-%d", revision), ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
		result, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveReserve: &reserve})
		if err != nil {
			t.Fatal(err)
		}
		permit := result.Derivation.FreshPermitID
		derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: current.DefinitionAuthoring.InputHash, TemplateSnapshotHash: current.DefinitionAuthoring.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
		complete := workspaceevents.CompleteDefinitionDerivationCommand{SuccessAudit: derivationTestAudit(t, &derived), WorkspaceID: id, PermitID: permit, Proposal: &derived}
		if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveComplete: &complete}); err != nil {
			t.Fatal(err)
		}
		proposal := authoringOwnerProposal(t, current.DefinitionAuthoring.Input, template, derived)
		for _, kind := range append([]string{""}, kinds...) {
			for attempt := 0; attempt < workspace.DefinitionAuthoringSpecificationAttemptLimit; attempt++ {
				cmd := workspaceevents.ReserveAuthoringSpecificationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, func() string {
					if kind == "" {
						return "proposal"
					}
					return kind
				}()), WorkspaceID: id, InputHash: current.DefinitionAuthoring.InputHash, RequestID: fmt.Sprintf("spec-%d-%s-%d", revision, kind, attempt), DerivationPermitID: permit, ReviewKind: kind, ProviderBindingHash: "provider"}
				if kind != "" {
					cmd.ProposalHash = proposal.ContentHash()
				}
				reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &cmd})
				if err != nil || reserved.AuthoringSpecification.FreshPermitID == "" {
					t.Fatalf("预算内规格 reserve: %v", err)
				}
				done := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: id, PermitID: reserved.AuthoringSpecification.FreshPermitID, FailureCode: "provider_failed"}
				if kind == "" {
					sealed, err := owners.repositories.Workspaces.Get(ctx, id)
					if err != nil {
						t.Fatal(err)
					}
					done = authoringOwnerCompletion(t, *sealed, done.PermitID, proposal)
				}
				if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecComplete: &done}); err != nil {
					t.Fatal(err)
				}
			}
			denied := workspaceevents.ReserveAuthoringSpecificationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, func() string {
				if kind == "" {
					return "proposal"
				}
				return kind
			}()), WorkspaceID: id, InputHash: current.DefinitionAuthoring.InputHash, RequestID: fmt.Sprintf("third-%d-%s", revision, kind), DerivationPermitID: permit, ReviewKind: kind, ProviderBindingHash: "provider"}
			if kind != "" {
				denied.ProposalHash = proposal.ContentHash()
			}
			if transition, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &denied}); err == nil {
				t.Fatal("阶段 attempt 上限被新 RequestID 绕过")
			} else {
				var refusal *workspace.DefinitionAuthoringBudgetRefusalError
				if !errors.As(err, &refusal) || transition.AuthoringSpecification != nil || refusal.RequestID != denied.RequestID {
					t.Fatalf("阶段 no-permit refusal 不具体: %v", err)
				}
			}
		}
		current, err = owners.repositories.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		next := current.DefinitionAuthoring.Input.Clone()
		next.Revision++
		next.GoalText = fmt.Sprintf("新目标 %d", revision)
		save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: fmt.Sprintf("save-%d", revision), Input: next, ExpectedWorkspaceRevision: current.Revision, ExpectedInputHash: current.DefinitionAuthoring.InputHash}
		if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
			t.Fatal(err)
		}
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	current, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "derive"), WorkspaceID: id, InputHash: current.DefinitionAuthoring.InputHash, RequestID: "derive-after-reopen", ProviderBindingHash: "provider", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
	reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveReserve: &reserve})
	if err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: current.DefinitionAuthoring.InputHash, TemplateSnapshotHash: current.DefinitionAuthoring.TemplateSnapshotHash, AdditionalItems: []string{"补充验收"}}
	done := workspaceevents.CompleteDefinitionDerivationCommand{SuccessAudit: derivationTestAudit(t, &derived), WorkspaceID: id, PermitID: reserved.Derivation.FreshPermitID, Proposal: &derived}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, DeriveComplete: &done}); err != nil {
		t.Fatal(err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	revision := current.Revision
	overflow := workspaceevents.ReserveAuthoringSpecificationCommand{WireContractRef: workspace.DefinitionAuthoringOwnerBound, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBound, "proposal"), WorkspaceID: id, InputHash: current.DefinitionAuthoring.InputHash, RequestID: "new-request-after-reopen", DerivationPermitID: reserved.Derivation.FreshPermitID, ProviderBindingHash: "provider"}
	if transition, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, AuthoringSpecReserve: &overflow}); err == nil {
		t.Fatal("新 revision/permit/requestID 重置规格累计预算")
	} else {
		var refusal *workspace.DefinitionAuthoringBudgetRefusalError
		if !errors.As(err, &refusal) || refusal.Stage != workspace.DefinitionAuthoringPreparationProposal || refusal.RequestID != overflow.RequestID || refusal.Budget != "request" || refusal.Limit != workspace.DefinitionAuthoringSpecificationRequestLimit || transition.AuthoringSpecification != nil {
			t.Fatalf("规格预算拒绝不含 no-permit typed 事实: %v", err)
		}
	}
	current, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || current.Revision != revision || len(current.DefinitionAuthoring.SpecificationRequests) != workspace.DefinitionAuthoringSpecificationRequestLimit || current.ActiveRunID != "" || current.StartupInput != nil {
		t.Fatal("规格预算耗尽后仍写入或准入")
	}
}
