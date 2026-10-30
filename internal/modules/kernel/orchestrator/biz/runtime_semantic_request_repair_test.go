package orchestrator

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only the legacy source snapshot is seeded. Scope/order acquisition uses the
// production driver, and recovery uses real Decision and Run owner commands.
// No raw live archive or credentials are included in this synthetic fixture.
func legacyRequestRepairDriverFixture(t *testing.T) (*Biz, runtime.RuntimeAggregate, runtimecontract.RuntimeSemanticRequestRepairProof) {
	t.Helper()
	module, repo, a := implementationWorkPurposeDebtDriverFixture(t)
	installImplementationWorkProcessProvider(t, module.EventHub(), 0, true)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), a.Facts.WorkspaceID)
	for i := 0; i < 40; i++ {
		if a.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen && a.SemanticPlanning.Manifest != nil && a.SemanticPlanning.Manifest.ProofObligations[0].ImplementationWork != nil && !a.SemanticPlanning.Manifest.ProofObligations[0].ImplementationWork.PurposeChoiceOnly {
			break
		}
		out, err := module.driveOneRuntimeTransition(ctx, nil, a, orchestratorevents.DriveRuntimeCommand{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, "")
		if err != nil {
			t.Fatal(err)
		}
		a = out.aggregate
	}
	if a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen {
		t.Fatal("fixture did not reach work request")
	}
	expected, err := runtime.SemanticExpectedDocument(a)
	if err != nil {
		t.Fatal(err)
	}
	expected.SemanticRenderContractRef = ""
	expected.MaxOutputTokens = 4096
	expected.MaxRecoveryAttempts = 1
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	gap := runtimecontract.RuntimeDefinitionGapProviderDocument{DocumentKind: expected.DocumentKind, SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: expected.ManifestHash, DefinitionSeedHash: expected.DefinitionSeedHash, ResultKind: runtimecontract.RuntimeDefinitionGapResultKind, PlanStep: expected.PlanStep, BatchID: expected.BatchID, BatchSequence: &expected.BatchSequence, BatchCoverageWorkItemIDs: expected.BatchCoverageWorkItemIDs, GapKind: runtimecontract.RuntimeDefinitionGapSemanticAmbiguity, OffendingItemRefs: []string{string(expected.BatchCoverageWorkItemIDs[0])}, DiagnosticMessage: "synthetic original: request contracts conflict"}
	payload, err := json.Marshal(gap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimecontract.DecodeRuntimeDefinitionGapPayloadWithManifest(*a.SemanticPlanning.Manifest, expected, payload); err != nil {
		t.Fatal(err)
	}
	session := runtimecontract.RuntimeCandidateRequestSession{LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected, Transaction: a.Decision.AuthorityTransaction(), StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash, State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1, LastPhysicalCallID: "legacy-complete-work", ProviderSubmitted: true, LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete}}
	a.CandidateRequestSessions = append(a.CandidateRequestSessions, session)
	repo.aggregate = a
	if _, _, err := persistRuntimeCandidateRequestDocument(ctx, module.evidence.ArtifactContinuations, a, expected.LogicalRequestID, 1, session.LastPhysicalCallID, payload); err != nil {
		t.Fatal(err)
	}
	observation := runtimecontract.RuntimeDefinitionGapObservation{SchemaVersion: runtimecontract.RuntimeDefinitionClarificationSchemaCurrent, WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, DefinitionRevision: a.Definition.Revision, StartupInputHash: a.Definition.StartupInputHash, AuthoritySnapshotID: a.Decision.AuthoritySnapshot.ID, FocusRef: a.Facts.Focus.ID, LogicalRequestID: expected.LogicalRequestID, SemanticDocumentHash: runtimecontract.StableRuntimeProviderResponseHash(payload), GapKind: gap.GapKind, OffendingItemRefs: gap.OffendingItemRefs, OriginalDiagnostic: runtimecontract.RuntimeOriginalDiagnosticFromDefinitionGap(gap)}
	observation.ObservationID = runtimecontract.StableRuntimeDefinitionGapObservationID(observation)
	scope := runtime.RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}
	for _, ev := range []runtime.RuntimeInternalEvent{{Kind: runtime.RuntimeInternalEventDefinitionGapObserved, Scope: scope, Authority: &runtime.RuntimeInternalAuthorityPayload{DefinitionGap: &observation}}, {Kind: runtime.RuntimeInternalEventDefinitionClarificationDispatchSealed, Scope: scope, Transaction: &runtime.RuntimeInternalTransactionPayload{}}} {
		out, err := module.submitRuntimeInternalEvent(ctx, ev)
		if err != nil {
			t.Fatal(err)
		}
		a = out.Aggregate
	}
	original := (decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand{Proof: runtimecontract.RuntimeSemanticRequestRepairProof{Dispatch: *a.DefinitionClarification.Dispatch}}).OriginalRequest()
	original.TicketID = decisionevents.StableRuntimeDefinitionClarificationTicketID(original)
	record := runtimecontract.RuntimeDefinitionClarificationRequestRecord{DispatchID: original.ClarificationDispatchID, TicketID: original.TicketID, RequestBindingHash: original.BindingHash()}
	out, err := module.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventDefinitionClarificationRequestObserved, Scope: scope, Transaction: &runtime.RuntimeInternalTransactionPayload{DefinitionClarificationRequest: &record}})
	if err != nil {
		t.Fatal(err)
	}
	a = out.Aggregate
	proof, err := runtime.CompileRuntimeSemanticRequestRepairProof(a)
	if err != nil {
		t.Fatal(err)
	}
	return module, a, proof
}

