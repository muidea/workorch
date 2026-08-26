package workspace

import (
	"context"
	"errors"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	definitionadmission "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefinitionAdmissionInternalEventAcceptsAtMostOneDecisionFact(t *testing.T) {
	candidate := testAdmissionCandidate()
	ticket := workspaceevents.WorkspaceDefinitionClarificationTicketFact{}
	resolution := workspaceevents.WorkspaceDefinitionClarificationResolutionFact{}
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate, ClarificationTicket: &ticket, ClarificationResolution: &resolution}
	_, err := normalizeDefinitionAdmissionInternalEvent(definitionAdmissionInternalEvent{WorkspaceID: candidate.WorkspaceID, Intent: &intent})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("multi-fact admission event err = %v, want invalid argument", err)
	}
}

func TestDefinitionAdmissionOwnerDoesNotResealCallerCandidate(t *testing.T) {
	candidate := testAdmissionCandidate()
	candidate.ResolvedProjectWorkspaceDirectory = "/tmp/tampered"
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate}
	_, err := normalizeDefinitionAdmissionInternalEvent(definitionAdmissionInternalEvent{WorkspaceID: candidate.WorkspaceID, Intent: &intent})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("tampered immutable candidate err=%v, want invalid argument", err)
	}
}

func testAdmissionCandidate() workspacemodel.DefinitionAdmissionCandidate {
	return (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                   workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal", Text: "goal", SourceIdentity: "user"},
		DoDInputs:                   []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "dod", Text: "done", SourceIdentity: "user"}},
		WorkspaceTemplateSnapshot:   workspacemodel.DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: "template-hash"},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:    workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1",
		ProposedRelationInputs: []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "all", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
}

func testClarificationTicketFact(t *testing.T, binding workspaceevents.DefinitionAdmissionClarificationBinding) workspaceevents.WorkspaceDefinitionClarificationTicketFact {
	t.Helper()
	command, err := binding.DecisionRequestIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return workspaceevents.WorkspaceDefinitionClarificationTicketFact{
		RequestIdentity: command,
		TicketBinding: definitioncontract.WorkspaceDefinitionClarificationTicketBinding{
			TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, CandidateID: command.CandidateID, CandidateRevision: command.CandidateRevision,
			Disposition: definitioncontract.DefinitionClarificationCreated, ClarificationRequestBindingHash: command.BindingHash(),
		},
	}
}

func testClarificationResolutionFact(t *testing.T, prior workspaceevents.DefinitionAdmissionClarificationBinding, candidate workspacemodel.DefinitionAdmissionCandidate, admissionResult workspacemodel.DefinitionAdmissionResult, startup workspacemodel.WorkspaceStartupInputContract, activation workspacemodel.WorkspaceActivationAdmission) workspaceevents.WorkspaceDefinitionClarificationResolutionFact {
	t.Helper()
	command := definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity{
		TicketID: activation.BlockingTicketID, WorkspaceID: candidate.WorkspaceID,
		OldCandidateID: prior.Candidate.CandidateID, OldCandidateRevision: prior.Candidate.CandidateRevision, OldCandidateHash: prior.Candidate.CandidateHash,
		NewCandidateID: candidate.CandidateID, NewCandidateRevision: candidate.CandidateRevision, NewCandidateHash: candidate.CandidateHash,
		AdmissionResultHash: admissionResult.AdmissionResultHash, DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash,
	}
	seed := command.AdmissionResultHash + "\x00" + command.StartupInputHash
	resolutionID := definitioncontract.StableDefinitionClarificationResolutionID(command.TicketID, seed)
	return workspaceevents.WorkspaceDefinitionClarificationResolutionFact{
		ResolutionIdentity: command,
		ResolutionBinding: definitioncontract.WorkspaceDefinitionClarificationResolutionBinding{
			TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, ResolutionID: resolutionID,
			ResolutionType: definitioncontract.DefinitionClarificationResolutionCandidateAdjusted, Disposition: definitioncontract.DefinitionClarificationResolved,
			ResolutionBindingHash: command.BindingHash(resolutionID),
		},
	}
}

