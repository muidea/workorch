package proof

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SealedValidationOperation is the detached immutable causation surface that
// Runtime projects from a completed validation dispatch. It contains no
// aggregate, state, retry, or operation-selection authority.
type SealedValidationOperation struct {
	DispatchID          string
	AuthoritySnapshotID string
	OperationID         string
	TargetRole          string
	TargetRefs          []string
}

// RepairSurfaceConfirmationExists recognizes an exact persisted owner reply.
func RepairSurfaceConfirmationExists(records []runtimecontract.RuntimeRepairSurfaceConfirmationRecord, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	signature := runtimecontract.StableRuntimeRepairSurfaceConfirmationHash(confirmation)
	for _, record := range records {
		record = record.Normalized()
		if record.Signature == signature || runtimecontract.SameRuntimeRepairSurfaceConfirmation(record.Confirmation, confirmation) {
			return true
		}
	}
	return false
}

// RepairSurfaceConfirmationIdentityConflict detects a different response for
// the same owner observation/revision identity. It never decides how Runtime
// handles that conflict.
func RepairSurfaceConfirmationIdentityConflict(records []runtimecontract.RuntimeRepairSurfaceConfirmationRecord, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	identity := runtimecontract.StableRuntimeRepairSurfaceConfirmationIdentityHash(confirmation)
	for _, record := range records {
		record = record.Normalized()
		if record.IdentitySignature == identity || runtimecontract.SameRuntimeRepairSurfaceConfirmationIdentity(record.Confirmation, confirmation) {
			return !runtimecontract.SameRuntimeRepairSurfaceConfirmation(record.Confirmation, confirmation)
		}
	}
	return false
}

// ValidationFailureObservationMatchesSealedOperation binds diagnostic target
// candidates to the completed validation operation that produced them. It
// compares only typed immutable facts and does not consult a capability owner.
func ValidationFailureObservationMatchesSealedOperation(operations []SealedValidationOperation, observation runtimecontract.RuntimeValidationFailureObservation) bool {
	observation = observation.Normalized()
	for _, operation := range operations {
		if strings.TrimSpace(operation.DispatchID) != observation.DispatchID ||
			strings.TrimSpace(operation.AuthoritySnapshotID) != observation.AuthoritySnapshotID ||
			strings.TrimSpace(operation.OperationID) != observation.OperationID ||
			strings.TrimSpace(operation.TargetRole) != observation.TargetRole ||
			!grounding.SameTargetRefs(operation.TargetRefs, observation.ValidationScopeTargetRefs) {
			continue
		}
		return true
	}
	return false
}
