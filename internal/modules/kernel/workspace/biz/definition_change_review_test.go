package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestDefinitionChangeCandidateReserveRejectsDriftButSettlesOriginalPermit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "change-reserve")
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	w, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	runOwner := runbiz.New()
	if err := runOwner.Configure(runbiz.Dependencies{OwnerRepositories: owners.repositories.RunOwner, EventHub: owners.hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runOwner.Stop(ctx); err != nil {
			t.Error(err)
		}
	}()
	proposal := definitionChangeProposalForTest(w)
	register := decisionevents.RegisterContractChangeProposalCommand{TicketID: "candidate-ticket", RequestID: "register", Proposal: proposal}
	registrationEvent := func(cmd decisionevents.RegisterContractChangeProposalCommand) decisionevents.RegisterContractChangeProposalResult {
		t.Helper()
		response := owners.hub.Send(magicevent.NewEventWithContext(decisionevents.TopicRegisterContractChangeProposal, definitionSpecificationCommandSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, cmd))
		if response == nil || response.Error() != nil {
			t.Fatalf("登记=%v", response)
		}
		data, resultErr := response.Get()
		result, ok := data.(decisionevents.RegisterContractChangeProposalResult)
		if resultErr != nil || !ok || result.ValidateFor(cmd) != nil {
			t.Fatalf("登记回执=%v", resultErr)
		}
		return result
	}
	registration := registrationEvent(register)
	prepare := workspaceevents.PrepareDefinitionChangeCommand{WorkspaceID: w.ID, RequestID: "prepare", ExpectedWorkspaceRevision: w.Revision, TicketID: registration.TicketID, ProposalRevision: registration.Revision, ProposalHash: registration.Hash}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, DefinitionChange: &prepare}); err != nil {
		t.Fatal(err)
	}
	prepared, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: w.ID, InputHash: prepared.DefinitionChangeCandidate.Record.InputHash, RequestID: "candidate-derive", ProviderBindingHash: "provider-binding", ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(nil)}
	event := definitionSpecificationInternalEvent{WorkspaceID: w.ID, DeriveReserve: &reserve}
	reserved, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, event)
	if err != nil || reserved.Derivation == nil || reserved.Derivation.FreshPermitID == "" {
		t.Fatalf("候选 reserve=%v", err)
	}
	duplicate, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, event)
	if err != nil || duplicate.Derivation.FreshPermitID != "" {
		t.Fatalf("未决 reserve 重授权=%v", err)
	}
	pending, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	original := reserve
	original.InputHash, original.RequestID = w.DefinitionAuthoring.InputHash, "new-original"
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, DeriveReserve: &original}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("候选期间接纳原输入新调用=%v", err)
	}
	register.RequestID, register.ExpectedRevision, register.ExpectedHash = "revise", registration.Revision, registration.Hash
	register.Proposal.WhyChange = "实际提案修订"
	registrationEvent(register)
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, event); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("漂移提案仍可 reserve=%v", err)
	}
	afterRejected, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(pending, afterRejected) {
		t.Fatalf("拒绝改写 unknown=%v", err)
	}
	proposalResult := &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: reserve.InputHash, TemplateSnapshotHash: prepared.DefinitionChangeCandidate.Record.TemplateSnapshotHash, AdditionalItems: []string{}}
	completion := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: w.ID, PermitID: reserved.Derivation.Request.PermitID, Proposal: proposalResult}
	completed, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, DeriveComplete: &completion})
	if err != nil || completed.Derivation.ValidateCompletion(completion) != nil {
		t.Fatalf("漂移后已知结果未结算=%v", err)
	}
	loaded := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	fresh, err := New(owners.hub, nil, Dependencies{OwnerRepositories: loaded.WorkspaceOwner, EventHub: owners.hub})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fresh.Teardown(ctx); err != nil {
			t.Error(err)
		}
	}()
	replay, err := fresh.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, DeriveComplete: &completion})
	if err != nil || !reflect.DeepEqual(completed.Derivation, replay.Derivation) {
		t.Fatalf("fresh owner completion 重放=%v", err)
	}
	conflicting := completion
	conflicting.Proposal, conflicting.FailureCode = nil, "invalid_proposal"
	if _, err := fresh.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, DeriveComplete: &conflicting}); err == nil || err.Error() != "derivation completion conflict" {
		t.Fatalf("同 permit 异结果=%v", err)
	}
	stored, err := loaded.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(stored.DefinitionAuthoring, w.DefinitionAuthoring) || !reflect.DeepEqual(stored.DefinitionAuthoringConfirmation, w.DefinitionAuthoringConfirmation) {
		t.Fatalf("结算改变原定义=%v", err)
	}
}