type definitionAdmissionProvenanceLedger struct {
	mu    sync.Mutex
	facts map[common.TicketID]decisionevents.QueryWorkspaceDefinitionClarificationFactResult
}

func newDefinitionAdmissionProvenanceTestBiz(t *testing.T, workspaces *memoryrepo.WorkspaceRepository) (*Biz, *definitionAdmissionProvenanceLedger) {
	t.Helper()
	hub := magicevent.NewHub(16)
	owner, err := New(hub, nil, Dependencies{
		Workspaces: workspaces, DeletionMaterialCleaner: workspaces, Goals: memoryrepo.NewGoalRepository(), DoDs: memoryrepo.NewDoDRepository(),
		Templates: memoryrepo.NewWorkspaceTemplateRepository(), EventHub: hub,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger := &definitionAdmissionProvenanceLedger{facts: map[common.TicketID]decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}}
	observer := magicevent.NewSimpleObserver(decisionevents.CommandDestination, hub)
	observer.Subscribe(decisionevents.TopicQueryWorkspaceDefinitionClarificationFact, func(event magicevent.Event, result magicevent.Result) {
		query, ok := event.Data().(decisionevents.QueryWorkspaceDefinitionClarificationFactCommand)
		if !ok {
			result.Set(decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, nil)
			return
		}
		ledger.mu.Lock()
		response, found := ledger.facts[query.TicketID]
		ledger.mu.Unlock()
		if !found {
			response = decisionevents.QueryWorkspaceDefinitionClarificationFactResult{Found: false}
		}
		result.Set(response.Normalized(), nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(decisionevents.TopicQueryWorkspaceDefinitionClarificationFact)
		owner.Teardown(context.Background())
		hub.Terminate(context.Background())
	})
	return owner, ledger
}

func (l *definitionAdmissionProvenanceLedger) persistTicket(fact workspaceevents.WorkspaceDefinitionClarificationTicketFact) {
	fact = fact.Normalized()
	record := definitioncontract.WorkspaceDefinitionClarificationTicketRecord{
		RequestIdentity: fact.RequestIdentity,
		OpenBindingHash: fact.TicketBinding.ClarificationRequestBindingHash,
	}
	l.mu.Lock()
	l.facts[fact.RequestIdentity.TicketID] = decisionevents.QueryWorkspaceDefinitionClarificationFactResult{
		Found: true, Ticket: &record,
	}
	l.mu.Unlock()
}

func (l *definitionAdmissionProvenanceLedger) persistResolution(fact workspaceevents.WorkspaceDefinitionClarificationResolutionFact) {
	fact = fact.Normalized()
	l.mu.Lock()
	response := l.facts[fact.ResolutionIdentity.TicketID]
	record := definitioncontract.WorkspaceDefinitionClarificationResolutionRecord{
		ResolutionIdentity:    fact.ResolutionIdentity,
		ResolutionID:          fact.ResolutionBinding.ResolutionID,
		ResolutionType:        fact.ResolutionBinding.ResolutionType,
		ResolutionBindingHash: fact.ResolutionBinding.ResolutionBindingHash,
	}
	response.Found = true
	response.Resolution = &record
	l.facts[fact.ResolutionIdentity.TicketID] = response
	l.mu.Unlock()
}

func TestDefinitionAdmissionIntentAutomaticallyPromotesEligibleCandidate(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	candidate := testAdmissionCandidate()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate}
	created, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || created.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateCreated || created.Disposition != workspaceevents.DefinitionAdmissionIntentEligible || created.ActivationAdmission == nil || created.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
		t.Fatalf("created intent = %+v err=%v", created, err)
	}
	duplicate, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || duplicate.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || duplicate.Disposition != workspaceevents.DefinitionAdmissionIntentEligible {
		t.Fatalf("duplicate intent = %+v err=%v", duplicate, err)
	}
}

