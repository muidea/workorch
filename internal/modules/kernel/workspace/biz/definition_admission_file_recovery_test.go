package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type fileDefinitionAdmissionOwners struct {
	hub          magicevent.Hub
	workspace    *Biz
	decision     *decisionbiz.Biz
	repositories workorchrepo.WorkspaceScopedFileRepositorySet
}

func openFileDefinitionAdmissionOwners(t *testing.T, root string) *fileDefinitionAdmissionOwners {
	t.Helper()
	hub := magicevent.NewHub(32)
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	workspaceOwner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.WorkspaceOwner, EventHub: hub})
	if err != nil {
		hub.Terminate(context.Background())
		t.Fatalf("open Workspace owner: %v", err)
	}
	decisionOwner, err := decisionbiz.New(hub, nil, decisionbiz.Dependencies{OwnerRepositories: repositories.DecisionOwner, EventHub: hub})
	if err != nil {
		workspaceOwner.Teardown(context.Background())
		hub.Terminate(context.Background())
		t.Fatalf("open Decision owner: %v", err)
	}
	if err := decisionOwner.Start(); err != nil {
		decisionOwner.Teardown(context.Background())
		workspaceOwner.Teardown(context.Background())
		hub.Terminate(context.Background())
		t.Fatalf("start Decision owner: %v", err)
	}
	return &fileDefinitionAdmissionOwners{hub: hub, workspace: workspaceOwner, decision: decisionOwner, repositories: repositories}
}

func (o *fileDefinitionAdmissionOwners) close() {
	if o == nil {
		return
	}
	if o.decision != nil {
		o.decision.Teardown(context.Background())
	}
	if o.workspace != nil {
		o.workspace.Teardown(context.Background())
	}
	if o.hub != nil {
		o.hub.Terminate(context.Background())
	}
}

