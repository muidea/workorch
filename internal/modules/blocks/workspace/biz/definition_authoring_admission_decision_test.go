package workspace

import (
	"context"
	"reflect"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringConfirmAndAdmitReplaysAfterDecisionFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "ws-authoring-composed-decision-cut")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	owners.decision.Teardown(ctx)
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation); err == nil {
		t.Fatal("missing Decision was treated as confirmed admission")
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("Decision failure changed blocked owner authority: %+v %v", stored, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	result, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation)
	if err != nil || result.BlockingTicketID == "" || result.WorkspaceRevision != stored.Revision {
		t.Fatalf("failed Decision step did not recover: %+v %v", result, err)
	}
	tickets, err := owners.repositories.Tickets.ListByWorkspaceID(common.ContextWithRuntimeWorkspaceID(ctx, confirmation.WorkspaceID), confirmation.WorkspaceID)
	if err != nil || len(tickets) != 1 || tickets[0].ID != result.BlockingTicketID {
		t.Fatalf("Decision recovery did not create one ticket: %+v %v", tickets, err)
	}
}

func TestAuthoringConfirmAndAdmitReplaysEligibleAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	confirmation := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-authoring-eligible-replay", true)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	first, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation)
	if err != nil || first.ActivationAdmission == nil || first.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible || first.StartupInput == nil {
		t.Fatalf("eligible admission did not seal startup: %+v %v", first, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	second, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("eligible response-loss replay drifted: %+v %+v %v", first, second, err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil || current.Revision != first.WorkspaceRevision || current.ActiveRunID != "" {
		t.Fatalf("replay rewrote Workspace or started Run: %+v %v", current, err)
	}
}

func TestAuthoringConfirmAndAdmitReplaysBlockedOwnerAndDecisionCuts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "ws-authoring-composed-replay")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	first, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation)
	if err != nil || first.BlockingTicketID == "" {
		t.Fatalf("blocked admission did not open Decision ticket: %+v %v", first, err)
	}
	view, err := application.ReadDefinitionAuthoringView(ctx, confirmation.WorkspaceID)
	if err != nil || view.State != "clarification_required" || view.BlockingTicketID != string(first.BlockingTicketID) || view.CandidateHash != first.Candidate.CandidateHash || view.Proposal == nil || len(view.Proposal.Reviews) != 6 {
		t.Fatalf("management view did not project persisted blocker and reviews: %+v %v", view, err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil || len(current.DefinitionAdmissionCandidates) != 1 || current.StartupInput != nil || current.ActiveRunID != "" {
		t.Fatalf("blocked confirmation did not persist: %+v %v", current, err)
	}
	firstRevision := current.Revision
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	replayed, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation)
	if err != nil || replayed.BlockingTicketID == "" || replayed.WorkspaceRevision != firstRevision {
		t.Fatalf("blocked response-loss retry did not converge: %+v %v", replayed, err)
	}
	again, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, confirmation)
	if err != nil || !reflect.DeepEqual(replayed, again) {
		t.Fatalf("duplicate confirmation drifted: %+v %+v %v", replayed, again, err)
	}
	changed := confirmation
	changed.ProposalHash = "stale-proposal"
	if _, err := application.ConfirmAndAdmitDefinitionAuthoring(ctx, changed); err == nil {
		t.Fatal("stale confirmation reused blocked admission")
	}
	current, err = owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil || current.Revision != firstRevision || current.ActiveRunID != "" {
		t.Fatalf("replay mutated Workspace: %+v %v", current, err)
	}
}

func TestAuthoringBlockedAdmissionDecisionFailureReplaysAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "ws-authoring-decision-unavailable")
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	// Simulate Decision transport unavailable immediately after Workspace's CAS.
	owners.decision.Teardown(ctx)
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	if _, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate); err == nil {
		t.Fatal("Decision failure was treated as successful admission")
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("Decision failure lost blocked candidate or created startup: %+v %v", stored, err)
	}
	decisionCtx := common.ContextWithRuntimeWorkspaceID(ctx, current.ID)
	if tickets, err := owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, current.ID); err != nil || len(tickets) != 0 {
		t.Fatalf("unavailable Decision unexpectedly opened ticket: %+v %v", tickets, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	result, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate)
	if err != nil || result.BlockingTicketID == "" || result.StartupInput != nil || result.ActivationAdmission != nil {
		t.Fatalf("reopened Decision did not reconcile original candidate: %+v %v", result, err)
	}
	tickets, err := owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, current.ID)
	if err != nil || len(tickets) != 1 || tickets[0].ID != result.BlockingTicketID {
		t.Fatalf("reopened reconciliation did not persist exact ticket: %+v %v", tickets, err)
	}
	stale := candidate.Clone()
	stale.CandidateID = "unreviewed-successor"
	stale = stale.WithHash()
	if _, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, stale); err == nil {
		t.Fatal("unreviewed successor bypassed blocked candidate")
	}
	stored, err = owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
		t.Fatalf("stale successor mutated authority: %+v %v", stored, err)
	}
}

