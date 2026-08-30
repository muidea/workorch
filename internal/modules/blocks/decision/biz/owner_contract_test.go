package decision

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDecisionPrivilegedIngressRejectsForgedSourcesWithoutMutation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	evidence := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	evidence.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	t.Cleanup(func() { evidence.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence) })

	t.Run("workspace open", func(t *testing.T) {
		command := workspaceDefinitionClarificationSourceCommand("open")
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid open command: %v", err)
		}
		bound := common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenWorkspaceDefinitionClarification, "untrusted/module", command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged open source error = %v, want invalid argument", err)
		}
		if _, err := repositories.Tickets.Get(bound, command.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("forged open source persisted ticket: %v", err)
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenWorkspaceDefinitionClarification, workspaceDefinitionClarificationSource, command)
		result, ok := data.(decisionevents.OpenWorkspaceDefinitionClarificationRequestResult)
		if err != nil || !ok || result.ValidateFor(command) != nil {
			t.Fatalf("trusted open result = %#v err=%v", data, err)
		}
	})

	t.Run("workspace fact query", func(t *testing.T) {
		open := workspaceDefinitionClarificationSourceCommand("query")
		bound := common.ContextWithRuntimeWorkspaceID(ctx, open.WorkspaceID)
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenWorkspaceDefinitionClarification, workspaceDefinitionClarificationSource, open); err != nil {
			t.Fatal(err)
		}
		command := decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{WorkspaceID: open.WorkspaceID, TicketID: open.TicketID}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid query command: %v", err)
		}
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicQueryWorkspaceDefinitionClarificationFact, workspaceDefinitionClarificationSource, command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged query source error = %v, want invalid argument", err)
		}
		stored, err := repositories.Tickets.Get(bound, open.TicketID)
		if err != nil || stored.State != common.DecisionTicketStateOpen {
			t.Fatalf("forged query mutated ticket = %+v err=%v", stored, err)
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicQueryWorkspaceDefinitionClarificationFact, workspaceDefinitionFactQuerySource, command)
		result, ok := data.(decisionevents.QueryWorkspaceDefinitionClarificationFactResult)
		if err != nil || !ok || !result.Found || result.ValidateFor(command) != nil {
			t.Fatalf("trusted query result = %#v err=%v", data, err)
		}
	})

	t.Run("workspace resolve", func(t *testing.T) {
		open := workspaceDefinitionClarificationSourceCommand("resolve")
		bound := common.ContextWithRuntimeWorkspaceID(ctx, open.WorkspaceID)
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenWorkspaceDefinitionClarification, workspaceDefinitionClarificationSource, open); err != nil {
			t.Fatal(err)
		}
		command := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
			TicketID: open.TicketID, WorkspaceID: open.WorkspaceID,
			OldCandidateID: open.CandidateID, OldCandidateRevision: open.CandidateRevision, OldCandidateHash: open.CandidateHash,
			NewCandidateID: open.CandidateID, NewCandidateRevision: open.CandidateRevision + 1, NewCandidateHash: "candidate-hash-resolved",
			AdmissionResultHash: "admission-result-resolved", DefinitionRevision: "definition-resolved", StartupInputHash: "startup-resolved",
		}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid resolve command: %v", err)
		}
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission, runtimeDefinitionReplacementSource, command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged resolve source error = %v, want invalid argument", err)
		}
		stored, err := repositories.Tickets.Get(bound, open.TicketID)
		if err != nil || stored.State != common.DecisionTicketStateOpen {
			t.Fatalf("forged resolve mutated ticket = %+v err=%v", stored, err)
		}
		if _, err := repositories.Resolutions.GetByTicketID(bound, open.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("forged resolve persisted resolution: %v", err)
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission, workspaceDefinitionClarificationSource, command)
		result, ok := data.(decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult)
		if err != nil || !ok || result.ValidateFor(command) != nil {
			t.Fatalf("trusted resolve result = %#v err=%v", data, err)
		}
	})

	t.Run("workspace reconcile", func(t *testing.T) {
		oldRequest := workspaceDefinitionClarificationSourceCommand("reconcile")
		bound := common.ContextWithRuntimeWorkspaceID(ctx, oldRequest.WorkspaceID)
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenWorkspaceDefinitionClarification, workspaceDefinitionClarificationSource, oldRequest); err != nil {
			t.Fatal(err)
		}
		newRequest := oldRequest
		newRequest.CandidateRevision++
		newRequest.CandidateHash = "candidate-hash-reconcile-new"
		newRequest.CanonicalSignature = "one-of:reconcile-new"
		newRequest.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", newRequest.CanonicalSignatureVersion, newRequest.CanonicalSignature)
		newRequest.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(newRequest)
		command := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: oldRequest, NewRequest: newRequest}.Normalized()
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid reconcile command: %v", err)
		}
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, "blocks/workspace", command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged reconcile source error = %v, want invalid argument", err)
		}
		oldTicket, err := repositories.Tickets.Get(bound, oldRequest.TicketID)
		if err != nil || oldTicket.State != common.DecisionTicketStateOpen {
			t.Fatalf("forged reconcile mutated old ticket = %+v err=%v", oldTicket, err)
		}
		if _, err := repositories.Tickets.Get(bound, newRequest.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("forged reconcile opened replacement ticket: %v", err)
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, workspaceDefinitionClarificationSource, command)
		result, ok := data.(decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult)
		if err != nil || !ok || result.ValidateFor(command) != nil {
			t.Fatalf("trusted reconcile result = %#v err=%v", data, err)
		}

		staleNewRequest := newRequest
		staleNewRequest.CandidateRevision++
		staleNewRequest.CandidateHash = "candidate-hash-reconcile-stale"
		staleNewRequest.CanonicalSignature = "one-of:reconcile-stale"
		staleNewRequest.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", staleNewRequest.CanonicalSignatureVersion, staleNewRequest.CanonicalSignature)
		staleNewRequest.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(staleNewRequest)
		stale := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: oldRequest, NewRequest: staleNewRequest}.Normalized()
		if err := stale.Validate(); err != nil {
			t.Fatalf("production-valid stale reconcile command: %v", err)
		}
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, workspaceDefinitionClarificationSource, stale); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("persisted stale reconcile error = %v, want conflict", err)
		}
		if _, err := repositories.Tickets.Get(bound, staleNewRequest.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("persisted stale reconcile opened an unrelated blocker: %v", err)
		}
		oldTicket, err = repositories.Tickets.Get(bound, oldRequest.TicketID)
		if err != nil || oldTicket.DefinitionClarification == nil || oldTicket.DefinitionClarification.ResolutionBindingHash != result.ReconciliationBindingHash {
			t.Fatalf("persisted stale reconcile changed old binding = %+v err=%v", oldTicket, err)
		}
	})

	t.Run("runtime open", func(t *testing.T) {
		command := runtimeDefinitionClarificationSourceCommand("open")
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid runtime open command: %v", err)
		}
		bound := common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenRuntimeDefinitionClarification, workspaceDefinitionClarificationSource, command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged runtime open source error = %v, want invalid argument", err)
		}
		if _, err := repositories.Tickets.Get(bound, command.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("forged runtime open persisted ticket: %v", err)
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenRuntimeDefinitionClarification, runtimeDefinitionClarificationSource, command)
		result, ok := data.(decisionevents.OpenRuntimeDefinitionClarificationRequestResult)
		if err != nil || !ok || result.TicketID != command.TicketID || result.Disposition != decisionevents.DefinitionClarificationCreated || result.ClarificationRequestBindingHash != command.BindingHash() {
			t.Fatalf("trusted runtime open result = %#v err=%v", data, err)
		}
	})

	t.Run("runtime replacement resolve", func(t *testing.T) {
		open := runtimeDefinitionClarificationSourceCommand("replacement")
		bound := common.ContextWithRuntimeWorkspaceID(ctx, open.WorkspaceID)
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicOpenRuntimeDefinitionClarification, runtimeDefinitionClarificationSource, open); err != nil {
			t.Fatal(err)
		}
		command := decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand{
			TicketID: open.TicketID, WorkspaceID: open.WorkspaceID, PredecessorRunID: open.RunID,
			ReplacementIntentID: "replacement-intent-source", PredecessorDefinitionRevision: open.DefinitionRevision,
			PredecessorStartupInputHash: open.StartupInputHash, SuccessorDefinitionRevision: "definition-successor",
			SuccessorStartupInputHash: "startup-successor", ReplacementIntentHash: "replacement-intent-hash-source",
		}
		if err := command.Validate(); err != nil {
			t.Fatalf("production-valid runtime replacement command: %v", err)
		}
		if _, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement, runtimeDefinitionClarificationSource, command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("forged runtime replacement source error = %v, want invalid argument", err)
		}
		stored, err := repositories.Tickets.Get(bound, open.TicketID)
		if err != nil || stored.State != common.DecisionTicketStateOpen {
			t.Fatalf("forged runtime replacement mutated ticket = %+v err=%v", stored, err)
		}
		if _, err := repositories.Resolutions.GetByTicketID(bound, open.TicketID); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("forged runtime replacement persisted resolution: %v", err)
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement, runtimeDefinitionReplacementSource, command)
		result, ok := data.(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult)
		if err != nil || !ok || result.ValidateFor(command) != nil {
			t.Fatalf("trusted runtime replacement result = %#v err=%v", data, err)
		}
	})
}

