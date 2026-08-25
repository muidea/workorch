package workorchruntimecontract

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateRuntimeCWILedgerRejectsDuplicateQueueAndRecursivePendingID(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	facts.CompletedFocuses = []CoverageFocus{facts.Focus.Normalized()}
	if err := ValidateRuntimeCWILedger(facts); err == nil || (!strings.Contains(err.Error(), "appears in both focus") && !strings.Contains(err.Error(), "duplicates focus")) {
		t.Fatalf("duplicate CWI queue projection error = %v", err)
	}

	facts = runtimeCWILedgerFacts(t)
	facts.Focus.ID = "semantic-pending/semantic-pending/cwi-1"
	facts.Focus.CoverageEvidenceBindings = nil
	facts.Coverage[0].FocusID = facts.Focus.ID
	facts.Coverage[0].CWIConvergenceState = RuntimeCWIConvergenceStateUnbound
	facts.Coverage[0].CurrentBindingRef = nil
	facts.Coverage[0].BindingLineage = nil
	facts.ReconciliationContract = RuntimeCWIReconciliationContract{}
	if err := ValidateRuntimeCWILedger(facts); err == nil || !strings.Contains(err.Error(), "canonical CWI identity") {
		t.Fatalf("recursive pending focus error = %v", err)
	}
}

func TestValidateRuntimeStateCWILedgerRejectsDuplicateCWI(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	facts.CompletedFocuses = []CoverageFocus{facts.Focus.Normalized()}

	err := ValidateRuntimeStateCWILedger(facts, RuntimeCWIReconciliationLedger{})
	if err == nil {
		t.Fatal("duplicate persisted CWI was accepted")
	}
}

func TestValidateRuntimeCWILedgerRequiresOneCurrentBindingPerCWI(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	first := facts.Focus.CoverageEvidenceBindings[0].Clone()
	second := first.Clone()
	second.CoverageWorkItemID = "cwi-2"
	second.FocusID = "focus-2"
	second.Hash = StableCoverageEvidenceBindingHash(second)
	secondFocus := CoverageFocus{
		ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"},
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, CoverageEvidenceBindings: []CoverageEvidenceBinding{second},
		TargetRefs: second.TargetRefs,
	}
	secondRef := RuntimeCWIBindingRefForCoverageBinding(second, "semantic-manifest-1")
	facts.RemainingFocuses = []CoverageFocus{secondFocus}
	facts.Coverage = append(facts.Coverage, RuntimeCoverageItemFact{
		ID: "cwi-2", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateBound,
		CurrentBindingRef: &secondRef, BindingLineage: []RuntimeCWIBindingRef{secondRef}, FocusID: secondFocus.ID,
	})
	if err := ValidateRuntimeCWILedger(facts); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("shared current binding error = %v", err)
	}
}

func TestValidateRuntimeCWIFocusBindingsAllowsUnboundActivationAndRejectsMalformedSealedBinding(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	facts.Focus.ID = "semantic-pending/cwi-1"
	facts.Focus.CoverageEvidenceBindings = nil
	facts.Coverage[0].FocusID = facts.Focus.ID
	facts.Coverage[0].CWIConvergenceState = RuntimeCWIConvergenceStateUnbound
	facts.Coverage[0].CurrentBindingRef = nil
	facts.Coverage[0].BindingLineage = nil
	facts.ReconciliationContract = RuntimeCWIReconciliationContract{}
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		t.Fatalf("unbound activation ledger: %v", err)
	}
	if err := ValidateRuntimeCWIFocusBindings(facts); err != nil {
		t.Fatalf("unbound activation focus binding validation: %v", err)
	}

	facts = runtimeCWILedgerFacts(t)
	facts.Focus.CoverageEvidenceBindings[0].CoverageWorkItemID = "cwi-other"
	if err := ValidateRuntimeCWIFocusBindings(facts); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("malformed sealed binding error = %v", err)
	}
}

