package workorchruntimecontract

import (
	"fmt"
)

func validateImplementationWorkingBindingSemanticSource(round RuntimeImplementationWorkRound, semantic RuntimeSemanticPlanningLedger) error {
	binding := round.WorkingBinding
	if binding == nil || semantic.Manifest == nil || semantic.Document == nil || semantic.Phase != RuntimeSemanticPlanningSealed || semantic.SealedBindingSetHash != binding.Hash || binding.SemanticManifestHash != semantic.Manifest.Hash {
		return fmt.Errorf("implementation working binding has no original sealed semantic transaction")
	}
	batchMatches := 0
	for _, batch := range semantic.Batches {
		if batch.BatchID == semantic.Document.BatchID && batch.State == RuntimeSemanticPlanBatchAccepted && batch.RequestLogicalID == binding.LogicalRequestID && batch.DocumentHash == semantic.DocumentHash {
			batchMatches++
		}
	}
	if batchMatches != 1 || !runtimeRawEqual(binding.Proposals, semantic.Document.Proposals) || !runtimeRawEqual(binding.Feasibilities, semantic.Feasibilities) {
		return fmt.Errorf("implementation working binding differs from original accepted document/owner feasibility")
	}
	for _, obligation := range semantic.Manifest.ProofObligations {
		if obligation.CoverageWorkItemID != round.CoverageWorkItemID || obligation.ImplementationWork == nil {
			continue
		}
		work := obligation.ImplementationWork
		reservation := round.Clone()
		reservation.WorkingBinding = nil
		reservation.DispatchID, reservation.DispatchAuthorityHash, reservation.CompletedReceiptSetHash, reservation.FailedReceiptHash = "", "", "", ""
		reservation.ReceiptDisposition = ""
		reservation.GovernanceReceiptHash, reservation.GovernanceDispatchIDs = "", nil
		reservation.OutcomeUnknownExecutionKey, reservation.OutcomeUnknownFence = "", 0
		reservation.Hash = StableRuntimeImplementationWorkRoundHash(reservation)
		if work.RoundHash != reservation.Hash || work.Ordinal != round.Ordinal || work.PolicyHash != round.PolicyHash || work.PrecedingReceiptSetHash != round.PrecedingReceiptSetHash || round.ScopeObservation == nil || !runtimeRawEqual(work.Scope, *round.ScopeObservation) || !runtimeRawEqual(work.MutationInputCatalogs, binding.MutationInputCatalogs) || !runtimeRawEqual(work.ExcludedTargetRefs, binding.ExcludedTargetRefs) || !runtimeRawEqual(work.Scope.Request.InputRevision, binding.InputRevision) {
			return fmt.Errorf("implementation working binding does not retain exact manifest reservation/protection source")
		}
		return nil
	}
	return fmt.Errorf("implementation working binding has no original semantic work purpose")
}