func TestDecisionOrdinaryResolutionAcceptsOnlyApplicationOrGovernanceSources(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	evidence := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	evidence.Subscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RecordDecisionEvidenceCommand)
		result.Set(orchestratorevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	t.Cleanup(func() { evidence.Unsubscribe(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence) })

	resolve := func(t *testing.T, suffix, source string, expectAccepted bool) {
		t.Helper()
		workspaceID := common.WorkspaceID("ws-ordinary-source-" + suffix)
		runID := common.RunID("run-ordinary-source-" + suffix)
		ticketID := common.TicketID("ticket-ordinary-source-" + suffix)
		bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := repositories.Tickets.Create(bound, &decisionmodel.Ticket{
			ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen, Reason: "need operator input",
		}); err != nil {
			t.Fatal(err)
		}
		command := decisionevents.EnsureResolvedCommand{
			ResolutionID: common.ResolutionID("resolution-ordinary-source-" + suffix), TicketID: ticketID,
			WorkspaceID: workspaceID, RunID: runID, Type: decisioncommon.ResolutionTypeProvideMissingInfo,
			Payload: mustDecisionPayload(t, map[string]any{"answer": "continue"}).Fields,
		}
		data, err := sendDecisionOwnerContractCommand(bound, hub, decisionevents.TopicDecisionCommandEnsureResolved, source, command)
		if !expectAccepted {
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("forged ordinary source error = %v, want invalid argument", err)
			}
			stored, readErr := repositories.Tickets.Get(bound, ticketID)
			if readErr != nil || stored.State != common.DecisionTicketStateOpen {
				t.Fatalf("forged ordinary source mutated ticket = %+v err=%v", stored, readErr)
			}
			if _, readErr := repositories.Resolutions.GetByTicketID(bound, ticketID); !errors.Is(readErr, common.ErrNotFound) {
				t.Fatalf("forged ordinary source persisted resolution: %v", readErr)
			}
			return
		}
		result, ok := data.(decisionevents.EnsureResolvedResult)
		if err != nil || !ok || result.Ticket == nil || result.Resolution == nil || result.Ticket.State != common.DecisionTicketStateResolved || result.Resolution.ID != command.ResolutionID {
			t.Fatalf("trusted ordinary result = %#v err=%v", data, err)
		}
	}

	resolve(t, "forged", "blocks/run", false)
	resolve(t, "application", decisionResolutionApplicationSource, true)
	resolve(t, "governance", decisionResolutionGovernanceSource, true)
}