func TestRuntimeCWIsEligibleForPlanningRequiresCurrentSettledPrerequisites(t *testing.T) {
	settledRef := RuntimeCWIBindingRef{BindingID: "binding-source", BindingHash: "hash-source", FocusID: "focus-source", SemanticManifestHash: "manifest-source"}
	facts := RuntimeFacts{
		DefinitionSeed: RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{
			{ID: "cwi-dependent", Order: 1},
			{ID: "cwi-source", Order: 2},
			{ID: "cwi-independent", Order: 3},
		}},
		Coverage: []RuntimeCoverageItemFact{
			{ID: "cwi-dependent", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}},
			{ID: "cwi-source", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound},
			{ID: "cwi-independent", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStatePlanning},
		},
	}
	if eligible := RuntimeCWIsEligibleForPlanning(facts); len(eligible) != 2 || eligible[0] != "cwi-source" || eligible[1] != "cwi-independent" {
		t.Fatalf("eligible with unsettled prerequisite = %+v", eligible)
	}
	facts.Coverage[1].State = RuntimeCoverageItemStateSatisfied
	facts.Coverage[1].CWIConvergenceState = RuntimeCWIConvergenceStateSettled
	facts.Coverage[1].CurrentBindingRef = &settledRef
	if eligible := RuntimeCWIsEligibleForPlanning(facts); len(eligible) != 2 || eligible[0] != "cwi-dependent" || eligible[1] != "cwi-independent" {
		t.Fatalf("eligible with settled prerequisite = %+v", eligible)
	}
	facts.Coverage[1].CurrentBindingRef = nil
	if eligible := RuntimeCWIsEligibleForPlanning(facts); len(eligible) != 1 || eligible[0] != "cwi-independent" {
		t.Fatalf("settled prerequisite without current credit was admitted: %+v", eligible)
	}
}

func TestRuntimeCWIsEligibleForPlanningPreservesDefinitionOrder(t *testing.T) {
	facts := RuntimeFacts{
		DefinitionSeed: RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{{ID: "cwi-z", Order: 1}, {ID: "cwi-a", Order: 2}}},
		Coverage: []RuntimeCoverageItemFact{
			{ID: "cwi-a", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound},
			{ID: "cwi-z", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound},
		},
	}
	if eligible := RuntimeCWIsEligibleForPlanning(facts); len(eligible) != 2 || eligible[0] != "cwi-z" || eligible[1] != "cwi-a" {
		t.Fatalf("eligible definition order = %+v", eligible)
	}
}