// ValidateRuntimeImplementationWorkingBindingSemanticSource joins the work
// commitment to exactly one original current or retired semantic transaction.
func ValidateRuntimeImplementationWorkingBindingSemanticSource(round RuntimeImplementationWorkRound, semantic RuntimeSemanticPlanningLedger) error {
	matches := 0
	if validateImplementationWorkingBindingSemanticSource(round, semantic) == nil {
		matches++
	}
	for _, generation := range semantic.AllHistory() {
		if validateImplementationWorkingBindingSemanticSource(round, generation.Snapshot) == nil {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("implementation work has no unique original semantic transaction")
	}
	return nil
}

// RuntimeImplementationWorkReceiptMembers projects only the original dispatch,
// receipt and evidence ledgers. No round labels or provider prose create facts.
func RuntimeImplementationWorkReceiptMembers(dispatch RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, facts RuntimeFacts, governance []RuntimeOperationGovernanceBinding) ([]RuntimeImplementationWorkReceiptMember, error) {
	return implementationWorkReceiptMembersBefore(dispatch, receipts, facts, governance, "")
}

func implementationWorkReceiptMembersBefore(dispatch RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, facts RuntimeFacts, governance []RuntimeOperationGovernanceBinding, beforeOperationID string) ([]RuntimeImplementationWorkReceiptMember, error) {
	members := []RuntimeImplementationWorkReceiptMember{}
	for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
		if operation.ID == beforeOperationID {
			break
		}
		var execution RuntimeOperationExecutionRecord
		executionMatches := 0
		for _, value := range dispatch.OperationExecutions {
			if value.OperationID == operation.ID {
				execution, executionMatches = value, executionMatches+1
			}
		}
		if executionMatches != 1 {
			return nil, fmt.Errorf("implementation work operation has no unique sealed execution")
		}
		if execution.State != OperationExecutionEvidenceCommitted {
			continue
		}
		var receipt RuntimeOperationReceipt
		receiptMatches := 0
		for _, value := range receipts {
			if value.ExecutionKey == execution.ExecutionKey && value.Fence == execution.Fence {
				receipt, receiptMatches = value, receiptMatches+1
			}
		}
		var evidence EvidenceFact
		evidenceMatches := 0
		for _, value := range facts.Evidence.Accepted {
			if value.ID == execution.EvidenceFactID {
				evidence, evidenceMatches = value, evidenceMatches+1
			}
		}
		if receiptMatches != 1 || evidenceMatches != 1 {
			return nil, fmt.Errorf("implementation effect has no unique persisted owner receipt/evidence")
		}
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		if !known {
			return nil, fmt.Errorf("implementation effect operation owner is unknown")
		}
		effectDispatchID, err := RuntimeOperationExecutionKeyOrigin(facts.WorkspaceID, facts.RunID, execution.ExecutionKey, operation.ID)
		if err != nil {
			return nil, err
		}
		grantMatches := 0
		for _, binding := range governance {
			if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil && binding.ExecutionKey == execution.ExecutionKey && binding.OperationID == operation.ID && binding.Grant.ResumeFence == execution.Fence {
				if err := binding.Grant.Validate(); err != nil {
					return nil, err
				}
				if binding.Grant.ExecutionKey != execution.ExecutionKey {
					return nil, fmt.Errorf("implementation effect governance grant key mismatch")
				}
				grantMatches++
				effectDispatchID = binding.Grant.ResumeDispatchID
			}
		}
		if grantMatches > 1 {
			return nil, fmt.Errorf("implementation effect has multiple original governance hops")
		}
		members = append(members, RuntimeImplementationWorkReceiptMember{OwnerModuleID: descriptor.OwnerModuleID, DispatchID: effectDispatchID, Execution: execution, Operation: operation, Receipt: receipt, Evidence: evidence})
	}
	return members, nil
}

// ValidateRuntimeImplementationWorkState checks original source joins at the
// aggregate load/CAS boundary. Historical scope is not compared to today's
// resources; the immutable dispatch and earlier semantic transaction retain it.
func ValidateRuntimeImplementationWorkState(definition RuntimeDefinitionContext, facts RuntimeFacts, ledger RuntimeImplementationWorkLedger, semantic RuntimeSemanticPlanningLedger, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, governance []RuntimeOperationGovernanceBinding, budget RuntimeExecutionBudgetLedger) error {
	return validateRuntimeImplementationWorkStateComplete(definition, facts, ledger, semantic, active, completed, receipts, governance, budget, newRuntimeSemanticValidationSession())
}

func validateRuntimeImplementationWorkStateInSession(definition RuntimeDefinitionContext, facts RuntimeFacts, ledger RuntimeImplementationWorkLedger, semantic RuntimeSemanticPlanningLedger, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, governance []RuntimeOperationGovernanceBinding, budget RuntimeExecutionBudgetLedger, semanticSession *runtimeSemanticValidationSession) error {
	if semanticSession == nil {
		return ValidateRuntimeImplementationWorkState(definition, facts, ledger, semantic, active, completed, receipts, governance, budget)
	}
	raw := RuntimeStateRecord{Definition: definition, Facts: facts, ImplementationWork: ledger, SemanticPlanning: semantic, ActiveDispatch: active, CompletedDispatches: completed, OperationReceipts: receipts, OperationGovernanceBindings: governance, ExecutionBudget: budget}
	for _, prior := range semanticSession.workStates {
		if runtimeRawEqual(prior, raw) {
			return nil
		}
	}
	if err := validateRuntimeImplementationWorkStateComplete(definition, facts, ledger, semantic, active, completed, receipts, governance, budget, semanticSession); err != nil {
		return err
	}
	semanticSession.workStates = append(semanticSession.workStates, raw.Clone())
	return nil
}

