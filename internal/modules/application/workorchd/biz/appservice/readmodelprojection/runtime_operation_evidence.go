package readmodelprojection

import (
	"strings"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// committedRuntimeOperationEvidenceRefs projects the canonical identity of
// every operation whose evidence transition is already present in the bounded
// Acceptance read model. It deliberately excludes owner-committed,
// governance-pending and outcome-unknown executions: those are operation
// observations, but no SM-3 evidence commit exists yet.
func committedRuntimeOperationEvidenceRefs(operations []projectioncontract.RuntimeOperationStatus) []string {
	refs := make([]string, 0, len(operations))
	for _, operation := range operations {
		if strings.TrimSpace(operation.ExecutionState) != string(runtimecontract.OperationExecutionEvidenceCommitted) {
			continue
		}
		if ref := orchestratorRuntimeOperationEvidenceRef(operation.DispatchID, operation.OperationID); ref != "" {
			refs = append(refs, ref)
		}
	}
	return compactUniqueStrings(refs)
}

func orchestratorRuntimeOperationEvidenceRef(dispatchID string, operationID string) string {
	dispatchID = strings.TrimSpace(dispatchID)
	operationID = strings.TrimSpace(operationID)
	if dispatchID == "" || operationID == "" {
		return ""
	}
	return orchestratorevents.RuntimeOperationSourceRef(dispatchID, operationID)
}