func workspaceDefinitionClarificationSourceCommand(suffix string) decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand {
	command := workspaceDefinitionClarificationCommandForBizTest()
	command.WorkspaceID = common.WorkspaceID("workspace-source-" + suffix)
	command.CandidateID = "candidate-source-" + suffix
	command.CandidateHash = "candidate-hash-source-" + suffix
	command.CanonicalSignature = "one-of:source:" + suffix
	command.CanonicalSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = decisionevents.StableWorkspaceDefinitionClarificationTicketID(command)
	return command.Normalized()
}

func runtimeDefinitionClarificationSourceCommand(suffix string) decisionevents.OpenRuntimeDefinitionClarificationRequestCommand {
	command := decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{
		SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:   "workspace-runtime-source-" + common.WorkspaceID(suffix), RunID: "run-runtime-source-" + common.RunID(suffix),
		DefinitionRevision: "definition-runtime-source-" + suffix, StartupInputHash: "startup-runtime-source-" + suffix,
		AuthoritySnapshotID: "authority-runtime-source-" + suffix, ClarificationDispatchID: "dispatch-runtime-source-" + suffix,
		FocusRef: "focus-runtime-source-" + suffix, LogicalRequestID: "request-runtime-source-" + suffix,
		SemanticDocumentHash: "document-runtime-source-" + suffix, GapKind: decisionevents.RuntimeDefinitionGapSemanticAmbiguity,
		OffendingItemRefs: []string{"dod:item-source-" + suffix}, CanonicalGapSignatureVersion: "v1",
		CanonicalGapSignature: "semantic-ambiguity:" + suffix, RequiredActionKind: decisionevents.DefinitionClarificationActionReviseAndRestart,
	}
	command.CanonicalGapSignatureHash = decisionevents.CanonicalDefinitionClarificationSignatureHash("runtime-signature", command.CanonicalGapSignatureVersion, command.CanonicalGapSignature)
	command.TicketID = decisionevents.StableRuntimeDefinitionClarificationTicketID(command)
	return command.Normalized()
}

