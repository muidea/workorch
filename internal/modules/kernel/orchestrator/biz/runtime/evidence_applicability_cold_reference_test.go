package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Test-only independent full-input query oracle.
func CompileRuntimeEvidenceApplicabilityRead(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, RuntimeOperationReadSource, bool, error) {
	if err := aggregate.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, RuntimeOperationReadSource{}, false, err
	}
	return compileRuntimeEvidenceApplicabilityRead(aggregate)
}

// Retired public raw wrappers are cold test oracles only.
func CompileRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate, evidenceID common.EvidenceRecordID) (runtimecontract.RuntimeEvidenceApplicabilityRequest, error) {
	if err := aggregate.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, err
	}
	return compileRuntimeEvidenceApplicabilityRequest(aggregate, evidenceID)
}

func NextRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, bool, error) {
	if err := aggregate.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, err
	}
	return nextRuntimeEvidenceApplicabilityRequest(aggregate)
}