func TestProjectRuntimeCWIEligibilityKeepsBlockedQueueHeadBehindEligibleClosure(t *testing.T) {
	settled := RuntimeCWIBindingRef{BindingID: "binding-source", BindingHash: "hash-source", FocusID: "focus-source", SemanticManifestHash: "manifest-source"}
	facts := RuntimeFacts{
		DefinitionSeed:   RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{{ID: "cwi-blocked", Order: 1}, {ID: "cwi-source", Order: 2}, {ID: "cwi-later", Order: 3}, {ID: "cwi-gate", Order: 4}}},
		Focus:            CoverageFocus{ID: "focus-blocked", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-blocked"}},
		RemainingFocuses: []CoverageFocus{{ID: "focus-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}}, {ID: "focus-later", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-later"}}},
		Coverage: []RuntimeCoverageItemFact{
			{ID: "cwi-blocked", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-gate"}},
			{ID: "cwi-source", State: RuntimeCoverageItemStateSatisfied, CWIConvergenceState: RuntimeCWIConvergenceStateSettled, CurrentBindingRef: &settled},
			{ID: "cwi-later", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound},
			{ID: "cwi-gate", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound},
		},
	}
	projection, err := ProjectRuntimeCWIEligibility(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := projection.OrderedEligibleCoverageIDs; len(got) != 2 || got[0] != "cwi-later" || got[1] != "cwi-gate" {
		t.Fatalf("eligible closure = %+v", got)
	}
	if len(projection.BlockedCoverageWorkItemIDs) == 0 || projection.BlockedCoverageWorkItemIDs[0] != "cwi-blocked" {
		t.Fatalf("blocked stable order = %+v", projection.BlockedCoverageWorkItemIDs)
	}
	if _, err := ProjectRuntimeCWIEligibility(facts, []common.CoverageWorkItemID{"cwi-blocked"}); err == nil {
		t.Fatal("blocked semantic order was accepted")
	}
}

func TestProjectRuntimeCWIEligibilityFencesBoundCurrentFocusAndDetachesOutput(t *testing.T) {
	ref := RuntimeCWIBindingRef{BindingID: "binding-current", BindingHash: "hash-current", FocusID: "focus-current", SemanticManifestHash: "manifest-current"}
	facts := RuntimeFacts{
		DefinitionSeed:   RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{{ID: "cwi-current", Order: 1}, {ID: "cwi-later", Order: 2}}},
		Focus:            CoverageFocus{ID: "focus-current", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-current"}},
		RemainingFocuses: []CoverageFocus{{ID: "focus-later", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-later"}}},
		Coverage: []RuntimeCoverageItemFact{
			{ID: "cwi-current", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateBound, CurrentBindingRef: &ref},
			{ID: "cwi-later", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateUnbound},
		},
	}
	projection, err := ProjectRuntimeCWIEligibility(facts, []common.CoverageWorkItemID{"cwi-later"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.FencedCurrentCoverageIDs) != 1 || projection.FencedCurrentCoverageIDs[0] != "cwi-current" {
		t.Fatalf("bound current fence = %+v", projection.FencedCurrentCoverageIDs)
	}
	projection.OrderedEligibleCoverageIDs[0] = "mutated"
	if facts.RemainingFocuses[0].CoverageWorkItemIDs[0] != "cwi-later" {
		t.Fatal("projection aliases RuntimeFacts")
	}
}

func TestValidateRuntimeCWILedgerAllowsOneReopenedSuccessorAndBoundsLineage(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	previous := facts.Coverage[0].CurrentBindingRef.Clone()
	currentFocus := facts.Focus.Normalized()
	currentBinding := currentFocus.CoverageEvidenceBindings[0].Clone()
	currentBinding.ID = "binding-2"
	currentBinding.FocusID = "focus-2"
	currentBinding.Hash = StableCoverageEvidenceBindingHash(currentBinding)
	currentFocus.ID = currentBinding.FocusID
	currentFocus.CoverageEvidenceBindings = []CoverageEvidenceBinding{currentBinding}
	current := RuntimeCWIBindingRefForCoverageBinding(currentBinding, "semantic-manifest-2")
	facts.Focus = currentFocus
	facts.Coverage[0].FocusID = currentFocus.ID
	facts.Coverage[0].CurrentBindingRef = &current
	facts.Coverage[0].BindingLineage = []RuntimeCWIBindingRef{previous, current}
	facts.Coverage[0].ReopenLineage = []RuntimeCWIReopenLineage{{
		PreviousBindingRef: previous, TriggerRef: "receipt/changed-source", Reason: "declared dependency changed",
	}}
	contract, err := CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, []CoverageFocus{facts.Focus})
	if err != nil {
		t.Fatal(err)
	}
	facts.ReconciliationContract = contract
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		t.Fatalf("one reopened successor was rejected: %v", err)
	}

	reactivated := runtimeCWILedgerFacts(t)
	reactivatedRef := reactivated.Coverage[0].CurrentBindingRef.Clone()
	reactivated.Coverage[0].ReopenLineage = []RuntimeCWIReopenLineage{{
		PreviousBindingRef: reactivatedRef, TriggerRef: "assessment/owner-fact-open", Reason: "owner fact remains open",
	}}
	if err := ValidateRuntimeCWILedger(reactivated); err == nil || !strings.Contains(err.Error(), "current binding is already reopened") {
		t.Fatalf("historical reopened binding was reactivated as current: %v", err)
	}

	duplicate := facts
	duplicate.Coverage = append([]RuntimeCoverageItemFact(nil), facts.Coverage...)
	duplicate.Coverage[0].ReopenLineage = append(duplicate.Coverage[0].ReopenLineage, duplicate.Coverage[0].ReopenLineage[0])
	if err := ValidateRuntimeCWILedger(duplicate); err == nil || !strings.Contains(err.Error(), "duplicates reopen lineage") {
		t.Fatalf("duplicate reopen lineage error = %v", err)
	}

	forged := facts
	forged.Coverage = append([]RuntimeCoverageItemFact(nil), facts.Coverage...)
	forged.Coverage[0].ReopenLineage = append([]RuntimeCWIReopenLineage(nil), facts.Coverage[0].ReopenLineage...)
	forged.Coverage[0].ReopenLineage[0].PreviousBindingRef.BindingHash = "forged-binding-hash"
	if err := ValidateRuntimeCWILedger(forged); err == nil || !strings.Contains(err.Error(), "outside binding lineage") {
		t.Fatalf("forged reopen binding identity error = %v", err)
	}

	tooMany := facts
	tooMany.Coverage = append([]RuntimeCoverageItemFact(nil), facts.Coverage...)
	lineage := append([]RuntimeCWIBindingRef(nil), tooMany.Coverage[0].BindingLineage...)
	for index := len(lineage); index <= RuntimeCWIBindingLineageLimit; index++ {
		lineage = append(lineage, RuntimeCWIBindingRef{
			BindingID:            fmt.Sprintf("historical-binding-%d", index),
			BindingHash:          fmt.Sprintf("historical-hash-%d", index),
			FocusID:              fmt.Sprintf("historical-focus-%d", index),
			SemanticManifestHash: fmt.Sprintf("historical-manifest-%d", index),
		})
	}
	tooMany.Coverage[0].BindingLineage = lineage
	if err := ValidateRuntimeCWILedger(tooMany); err == nil || !strings.Contains(err.Error(), "binding lineage exceeds") {
		t.Fatalf("unbounded CWI binding lineage error = %v", err)
	}
}

func TestValidateRuntimeCWILedgerRequiresExplicitBindingRetirementOrReopen(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	previous := facts.Coverage[0].CurrentBindingRef.Clone()
	facts.Focus.ID = "semantic-pending/cwi-1"
	facts.Focus.CoverageEvidenceBindings = nil
	facts.Coverage[0].FocusID = facts.Focus.ID
	facts.Coverage[0].CWIConvergenceState = RuntimeCWIConvergenceStatePlanning
	facts.Coverage[0].CurrentBindingRef = nil
	facts.ReconciliationContract = RuntimeCWIReconciliationContract{}

	if err := ValidateRuntimeCWILedger(facts); err == nil || !strings.Contains(err.Error(), "no retirement or reopen lineage") {
		t.Fatalf("implicit cleared binding error = %v", err)
	}

	facts.Coverage[0].BindingRetirementLineage = []RuntimeCWIBindingRetirementLineage{{
		RetiredBindingRef: previous,
		CausationRef:      "binding-retirement/catalog-revision-2",
		Reason:            "unexecuted binding depends on retired catalog revision",
	}}
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		t.Fatalf("explicit no-effect binding retirement rejected: %v", err)
	}

	conflict := facts
	conflict.Coverage = append([]RuntimeCoverageItemFact(nil), facts.Coverage...)
	conflict.Coverage[0].ReopenLineage = []RuntimeCWIReopenLineage{{
		PreviousBindingRef: previous,
		TriggerRef:         "receipt/changed-source",
		Reason:             "declared dependency changed",
	}}
	if err := ValidateRuntimeCWILedger(conflict); err == nil || !strings.Contains(err.Error(), "both retired and reopened") {
		t.Fatalf("retirement/reopen conflict error = %v", err)
	}
}

func TestValidateRuntimeCWILedgerAllowsRetiredNoEffectSuccessor(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	previous := facts.Coverage[0].CurrentBindingRef.Clone()
	currentFocus := facts.Focus.Normalized()
	currentBinding := currentFocus.CoverageEvidenceBindings[0].Clone()
	currentBinding.ID = "binding-2"
	currentBinding.FocusID = "focus-2"
	currentBinding.Hash = StableCoverageEvidenceBindingHash(currentBinding)
	currentFocus.ID = currentBinding.FocusID
	currentFocus.CoverageEvidenceBindings = []CoverageEvidenceBinding{currentBinding}
	current := RuntimeCWIBindingRefForCoverageBinding(currentBinding, "semantic-manifest-2")
	facts.Focus = currentFocus
	facts.Coverage[0].FocusID = currentFocus.ID
	facts.Coverage[0].CurrentBindingRef = &current
	facts.Coverage[0].BindingLineage = []RuntimeCWIBindingRef{previous, current}
	facts.Coverage[0].BindingRetirementLineage = []RuntimeCWIBindingRetirementLineage{{
		RetiredBindingRef: previous,
		CausationRef:      "binding-retirement/catalog-revision-2",
		Reason:            "unexecuted binding depends on retired catalog revision",
	}}
	contract, err := CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, []CoverageFocus{facts.Focus})
	if err != nil {
		t.Fatal(err)
	}
	facts.ReconciliationContract = contract
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		t.Fatalf("retired no-effect successor rejected: %v", err)
	}
}

