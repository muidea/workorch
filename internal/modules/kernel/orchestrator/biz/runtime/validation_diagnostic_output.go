package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeValidationDiagnosticOutputUnavailable = "validation_diagnostic_output_unavailable"

// SM-2 consumes the exact rejected receipt in its existing evidence CAS. Missing
// native output cannot authorize a model guess, repair target or same-state retry.
// The owner stage/exit and original failure remain immutable receipt facts.
func markValidationDiagnosticOutputUnavailable(aggregate *RuntimeAggregate, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact) (bool, error) {
	if admission.Evidence.Status != ExecutionEvidenceStatusRejected || fact.Kind != runtimecontract.EvidenceKindValidation || admission.Evidence.Receipt == nil || aggregate.Facts.Failure == nil {
		return false, nil
	}
	observation := admission.Evidence.Receipt.Evidence.Validation
	if observation == nil || observation.ExecutionConjunction == nil {
		return false, nil
	}
	for _, stage := range observation.ExecutionConjunction.Stages {
		for _, stream := range []struct {
			name   string
			output runtimecontract.RuntimeValidationOutputIdentity
		}{{"stdout", stage.Stdout}, {"stderr", stage.Stderr}} {
			reason := stream.output.OmittedReason
			if stream.output.Storage != nil && stream.output.Storage.OmittedReason != "" {
				reason = stream.output.Storage.OmittedReason
			}
			if stream.output.Complete && reason == "" {
				continue
			}
			failure := aggregate.Facts.Failure.Clone()
			failure.Code = runtimeValidationDiagnosticOutputUnavailable
			failure.Message = fmt.Sprintf("validation diagnostic unavailable: stage=%s stream=%s exit=%d knownFailure=%t saved=%d received=%d reason=%s; original failure=%s", stage.Identity.ID, stream.name, stage.ExitCode, stage.KnownFailure, stream.output.SavedBytes, stream.output.Bytes, reason, fact.FailureCode)
			if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}