func TestDefinitionAdmissionWaitingDeletionRejectsBeforeWorkspaceCAS(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	current := &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}
	waiting := workspaceWithWaitingDeletionForTest(t, current, "delete-definition")
	if err := repo.Create(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	biz, _ := newDefinitionAdmissionProvenanceTestBiz(t, repo)
	before, err := repo.Get(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: testAdmissionCandidate()}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("definition admission err=%v, want conflict", err)
	}
	after, err := repo.Get(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.Deletion.Revision != before.Deletion.Revision || len(after.DefinitionAdmissionCandidates) != 0 {
		t.Fatalf("definition admission mutated waiting deletion: before=%+v after=%+v", before, after)
	}
}

func TestDefinitionAdmissionIntentKeepsUnsupportedAlternativePreseal(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	candidate := testAdmissionCandidate()
	candidate.ProposedRelationInputs[0].RelationKind = "one_of"
	candidate = candidate.WithHash()
	result, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: candidate})
	if err != nil {
		t.Fatalf("submit unsupported candidate: %v", err)
	}
	if result.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired || result.AdmissionResult.ViolationCode != workspacemodel.DefinitionAdmissionOneOfUnsupported || result.StartupInput != nil || result.ActivationAdmission != nil {
		t.Fatalf("owner admission = %+v", result)
	}
	stored, err := repo.Get(ctx, candidate.WorkspaceID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("unsupported candidate escaped preseal: workspace=%+v err=%v", stored, err)
	}
}

func TestDefinitionAdmissionIntentAloneSelectsTicketPromotionAndResolution(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz, provenance := newDefinitionAdmissionProvenanceTestBiz(t, repo)
	invalid := testAdmissionCandidate()
	invalid.ProposedRelationInputs[0].RelationKind = "one_of"
	invalid = invalid.WithHash()
	invalidResult, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: invalid})
	if err != nil || invalidResult.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired {
		t.Fatalf("invalid intent = %+v err=%v", invalidResult, err)
	}
	invalidDuplicate, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: invalid})
	if err != nil || invalidDuplicate.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || invalidDuplicate.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationRequired {
		t.Fatalf("invalid recovery replay = %+v err=%v", invalidDuplicate, err)
	}

	corrected := invalid.Clone()
	corrected.CandidateRevision = 2
	corrected.ProposedRelationInputs[0].RelationKind = "all_of"
	corrected = corrected.WithHash()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: corrected, ExpectedCandidateRevision: 1}
	ticketRequired, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || ticketRequired.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationTicketRequired || ticketRequired.PriorClarification == nil || ticketRequired.PriorClarification.Candidate.CandidateHash != invalid.CandidateHash {
		t.Fatalf("ticket-required intent = %+v err=%v", ticketRequired, err)
	}
	ticket := testClarificationTicketFact(t, *ticketRequired.PriorClarification)
	// A lost open response followed by exact Decision replay returns duplicate;
	// persisted provenance, not response disposition, authorizes the fact.
	ticket.TicketBinding.Disposition = definitioncontract.DefinitionClarificationDuplicate
	provenance.persistTicket(ticket)
	intent.ClarificationTicket = &ticket
	pending, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || pending.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired || pending.ActivationAdmission == nil || pending.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
		t.Fatalf("pending intent = %+v err=%v", pending, err)
	}
	pendingReplay, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || pendingReplay.CandidateDisposition != workspaceevents.DefinitionAdmissionCandidateDuplicate || pendingReplay.Disposition != workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired || pendingReplay.ActivationAdmission == nil || pendingReplay.ActivationAdmission.BlockingTicketID != pending.ActivationAdmission.BlockingTicketID {
		t.Fatalf("pending recovery replay = %+v err=%v", pendingReplay, err)
	}
	resolution := testClarificationResolutionFact(t, *pending.PriorClarification, corrected, pending.AdmissionResult, *pending.StartupInput, *pending.ActivationAdmission)
	resolution.ResolutionBinding.Disposition = definitioncontract.DefinitionClarificationDuplicate
	provenance.persistResolution(resolution)
	intent.ClarificationTicket = nil
	intent.ClarificationResolution = &resolution
	eligible, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || eligible.Disposition != workspaceevents.DefinitionAdmissionIntentEligible || eligible.ActivationAdmission == nil || eligible.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible {
		t.Fatalf("resolved intent = %+v err=%v", eligible, err)
	}
	duplicate, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || duplicate.Disposition != workspaceevents.DefinitionAdmissionIntentEligible {
		t.Fatalf("duplicate resolution intent = %+v err=%v", duplicate, err)
	}
	stored, err := repo.Get(ctx, corrected.WorkspaceID)
	if err != nil || stored.ActivationAdmission == nil || stored.ActivationAdmission.ClarificationResolutionID != resolution.ResolutionBinding.ResolutionID {
		t.Fatalf("stored admission = %+v err=%v", stored, err)
	}
}