func TestValidateRuntimeCWIBindingRetirementRejectsAcceptedDurableExecutionAndEvidence(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	binding := facts.Coverage[0].CurrentBindingRef.Clone()
	facts.Coverage[0].BindingRetirementLineage = []RuntimeCWIBindingRetirementLineage{{
		RetiredBindingRef: binding,
		CausationRef:      "binding-retirement/catalog-revision-2",
		Reason:            "unexecuted binding depends on retired catalog revision",
	}}
	facts.Coverage[0].CurrentBindingRef = nil
	facts.Coverage[0].CWIConvergenceState = RuntimeCWIConvergenceStatePlanning
	facts.Focus.ID = "semantic-pending/cwi-1"
	facts.Focus.CoverageEvidenceBindings = nil
	facts.Coverage[0].FocusID = facts.Focus.ID
	facts.ReconciliationContract = RuntimeCWIReconciliationContract{}

	executionKey := StableRuntimeOperationExecutionKey(facts.WorkspaceID, facts.RunID, "dispatch-1", "op-1")
	dispatch := RuntimeDispatchRecord{
		ID: "dispatch-1",
		Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
			ID: "op-1", Kind: StepKindFileWrite, BindingRefs: []string{binding.BindingID}, TargetRefs: []string{"main.go"},
		}}}}},
		OperationExecutions: []RuntimeOperationExecutionRecord{{
			OperationID: "op-1", ExecutionKey: executionKey, ExpectedEffectScope: RuntimeEffectWorkspaceDurable,
			State: OperationExecutionOwnerCommitted, Fence: 1, OwnerReceiptRef: "receipt-1", OwnerReceiptHash: "receipt-hash-1",
		}},
	}
	receipt := RuntimeOperationReceipt{
		ExecutionKey: executionKey, Fence: 1, OperationID: "op-1", Outcome: RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"main.go"},
	}
	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(facts, &dispatch, nil, []RuntimeOperationReceipt{receipt}); err == nil || !strings.Contains(err.Error(), "accepted durable execution") {
		t.Fatalf("durable execution retirement error = %v", err)
	}
	dispatch.Candidate.StateChange.Operations[0].Operation.BindingRefs = nil
	dispatch.Transaction.FocusID = binding.FocusID
	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(facts, nil, []RuntimeDispatchRecord{dispatch}, []RuntimeOperationReceipt{receipt}); err == nil || !strings.Contains(err.Error(), "omitted its sealed binding identity") {
		t.Fatalf("ambiguous durable execution retirement error = %v", err)
	}

	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(facts, nil, nil, nil); err != nil {
		t.Fatalf("no-effect retirement rejected: %v", err)
	}
	facts.Evidence.Accepted = []EvidenceFact{{ID: "evidence-1", BindingRefs: []string{binding.BindingID}}}
	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(facts, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "accepted evidence") {
		t.Fatalf("accepted evidence retirement error = %v", err)
	}
}