func sendFileDefinitionDecisionCommand[Result any](
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	topic string,
	command any,
) Result {
	t.Helper()
	eventResult := hub.Send(magicevent.NewEventWithContext(
		topic,
		"blocks/workspace",
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if eventResult == nil || eventResult.Error() != nil {
		t.Fatalf("send Decision command %s: %v", topic, eventResult)
	}
	data, err := eventResult.Get()
	if err != nil {
		t.Fatalf("read Decision command %s result: %v", topic, err)
	}
	result, ok := data.(Result)
	if !ok {
		t.Fatalf("Decision command %s result type = %T", topic, data)
	}
	return result
}

func TestDefinitionAdmissionFileBackedCutpointsRecoverThroughClarificationPromotion(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()

	workspaceID := common.WorkspaceID("workspace-1")
	if err := owners.repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	// Cut 1: the unsupported pre-seal candidate is durable before any Decision
	// fact exists. A fresh owner must return the same duplicate outcome without
	// promoting startup input.
	unsupported := testAdmissionCandidate()
	unsupported.ProposedRelationInputs[0].RelationKind = "one_of"
	unsupported = unsupported.WithHash()
	result, err := owners.workspace.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: unsupported})
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired || result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateCreated {
		t.Fatalf("persist unsupported candidate: result=%+v err=%v", result, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: unsupported})
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired || result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate {
		t.Fatalf("recover unsupported candidate: result=%+v err=%v", result, err)
	}

	// Cut 2: the corrected candidate is durable before its blocking Decision
	// ticket exists. Restart cannot synthesize provenance or skip the ticket.
	corrected := unsupported.Clone()
	corrected.CandidateRevision = 2
	corrected.ProposedRelationInputs[0].RelationKind = "all_of"
	corrected = corrected.WithHash()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: corrected, ExpectedCandidateRevision: 1}
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationTicketRequired || result.PriorClarification == nil || result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateCreated {
		t.Fatalf("persist corrected candidate: result=%+v err=%v", result, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationTicketRequired || result.PriorClarification == nil || result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate {
		t.Fatalf("recover corrected candidate before ticket: result=%+v err=%v", result, err)
	}

	requestIdentity, err := result.PriorClarification.DecisionRequestIdentity()
	if err != nil {
		t.Fatal(err)
	}
	opened, err := governanceports.OpenWorkspaceDefinitionClarification(ctx, owners.hub, decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(requestIdentity))
	if err != nil {
		t.Fatalf("persist Decision ticket: %v", err)
	}
	ticketFact := workspaceevents.WorkspaceDefinitionClarificationTicketFact{
		RequestIdentity: requestIdentity,
		TicketBinding:   opened.BindingValue(),
	}
	if err := ticketFact.ValidateFor(*result.PriorClarification); err != nil {
		t.Fatalf("ticket fact: %v", err)
	}

	// Cut 3: Decision has durably confirmed the ticket but Workspace has not
	// promoted the corrected candidate. Fresh provenance lookup authorizes only
	// the pending admission, never immediate eligibility.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	intent.ClarificationTicket = &ticketFact
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired || result.StartupInput == nil || result.ActivationAdmission == nil || result.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
		t.Fatalf("promote pending admission after ticket restart: result=%+v err=%v", result, err)
	}
	pending := result

	// Cut 4: Workspace pending-promotion CAS committed but its response may be
	// lost. Exact replay after reopen is a duplicate with the identical ticket
	// and startup hash.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired || result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || result.StartupInput == nil || result.ActivationAdmission == nil || result.StartupInput.StartupInputHash != pending.StartupInput.StartupInputHash || result.ActivationAdmission.BlockingTicketID != pending.ActivationAdmission.BlockingTicketID {
		t.Fatalf("recover pending promotion: result=%+v err=%v", result, err)
	}

	resolveCommand := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: pending.ActivationAdmission.BlockingTicketID, WorkspaceID: corrected.WorkspaceID,
		OldCandidateID: pending.PriorClarification.Candidate.CandidateID, OldCandidateRevision: pending.PriorClarification.Candidate.CandidateRevision, OldCandidateHash: pending.PriorClarification.Candidate.CandidateHash,
		NewCandidateID: corrected.CandidateID, NewCandidateRevision: corrected.CandidateRevision, NewCandidateHash: corrected.CandidateHash,
		AdmissionResultHash: pending.AdmissionResult.AdmissionResultHash, DefinitionRevision: pending.StartupInput.DefinitionRevision, StartupInputHash: pending.StartupInput.StartupInputHash,
	}
	resolved, err := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, owners.hub, resolveCommand)
	if err != nil {
		t.Fatalf("persist Decision resolution: %v", err)
	}
	resolutionFact := workspaceevents.WorkspaceDefinitionClarificationResolutionFact{
		ResolutionIdentity: definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity(resolveCommand),
		ResolutionBinding:  resolved.BindingValue(),
	}
	if err := resolutionFact.ValidateFor(*pending.PriorClarification, corrected, pending.AdmissionResult, *pending.StartupInput, *pending.ActivationAdmission); err != nil {
		t.Fatalf("resolution fact: %v", err)
	}

	// Cut 5: Decision resolution is durable before Workspace eligibility CAS.
	// Fresh owners verify both repositories, then Workspace alone selects the
	// pending -> eligible transition. Another reopen proves response-loss replay.
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	intent.ClarificationTicket = nil
	intent.ClarificationResolution = &resolutionFact
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentEligible || result.ActivationAdmission == nil || result.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
		t.Fatalf("complete eligible admission after resolution restart: result=%+v err=%v", result, err)
	}
	eligible := result
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	result, err = owners.workspace.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || result.Disposition != workspaceevents.DefinitionAdmissionIntentEligible || result.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || result.ActivationAdmission == nil || result.ActivationAdmission.ClarificationResolutionID != eligible.ActivationAdmission.ClarificationResolutionID {
		t.Fatalf("recover eligible admission: result=%+v err=%v", result, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, workspaceID)
	if err != nil || stored.StartupInput == nil || stored.ActivationAdmission == nil || stored.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible || stored.StartupInput.StartupInputHash != eligible.StartupInput.StartupInputHash {
		t.Fatalf("stored eligible admission: workspace=%+v err=%v", stored, err)
	}

	// The pre-seal Decision aggregate is workspace-scoped: it has no RunID.
	// Verify the resolution transition updated the ticket in that path and that
	// workspace list/get survive a fresh repository instance before exercising
	// the normal Decision-owner purge path.
	decisionCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	tickets, err := owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, workspaceID)
	if err != nil || len(tickets) != 1 || tickets[0].ID != resolveCommand.TicketID || tickets[0].RunID != "" || tickets[0].State != common.DecisionTicketStateResolved || tickets[0].DefinitionClarification == nil || tickets[0].DefinitionClarification.ResolutionID == "" {
		t.Fatalf("workspace-scoped resolved clarification tickets = %+v err=%v", tickets, err)
	}
	if persistedResolution, loadErr := owners.repositories.Resolutions.GetByTicketID(decisionCtx, resolveCommand.TicketID); loadErr != nil || persistedResolution.ID != resolved.ResolutionID {
		t.Fatalf("workspace-scoped clarification resolution = %+v err=%v", persistedResolution, loadErr)
	}
	reopenedBeforePurge := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	if reopenedTickets, listErr := reopenedBeforePurge.Tickets.ListByWorkspaceID(decisionCtx, workspaceID); listErr != nil || len(reopenedTickets) != 1 || reopenedTickets[0].State != common.DecisionTicketStateResolved {
		t.Fatalf("reopened workspace-scoped clarification tickets = %+v err=%v", reopenedTickets, listErr)
	}

	deletionScope := deletioncontract.Scope{
		WorkspaceID:        workspaceID,
		DeletionID:         "delete-workspace-1-decisions",
		DeletionGeneration: 1,
		Mode:               deletioncontract.ModePurge,
		QuiescenceID:       "quiescence-workspace-1-decisions",
	}
	deletionScope.RequestHash = deletioncontract.StableRequestHash(deletionScope)
	sealCommand := decisionevents.SealWorkspaceDecisionDeletionCommand{
		WorkspaceID:           deletionScope.WorkspaceID,
		DeletionID:            deletionScope.DeletionID,
		DeletionGeneration:    deletionScope.DeletionGeneration,
		DataDispositionIntent: deletionScope.Mode,
		QuiescenceID:          deletionScope.QuiescenceID,
		RequestHash:           deletionScope.RequestHash,
	}
	sealed := sendFileDefinitionDecisionCommand[decisionevents.SealWorkspaceDecisionDeletionResult](
		t, ctx, owners.hub, decisionevents.TopicDecisionCommandSealWorkspaceDeletion, sealCommand,
	)
	if err := sealed.ValidateFor(sealCommand); err != nil {
		t.Fatalf("validate Decision deletion seal: %v", err)
	}
	purgeCommand := decisionevents.PurgeWorkspaceDecisionsCommand{
		WorkspaceID:        deletionScope.WorkspaceID,
		DeletionID:         deletionScope.DeletionID,
		DeletionGeneration: deletionScope.DeletionGeneration,
		QuiescenceID:       deletionScope.QuiescenceID,
		RequestHash:        deletionScope.RequestHash,
	}
	purged := sendFileDefinitionDecisionCommand[decisionevents.PurgeWorkspaceDecisionsResult](
		t, ctx, owners.hub, decisionevents.TopicDecisionCommandPurgeWorkspace, purgeCommand,
	)
	if err := purged.ValidateFor(purgeCommand); err != nil {
		t.Fatalf("validate Decision workspace purge: %v", err)
	}
	if remaining, listErr := owners.repositories.Tickets.ListByWorkspaceID(decisionCtx, workspaceID); listErr != nil || len(remaining) != 0 {
		t.Fatalf("tickets after Decision workspace purge = %+v err=%v", remaining, listErr)
	}
	if _, loadErr := owners.repositories.Tickets.Get(decisionCtx, resolveCommand.TicketID); !errors.Is(loadErr, common.ErrNotFound) {
		t.Fatalf("ticket after Decision workspace purge err=%v, want not found", loadErr)
	}
	if _, loadErr := owners.repositories.Resolutions.GetByTicketID(decisionCtx, resolveCommand.TicketID); !errors.Is(loadErr, common.ErrNotFound) {
		t.Fatalf("resolution after Decision workspace purge err=%v, want not found", loadErr)
	}

	owners.close()
	owners = nil
	reopenedAfterPurge := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	if remaining, listErr := reopenedAfterPurge.Tickets.ListByWorkspaceID(decisionCtx, workspaceID); listErr != nil || len(remaining) != 0 {
		t.Fatalf("reopened tickets after Decision workspace purge = %+v err=%v", remaining, listErr)
	}
	if deletion, loadErr := reopenedAfterPurge.DecisionDeletions.Get(ctx, workspaceID); loadErr != nil || deletion.Phase != "purged" || deletion.Revision != 2 {
		t.Fatalf("reopened Decision deletion = %+v err=%v", deletion, loadErr)
	}
}
