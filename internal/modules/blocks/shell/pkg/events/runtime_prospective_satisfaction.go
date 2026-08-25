package events

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ProbeRuntimeOwnerFactProspectiveSatisfactionCommand runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest

type ProbeRuntimeOwnerFactProspectiveSatisfactionResult struct {
	Observation runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation
}

func ValidateRuntimeOwnerFactProspectiveSatisfactionCommand(command ProbeRuntimeOwnerFactProspectiveSatisfactionCommand, ownerModuleID, contractRef string) error {
	request := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest(command)
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	contractRef = strings.TrimSpace(contractRef)
	if ownerModuleID == "" || contractRef == "" || request.Requirement.OwnerModuleID != ownerModuleID ||
		request.ProspectiveSatisfactionContractRef != contractRef {
		return fmt.Errorf("runtime owner-fact prospective satisfaction request does not match Shell owner contract")
	}
	return request.Validate()
}

func NewRuntimeOwnerFactProspectiveSatisfactionResult(command ProbeRuntimeOwnerFactProspectiveSatisfactionCommand, outcome runtimecontract.RuntimeOwnerFactProspectiveSatisfactionOutcome, recipeRefs []string, reason string) (ProbeRuntimeOwnerFactProspectiveSatisfactionResult, error) {
	observation, err := runtimecontract.NewRuntimeOwnerFactProspectiveSatisfactionObservation(
		runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest(command), outcome, recipeRefs, reason,
	)
	if err != nil {
		return ProbeRuntimeOwnerFactProspectiveSatisfactionResult{}, err
	}
	return ProbeRuntimeOwnerFactProspectiveSatisfactionResult{Observation: observation}, nil
}

func NewRuntimeOwnerFactProspectiveSatisfactionStaleResult(command ProbeRuntimeOwnerFactProspectiveSatisfactionCommand, observedRevision, reason string) (ProbeRuntimeOwnerFactProspectiveSatisfactionResult, error) {
	observation, err := runtimecontract.NewRuntimeOwnerFactProspectiveSatisfactionStaleObservation(
		runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest(command), observedRevision, reason,
	)
	if err != nil {
		return ProbeRuntimeOwnerFactProspectiveSatisfactionResult{}, err
	}
	return ProbeRuntimeOwnerFactProspectiveSatisfactionResult{Observation: observation}, nil
}