func TestValidateRuntimeCWIReconciliationLedgerFencesCurrentAndHistoricalScope(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	request := runtimeCWILedgerRequest(t, facts, "request-current", 2)
	oldRequest := request.Clone()
	oldRequest.RequestID = "request-old"
	oldRequest.Generation = 1
	oldContract := runtimeCWILedgerDifferentContract(t, facts.ReconciliationContract)
	oldRequest.BindingSetHash = oldContract.BindingSetHash
	oldRequest.Bindings = oldContract.Bindings
	oldRequestHash := StableRuntimeCWIReconciliationRequestHash(oldRequest)
	ledger := RuntimeCWIReconciliationLedger{
		Request: &request, RequestHash: StableRuntimeCWIReconciliationRequestHash(request),
		History: []RuntimeCWIReconciliationGeneration{{Superseded: true, Request: oldRequest, RequestHash: oldRequestHash}},
	}
	if err := ValidateRuntimeCWIReconciliationLedger(facts, ledger); err != nil {
		t.Fatalf("superseded historical binding set was rejected: %v", err)
	}

	wrongCurrent := ledger.Clone()
	wrongContract := runtimeCWILedgerDifferentContract(t, facts.ReconciliationContract)
	wrongCurrent.Request.BindingSetHash = wrongContract.BindingSetHash
	wrongCurrent.Request.Bindings = wrongContract.Bindings
	wrongCurrent.RequestHash = StableRuntimeCWIReconciliationRequestHash(*wrongCurrent.Request)
	if err := ValidateRuntimeCWIReconciliationLedger(facts, wrongCurrent); err == nil || !strings.Contains(err.Error(), "active request") {
		t.Fatalf("current request binding drift error = %v", err)
	}

	historyOnly := RuntimeCWIReconciliationLedger{History: ledger.History}
	historyOnly.History[0].Request.WorkspaceID = "ws-other"
	historyOnly.History[0].RequestHash = StableRuntimeCWIReconciliationRequestHash(historyOnly.History[0].Request)
	if err := ValidateRuntimeCWIReconciliationLedger(facts, historyOnly); err == nil || !strings.Contains(err.Error(), "history scope") {
		t.Fatalf("historical cross-workspace lineage error = %v", err)
	}
}