func TestDefinitionAdmissionRejectsSelfConsistentUnpersistedDecisionTicketWithoutPromotion(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz, _ := newDefinitionAdmissionProvenanceTestBiz(t, repo)
	invalid := testAdmissionCandidate()
	invalid.ProposedRelationInputs[0].RelationKind = "one_of"
	invalid = invalid.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: invalid}); err != nil {
		t.Fatal(err)
	}
	corrected := invalid.Clone()
	corrected.CandidateRevision = 2
	corrected.ProposedRelationInputs[0].RelationKind = "all_of"
	corrected = corrected.WithHash()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: corrected, ExpectedCandidateRevision: 1}
	required, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	ticket := testClarificationTicketFact(t, *required.PriorClarification)
	// The identity and receipt are fully self-consistent, but no Decision-owned
	// persisted ticket exists in the provenance query.
	intent.ClarificationTicket = &ticket
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, intent); !errors.Is(err, common.ErrInvalidArgument) && !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged ticket err=%v, want fail closed", err)
	}
	stored, err := repo.Get(ctx, corrected.WorkspaceID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || len(stored.DefinitionAdmissionCandidates) != 2 {
		t.Fatalf("forged ticket promoted candidate: workspace=%+v err=%v", stored, err)
	}
}

func TestDefinitionAdmissionRejectsSelfConsistentUnpersistedDecisionResolutionWithoutStateChange(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz, provenance := newDefinitionAdmissionProvenanceTestBiz(t, repo)
	invalid := testAdmissionCandidate()
	invalid.ProposedRelationInputs[0].RelationKind = "one_of"
	invalid = invalid.WithHash()
	invalidResult, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: invalid})
	if err != nil {
		t.Fatal(err)
	}
	corrected := invalid.Clone()
	corrected.CandidateRevision = 2
	corrected.ProposedRelationInputs[0].RelationKind = "all_of"
	corrected = corrected.WithHash()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: corrected, ExpectedCandidateRevision: 1}
	required, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	ticket := testClarificationTicketFact(t, *required.PriorClarification)
	provenance.persistTicket(ticket)
	intent.ClarificationTicket = &ticket
	pending, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	resolution := testClarificationResolutionFact(t, *pending.PriorClarification, corrected, pending.AdmissionResult, *pending.StartupInput, *pending.ActivationAdmission)
	// The result is cryptographically self-consistent, but Decision has not
	// persisted this resolution record, so provenance remains ticket-only.
	intent.ClarificationTicket = nil
	intent.ClarificationResolution = &resolution
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, intent); !errors.Is(err, common.ErrInvalidArgument) && !errors.Is(err, common.ErrConflict) {
		t.Fatalf("forged resolution err=%v, want fail closed", err)
	}
	stored, err := repo.Get(ctx, corrected.WorkspaceID)
	if err != nil || stored.ActivationAdmission == nil || stored.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution || stored.ActivationAdmission.ClarificationResolutionID != "" {
		t.Fatalf("forged resolution changed owner state: workspace=%+v err=%v invalid=%+v", stored, err, invalidResult)
	}
}

