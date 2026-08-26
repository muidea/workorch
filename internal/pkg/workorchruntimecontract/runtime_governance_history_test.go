package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateRuntimeOperationExecutionContextScopeRequiresPriorKeyForGovernanceResume(t *testing.T) {
	workspaceID := common.WorkspaceID("workspace-1")
	runID := common.RunID("run-1")
	operationID := "op-1"
	resumeDispatchID := "resume-dispatch-1"
	priorKey := StableRuntimeOperationExecutionKey(workspaceID, runID, "source-dispatch-1", operationID)
	grant := RuntimeOperationGovernanceGrant{ResumeDispatchID: resumeDispatchID}
	context := RuntimeOperationExecutionContext{
		WorkspaceID: workspaceID,
		RunID:       runID,
		DispatchID:  resumeDispatchID,
		OperationID: operationID,
		Execution: RuntimeOperationExecutionEnvelope{
			ExecutionKey:    priorKey,
			GovernanceGrant: &grant,
		},
	}
	if err := ValidateRuntimeOperationExecutionContextScope(context); err != nil {
		t.Fatalf("valid governance resume context: %v", err)
	}

	currentOrigin := context
	currentOrigin.Execution.ExecutionKey = StableRuntimeOperationExecutionKey(workspaceID, runID, resumeDispatchID, operationID)
	if err := ValidateRuntimeOperationExecutionContextScope(currentOrigin); err == nil {
		t.Fatal("governance grant with a current-dispatch execution-key origin was accepted")
	}

	wrongDestination := context
	wrongGrant := grant
	wrongGrant.ResumeDispatchID = "resume-dispatch-other"
	wrongDestination.Execution.GovernanceGrant = &wrongGrant
	if err := ValidateRuntimeOperationExecutionContextScope(wrongDestination); err == nil {
		t.Fatal("governance grant bound to another resume dispatch was accepted")
	}
}

func TestValidateRuntimeDispatchExecutionKeyLineageRequiresCommittedOrderedGovernanceHistory(t *testing.T) {
	workspaceID, runID, source, governance, resume, binding := runtimeGovernanceDispatchHistoryFixture()
	if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{source, governance}, []RuntimeOperationGovernanceBinding{binding}); err != nil {
		t.Fatalf("valid ordered governance continuation history: %v", err)
	}

	t.Run("source is not committed", func(t *testing.T) {
		invalidSource := source
		invalidSource.State = RuntimeDispatchStateInvalidated
		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{invalidSource, governance}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("governance continuation from an invalidated source dispatch was accepted")
		}
	})

	t.Run("governance dispatch follows source and precedes resume", func(t *testing.T) {
		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{governance, source}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("governance continuation with governance dispatch before its source was accepted")
		}

		completedResume := resume
		completedResume.State = RuntimeDispatchStateCommitted
		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, nil, []RuntimeDispatchRecord{source, completedResume, governance}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("governance continuation with governance dispatch after its resume was accepted")
		}
	})

	t.Run("governance dispatch is committed governance authority", func(t *testing.T) {
		invalidState := governance
		invalidState.State = RuntimeDispatchStateInvalidated
		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{source, invalidState}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("continuation from a non-committed governance dispatch was accepted")
		}

		missingCandidate := governance
		missingCandidate.Candidate.Governance = nil
		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{source, missingCandidate}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("continuation from a non-governance dispatch was accepted")
		}

		wrongSnapshot := governance
		wrongSnapshot.Transaction.AuthoritySnapshotID = "governance-snapshot-other"
		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{source, wrongSnapshot}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("continuation from a different governance authority snapshot was accepted")
		}

		if err := ValidateRuntimeDispatchExecutionKeyLineage(workspaceID, runID, &resume, []RuntimeDispatchRecord{source}, []RuntimeOperationGovernanceBinding{binding}); err == nil {
			t.Fatal("continuation without its exact governance dispatch was accepted")
		}
	})
}

func runtimeGovernanceDispatchHistoryFixture() (common.WorkspaceID, common.RunID, RuntimeDispatchRecord, RuntimeDispatchRecord, RuntimeDispatchRecord, RuntimeOperationGovernanceBinding) {
	workspaceID := common.WorkspaceID("workspace-1")
	runID := common.RunID("run-1")
	operationID := "op-1"
	sourceID := "operation-dispatch-1"
	governanceID := "governance-dispatch-1"
	resumeID := "resume-dispatch-1"
	executionKey := StableRuntimeOperationExecutionKey(workspaceID, runID, sourceID, operationID)

	sourceExecution := RuntimeOperationExecutionRecord{
		OperationID:         operationID,
		OperationHash:       "operation-hash-1",
		ExecutionKey:        executionKey,
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    RuntimeOperationIdempotencyAtMostOnce,
		State:               OperationExecutionOwnerCommitted,
		Fence:               4,
		OwnerReceiptHash:    "governance-receipt-hash-1",
	}
	sourceExecution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(sourceExecution)
	source := RuntimeDispatchRecord{
		ID:                    sourceID,
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		CandidateHash:         "candidate-hash-1",
		State:                 RuntimeDispatchStateCommitted,
		CompletedOperationIDs: []string{operationID},
		OperationExecutions:   []RuntimeOperationExecutionRecord{sourceExecution},
	}

	governance := RuntimeDispatchRecord{
		ID:          governanceID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Transaction: AuthorityTransaction{AuthoritySnapshotID: "governance-snapshot-1"},
		Candidate:   RuntimeCandidate{Governance: &GovernanceCandidate{}},
		State:       RuntimeDispatchStateCommitted,
	}

	grant := RuntimeOperationGovernanceGrant{
		ExecutionKey:          executionKey,
		GovernanceFence:       sourceExecution.Fence,
		ResumeFence:           sourceExecution.Fence + 1,
		ResumeDispatchID:      resumeID,
		GovernanceReceiptHash: sourceExecution.OwnerReceiptHash,
	}
	resumeExecution := sourceExecution.Clone()
	resumeExecution.State = OperationExecutionClaimed
	resumeExecution.Fence = grant.ResumeFence
	resumeExecution.ClaimedAt = RuntimeOperationClaimMarkerGovernanceGranted
	resumeExecution.OwnerReceiptHash = ""
	resume := RuntimeDispatchRecord{
		ID:                  resumeID,
		WorkspaceID:         workspaceID,
		RunID:               runID,
		CandidateHash:       source.CandidateHash,
		State:               RuntimeDispatchStateSealed,
		OperationExecutions: []RuntimeOperationExecutionRecord{resumeExecution},
	}
	binding := RuntimeOperationGovernanceBinding{
		GovernanceDispatchID:   governanceID,
		GovernanceSnapshotID:   governance.Transaction.AuthoritySnapshotID,
		OperationDispatchID:    sourceID,
		OperationID:            operationID,
		ExecutionKey:           executionKey,
		Fence:                  sourceExecution.Fence,
		ExpectedEffectScope:    sourceExecution.ExpectedEffectScope,
		IdempotencyClass:       sourceExecution.IdempotencyClass,
		ExecutionAuthorityHash: sourceExecution.AuthorityHash,
		GovernanceReceiptHash:  sourceExecution.OwnerReceiptHash,
		State:                  RuntimeOperationGovernanceBindingGranted,
		Grant:                  &grant,
	}
	return workspaceID, runID, source, governance, resume, binding
}