func TestValidateRuntimeCWIReconciliationLedgerRejectsUnboundedHistory(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	history := make([]RuntimeCWIReconciliationGeneration, 0, RuntimeCWIReconciliationHistoryLimit+1)
	for generation := uint64(1); generation <= RuntimeCWIReconciliationHistoryLimit+1; generation++ {
		request := runtimeCWILedgerRequest(t, facts, fmt.Sprintf("request-history-%d", generation), generation)
		history = append(history, RuntimeCWIReconciliationGeneration{
			Superseded:  true,
			Request:     request,
			RequestHash: StableRuntimeCWIReconciliationRequestHash(request),
		})
	}
	if err := ValidateRuntimeCWIReconciliationLedger(facts, RuntimeCWIReconciliationLedger{History: history}); err == nil || !strings.Contains(err.Error(), "history exceeds bounded limit") {
		t.Fatalf("unbounded reconciliation history error = %v", err)
	}
}

func runtimeCWILedgerFacts(t *testing.T) RuntimeFacts {
	t.Helper()
	focus := runtimeCWIReconciliationFocus()
	binding := focus.CoverageEvidenceBindings[0].Clone()
	ref := RuntimeCWIBindingRefForCoverageBinding(binding, "semantic-manifest-1")
	contract, err := CompileRuntimeCWIReconciliationContract("definition-1", []CoverageFocus{focus})
	if err != nil {
		t.Fatal(err)
	}
	return RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1",
		CWILedgerSchemaVersion: RuntimeCWILedgerSchemaCurrent,
		Focus:                  focus,
		Coverage: []RuntimeCoverageItemFact{{
			ID: "cwi-1", State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateBound,
			CurrentBindingRef: &ref, BindingLineage: []RuntimeCWIBindingRef{ref}, FocusID: focus.ID,
		}},
		ReconciliationContract: contract,
	}
}

func runtimeCWILedgerRequest(t *testing.T, facts RuntimeFacts, requestID string, generation uint64) RuntimeCWIReconciliationRequest {
	t.Helper()
	request := RuntimeCWIReconciliationRequest{
		RequestID: requestID, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		Scope: RuntimeCWIReconciliationScopeCurrentRun, DefinitionRevision: facts.DefinitionRevision,
		BindingSetHash: facts.ReconciliationContract.BindingSetHash, Bindings: facts.ReconciliationContract.Bindings,
		WorkspaceBaselineRevision: "workspace-tree-1", WorkspaceBaselineRef: "workspace-tree/ws-1/tree-1", Generation: generation,
	}
	if err := request.ValidateAgainstContract(facts.ReconciliationContract); err != nil {
		t.Fatal(err)
	}
	return request
}

func runtimeCWILedgerDifferentContract(t *testing.T, current RuntimeCWIReconciliationContract) RuntimeCWIReconciliationContract {
	t.Helper()
	value := current.Clone()
	value.Bindings = append([]RuntimeCWIReconciliationBindingRef(nil), value.Bindings...)
	value.Bindings[0].BindingID = value.Bindings[0].BindingID + "-superseded"
	value.Bindings[0].BindingHash = value.Bindings[0].BindingHash + "-superseded"
	value.BindingSetHash = StableRuntimeCWIReconciliationContractHash(value)
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}