func TestDefinitionAdmissionRejectsTicketOnlyIntentAfterDecisionResolvedWithoutCAS(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz, provenance := newDefinitionAdmissionProvenanceTestBiz(t, repo)
	invalid := testAdmissionCandidate()
	invalid.ProposedRelationInputs[0].RelationKind = "one_of"
	invalid = invalid.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: invalid}); err != nil {
		t.Fatal(err)
	}
	corrected := invalid.Clone()
	corrected.CandidateRevision = 2
	corrected.ProposedRelationInputs[0].RelationKind = "all_of"
	corrected = corrected.WithHash()
	intent := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: corrected, ExpectedCandidateRevision: 1}
	required, err := biz.submitDefinitionAdmissionIntent(ctx, intent)
	if err != nil || required.PriorClarification == nil {
		t.Fatalf("ticket-required intent=%+v err=%v", required, err)
	}
	ticket := testClarificationTicketFact(t, *required.PriorClarification)
	provenance.persistTicket(ticket)
	admission, err := definitionadmission.Compile(corrected)
	if err != nil {
		t.Fatal(err)
	}
	startup := startupInputFromCandidate(corrected, admission)
	resolution := testClarificationResolutionFact(t, *required.PriorClarification, corrected, admission, startup, workspacemodel.WorkspaceActivationAdmission{BlockingTicketID: ticket.TicketBinding.TicketID})
	provenance.persistResolution(resolution)
	before, err := repo.Get(ctx, corrected.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	intent.ClarificationTicket = &ticket
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, intent); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ticket-only intent after Decision resolution err=%v, want conflict", err)
	}
	after, err := repo.Get(ctx, corrected.WorkspaceID)
	if err != nil || after.Revision != before.Revision || after.StartupInput != nil || after.ActivationAdmission != nil {
		t.Fatalf("rejected ticket-only intent mutated Workspace before=%+v after=%+v err=%v", before, after, err)
	}
}

func TestDefinitionAdmissionRejectsNewCandidateIDLineageBypass(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	invalid := testAdmissionCandidate()
	invalid.ProposedRelationInputs[0].RelationKind = "one_of"
	invalid = invalid.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: invalid}); err != nil {
		t.Fatal(err)
	}
	bypass := invalid.Clone()
	bypass.CandidateID = "zz-new-lineage"
	bypass.CandidateRevision = 1
	bypass.ProposedRelationInputs[0].RelationKind = "all_of"
	bypass = bypass.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: bypass}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("new CandidateID bypass err=%v, want conflict", err)
	}
	stored, err := repo.Get(ctx, invalid.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("new CandidateID bypass changed state: workspace=%+v err=%v", stored, err)
	}
}

func TestDefinitionAdmissionRejectsStaleLineageCAS(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	first := testAdmissionCandidate()
	first.ProposedRelationInputs[0].RelationKind = "one_of"
	first = first.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: first}); err != nil {
		t.Fatal(err)
	}
	next := first.Clone()
	next.CandidateRevision = 2
	next.ProposedRelationInputs[0].RelationKind = "all_of"
	next = next.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: next, ExpectedCandidateRevision: 0}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale lineage CAS err=%v, want conflict", err)
	}
	stored, err := repo.Get(ctx, first.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("stale lineage CAS changed state: workspace=%+v err=%v", stored, err)
	}
}

func TestDefinitionAdmissionRejectsGappedPersistedLineage(t *testing.T) {
	ctx := context.Background()
	first := testAdmissionCandidate()
	third := first.Clone()
	third.CandidateRevision = 3
	third.GoalInput.Text = "third"
	third = third.WithHash()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: first.WorkspaceID, State: common.WorkspaceStateActive, DefinitionAdmissionCandidates: []workspacemodel.DefinitionAdmissionCandidate{first, third}}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	fourth := third.Clone()
	fourth.CandidateRevision = 4
	fourth.GoalInput.Text = "fourth"
	fourth = fourth.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: fourth, ExpectedCandidateRevision: 3}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("gapped persisted lineage err=%v, want conflict", err)
	}
	stored, err := repo.Get(ctx, first.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 2 {
		t.Fatalf("gapped persisted lineage was rewritten: workspace=%+v err=%v", stored, err)
	}
}

