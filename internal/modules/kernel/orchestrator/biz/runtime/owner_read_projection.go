package runtime

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// EvidenceApplicabilityRead compiles only the next required owner query from
// current facts, when the drive actually asks for it. Loading or committing a
// state does not eagerly rebuild this unused projection.
func (a RuntimeAggregate) EvidenceApplicabilityRead() (rc.RuntimeEvidenceApplicabilityRequest, RuntimeOperationReadSource, bool, error) {
	return compileRuntimeEvidenceApplicabilityRead(a)
}

// AdmitRuntimeOwnerStateRecord is the single standalone raw-record admission
// used by explicit unaccepted-snapshot admission and offline audit. Conversion grants
// nothing; common DTO rules and aggregate-only joins execute in one pipeline.
func AdmitRuntimeOwnerStateRecord(record rc.RuntimeStateRecord) (RuntimeAggregate, error) {
	a := RuntimeAggregateFromRecord(record)
	err := rc.CompileRuntimeOwnerStateTransaction(func(validate func(rc.RuntimeOwnerStateSourceImage) error) error {
		return validate(ownerStateSourceImage(&record, a))
	})
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return a, nil
}

// ReadCommittedRuntimeOwnerStateRecord consumes only a record returned by the
// Run owner's integrity-checked query. It does not admit an external snapshot or
// recompile historical business decisions. New state still enters through the
// private event and CAS boundary; standalone snapshots use Admit instead.
func ReadCommittedRuntimeOwnerStateRecord(record rc.RuntimeStateRecord) (RuntimeAggregate, error) {
	if record.StateRevision == 0 || record.Facts.WorkspaceID == "" || record.Facts.RunID == "" ||
		record.Decision.AuthoritySnapshot.WorkspaceID != record.Facts.WorkspaceID || record.Decision.AuthoritySnapshot.RunID != record.Facts.RunID {
		return RuntimeAggregate{}, fmt.Errorf("%w: committed runtime record scope/revision mismatch", common.ErrConflict)
	}
	if err := rc.ValidateRuntimeResultFeedbackVersion(record.ResultFeedbackVersion); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rc.ValidateRuntimeStateTerminalDeliveryLedger(record.Decision, record.Termination, record.DeliveryAcknowledgements, record.Facts.Recovery); err != nil {
		return RuntimeAggregate{}, err
	}
	return RuntimeAggregate(record), nil
}