func TestAuthoringBlockedAdmissionDecisionTicketRecoversEveryOwnerCut(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "ws-authoring-decision-recovery")
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	command := workspaceevents.AdmitDefinitionAuthoringCommand{
		Candidate: candidate, ExpectedWorkspaceRevision: current.Revision,
		ExpectedConfigurationHash: workspacemodel.WorkspaceConfigurationHash(current),
	}
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	first, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || first.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired {
		t.Fatalf("Workspace did not persist blocked candidate: %+v %v", first, err)
	}
	decisionCtx := common.ContextWithRuntimeWorkspaceID(ctx, current.ID)
	if tickets, err := owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, current.ID); err != nil || len(tickets) != 0 {
		t.Fatalf("Decision ticket was opened before the Application step: %+v %v", tickets, err)
	}
	query := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: current.ID}
	view, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || view.AdmissionCandidate == nil || view.AdmissionCandidate.CandidateHash != candidate.CandidateHash || view.AdmissionResult == nil || view.ActivationAdmission != nil {
		t.Fatalf("blocked candidate cannot be recovered without Decision: %+v %v", view, err)
	}

	// Workspace CAS committed but Decision did not run; resume using a fresh Hub
	// and both actual owners. The original Workspace revision is intentionally stale.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	opened, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate)
	if err != nil || opened.BlockingTicketID == "" || opened.StartupInput != nil || opened.ActivationAdmission != nil || opened.WorkspaceRevision != first.WorkspaceRevision {
		t.Fatalf("Application did not reconcile persisted candidate into one ticket: %+v %v", opened, err)
	}
	view, err = owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || view.AdmissionCandidate == nil || view.AdmissionCandidate.CandidateHash != candidate.CandidateHash || view.ActivationAdmission != nil {
		t.Fatalf("Decision coordination changed Workspace authority: %+v %v", view, err)
	}
	tickets, err := owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, current.ID)
	if err != nil || len(tickets) != 1 || tickets[0].ID != opened.BlockingTicketID || tickets[0].RunID != "" {
		t.Fatalf("Decision ticket is not one workspace-scoped owner fact: %+v %v", tickets, err)
	}

	// Decision CAS committed, but the response may be lost. Reopen again and
	// replay the same exact candidate without duplicating ticket or creating Run.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = appbiz.New()
	application.Base = basebiz.New(appbiz.ModuleID, owners.hub, nil)
	replayed, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate)
	if err != nil || !reflect.DeepEqual(opened, replayed) {
		t.Fatalf("response-loss replay changed admission or ticket: %+v %+v %v", opened, replayed, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.Revision != first.WorkspaceRevision || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
		t.Fatalf("blocked candidate became eligible or created a Run: %+v %v", stored, err)
	}
	tickets, err = owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, current.ID)
	if err != nil || len(tickets) != 1 || tickets[0].ID != opened.BlockingTicketID {
		t.Fatalf("response-loss replay duplicated Decision ticket: %+v %v", tickets, err)
	}

	// An alternate, self-consistent candidate is not an idempotent replay of
	// the original six reviews, even when its hashes were recomputed.
	changed := candidate.Clone()
	changed.GoalInput.Text = "未审查的修改"
	changed = changed.WithHash()
	if _, err := application.AdmitConfirmedDefinitionAuthoring(ctx, current, changed); err == nil {
		t.Fatal("different candidate silently replaced blocked review")
	}
	stale := *current
	stale.Revision = first.WorkspaceRevision + 2
	if _, err := application.AdmitConfirmedDefinitionAuthoring(ctx, &stale, candidate); err == nil {
		t.Fatal("changed expected revision replayed without exact fence")
	}
	tickets, err = owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, current.ID)
	if err != nil || len(tickets) != 1 {
		t.Fatalf("stale replay wrote another Decision ticket: %+v %v", tickets, err)
	}
}
