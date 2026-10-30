package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// ValidateRuntimeImplementationWorkDispatchCommitment checks immutable purpose,
// target and no-credit constraints. Aggregate validation additionally replays the
// earlier semantic and request-session sources; this check is not admission.
func ValidateRuntimeImplementationWorkDispatchCommitment(d RuntimeDispatchRecord) error {
	return validateRuntimeImplementationWorkDispatchCommitmentInSession(d, nil)
}

func validateRuntimeImplementationWorkDispatchCommitmentInSession(d RuntimeDispatchRecord, session *runtimeSemanticValidationSession) error {
	round := d.ImplementationWorkCommitment
	if round == nil {
		return nil
	}
	if err := round.validateInSemanticSession(session); err != nil {
		return err
	}
	if round.WorkspaceID != d.WorkspaceID || round.RunID != d.RunID || round.WorkingBinding == nil || round.DispatchID != "" || round.DispatchAuthorityHash != "" || round.CompletedReceiptSetHash != "" || round.FailedReceiptHash != "" ||
		d.Transaction.State != ConvergenceStateNeedStateChange || d.Candidate.StateChange == nil || d.RepairCommitment != nil || d.AssessmentAuthorityInput != nil || len(d.CoverageBindings) != 0 || len(d.ValidationRecipes) != 0 {
		return fmt.Errorf("implementation dispatch requires exact pre-dispatch working purpose without terminal/repair authority")
	}
	wanted := map[string]RuntimeSemanticProofProposal{}
	for _, proposal := range round.WorkingBinding.Proposals {
		for _, target := range RuntimeSemanticProposalTargetRefs(proposal) {
			wanted[target] = proposal
		}
	}
	operations := runtimeCandidateAdmissionOperations(d.Candidate)
	if len(operations) != len(wanted) {
		return fmt.Errorf("implementation dispatch must cover the complete bounded working proposal")
	}
	seen := map[string]bool{}
	for _, operation := range operations {
		if len(operation.TargetRefs) != 1 || len(operation.BindingRefs) != 0 || len(operation.CoverageWorkItemIDs) != 0 {
			return fmt.Errorf("implementation operation cannot confer coverage or exceed its singleton target")
		}
		proposal, found := wanted[operation.TargetRefs[0]]
		if !found || seen[operation.TargetRefs[0]] || operation.Kind != proposal.OperationKind {
			return fmt.Errorf("implementation dispatch target/kind differs from original semantic proposal")
		}
		seen[operation.TargetRefs[0]] = true
		if v := round.ScopeRequest.InputRevision; v != nil {
			matches := 0
			for _, f := range d.OperationExecutions {
				if f.OperationID == operation.ID && v.MatchesReplayProof(f.ReplayProof) {
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("input revision dispatch lacks exact changed reviewed preimage")
			}
		}
		contract, known := RuntimeOperationContractForKind(operation.Kind)
		if !known {
			return fmt.Errorf("implementation dispatch has no executable operation contract")
		}
		variantID := ""
		if len(contract.Variants) != 0 {
			variant, err := MatchRuntimeOperationVariant(operation, contract)
			if err != nil {
				return err
			}
			variantID = variant.ID
		}
		if variantID != proposal.OperationVariantID {
			return fmt.Errorf("implementation dispatch operation variant differs from working proposal")
		}
	}
	return nil
}

type RuntimeImplementationWorkReceiptDisposition string

const (
	RuntimeImplementationWorkReceiptRejected            RuntimeImplementationWorkReceiptDisposition = "effect_rejected"
	RuntimeImplementationWorkReceiptStale               RuntimeImplementationWorkReceiptDisposition = "prerequisite_stale"
	RuntimeImplementationWorkReceiptGovernance          RuntimeImplementationWorkReceiptDisposition = "governance_blocked"
	RuntimeImplementationWorkReceiptUnknown             RuntimeImplementationWorkReceiptDisposition = "outcome_unknown"
	RuntimeImplementationWorkReceiptInvalidationMissing RuntimeImplementationWorkReceiptDisposition = "resource_invalidation_missing"
)

func validImplementationWorkReceiptDisposition(value RuntimeImplementationWorkReceiptDisposition) bool {
	switch value {
	case RuntimeImplementationWorkReceiptRejected, RuntimeImplementationWorkReceiptStale, RuntimeImplementationWorkReceiptGovernance, RuntimeImplementationWorkReceiptUnknown, RuntimeImplementationWorkReceiptInvalidationMissing:
		return true
	default:
		return false
	}
}

// 工作 effect 沿既有 owner 合同接受未声明 quality；不补写 valid，也不授 terminal credit。
// 显式 weak 与未定义 enum 均不可作为 durable working effect。
func runtimeImplementationWorkEffectQualityAcceptable(quality common.EvidenceQuality) bool {
	return quality == "" || quality == common.EvidenceQualityValid
}

// RuntimeImplementationWorkDispositionForReceipt 从原 owner fact 推导不续权处置。
// 空值仅表示可进入 collective receipt 校验，不授 terminal credit。
func RuntimeImplementationWorkDispositionForReceipt(round RuntimeImplementationWorkRound, receipt RuntimeOperationReceipt) RuntimeImplementationWorkReceiptDisposition {
	if disposition := implementationWorkReceiptOutcomeDisposition(receipt); disposition != "" {
		return disposition
	}
	if round.WorkingBinding != nil {
		for _, dependency := range round.WorkingBinding.CatalogObservation.Dependencies {
			for _, change := range receipt.ResourceChanges {
				if RuntimeFreshnessDependencyIntersectsChange(dependency, change) && dependency.Revision == change.BeforeRevision && change.BeforeRevision != change.AfterRevision {
					return ""
				}
			}
		}
	}
	return RuntimeImplementationWorkReceiptInvalidationMissing
}

func implementationWorkReceiptOutcomeDisposition(receipt RuntimeOperationReceipt) RuntimeImplementationWorkReceiptDisposition {
	if receipt.PrerequisiteStale != nil {
		return RuntimeImplementationWorkReceiptStale
	}
	if receipt.Outcome == RuntimeOperationOutcomeGovernanceRequired {
		return RuntimeImplementationWorkReceiptGovernance
	}
	if receipt.Outcome == RuntimeOperationOutcomeUnknown {
		return RuntimeImplementationWorkReceiptUnknown
	}
	if receipt.Outcome != RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != RuntimeEffectWorkspaceDurable || !receipt.Changed || receipt.BeforeHash == receipt.AfterHash || receipt.AfterHash == "" || receipt.Evidence.Validation != nil || receipt.Evidence.ZeroWork || !runtimeImplementationWorkEffectQualityAcceptable(receipt.Evidence.Quality) {
		return RuntimeImplementationWorkReceiptRejected
	}
	return ""
}

func runtimeImplementationWorkCommitmentsEqual(a, b *RuntimeImplementationWorkRound) bool {
	return reflect.DeepEqual(a, b)
}

// RuntimeImplementationWorkPrerequisiteRequired 只延后原 current Focus 的终验目标和 scratch。
// 所有 manifest/capability/catalog 仍是工作前置，尤其不得漏掉其它 owner 的验证输入保护目录。
// 工作 binding 已点名的 exact target snapshot 仍须取得；未知种类也保持阻塞。
func RuntimeImplementationWorkPrerequisiteRequired(value RuntimePrerequisite, focusID string, workingTargets []string) bool {
	switch value.Kind {
	case PrerequisiteKindValidationScratch:
		return !((value.Scope == RuntimePrerequisiteScopeGlobal && value.FocusID == "") || (value.Scope == RuntimePrerequisiteScopeFocus && value.FocusID == focusID))
	case PrerequisiteKindTargetSelection, PrerequisiteKindTargetSnapshot:
		if value.Scope == RuntimePrerequisiteScopeFocus && value.FocusID == focusID {
			return value.Kind == PrerequisiteKindTargetSnapshot && slices.Contains(workingTargets, value.SubjectRef)
		}
	}
	return true
}

func RuntimeImplementationWorkPrerequisitesPending(facts RuntimeFacts, binding *RuntimeImplementationWorkingBinding) bool {
	var targets []string
	if binding != nil {
		for _, proposal := range binding.Proposals {
			targets = append(targets, RuntimeSemanticProposalTargetRefs(proposal)...)
		}
	}
	for _, value := range facts.Prerequisites.Pending {
		if RuntimeImplementationWorkPrerequisiteRequired(value, facts.Focus.ID, targets) {
			return true
		}
	}
	return false
}

// ValidateRuntimeImplementationWorkInputCatalogs 反查所有已要求的 owner catalog 与原观察。
// 没有 pending 不等于完整保护来源；不能通过删掉另一个 owner 的 catalog 缩小 exclusions。
func ValidateRuntimeImplementationWorkInputCatalogs(facts RuntimeFacts, capabilityContract []string) error {
	// sealed capability inventory 独立于可删除的 Pending/Satisfied；协调删去整组来源仍拒绝。
	for _, kind := range capabilityContract {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found {
			return fmt.Errorf("implementation work input-protection inventory has unknown sealed operation")
		}
		if !descriptor.RequiresOwnerReceipt {
			continue
		}
		matches := 0
		for _, catalog := range facts.Resources.OperationAdmissibilities {
			if catalog.OwnerModuleID == descriptor.OwnerModuleID {
				declared := false
				for _, prerequisite := range facts.Prerequisites.Satisfied {
					if prerequisite.Kind == PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == catalog.OwnerModuleID && prerequisite.ID == catalog.PrerequisiteID {
						declared = true
					}
				}
				if !declared {
					return fmt.Errorf("implementation work sealed owner catalog has no satisfied declared prerequisite")
				}
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("implementation work requires complete sealed owner input-protection inventory")
		}
	}
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.Kind != PrerequisiteKindOperationAdmissibility {
			continue
		}
		matches := 0
		for _, catalog := range facts.Resources.OperationAdmissibilities {
			if catalog.OwnerModuleID != prerequisite.OwnerModuleID || catalog.PrerequisiteID != prerequisite.ID {
				continue
			}
			observations := 0
			for _, observation := range facts.Resources.PrerequisiteObservations {
				if observation.Kind == prerequisite.Kind && observation.OwnerModuleID == prerequisite.OwnerModuleID && observation.PrerequisiteID == prerequisite.ID && observation.ContentHash == catalog.SnapshotID {
					observations++
				}
			}
			if observations != 1 {
				return fmt.Errorf("implementation work requires unique original input-protection catalog observation")
			}
			matches++
		}
		if matches != 1 {
			return fmt.Errorf("implementation work requires complete declared owner input-protection catalogs")
		}
	}
	return nil
}

// ValidateRuntimeImplementationWorkAdmission validates the independent private
// work reservation against current facts and its original sealed semantic source.
// It never requires or creates terminal CoverageEvidenceBinding.
func ValidateRuntimeImplementationWorkAdmission(facts RuntimeFacts, decision RuntimeDecision, semantic RuntimeSemanticPlanningLedger, ledger RuntimeImplementationWorkLedger, manifest RuntimeCandidateContractManifest) (*RuntimeImplementationWorkRound, error) {
	if decision.RequiredOutput.ImplementationWorkBindingHash == "" || manifest.ImplementationWorkingBinding == nil || decision.State != ConvergenceStateNeedStateChange || facts.Failure != nil || facts.RepairDirective != nil || RuntimeImplementationWorkPrerequisitesPending(facts, manifest.ImplementationWorkingBinding) {
		return nil, fmt.Errorf("implementation work admission lacks exact unblocked working purpose")
	}
	if err := ValidateRuntimeImplementationWorkInputCatalogs(facts, nil); err != nil {
		return nil, err
	}
	if err := ValidateRuntimeCWILedger(facts); err != nil {
		return nil, err
	}
	var selected *RuntimeImplementationWorkRound
	for _, round := range ledger.Rounds {
		if round.CompletedReceiptSetHash != "" || round.FailedReceiptHash != "" {
			continue
		}
		if selected != nil {
			return nil, fmt.Errorf("implementation work admission has multiple outstanding rounds")
		}
		value := round.Clone()
		selected = &value
	}
	if selected == nil || selected.DispatchID != "" || selected.WorkingBinding == nil || selected.ScopeObservation == nil || selected.WorkingBinding.Hash != decision.RequiredOutput.ImplementationWorkBindingHash || !reflect.DeepEqual(selected.WorkingBinding, manifest.ImplementationWorkingBinding) {
		return nil, fmt.Errorf("implementation work admission does not match private working reservation")
	}
	if !reflect.DeepEqual(facts.Resources.OperationAdmissibilities, selected.WorkingBinding.MutationInputCatalogs) {
		return nil, fmt.Errorf("implementation work admission lost original complete input-protection inventory")
	}
	if err := selected.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateRuntimeImplementationWorkingBindingSemanticSource(*selected, semantic); err != nil {
		return nil, err
	}
	eligible := false
	for _, item := range facts.Coverage {
		if item.ID == selected.CoverageWorkItemID && slices.Contains(RuntimeCWIsEligibleForPlanning(facts), item.ID) && item.State != RuntimeCoverageItemStateSatisfied && item.CurrentBindingRef == nil {
			eligible = true
		}
	}
	if !eligible || len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.Focus.CoverageWorkItemIDs[0] != selected.CoverageWorkItemID {
		return nil, fmt.Errorf("implementation work admission requires exact eligible original unbound CWI focus")
	}
	if facts.Resources.Manifest == nil || facts.Resources.Manifest.Revision != selected.ScopeRequest.ManifestBinding.Manifest.Revision {
		return nil, fmt.Errorf("implementation work admission workspace manifest is stale")
	}
	catalogMatches := 0
	for _, catalog := range facts.Resources.OperationAdmissibilities {
		if reflect.DeepEqual(catalog, selected.WorkingBinding.OwnerCatalog) {
			catalogMatches++
		}
	}
	if catalogMatches != 1 {
		return nil, fmt.Errorf("implementation work admission has no exact current owner catalog")
	}
	return selected, nil
}