func TestSemanticRequestRepairDriverDecisionResponseLossAndRunRestartHandoff(t *testing.T) {
	for _, waitAck := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-original-wait-ack", true: "blocked-on-original-ticket"}[waitAck], func(t *testing.T) {
			testSemanticRequestRepairDriverDecisionResponseLossAndRunRestartHandoff(t, waitAck)
		})
	}
}
func testSemanticRequestRepairDriverDecisionResponseLossAndRunRestartHandoff(t *testing.T, waitAck bool) {
	module, before, proof := legacyRequestRepairDriverFixture(t)
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), before.Facts.WorkspaceID)
	hub := module.EventHub()
	decisionRepos := memory.NewDecisionOwnerRepositorySet()
	decision, err := decisionbiz.New(hub, nil, decisionbiz.Dependencies{EventHub: hub, OwnerRepositories: decisionRepos.OwnerRepositories()})
	if err != nil {
		t.Fatal(err)
	}
	if err := decision.Start(); err != nil {
		t.Fatal(err)
	}
	defer decision.Teardown(ctx)
	open := (decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID, Proof: proof}).OriginalRequest()
	result := hub.Send(magicevent.NewEventWithContext(decisionevents.TopicOpenRuntimeDefinitionClarification, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, open))
	if result == nil {
		t.Fatal("missing open response")
	}
	if _, err := result.Get(); err != nil {
		t.Fatal(err)
	}
	// Seed an existing opaque snapshot without altering its original history revisions.
	runs := memory.NewRunRepository()
	runInput := runtimeRunForTest(common.RunStateActive, before.Definition)
	record := runtimeStateRecordFromAggregate(before)
	runInput.RuntimeState = &record
	runInput.RuntimeLifecycleFence = mustRuntimeLifecycleFence(t, runInput)
	if err := runs.Create(ctx, runInput); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runOwner.Stop(ctx)
	stateRepo := runtimeStateRepository{hub: hub}
	if err := module.ConfigureRuntimeRepository(stateRepo); err != nil {
		t.Fatal(err)
	}
	handoff, err := runtimeGovernedWaitDriveResult(before)
	if err != nil {
		t.Fatal(err)
	}
	if waitAck {
		if err := module.applyRunGovernanceWait(ctx, before.Facts.WorkspaceID, before.Facts.RunID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision, handoff.EvidenceRevision, before.DefinitionClarification.Dispatch.DispatchID, handoff.BlockingTicketID, "", before.StateRevision); err != nil {
			t.Fatal(err)
		}
	}
	background := &recordingBackgroundRoutine{}
	startupOwner := newTestBiz(newRuntimeTestOwner(), hub, background)
	startupOwner.evidence = module.evidence
	if err := startupOwner.ConfigureRuntimeRepository(stateRepo); err != nil {
		t.Fatal(err)
	}
	if err := startupOwner.recoverRuntimeStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatal("startup did not schedule exact legacy repair source")
	}
	// Seal the intent, then let Decision finish without delivering its reply to
	// Runtime. Restart/replay must receive the same owner resolution.
	if _, err := module.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticRequestRepairPrepared, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{RequestContractRepairProof: &proof}}); err != nil {
		t.Fatal(err)
	}
	command := decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID, Proof: proof}
	result = hub.Send(magicevent.NewEventWithContext(decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		t.Fatal("missing repair response")
	}
	if _, err := result.Get(); err != nil {
		t.Fatal(err)
	}
	// Recreate the Orchestrator while retaining only owner-managed repositories.
	restarted := newTestBiz(newRuntimeTestOwner(), hub, &recordingBackgroundRoutine{})
	restarted.evidence = module.evidence
	if err := restarted.ConfigureRuntimeRepository(stateRepo); err != nil {
		t.Fatal(err)
	}
	repaired, changed, err := restarted.recoverRuntimeSemanticRequestContract(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil || !changed {
		t.Fatalf("repair replay: changed=%v err=%v", changed, err)
	}
	blocked, err := runs.GetByWorkspaceID(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != common.RunStateBlockedOnDecision || repaired.DriveLease == nil {
		t.Fatal("SM-2 recovery bypassed SM-1 or lost successor")
	}
	if !reflect.DeepEqual(repaired.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(repaired.ImplementationWork, before.ImplementationWork) || len(repaired.OperationReceipts) != len(before.OperationReceipts) {
		t.Fatal("repair consumed or replayed work")
	}
	delivered, err := restarted.deliverPersistedRuntimeGovernanceContinuation(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil || !delivered {
		t.Fatalf("Run handoff delivered=%v err=%v", delivered, err)
	}
	active, err := runs.GetByWorkspaceID(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if active.State != common.RunStateActive || active.BlockingTicketID != "" || active.RuntimeLifecycleFence.GovernanceResolutionID == "" {
		t.Fatal("Run owner did not acknowledge exact repair")
	}
	revision := active.Revision
	if delivered, err := restarted.deliverPersistedRuntimeGovernanceContinuation(ctx, before.Facts.WorkspaceID, before.Facts.RunID); err != nil || delivered {
		t.Fatalf("duplicate handoff: %v %v", delivered, err)
	}
	duplicate, _ := runs.GetByWorkspaceID(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if duplicate.Revision != revision {
		t.Fatal("duplicate handoff mutated lifecycle")
	}
	if _, changed, err := restarted.recoverRuntimeSemanticRequestContract(ctx, before.Facts.WorkspaceID, before.Facts.RunID); err != nil || changed {
		t.Fatalf("recovery replay changed state: %v", err)
	}
	if _, _, err := restarted.recoverRuntimeSemanticRequestContract(ctx, "foreign", before.Facts.RunID); err == nil {
		t.Fatal("cross-workspace context was accepted")
	}
}

func TestSemanticRequestRepairLeavesOrdinaryDefinitionGapUnchanged(t *testing.T) {
	a := runtimeBoundDefinitionClarificationAggregate(t)
	repo := &inMemoryRuntimeStateRepository{aggregate: &a}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repo); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	defer hub.Terminate(context.Background())
	module := newTestBiz(owner, hub, nil)
	before, _ := json.Marshal(a)
	_, changed, err := module.recoverRuntimeSemanticRequestContract(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil || changed {
		t.Fatalf("ordinary clarification consumed as technical repair: %v", err)
	}
	after, _ := json.Marshal(repo.aggregate)
	if string(before) != string(after) {
		t.Fatal("genuine Definition wait was modified")
	}
}
