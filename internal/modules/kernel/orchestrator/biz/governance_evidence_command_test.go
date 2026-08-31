package orchestrator

// Owner Biz evidence tests.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspacePreSealDefinitionResolutionSkipsRunScopedEvidenceAndReplaysAfterResponseLoss(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })

	// Install the real Orchestrator evidence handler. It intentionally rejects
	// an empty RunID; a pre-seal Decision resolution must never call it.
	orchestratorOwner := New()
	if err := orchestratorOwner.Configure(Dependencies{
		Evidence:          memoryrepo.NewOrchestratorOwnerRepositorySet(),
		CommandHub:        hub,
		BackgroundRoutine: &recordingBackgroundRoutine{},
	}); err != nil {
		t.Fatalf("configure Orchestrator owner: %v", err)
	}
	var evidenceCalls atomic.Int64
	evidenceObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	evidenceObserver.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, func(event magicevent.Event, result magicevent.Result) {
		evidenceCalls.Add(1)
		orchestratorOwner.handleDecisionEvidenceCommand(event, result)
	})
	t.Cleanup(func() {
		evidenceObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence)
		_ = orchestratorOwner.Stop(ctx)
	})

	decisionRepositories := memoryrepo.NewDecisionOwnerRepositorySet()
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionbiz.Dependencies{
		OwnerRepositories: decisionRepositories.OwnerRepositories(),
		EventHub:          hub,
	}); err != nil {
		t.Fatalf("configure Decision owner: %v", err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatalf("start Decision owner: %v", err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	open := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{
		SchemaVersion:             decisionevents.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:               "ws-pre-seal-resolution",
		CandidateID:               "definition-candidate",
		CandidateRevision:         1,
		CandidateHash:             "candidate-hash-1",
		ViolationCode:             "definition_one_of_unsupported",
		OffendingRefs:             []string{"dod:item-1"},
		CanonicalSignatureVersion: "v1",
		CanonicalSignature:        "one-of:dod:item-1",
		RequiredActionKind:        decisionevents.DefinitionClarificationActionRewriteAllOf,
	}
	open.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", open.CanonicalSignatureVersion, open.CanonicalSignature)
	open.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(open)
	openEvent := magicevent.NewEventWithContext(
		decisionevents.TopicOpenWorkspaceDefinitionClarification,
		"application/workorchd",
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		open,
	)
	openResult := hub.Send(openEvent)
	if openResult == nil || openResult.Error() != nil {
		var resultErr error
		if openResult != nil {
			resultErr = openResult.Error()
		}
		t.Fatalf("open pre-seal clarification: %v", resultErr)
	}
	openData, resultErr := openResult.Get()
	if resultErr != nil {
		t.Fatalf("read open result: %v", resultErr)
	}
	opened, ok := openData.(decisionevents.OpenWorkspaceDefinitionClarificationRequestResult)
	if !ok || opened.ValidateFor(open) != nil || opened.Disposition != decisionevents.DefinitionClarificationCreated {
		t.Fatalf("open result = %#v", openData)
	}

	resolve := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID:             open.TicketID,
		WorkspaceID:          open.WorkspaceID,
		OldCandidateID:       open.CandidateID,
		OldCandidateRevision: open.CandidateRevision,
		OldCandidateHash:     open.CandidateHash,
		NewCandidateID:       open.CandidateID,
		NewCandidateRevision: 2,
		NewCandidateHash:     "candidate-hash-2",
		AdmissionResultHash:  "admission-hash-2",
		DefinitionRevision:   "definition-revision-2",
		StartupInputHash:     "startup-input-hash-2",
	}
	resolveEvent := func() magicevent.Event {
		return magicevent.NewEventWithContext(
			decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission,
			"application/workorchd",
			decisionevents.CommandDestination,
			magicevent.NewHeader(),
			ctx,
			resolve,
		)
	}

	// Treat the first successful response as lost by deliberately not decoding
	// its payload. The exact command replay must join the already persisted
	// Decision aggregate and return a stable duplicate acknowledgement.
	first := hub.Send(resolveEvent())
	if first == nil || first.Error() != nil {
		var firstErr error
		if first != nil {
			firstErr = first.Error()
		}
		t.Fatalf("first pre-seal resolution: %v", firstErr)
	}
	second := hub.Send(resolveEvent())
	if second == nil || second.Error() != nil {
		var secondErr error
		if second != nil {
			secondErr = second.Error()
		}
		t.Fatalf("replay pre-seal resolution: %v", secondErr)
	}
	secondData, resultErr := second.Get()
	if resultErr != nil {
		t.Fatalf("read replay result: %v", resultErr)
	}
	replayed, ok := secondData.(decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult)
	if !ok || replayed.ValidateFor(resolve) != nil || replayed.Disposition != decisionevents.DefinitionClarificationDuplicate {
		t.Fatalf("replay result = %#v", secondData)
	}
	if got := evidenceCalls.Load(); got != 0 {
		t.Fatalf("pre-seal resolution called run-scoped Orchestrator evidence %d times", got)
	}

	query := decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{WorkspaceID: open.WorkspaceID, TicketID: open.TicketID}
	queryResult := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicQueryWorkspaceDefinitionClarificationFact,
		"blocks/workspace",
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		query,
	))
	if queryResult == nil || queryResult.Error() != nil {
		var queryErr error
		if queryResult != nil {
			queryErr = queryResult.Error()
		}
		t.Fatalf("query persisted pre-seal resolution: %v", queryErr)
	}
	queryData, resultErr := queryResult.Get()
	if resultErr != nil {
		t.Fatalf("read persisted clarification: %v", resultErr)
	}
	fact, ok := queryData.(decisionevents.QueryWorkspaceDefinitionClarificationFactResult)
	if !ok || fact.ValidateFor(query) != nil || !fact.Found || fact.Resolution == nil || fact.Resolution.ResolutionID != replayed.ResolutionID {
		t.Fatalf("persisted clarification fact = %#v", queryData)
	}
}

