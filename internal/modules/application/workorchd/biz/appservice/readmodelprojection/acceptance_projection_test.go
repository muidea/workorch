package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapAcceptanceProjectionReadsPersistedRuntimeFacts(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		Focus:  runtimecontract.CoverageFocus{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-1", SliceID: "slice-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied},
			{ID: "cwi-2", SliceID: "slice-1", State: runtimecontract.RuntimeCoverageItemStateOpen},
		},
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
		Recovery:          runtimecontract.RuntimeRecoveryLedger{CASReloadAttempts: 1},
		Repair:            runtimecontract.RuntimeRepairLedger{FocusID: "focus-1", SliceID: "slice-1", RepairAttempts: 2},
		RepairDirective: &runtimecontract.RuntimeRepairDirective{ID: "repair-1", FocusID: "focus-1", RequiredAction: runtimecontract.RuntimeActionProduceValidation,
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
	}, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, Action: runtimecontract.RuntimeActionProduceValidation},
		ActiveDispatch: &runtimecontract.RuntimeDispatchRecord{ID: "dispatch-1", Transaction: runtimecontract.AuthorityTransaction{CapabilityStateHash: "capability-state-hash"}, Candidate: runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "validate-1", Kind: runtimecontract.StepKindGitRead, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: strPtr("status")}},
		}}}}}, OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID: "validate-1", ExecutionKey: "exec-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
			State: runtimecontract.OperationExecutionOwnerCommitted, OwnerReceiptRef: "runtime-receipt/exec-1", OwnerReceiptHash: "hash-1",
		}}},
		OperationReceipts: []runtimecontract.RuntimeOperationReceipt{{ExecutionKey: "exec-1", Fence: 1, OperationID: "validate-1",
			Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone}},
		GovernanceRequests:    []runtimecontract.RuntimeGovernanceRequestRecord{{TicketID: "ticket-1", AuthoritySnapshotID: "snapshot-1", DispatchID: "dispatch-1", DependencyKind: "decision", SubjectRef: "workspace"}},
		GovernanceResolutions: []runtimecontract.RuntimeGovernanceResolutionRecord{{ResolutionID: "resolution-1", TicketID: "ticket-1", AuthoritySnapshotID: "snapshot-1", DispatchID: "dispatch-1", Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue}},
	}
	state.Facts.Resources.CapabilityStates = []runtimecontract.RuntimeCapabilityStateSnapshot{{
		RequirementID: "capability-state:git", SnapshotID: "snapshot-git", AcquisitionKey: "capability-state/ws/run/0", QueryReceiptRef: "receipt-git",
		OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-absent", ContentHash: "hash-git",
		Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired, RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
	}}
	projection := MapAcceptanceProjection(readmodelcore.Run{ID: "run-1", RuntimeState: &state})
	if projection.ActiveSliceID != "slice-1" || projection.CurrentFocus == nil || projection.CurrentFocus.ID != "focus-1" || projection.Budget == nil || projection.Budget.RepairAttempts != 2 {
		t.Fatalf("acceptance projection = %+v", projection)
	}
	if len(projection.Slices) != 1 || projection.Slices[0].State != "active" {
		t.Fatalf("slice projection = %+v", projection.Slices)
	}
	if projection.RepairDirective == nil || projection.RepairDirective.RequiredAction != string(runtimecontract.RuntimeActionProduceValidation) ||
		len(projection.Operations) != 1 || projection.Operations[0].Fence != 1 || projection.Operations[0].Outcome != string(runtimecontract.RuntimeOperationOutcomeAccepted) ||
		projection.Budget.CASReloadAttempts != 1 || projection.Operations[0].Kind != runtimecontract.StepKindGitRead || projection.Operations[0].VariantID != "status" ||
		projection.Operations[0].CapabilityStateHash != "capability-state-hash" || len(projection.CapabilityStates) != 1 ||
		len(projection.CapabilityStates[0].RequiredOperationSequence) != 1 || len(projection.Governance) != 1 || projection.Governance[0].ResolutionID != "resolution-1" {
		t.Fatalf("runtime remediation projection = %+v", projection)
	}
}

func strPtr(value string) *string { return &value }