// The complete original gate runs before any successful raw preimage is saved.
// A public independent consumer always executes this gate afresh.
func validateRuntimeImplementationWorkStateComplete(definition RuntimeDefinitionContext, facts RuntimeFacts, ledger RuntimeImplementationWorkLedger, semantic RuntimeSemanticPlanningLedger, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, receipts []RuntimeOperationReceipt, governance []RuntimeOperationGovernanceBinding, budget RuntimeExecutionBudgetLedger, semanticSession *runtimeSemanticValidationSession) error {
	if err := validateRuntimeInputRevisionState(definition, facts, ledger, semantic); err != nil {
		return err
	}
	if preview := ledger.ScopePreview; preview != nil {
		previewSources := RuntimeImplementationWorkScopeSources{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: preview.Request.AuthoritySnapshotID, Seed: facts.DefinitionSeed, Focuses: facts.DefinitionFocuses, Profiles: facts.Activation.ExecutionContract.ProfileContracts, CoverageWorkItemID: preview.Request.CoverageWorkItemID, ManifestBinding: preview.Request.ManifestBinding, InputRevision: preview.Request.InputRevision}
		if err := validateRuntimeWorkScopeInSession(preview.Request, previewSources, semanticSession); err != nil {
			return err
		}
		if preview.Observation != nil {
			if err := preview.Observation.ValidateForRequest(preview.Request); err != nil {
				return err
			}
		}
		if facts.Resources.Manifest == nil || facts.Resources.Manifest.Revision != preview.Request.ManifestBinding.Manifest.Revision {
			return fmt.Errorf("implementation purpose preview requires current owner manifest")
		}
	}
	if len(ledger.PurposeChoices) > len(facts.DefinitionSeed.CoverageItems)*(MaxRuntimeImplementationWorkRounds+1) {
		return fmt.Errorf("implementation purpose choices exceed bounded per-CWI history")
	}
	seenChoices := map[string]bool{}
	for _, choice := range ledger.PurposeChoices {
		if choice.Purpose != RuntimeImplementationWorkPurposeEffect && choice.Purpose != RuntimeImplementationWorkPurposeTerminal || seenChoices[choice.SemanticManifestHash] {
			return fmt.Errorf("implementation purpose choice is invalid or duplicated")
		}
		seenChoices[choice.SemanticManifestHash] = true
		matches := 0
		for _, history := range semantic.AllHistory() {
			snapshot := history.Snapshot
			if history.Reason != RuntimeSemanticPlanningRetirementImplementationPurpose || snapshot.Manifest == nil || snapshot.CoverageOrderDocument == nil || snapshot.Manifest.Hash != choice.SemanticManifestHash || snapshot.LogicalRequestID != choice.LogicalRequestID || snapshot.DocumentHash != choice.DocumentHash || StableRuntimeSemanticPlanDocumentHash(*snapshot.CoverageOrderDocument) != choice.DocumentHash || snapshot.CoverageOrderDocument.ImplementationPurpose != choice.Purpose {
				continue
			}
			if len(snapshot.Manifest.ProofObligations) != 1 || snapshot.Manifest.ProofObligations[0].ImplementationWork == nil {
				return fmt.Errorf("implementation purpose choice has no original one-CWI manifest")
			}
			work := snapshot.Manifest.ProofObligations[0].ImplementationWork
			prefixRounds := []RuntimeImplementationWorkRound{}
			for _, prior := range ledger.Rounds {
				if prior.CoverageWorkItemID == choice.CoverageWorkItemID && prior.Ordinal < work.Ordinal {
					prefixRounds = append(prefixRounds, prior)
				}
			}
			previewSource := RuntimeImplementationWorkScopePreview{Request: work.Scope.Request, Observation: &work.Scope}
			expectedManifest, compileErr := compileRuntimeWorkPurposeInSession(facts.WorkspaceID, facts.RunID, definition, facts.DefinitionSeed, facts.DefinitionFocuses, facts.Activation.ExecutionContract, work.MutationInputCatalogs, previewSource, prefixRounds, semanticSession)
			if compileErr != nil {
				return compileErr
			}
			expectedManifest.InputReviewInformation = snapshot.Manifest.InputReviewInformation
			expectedManifest.Hash = StableRuntimeSemanticPlanManifestHash(expectedManifest)
			if !runtimeRawEqual(expectedManifest, *snapshot.Manifest) {
				return fmt.Errorf("implementation purpose vocabulary differs from independent prior collective receipts")
			}
			if !work.PurposeChoiceOnly || work.Scope.Request.RequestID != choice.ScopeRequestID || work.Scope.Request.CoverageWorkItemID != choice.CoverageWorkItemID || choice.Purpose == RuntimeImplementationWorkPurposeEffect && !work.EffectAvailable {
				return fmt.Errorf("implementation purpose choice differs from original vocabulary")
			}
			choiceScopeSources := RuntimeImplementationWorkScopeSources{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: work.Scope.Request.AuthoritySnapshotID, Seed: facts.DefinitionSeed, Focuses: facts.DefinitionFocuses, Profiles: facts.Activation.ExecutionContract.ProfileContracts, CoverageWorkItemID: work.Scope.Request.CoverageWorkItemID, ManifestBinding: work.Scope.Request.ManifestBinding, InputRevision: work.Scope.Request.InputRevision}
			if err := validateRuntimeWorkScopeInSession(work.Scope.Request, choiceScopeSources, semanticSession); err != nil {
				return err
			}
			matches++
		}
		if matches != 1 {
			return fmt.Errorf("implementation purpose choice has no unique original accepted order")
		}
	}
	if len(ledger.Rounds) == 0 {
		if active != nil && active.ImplementationWorkCommitment != nil {
			return fmt.Errorf("implementation dispatch has no private round ledger")
		}
		for _, dispatch := range completed {
			if dispatch.ImplementationWorkCommitment != nil {
				return fmt.Errorf("historical implementation dispatch has no private round ledger")
			}
		}
		return nil
	}
	sources := RuntimeImplementationWorkLedgerSources{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, StartupInputHash: definition.StartupInputHash,
		Seed: facts.DefinitionSeed, Focuses: facts.DefinitionFocuses, Profiles: facts.Activation.ExecutionContract.ProfileContracts,
		MaxRecords: len(facts.DefinitionSeed.CoverageItems) * MaxRuntimeImplementationWorkRounds}
	dispatches := append([]RuntimeDispatchRecord(nil), completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	joined := map[string]bool{}
	outstanding := 0
	for _, round := range ledger.Rounds {
		choiceMatches := 0
		for _, choice := range ledger.PurposeChoices {
			if choice.Purpose == RuntimeImplementationWorkPurposeEffect && choice.CoverageWorkItemID == round.CoverageWorkItemID && choice.ScopeRequestID == round.ScopeRequest.RequestID {
				choiceMatches++
			}
		}
		if choiceMatches != 1 {
			return fmt.Errorf("implementation effect round requires unique accepted original purpose choice")
		}
		if round.CompletedReceiptSetHash == "" && round.FailedReceiptHash == "" {
			outstanding++
		}
		if round.WorkingBinding != nil {
			if err := ValidateRuntimeImplementationWorkingBindingSemanticSource(round, semantic); err != nil {
				return err
			}
		}
		if round.DispatchID == "" {
			if facts.Resources.Manifest == nil || facts.Resources.Manifest.Revision != round.ScopeRequest.ManifestBinding.Manifest.Revision {
				return fmt.Errorf("implementation work scope reservation has no current workspace manifest")
			}
			matches := 0
			for _, observation := range facts.Resources.PrerequisiteObservations {
				if observation.PrerequisiteID == round.ScopeRequest.ManifestBinding.Observation.PrerequisiteID && observation.ObservationID == round.ScopeRequest.ManifestBinding.Observation.ObservationID && observation.OwnerModuleID == round.ScopeRequest.Policy.OwnerModuleID && observation.ContentHash == facts.Resources.Manifest.Revision {
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("implementation work scope reservation has no unique current owner observation")
			}
			continue
		}
		for i, sourceID := range round.GovernanceDispatchIDs {
			nextID := round.DispatchID
			if i+1 < len(round.GovernanceDispatchIDs) {
				nextID = round.GovernanceDispatchIDs[i+1]
			}
			var source, next RuntimeDispatchRecord
			sourceMatches, nextMatches := 0, 0
			for _, dispatch := range dispatches {
				if dispatch.ID == sourceID {
					source, sourceMatches = dispatch, sourceMatches+1
				}
				if dispatch.ID == nextID {
					next, nextMatches = dispatch, nextMatches+1
				}
			}
			if sourceMatches != 1 || nextMatches != 1 || source.State != RuntimeDispatchStateCommitted || !runtimeImplementationWorkCommitmentsEqual(source.ImplementationWorkCommitment, next.ImplementationWorkCommitment) {
				return fmt.Errorf("implementation governance history lost immutable work purpose")
			}
			grants := 0
			for _, binding := range governance {
				if binding.OperationDispatchID == sourceID && binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil && binding.Grant.ResumeDispatchID == nextID && runtimeGovernanceResumeAdmissionInheritsAuthorityCommitments(binding, next, dispatches) {
					grants++
				}
			}
			if grants != 1 {
				return fmt.Errorf("implementation governance history requires one exact authorized continuation")
			}
			joined[sourceID] = true
		}
		matches := 0
		for _, dispatch := range dispatches {
			if dispatch.ID != round.DispatchID {
				continue
			}
			matches++
			if err := validateRuntimeImplementationWorkDispatchCommitmentInSession(dispatch, semanticSession); err != nil {
				return err
			}
			preDispatch := round.Clone()
			preDispatch.DispatchID, preDispatch.DispatchAuthorityHash, preDispatch.CompletedReceiptSetHash, preDispatch.FailedReceiptHash = "", "", "", ""
			preDispatch.ReceiptDisposition = ""
			preDispatch.GovernanceReceiptHash, preDispatch.GovernanceDispatchIDs = "", nil
			preDispatch.OutcomeUnknownExecutionKey, preDispatch.OutcomeUnknownFence = "", 0
			preDispatch.Hash = StableRuntimeImplementationWorkRoundHash(preDispatch)
			if !runtimeRawEqual(dispatch.ImplementationWorkCommitment, &preDispatch) || round.DispatchAuthorityHash != dispatch.DispatchAuthorityHash || dispatch.DispatchAuthorityHash != StableRuntimeDispatchAuthorityHash(dispatch) {
				return fmt.Errorf("implementation round differs from original pre-dispatch commitment")
			}
			joined[dispatch.ID] = true
			if round.WorkingBinding.ReceiptProtocolRef == RuntimeImplementationWorkReceiptChainV1 {
				for _, execution := range dispatch.OperationExecutions {
					if execution.State != OperationExecutionEvidenceCommitted {
						continue
					}
					for _, fact := range facts.Evidence.Accepted {
						if fact.ID != execution.EvidenceFactID {
							continue
						}
						receipt, found := RuntimeOperationReceiptForExecutionKeyFence(receipts, execution.ExecutionKey, execution.Fence)
						if !found {
							return fmt.Errorf("implementation work committed evidence lost its resource chain receipt")
						}
						disposition, err := RuntimeImplementationWorkDispositionForDispatchReceipt(round, dispatch, receipts, facts, governance, receipt)
						if err != nil {
							return err
						}
						if disposition != "" {
							return fmt.Errorf("implementation work committed evidence has invalid resource chain")
						}
					}
				}
			}
			if round.CompletedReceiptSetHash != "" {
				if dispatch.State != RuntimeDispatchStateCommitted {
					return fmt.Errorf("implementation completion requires committed original dispatch")
				}
				members, err := RuntimeImplementationWorkReceiptMembers(dispatch, receipts, facts, governance)
				if err != nil {
					return err
				}
				sources.ReceiptSets = append(sources.ReceiptSets, RuntimeImplementationWorkReceiptSet{DispatchID: dispatch.ID, Members: members})
			} else if round.FailedReceiptHash != "" {
				governanceClosure := round.ReceiptDisposition == RuntimeImplementationWorkReceiptGovernance
				if governanceClosure && dispatch.State != RuntimeDispatchStateCommitted || !governanceClosure && (dispatch.State != RuntimeDispatchStateInvalidated || facts.Failure == nil || facts.Failure.Code != "implementation_work_"+string(round.ReceiptDisposition)) {
					return fmt.Errorf("closed implementation round needs original dispatch and exact typed disposition")
				}
				if governanceClosure {
					governanceSources := 0
					for _, execution := range dispatch.OperationExecutions {
						for _, receipt := range receipts {
							if execution.OwnerReceiptHash == round.FailedReceiptHash && receipt.ExecutionKey == execution.ExecutionKey && receipt.Fence == execution.Fence && receipt.OperationID == execution.OperationID && receipt.Outcome == RuntimeOperationOutcomeGovernanceRequired && receipt.Governance != nil && StableRuntimeOperationReceiptHash(receipt) == round.FailedReceiptHash {
								governanceSources++
							}
						}
					}
					if governanceSources != 1 {
						return fmt.Errorf("implementation governance closure requires exact original owner requirement")
					}
				}
				closedMatches := 0
				for _, execution := range dispatch.OperationExecutions {
					if execution.OwnerReceiptHash != round.FailedReceiptHash {
						continue
					}
					originalReceiptMatches := 0
					for _, receipt := range receipts {
						if receipt.ExecutionKey == execution.ExecutionKey && receipt.Fence == execution.Fence && receipt.OperationID == execution.OperationID && StableRuntimeOperationReceiptHash(receipt) == round.FailedReceiptHash && execution.OwnerReceiptRef == "runtime-receipt/"+receipt.ExecutionKey {
							disposition, err := RuntimeImplementationWorkDispositionForDispatchReceipt(round, dispatch, receipts, facts, governance, receipt)
							if err != nil {
								return err
							}
							if disposition == round.ReceiptDisposition {
								originalReceiptMatches++
							}
						}
					}
					if originalReceiptMatches != 1 {
						return fmt.Errorf("closed implementation round has no unique original receipt/key/fence/outcome")
					}
					if execution.State == OperationExecutionOwnerCommitted {
						if execution.EvidenceFactID != "" || execution.EvidenceFactHash != "" {
							return fmt.Errorf("dedicated implementation disposition cannot claim ordinary evidence")
						}
						closedMatches++
						continue
					}
					if round.ReceiptDisposition != RuntimeImplementationWorkReceiptRejected || execution.State != OperationExecutionEvidenceCommitted {
						return fmt.Errorf("ordinary rejected implementation effect requires original evidence-committed execution")
					}
					for _, fact := range facts.Evidence.Rejected {
						if fact.ID == execution.EvidenceFactID && StableRuntimeEvidenceFactHash(fact) == execution.EvidenceFactHash && fact.DispatchID == dispatch.ID && fact.OperationID == execution.OperationID && len(fact.BindingRefs) == 0 && len(fact.CoverageWorkItemIDs) == 0 && fact.Validation == nil {
							closedMatches++
						}
					}
				}
				if closedMatches != 1 {
					return fmt.Errorf("closed implementation round has no exact no-credit disposition source")
				}
			} else if round.OutcomeUnknownExecutionKey != "" {
				if dispatch.State != RuntimeDispatchStateInvalidated || facts.Failure == nil || facts.Failure.Class != FailureClassOperationOutcomeUnknown || facts.Failure.Code != "implementation_work_outcome_unknown" {
					return fmt.Errorf("implementation unknown physical claim requires bounded original failure disposition")
				}
				matches := 0
				for _, execution := range dispatch.OperationExecutions {
					if execution.ExecutionKey != round.OutcomeUnknownExecutionKey || execution.Fence != round.OutcomeUnknownFence {
						continue
					}
					if err := execution.Validate(); err != nil {
						return err
					}
					receiptMatches := 0
					for _, receipt := range receipts {
						if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence {
							continue
						}
						receiptMatches++
						if err := receipt.Validate(); err != nil {
							return err
						}
						if execution.State != OperationExecutionOwnerCommitted || receipt.OperationID != execution.OperationID || execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || execution.OwnerReceiptHash != StableRuntimeOperationReceiptHash(receipt) {
							return fmt.Errorf("implementation late-known receipt lost original no-credit execution source")
						}
						operationMatches := 0
						for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
							if operation.ID == execution.OperationID && sameRuntimeStringSet(receipt.TargetRefs, operation.TargetRefs) {
								operationMatches++
							}
						}
						if operationMatches != 1 {
							return fmt.Errorf("implementation late-known receipt lost original sealed targets")
						}
					}
					if receiptMatches > 1 || receiptMatches == 0 && execution.State != OperationExecutionOutcomeUnknown {
						return fmt.Errorf("implementation unknown claim requires absent receipt or unique archived late-known receipt")
					}
					if err := validateImplementationWorkUnknownConsumption(dispatch, execution, budget); err != nil {
						return err
					}
					matches++
				}
				if matches != 1 {
					return fmt.Errorf("implementation unknown physical claim has no original execution")
				}
			} else if round.GovernanceReceiptHash != "" {
				if dispatch.State != RuntimeDispatchStateCommitted {
					return fmt.Errorf("implementation suspension requires original governance-committed dispatch")
				}
				matches := 0
				for _, execution := range dispatch.OperationExecutions {
					for _, receipt := range receipts {
						if execution.State == OperationExecutionOwnerCommitted && execution.OwnerReceiptHash == round.GovernanceReceiptHash && receipt.ExecutionKey == execution.ExecutionKey && receipt.Fence == execution.Fence && receipt.OperationID == execution.OperationID && receipt.Outcome == RuntimeOperationOutcomeGovernanceRequired && receipt.Governance != nil && StableRuntimeOperationReceiptHash(receipt) == round.GovernanceReceiptHash {
							matches++
						}
					}
				}
				if matches != 1 {
					return fmt.Errorf("implementation suspension has no unique original governance receipt")
				}
				bindings := 0
				for _, binding := range governance {
					if binding.OperationDispatchID == dispatch.ID && binding.GovernanceReceiptHash == round.GovernanceReceiptHash {
						bindings++
					}
				}
				if bindings != 1 {
					return fmt.Errorf("implementation suspension has no exact governance binding")
				}
			} else if dispatch.State != RuntimeDispatchStateSealed {
				return fmt.Errorf("unsettled implementation round cannot hide a closed dispatch")
			}
		}
		if matches != 1 {
			return fmt.Errorf("implementation round has no unique original dispatch")
		}
	}
	if outstanding > 1 {
		return fmt.Errorf("implementation work ledger has multiple outstanding rounds")
	}
	for _, dispatch := range dispatches {
		if dispatch.ImplementationWorkCommitment != nil && !joined[dispatch.ID] {
			return fmt.Errorf("implementation dispatch is not joined to private round ledger")
		}
	}
	return ledger.validateForSourcesInSemanticSession(sources, semanticSession)
}

// 原无 receipt claim 的 unknown 消费是关闭事实；迟到 receipt 不得确认或释放它。
func validateImplementationWorkUnknownConsumption(dispatch RuntimeDispatchRecord, execution RuntimeOperationExecutionRecord, budget RuntimeExecutionBudgetLedger) error {
	scopeRef := fmt.Sprintf("%s/%d", execution.ExecutionKey, execution.Fence)
	var allocation RuntimeBudgetAllocation
	allocations := 0
	for _, value := range budget.Allocations {
		if value.Dimension == RuntimeBudgetDimensionCapabilityOperation && value.LogicalConsumptionID == execution.ExecutionKey && value.ScopeRef == scopeRef {
			allocation = value
			allocations++
		}
	}
	if allocations != 1 || allocation.State != RuntimeBudgetAllocationUnknown || allocation.ReservedAmount != 1 {
		return fmt.Errorf("implementation unknown physical claim lost original occupied allocation")
	}
	owner := ""
	for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
		if operation.ID == execution.OperationID {
			owner = RuntimeModuleForOperationKind(operation.Kind)
		}
	}
	consumptions := 0
	for _, value := range budget.Consumptions {
		if value.AllocationID != allocation.AllocationID {
			continue
		}
		if owner == "" || value.Owner != owner || value.LogicalConsumptionID != execution.ExecutionKey || value.PhysicalConsumptionID != scopeRef || value.ConfirmedAmount != 0 || value.UnknownAmount != 1 || value.OwnerResultRef != "runtime-receipt/"+execution.ExecutionKey || value.OwnerResultHash != "runtime-operation-outcome-unknown/"+scopeRef {
			return fmt.Errorf("implementation unknown physical claim consumption differs from original no-receipt closure")
		}
		consumptions++
	}
	if consumptions != 1 {
		return fmt.Errorf("implementation unknown physical claim requires unique original unknown consumption")
	}
	return nil
}