func TestDecisionEvidenceCommandPersistsThroughOrchestratorConsumer(t *testing.T) {
	ctx := context.Background()
	deps := moduleContractDependencies()
	evidenceRepo := deps.Evidence.EvidenceRecords()
	installEmptyRecoveryInventory(t, deps.CommandHub)
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("configure orchestrator: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		deps.CommandHub.Terminate(ctx)
	})

	command := orchestratorevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID: "evidence-decision-pairwise",
		WorkspaceID:      "ws-pairwise-evidence",
		RunID:            "run-pairwise-evidence",
		StepID:           "step-pairwise-evidence",
		TicketID:         "ticket-pairwise-evidence",
		ResolutionID:     "resolution-pairwise-evidence",
		ResolutionType:   decisioncommon.ResolutionTypeProvideMissingInfo,
		DecisionKind:     "operator_input",
		Reason:           "need operator decision",
		Question:         "which path should continue",
		ResolutionPayload: workorcheventpayload.New(
			workorcheventpayload.StringField("operator", "tester"),
			workorcheventpayload.BoolField("retry", true),
			workorcheventpayload.StringArrayField("targets", []string{"step-a", "step-b"}),
		).Fields,
		ContinuationAction: "resume",
		ResumeRunID:        "run-pairwise-evidence",
	}
	if _, err := module.recordDecisionEvidence(ctx, command); err != nil {
		t.Fatalf("record decision evidence: %v", err)
	}
	if _, err := module.recordDecisionEvidence(ctx, command); err != nil {
		t.Fatalf("record exact duplicate decision evidence: %v", err)
	}
	changed := command
	changed.Reason = "different immutable reason"
	if _, err := module.recordDecisionEvidence(ctx, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed duplicate decision evidence err = %v, want conflict", err)
	}

	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.RunID)
	if err != nil {
		t.Fatalf("list decision evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("decision evidence count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != runtimemodel.EvidenceKindDecision || record.Status != runtimemodel.EvidenceStatusAccepted || record.SubjectID != string(command.TicketID) {
		t.Fatalf("decision evidence = %+v, want accepted decision record", record)
	}
	payload, ok := record.Metadata.Object("resolutionPayload")
	operator, _ := payload.String("operator")
	retry, _ := payload.Bool("retry")
	if !ok || operator != "tester" || !retry {
		t.Fatalf("decision evidence payload = %#v, want concrete fields restored", payload)
	}
	reason, _ := record.Metadata.String("reason")
	if reason != command.Reason {
		t.Fatalf("decision evidence reason = %q, want immutable %q", reason, command.Reason)
	}
}

func TestApprovalEvidenceCommandPersistsThroughOrchestratorConsumer(t *testing.T) {
	ctx := context.Background()
	deps := moduleContractDependencies()
	evidenceRepo := deps.Evidence.EvidenceRecords()
	installEmptyRecoveryInventory(t, deps.CommandHub)
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("configure orchestrator: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		deps.CommandHub.Terminate(ctx)
	})

	command := orchestratorevents.RecordApprovalEvidenceCommand{
		EvidenceRecordID: "evidence-approval-pairwise",
		WorkspaceID:      "ws-pairwise-evidence",
		RunID:            "run-pairwise-evidence",
		StepID:           "step-pairwise-evidence",
		ApprovalID:       "approval-pairwise-evidence",
		ApprovalState:    common.ApprovalStateApproved,
		ResolutionID:     "resolution-approval-pairwise-evidence",
		Principal: orchestratorevents.GovernanceEvidencePrincipal{
			Kind: "human_operator", SubjectID: "tester",
		},
		Reason: "need authorization",
		Action: "continue runtime action",
		Context: workorcheventpayload.New(
			workorcheventpayload.StringField("scope", "runtime"),
			workorcheventpayload.IntField("risk", 2),
		).Fields,
		ResolutionContext: workorcheventpayload.New(
			workorcheventpayload.StringField("resolvedBy", "tester"),
		).Fields,
	}
	if _, err := module.recordApprovalEvidence(ctx, command); err != nil {
		t.Fatalf("record approval evidence: %v", err)
	}
	if _, err := module.recordApprovalEvidence(ctx, command); err != nil {
		t.Fatalf("record exact duplicate approval evidence: %v", err)
	}
	changed := command
	changed.Action = "different immutable action"
	if _, err := module.recordApprovalEvidence(ctx, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed duplicate approval evidence err = %v, want conflict", err)
	}

	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.RunID)
	if err != nil {
		t.Fatalf("list approval evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("approval evidence count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != runtimemodel.EvidenceKindApproval || record.Status != runtimemodel.EvidenceStatusAccepted || record.SubjectID != string(command.ApprovalID) {
		t.Fatalf("approval evidence = %+v, want accepted approval record", record)
	}
	metadataContext, ok := record.Metadata.Object("context")
	scope, _ := metadataContext.String("scope")
	metadataResolution, resolutionOK := record.Metadata.Object("resolutionContext")
	resolvedBy, _ := metadataResolution.String("resolvedBy")
	if !ok || !resolutionOK || scope != "runtime" || resolvedBy != "tester" {
		t.Fatalf("approval evidence contexts = target:%#v resolution:%#v, want separate immutable target and resolution metadata", metadataContext, metadataResolution)
	}
	action, _ := record.Metadata.String("action")
	if action != command.Action {
		t.Fatalf("approval evidence action = %q, want immutable %q", action, command.Action)
	}
}

func TestDecisionEvidenceCommandRejectsForgedSource(t *testing.T) {
	ctx := context.Background()
	deps := moduleContractDependencies()
	installEmptyRecoveryInventory(t, deps.CommandHub)
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		deps.CommandHub.Terminate(ctx)
	})

	command := orchestratorevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID: "evidence-forged-source", WorkspaceID: "ws-forged-source", RunID: "run-forged-source",
		TicketID: "ticket-forged-source", ResolutionID: "resolution-forged-source",
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence,
		"application/workorchd",
		ModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	)
	result := deps.CommandHub.Send(event)
	if result == nil || result.Error() == nil || !errors.Is(result.Error().Cause, common.ErrInvalidArgument) {
		t.Fatalf("forged decision evidence result = %v", result)
	}
	if _, err := deps.Evidence.EvidenceRecords().Get(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.EvidenceRecordID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("forged source persisted evidence: %v", err)
	}
}

func installEmptyRecoveryInventory(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandWorkspaceRetirementInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryWorkspaceRetirementInventoryResult{}, nil)
		}
	})
	observer.Subscribe(runevents.TopicRunCommandWorkspaceRetirementFact, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
		}
	})
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		observer.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementInventory)
		observer.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementFact)
	})
}