func TestDefinitionAdmissionDuplicateRejectsLaterCorruptedPersistedLineage(t *testing.T) {
	ctx := context.Background()
	first := testAdmissionCandidate()
	for _, test := range []struct {
		name string
		late workspacemodel.DefinitionAdmissionCandidate
	}{
		{name: "gapped", late: func() workspacemodel.DefinitionAdmissionCandidate {
			value := first.Clone()
			value.CandidateRevision = 3
			value.GoalInput.Text = "third"
			return value.WithHash()
		}()},
		{name: "cross lineage", late: func() workspacemodel.DefinitionAdmissionCandidate {
			value := first.Clone()
			value.CandidateID = "forged-lineage"
			value.CandidateRevision = 2
			return value.WithHash()
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := memoryrepo.NewWorkspaceRepository()
			if err := repo.Create(ctx, &workspacemodel.Workspace{
				ID: first.WorkspaceID, State: common.WorkspaceStateActive,
				DefinitionAdmissionCandidates: []workspacemodel.DefinitionAdmissionCandidate{first, test.late},
			}); err != nil {
				t.Fatal(err)
			}
			biz := &Biz{workspaces: repo}
			if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: first}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("duplicate against corrupted lineage err=%v, want conflict", err)
			}
		})
	}
}

func TestDefinitionAdmissionRejectsNextRevisionOutsideCurrentStartupLineage(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewWorkspaceRepository()
	if err := repo.Create(ctx, &workspacemodel.Workspace{ID: "workspace-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	biz := &Biz{workspaces: repo}
	first := testAdmissionCandidate()
	eligible, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: first})
	if err != nil || eligible.StartupInput == nil {
		t.Fatalf("initial admission = %+v err=%v", eligible, err)
	}
	next := first.Clone()
	next.CandidateRevision = 2
	next.GoalInput.Text = "changed goal"
	// Deliberately omit the current DefinitionRevision/StartupInputHash base.
	next = next.WithHash()
	if _, err := biz.submitDefinitionAdmissionIntent(ctx, workspaceevents.DefinitionAdmissionIntentCommand{Candidate: next, ExpectedCandidateRevision: 1}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("detached startup lineage err=%v, want conflict", err)
	}
	stored, err := repo.Get(ctx, first.WorkspaceID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.StartupInput == nil || stored.StartupInput.StartupInputHash != eligible.StartupInput.StartupInputHash {
		t.Fatalf("detached startup lineage changed state: workspace=%+v err=%v", stored, err)
	}
}

func TestOutstandingDefinitionClarificationIgnoresClosedEligibleBaseline(t *testing.T) {
	workspace := &workspacemodel.Workspace{ID: "workspace-1"}
	invalid := testAdmissionCandidate()
	invalid.ProposedRelationInputs[0].RelationKind = "one_of"
	invalid = invalid.WithHash()
	eligible := invalid.Clone()
	eligible.CandidateRevision = 2
	eligible.ProposedRelationInputs[0].RelationKind = "all_of"
	eligible = eligible.WithHash()
	admission, err := definitionadmission.Compile(eligible)
	if err != nil {
		t.Fatal(err)
	}
	startup := startupInputFromCandidate(eligible, admission)
	workspace.DefinitionAdmissionCandidates = []workspacemodel.DefinitionAdmissionCandidate{invalid, eligible}
	workspace.StartupInput = &startup
	workspace.ActivationAdmission = &workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: workspace.ID, CandidateID: eligible.CandidateID, CandidateRevision: eligible.CandidateRevision, CandidateHash: eligible.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	current := eligible.Clone()
	current.CandidateRevision = 3
	current.BaseDefinitionRevision = startup.DefinitionRevision
	current.BaseStartupInputHash = startup.StartupInputHash
	current = current.WithHash()
	if binding := outstandingDefinitionClarification(workspace, current); binding != nil {
		t.Fatalf("closed invalid baseline was reopened: %+v", binding)
	}
}
