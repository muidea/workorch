package workorchruntimecontract

func referenceExecutionLineageBeforeConsumerReuse(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, evidenceRevision string, session *runtimeHistoricalPreimageSession) bool {
	if len(binding.LineageAssertions) == 0 && !runtimeProfilesDeclareLineageAssertions(sources.Facts.Activation.ExecutionContract.ProfileContracts) {
		return true
	}
	expected, err := runtimeExpectedLineageAssertions(binding, sources)
	if err != nil {
		return false
	}
	if len(expected) == 0 {
		return true
	}
	if binding.Validate() != nil || sources.Definition.StartupInputHash == "" {
		return false
	}
	executionRequired := false
	for _, assertion := range expected {
		executionRequired = executionRequired || assertion.Rule.ConsumptionPoint == RuntimeProofPhaseExecution
	}
	if !executionRequired {
		return true
	}
	if !runtimeCanonicalLineageCutoff(evidenceRevision) || ValidateRuntimeEvidenceLedgerSequence(sources.EvidenceSequence, sources.Facts, sources.Active, sources.Completed, sources.Resolutions) != nil || EvidenceRevisionAfter(evidenceRevision, sources.Facts.EvidenceRevision) {
		return false
	}
	for _, assertion := range expected {
		if assertion.Rule.ConsumptionPoint == RuntimeProofPhasePostTerminalDelivery {
			continue
		}
		proofBinding := binding
		if assertion.Rule.ExecutionSourceContractRef != "" {
			var err error
			proofBinding, err = runtimeExecutionProofSourceBindingInSession(assertion, sources, session)
			if err != nil {
				return false
			}
		}
		switch assertion.Rule.Kind {
		case RuntimeLineageCompletedWorkRounds:
			if !runtimeCompletedWorkRoundsInSession(proofBinding.CoverageWorkItemID, assertion.Rule.MinCompletedRounds, sources, evidenceRevision, session) {
				return false
			}
		case RuntimeLineageOwnerOperationReceipt:
			if !runtimeOwnerOperationReceiptInSession(binding.CoverageWorkItemID, assertion.Rule, sources, evidenceRevision, session) {
				return false
			}
		case RuntimeLineageRepairThenFreshValidation:
			satisfied := false
			if assertion.Rule.RepairHistoryContractRef == RuntimeConfirmedBindingRepairProofV1 {
				satisfied = runtimeConfirmedBindingRepairInSession(proofBinding, sources, evidenceRevision, session)
			} else if assertion.Rule.RepairHistoryContractRef == "" {
				satisfied = runtimeRepairThenFreshValidationInSession(proofBinding, sources, evidenceRevision, session)
			}
			if !satisfied {
				return false
			}
		default:
			return false
		}
	}
	return true
}