func sendDecisionOwnerContractCommand(ctx context.Context, hub magicevent.Hub, topic, source string, command any) (any, error) {
	result := hub.Send(magicevent.NewEventWithContext(topic, source, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return nil, resultErr.Cause
		}
		return nil, resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return nil, resultErr.Cause
		}
		return nil, resultErr
	}
	return data, nil
}

func TestDecisionRuntimeCapabilityAcquisitionIsOrchestratorOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-decision-capability", RunID: "run-decision-capability",
		AuthoritySnapshotID: "authority-decision-capability", PrerequisiteID: "prerequisite-decision-capability",
		OwnerModuleID: ModuleID, CapabilityClass: runtimecontract.RuntimeCapabilityClassGovernance,
		RequiredKinds: []string{runtimecontract.RuntimeBlockKindDecisionRequest},
	}
	bound := common.ContextWithRuntimeWorkspaceID(ctx, request.WorkspaceID)
	send := func(source string) (decisionevents.AcquireRuntimeCapabilitiesResult, error) {
		result := hub.Send(magicevent.NewEventWithContext(
			decisionevents.TopicAcquireRuntimeCapabilities, source, decisionevents.CommandDestination,
			magicevent.NewHeader(), bound, decisionevents.AcquireRuntimeCapabilitiesCommand(request),
		))
		if result == nil {
			return decisionevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument
		}
		if resultErr := result.Error(); resultErr != nil {
			if resultErr.Cause != nil {
				return decisionevents.AcquireRuntimeCapabilitiesResult{}, resultErr.Cause
			}
			return decisionevents.AcquireRuntimeCapabilitiesResult{}, resultErr
		}
		data, resultErr := result.Get()
		if resultErr != nil {
			return decisionevents.AcquireRuntimeCapabilitiesResult{}, resultErr
		}
		response, ok := data.(decisionevents.AcquireRuntimeCapabilitiesResult)
		if !ok {
			return decisionevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument
		}
		return response, nil
	}
	if _, err := send("untrusted/module"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged capability source error = %v, want invalid argument", err)
	}
	response, err := send(runtimecontract.RuntimeModuleOrchestrator)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("Decision capability acquisition: %v", err)
	}
}
