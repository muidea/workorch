package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

// DeriveRuntimeTerminalResultMaterialFromState 是 SM-4 唯一终态材料纯 compiler。
// 它只读取已裁决的 typed state，不选择 action、不调用 owner，也不持有 Hub、I/O 或 CAS。
// StateRevision 原样取传入 source，供拟提交后像生成及 fresh-load 反证复用；不递归校验 state。
func DeriveRuntimeTerminalResultMaterialFromState(source RuntimeStateRecord) (RuntimeTerminalResultMaterial, error) {
	if err := ValidateRuntimeResultFeedbackVersion(source.ResultFeedbackVersion); err != nil {
		return RuntimeTerminalResultMaterial{}, err
	}
	outcome, terminal, err := DeriveRuntimeTerminalOutcome(source.Decision, source.Termination)
	if err != nil {
		return RuntimeTerminalResultMaterial{}, err
	}
	if !terminal {
		return RuntimeTerminalResultMaterial{}, fmt.Errorf("%w: result material requires a sealed terminal aggregate", common.ErrConflict)
	}
	for _, ledger := range [][]EvidenceFact{source.Facts.Evidence.Accepted, source.Facts.Evidence.Rejected} {
		for _, evidence := range ledger {
			if err := ValidateRuntimeEvidencePurposeKind(evidence.ApplicabilityDependencies, evidence.Kind); err != nil {
				return RuntimeTerminalResultMaterial{}, err
			}
			if err := ValidateRuntimeEvidenceDependencyPair(evidence.OwnerRevisionSet, evidence.ApplicabilityDependencies, source.Facts.DefinitionRevision); err != nil {
				return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal evidence %q read dependencies: %w", evidence.ID, err)
			}
			if evidence.OwnerRevisionSet != nil && evidence.OwnerRevisionSet.DefinitionRevision != source.Facts.DefinitionRevision {
				return RuntimeTerminalResultMaterial{}, fmt.Errorf("terminal evidence %q read dependency definition mismatch", evidence.ID)
			}
		}
	}
	// Normalize authority projections without replacing the original diagnostic.
	// In particular semantic-violation normalization can allocate an empty ID
	// slice (omitted by persistence), and trim producer bytes. Neither belongs in
	// immutable delivery material or its independent fresh-load reconstruction.
	var originalFailureDiagnostic *RuntimeOriginalDiagnostic
	if source.Facts.Failure != nil {
		originalFailureDiagnostic = source.Facts.Failure.OriginalDiagnostic.CloneOriginal()
	}
	source.Facts = source.Facts.Normalized()
	if source.Facts.Failure != nil {
		source.Facts.Failure.OriginalDiagnostic = originalFailureDiagnostic
	}
	if outcome.Kind == TerminalOutcomeKindCompleted {
		if err := validateRuntimeTerminalEvidenceApplicability(source); err != nil {
			return RuntimeTerminalResultMaterial{}, err
		}
	}
	ownerSet, err := CompileRuntimeTerminalOwnerReadDependencies(source)
	if err != nil {
		return RuntimeTerminalResultMaterial{}, err
	}
	accepted := outcome.Kind == TerminalOutcomeKindCompleted
	terminalSummary := outcome.Summary
	assessment := workorchresultcontract.DoDAssessment{Satisfied: accepted, Summary: terminalSummary}
	evidenceRefs := runtimeTerminalEvidenceRefs(source.Facts)
	artifactRefs := runtimeTerminalArtifactRefs(source.Facts)
	goalCoverage := workorchresultcontract.DoDCoverage{Satisfied: true}
	acceptanceDimensions := workorchresultcontract.DoDCoverage{Satisfied: true}
	deliverableCoverage := workorchresultcontract.DoDCoverage{Satisfied: true}
	for _, fact := range source.Facts.Assessments {
		switch fact.Scope {
		case "goalCoverage":
			goalCoverage.Items = append(goalCoverage.Items, runtimeCoverageItem(fact))
			goalCoverage.Satisfied = goalCoverage.Satisfied && fact.Disposition == AssessmentDispositionSatisfied
		case "acceptanceDimension":
			acceptanceDimensions.Items = append(acceptanceDimensions.Items, runtimeCoverageItem(fact))
			acceptanceDimensions.Satisfied = acceptanceDimensions.Satisfied && fact.Disposition == AssessmentDispositionSatisfied
		case "deliverable":
			deliverableCoverage.Items = append(deliverableCoverage.Items, runtimeCoverageItem(fact))
			deliverableCoverage.Satisfied = deliverableCoverage.Satisfied && fact.Disposition == AssessmentDispositionSatisfied
		default:
			assessment.Items = append(assessment.Items, workorchresultcontract.DoDAssessmentItem{
				Item: fact.RequirementText, State: runtimeAssessmentState(fact.Disposition == AssessmentDispositionSatisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
				FailureReason: runtimeAssessmentFailureReason(fact),
			})
		}
	}
	if len(goalCoverage.Items) > 0 {
		assessment.GoalCoverage = &goalCoverage
	}
	if len(acceptanceDimensions.Items) > 0 {
		assessment.AcceptanceDimensions = &acceptanceDimensions
	}
	if len(deliverableCoverage.Items) > 0 {
		assessment.DeliverableCoverage = &deliverableCoverage
	}
	item := RuntimeTerminalResultMaterial{
		WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID,
		GoalID: source.Definition.GoalID, DoDID: source.Definition.DoDID,
		Title: "Runtime 验收结果", Summary: terminalSummary,
		SourceType: RuntimeTerminalResultSourceTypeDoDAssessment, SourceRef: source.Decision.AuthoritySnapshot.ID,
		EvidenceRefs: evidenceRefs, ArtifactRefs: artifactRefs, DoDAssessment: assessment,
		SummaryDetail: workorchresultcontract.ResultSummary{GoalOutcome: terminalSummary, EvidenceRefs: append([]string(nil), evidenceRefs...), ArtifactRefs: append([]common.ArtifactID(nil), artifactRefs...)},
	}
	item.RuntimeCloseout = runtimeResultCloseout(source, outcome, accepted, evidenceRefs, ownerSet)
	runtimeCompleteResultFeedback(&item, source)
	return item.Normalized(), nil
}

func runtimeResultCloseout(source RuntimeStateRecord, outcome TerminalOutcome, accepted bool, evidenceRefs []string, ownerSet RuntimeOwnerRevisionSet) *RuntimeResultCloseout {
	closeout := RuntimeResultCloseout{
		SchemaVersion:        RuntimeResultCloseoutSchemaVersionCurrent,
		CloseoutID:           runtimeCloseoutID(source),
		StartupInputHash:     source.Definition.Normalized().StartupInputHash,
		DefinitionRevision:   source.Facts.DefinitionRevision,
		RuntimeStateRevision: source.StateRevision,
		TerminalOutcome:      outcome,
		EvidenceRefs:         append([]string(nil), evidenceRefs...),
		ArtifactRefs:         runtimeTerminalArtifactRefs(source.Facts),
		OperationOutcomes:    runtimeOperationCloseouts(source),
		RecoveryPolicy:       source.Facts.RecoveryPolicy.Normalized(),
		RecoveryLedger:       source.Facts.Recovery,
	}
	accumulator := newRuntimeCloseoutAccumulator(accepted)
	// 持久化 coverage ledger 是当前 schema 的唯一收尾权威。
	// assessment 只保留审计与缺口 lineage，不替代当前 item state。
	accumulator.collectCoverage(source.Facts.Coverage)
	// 未满足的当前 coverage 在终态材料中仍保持未满足，不通过反馈补发 credit。
	accumulator.collectOpenCoverage(source.Facts.Focus, source.Facts.RemainingFocuses)
	accumulator.collectAssessments(source.Facts.Assessments, accepted)
	if !accepted {
		accumulator.collectActiveGap(source.Facts.Repair.ActiveGap)
	}
	accumulator.apply(&closeout)
	if !accepted {
		accumulator.applyTerminalFailure(&closeout, source)
	}
	closeout.OwnerRevisionSet = ownerSet
	closeout.RequirementOutcomes = DeriveRuntimeTerminalRequirementOutcomesFromState(source, closeout.OwnerRevisionSet)
	if source.Facts.DefinitionSeed.SourceObligations != nil {
		inventory := source.Facts.DefinitionSeed.SourceObligations.Clone()
		closeout.SourceObligations = &inventory
	}
	for _, outcome := range closeout.RequirementOutcomes {
		closeout.AcceptedEvidenceRefs = append(closeout.AcceptedEvidenceRefs, outcome.AcceptedEvidenceRefs...)
	}
	closeout.DeliverableRefs = runtimeCloseoutDeliverableRefs(source)
	value := closeout.WithCloseoutHash()
	return &value
}

func runtimeCloseoutID(source RuntimeStateRecord) string {
	return "runtime-closeout/" + string(source.Facts.WorkspaceID) + "/" + string(source.Facts.RunID) + "/" + source.Decision.AuthoritySnapshot.ID
}

// DeriveRuntimeTerminalRequirementOutcomesFromState 只投影当前 coverage 与历史评估的适用性。
// 它不校验或修改 terminal，不恢复历史 credit，也不执行 owner 操作。
func DeriveRuntimeTerminalRequirementOutcomesFromState(source RuntimeStateRecord, ownerSet RuntimeOwnerRevisionSet) []RuntimeRequirementOutcome {
	requiredAssessment := map[common.CoverageWorkItemID]bool{}
	requiredKinds := map[common.CoverageWorkItemID][]string{}
	for _, focus := range source.Facts.DefinitionFocuses {
		for _, id := range focus.CoverageWorkItemIDs {
			requiredAssessment[id] = requiredAssessment[id] || focus.RequireAssessment
			requiredKinds[id] = append(requiredKinds[id], focus.RequiredEvidenceKinds...)
		}
	}
	assessmentByCWI := map[common.CoverageWorkItemID]RuntimeAssessmentFact{}
	for _, assessment := range source.Facts.Assessments {
		for _, id := range assessment.CoverageWorkItemIDs {
			assessmentByCWI[id] = assessment
		}
	}
	ret := make([]RuntimeRequirementOutcome, 0, len(source.Facts.Coverage))
	for _, coverage := range source.Facts.Coverage {
		outcome := RuntimeRequirementOutcome{
			RequirementRef: coverage.SourceScope + "/" + coverage.SourceID, SourceItemRef: coverage.SourceID,
			CoverageWorkItemID: coverage.ID, DefinitionRevision: source.Facts.DefinitionRevision,
			OwnerRevisionSetHash: ownerSet.SetHash,
		}
		if outcome.RequirementRef == "/" || outcome.RequirementRef == "" {
			outcome.RequirementRef = string(coverage.ID)
		}
		if outcome.SourceItemRef == "" {
			outcome.SourceItemRef = string(coverage.ID)
		}
		switch coverage.State {
		case RuntimeCoverageItemStateSatisfied:
			outcome.CoverageState = RuntimeRequirementCovered
		case RuntimeCoverageItemStateOpen:
			if len(coverage.EvidenceRefs) != 0 {
				outcome.CoverageState = RuntimeRequirementPartiallyCovered
			} else {
				outcome.CoverageState = RuntimeRequirementUncovered
			}
		default:
			outcome.CoverageState = RuntimeRequirementUncovered
		}
		assessment, hasAssessment := assessmentByCWI[coverage.ID]
		if coverage.HistoricalBehavior != nil && coverage.State == RuntimeCoverageItemStateSatisfied && len(coverage.HistoricalAcceptanceProofs) != 0 {
			proof := coverage.HistoricalAcceptanceProofs[len(coverage.HistoricalAcceptanceProofs)-1]
			if proof.AssessmentDispatchID == coverage.LastAssessmentID {
				for _, fact := range source.Facts.Assessments {
					if fact.ID == proof.AssessmentDispatchID {
						assessment, hasAssessment = fact, true
					}
				}
			}
		}
		if !hasAssessment && !requiredAssessment[coverage.ID] && coverage.State == RuntimeCoverageItemStateSatisfied {
			outcome.AssessmentState = RuntimeRequirementSatisfied
		} else if !hasAssessment {
			outcome.AssessmentState = RuntimeRequirementUnassessed
		} else {
			switch assessment.Disposition {
			case AssessmentDispositionSatisfied:
				outcome.AssessmentState = RuntimeRequirementSatisfied
			case AssessmentDispositionTerminalRejection:
				outcome.AssessmentState = RuntimeRequirementUnsatisfied
			default:
				outcome.AssessmentState = RuntimeRequirementIndeterminate
				outcome.AssessmentReasonCode = "assessment_evidence_incomplete"
				if assessment.Gap != nil {
					outcome.MissingEvidenceKinds = append(outcome.MissingEvidenceKinds, assessment.Gap.MissingEvidenceKinds...)
					if assessment.Gap.FailureCode != "" {
						outcome.AssessmentReasonCode = assessment.Gap.FailureCode
					}
				}
			}
		}
		// 历史 assessment 是不可变审计事实；仅当前 SM-2 coverage ledger
		// 决定 satisfied 判定是否仍适用，反馈不得恢复历史 credit。
		if outcome.AssessmentState == RuntimeRequirementSatisfied && coverage.State != RuntimeCoverageItemStateSatisfied {
			outcome.AssessmentState = RuntimeRequirementIndeterminate
			outcome.AssessmentReasonCode = "current_coverage_not_satisfied"
		}
		outcome.MissingEvidenceKinds = append(outcome.MissingEvidenceKinds, requiredKinds[coverage.ID]...)
		outcome.EvidenceItems = runtimeRequirementEvidenceItems(source, coverage, ownerSet.SetHash)
		for _, item := range outcome.EvidenceItems {
			if item.State == RuntimeRequirementEvidenceAccepted {
				outcome.AcceptedEvidenceRefs = append(outcome.AcceptedEvidenceRefs, item.EvidenceRef)
			}
		}
		outcome = outcome.Normalized()
		if outcome.AssessmentState != RuntimeRequirementIndeterminate {
			outcome.MissingEvidenceKinds = nil
		}
		outcome.SourceObligations = runtimeTerminalSourceObligationOutcomes(source, coverage, outcome)
		ret = append(ret, outcome)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID })
	return ret
}

func runtimeRequirementEvidenceItems(source RuntimeStateRecord, coverage RuntimeCoverageItemFact, ownerSetHash string) []RuntimeRequirementEvidenceItem {
	ret := make([]RuntimeRequirementEvidenceItem, 0)
	binding, hasBinding := runtimeTerminalCurrentCoverageBinding(source.Facts, coverage)
	appendLedger := func(values []EvidenceFact, state RuntimeRequirementEvidenceState) {
		for _, evidence := range values {
			if !runtimeEvidenceAppliesToCoverage(evidence, coverage) {
				continue
			}
			bindingRef := ""
			if len(evidence.BindingRefs) != 0 {
				bindingRef = evidence.BindingRefs[0]
			}
			item := RuntimeRequirementEvidenceItem{EvidenceRef: string(evidence.ID), BindingRef: bindingRef, State: state, ReasonCode: evidence.FailureCode,
				OwnerRevisionSet:          CloneRuntimeOwnerRevisionSet(evidence.OwnerRevisionSet),
				ApplicabilityDependencies: CloneRuntimeEvidenceApplicabilityDependencies(evidence.ApplicabilityDependencies)}
			if state == RuntimeRequirementEvidenceAccepted {
				if hasBinding && runtimeTerminalEvidenceMatchesCoverageProof(source, evidence, coverage, binding) {
					item.OwnerRevisionSetHash = ownerSetHash
				} else {
					item.State = RuntimeRequirementEvidenceStaleOrSuperseded
					item.ReasonCode = "current_binding_proof_not_applicable"
				}
			}
			ret = append(ret, item)
		}
	}
	appendLedger(source.Facts.Evidence.Accepted, RuntimeRequirementEvidenceAccepted)
	appendLedger(source.Facts.Evidence.Rejected, RuntimeRequirementEvidenceRejected)
	return ret
}

func runtimeEvidenceAppliesToCoverage(evidence EvidenceFact, coverage RuntimeCoverageItemFact) bool {
	for _, id := range evidence.CoverageWorkItemIDs {
		if id == coverage.ID {
			return true
		}
	}
	for _, ref := range coverage.EvidenceRefs {
		if ref == string(evidence.ID) || ref == evidence.SourceRef {
			return true
		}
	}
	for _, proof := range coverage.HistoricalAcceptanceProofs {
		if containsString(proof.AcceptanceEvidenceRefs, string(evidence.ID)) {
			return true // Immutable audit association, not current credit.
		}
	}
	return false
}

func runtimeCloseoutDeliverableRefs(source RuntimeStateRecord) []string {
	refs := make([]string, 0)
	for _, focus := range source.Facts.DefinitionFocuses {
		refs = append(refs, focus.ArtifactRefs...)
	}
	return refs
}

type runtimeCloseoutAccumulator struct {
	satisfied   map[common.CoverageWorkItemID]struct{}
	unsatisfied map[common.CoverageWorkItemID]struct{}
	missing     map[string]struct{}
	targets     map[string]struct{}
	gaps        []RuntimeAssessmentGap
	disposition AssessmentDisposition
}

func newRuntimeCloseoutAccumulator(accepted bool) *runtimeCloseoutAccumulator {
	disposition := AssessmentDispositionSatisfied
	if !accepted {
		disposition = AssessmentDispositionTerminalRejection
	}
	return &runtimeCloseoutAccumulator{
		satisfied: map[common.CoverageWorkItemID]struct{}{}, unsatisfied: map[common.CoverageWorkItemID]struct{}{},
		missing: map[string]struct{}{}, targets: map[string]struct{}{}, disposition: disposition,
	}
}

func (a *runtimeCloseoutAccumulator) collectCoverage(items []RuntimeCoverageItemFact) {
	for _, item := range items {
		if item.State == RuntimeCoverageItemStateSatisfied {
			a.satisfied[item.ID] = struct{}{}
		} else {
			a.unsatisfied[item.ID] = struct{}{}
		}
	}
}

func (a *runtimeCloseoutAccumulator) collectAssessments(facts []RuntimeAssessmentFact, accepted bool) {
	for _, fact := range facts {
		current := a.coverageCurrentlyUnsatisfied(fact.CoverageWorkItemIDs)
		if !current && fact.Gap != nil {
			current = a.coverageCurrentlyUnsatisfied(fact.Gap.CoverageWorkItemIDs)
		}
		if !current {
			continue
		}
		a.collectAssessmentDisposition(fact.Disposition, accepted)
		if fact.Gap != nil && runtimeAssessmentDispositionCarriesGap(fact.Disposition) {
			if gap, current := a.currentGap(fact.Gap.Normalized()); current {
				a.collectAssessmentGap(gap)
			}
		}
	}
}

func (a *runtimeCloseoutAccumulator) collectAssessmentDisposition(disposition AssessmentDisposition, accepted bool) {
	if accepted {
		return
	}
	switch disposition {
	case AssessmentDispositionRepairableGap, AssessmentDispositionGovernanceRequired, AssessmentDispositionTerminalRejection:
		a.disposition = disposition
	}
}

func (a *runtimeCloseoutAccumulator) collectAssessmentGap(gap RuntimeAssessmentGap) {
	a.gaps = append(a.gaps, gap)
	for _, kind := range gap.MissingEvidenceKinds {
		a.missing[kind] = struct{}{}
	}
	for _, ref := range gap.TargetRefs {
		a.targets[ref] = struct{}{}
	}
}

func (a *runtimeCloseoutAccumulator) collectActiveGap(gap *RuntimeAssessmentGap) {
	if gap == nil || len(a.gaps) != 0 {
		return
	}
	if current, ok := a.currentGap(gap.Normalized()); ok {
		a.collectAssessmentGap(current)
	}
}

func (a *runtimeCloseoutAccumulator) currentGap(gap RuntimeAssessmentGap) (RuntimeAssessmentGap, bool) {
	ids := make([]common.CoverageWorkItemID, 0, len(gap.CoverageWorkItemIDs))
	for _, id := range gap.CoverageWorkItemIDs {
		if a.coverageItemCurrentlyUnsatisfied(id) {
			ids = append(ids, id)
		}
	}
	gap.CoverageWorkItemIDs = ids
	return gap.Normalized(), len(ids) != 0
}

func (a *runtimeCloseoutAccumulator) coverageCurrentlyUnsatisfied(ids []common.CoverageWorkItemID) bool {
	for _, id := range ids {
		if a.coverageItemCurrentlyUnsatisfied(id) {
			return true
		}
	}
	return false
}

func (a *runtimeCloseoutAccumulator) coverageItemCurrentlyUnsatisfied(id common.CoverageWorkItemID) bool {
	if _, satisfied := a.satisfied[id]; satisfied {
		return false
	}
	_, unsatisfied := a.unsatisfied[id]
	return unsatisfied
}

func runtimeAssessmentDispositionCarriesGap(disposition AssessmentDisposition) bool {
	switch disposition.Normalized() {
	case AssessmentDispositionRepairableGap, AssessmentDispositionGovernanceRequired, AssessmentDispositionTerminalRejection:
		return true
	default:
		return false
	}
}

func (a *runtimeCloseoutAccumulator) collectOpenCoverage(focus CoverageFocus, remaining []CoverageFocus) {
	a.collectOpenCoverageIDs(focus.CoverageWorkItemIDs)
	for _, item := range remaining {
		a.collectOpenCoverageIDs(item.CoverageWorkItemIDs)
	}
}

func (a *runtimeCloseoutAccumulator) collectOpenCoverageIDs(ids []common.CoverageWorkItemID) {
	for _, id := range ids {
		if _, ok := a.satisfied[id]; !ok {
			a.unsatisfied[id] = struct{}{}
		}
	}
}

func (a *runtimeCloseoutAccumulator) apply(closeout *RuntimeResultCloseout) {
	for id := range a.satisfied {
		delete(a.unsatisfied, id)
	}
	closeout.SatisfiedCoverageWorkItemIDs = runtimeTerminalCoverageIDsFromSet(a.satisfied)
	closeout.UnsatisfiedCoverageWorkItemIDs = runtimeTerminalCoverageIDsFromSet(a.unsatisfied)
	closeout.Gaps = a.gaps
	closeout.MissingEvidenceKinds = runtimeTerminalStringsFromSet(a.missing)
	closeout.TargetRefs = runtimeTerminalStringsFromSet(a.targets)
	closeout.Disposition = a.disposition
}

func (a *runtimeCloseoutAccumulator) applyTerminalFailure(closeout *RuntimeResultCloseout, source RuntimeStateRecord) {
	if source.Facts.Failure != nil {
		a.applyFailureFact(closeout, source.Facts.Failure)
	} else if source.Facts.Repair.ActiveGap != nil {
		if gap, current := a.currentGap(source.Facts.Repair.ActiveGap.Normalized()); current {
			a.applyGapFailure(closeout, gap)
		}
	} else if len(closeout.Gaps) != 0 {
		a.applyGapFailure(closeout, closeout.Gaps[0])
	}
	if closeout.FailureCode == "" && source.Decision.Terminal != nil {
		closeout.FailureCode = strings.TrimSpace(source.Decision.Terminal.Code)
	}
	if source.Decision.Terminal != nil && source.Decision.Terminal.OriginalDiagnostic != nil {
		closeout.OriginalDiagnostic = source.Decision.Terminal.OriginalDiagnostic.CloneOriginal()
		for _, ref := range closeout.OriginalDiagnostic.TargetRefs {
			a.targets[ref] = struct{}{}
		}
		closeout.TargetRefs = runtimeTerminalStringsFromSet(a.targets)
	}
	if closeout.FailureSignature == "" && source.Facts.Repair.LastCycleID != "" {
		closeout.FailureSignature = source.Facts.Repair.LastCycleID
	}
}

func (a *runtimeCloseoutAccumulator) applyFailureFact(closeout *RuntimeResultCloseout, failure *FailureFact) {
	closeout.OriginalDiagnostic = failure.OriginalDiagnostic.CloneOriginal()
	if failure.Class != "" {
		closeout.FailureClass = failure.Class
	}
	if strings.TrimSpace(failure.Code) != "" {
		closeout.FailureCode = strings.TrimSpace(failure.Code)
	}
	if strings.TrimSpace(failure.Signature) != "" {
		closeout.FailureSignature = strings.TrimSpace(failure.Signature)
	}
	for _, ref := range failure.TargetRefs {
		a.targets[ref] = struct{}{}
	}
	closeout.TargetRefs = runtimeTerminalStringsFromSet(a.targets)
}

func (a *runtimeCloseoutAccumulator) applyGapFailure(closeout *RuntimeResultCloseout, gap RuntimeAssessmentGap) {
	gap = gap.Normalized()
	if closeout.FailureClass == "" {
		closeout.FailureClass = gap.FailureClass
	}
	if closeout.FailureCode == "" {
		closeout.FailureCode = gap.FailureCode
	}
	if closeout.FailureSignature == "" {
		closeout.FailureSignature = gap.FailureSignature
	}
}

func runtimeOperationCloseouts(source RuntimeStateRecord) []RuntimeOperationCloseout {
	dispatches := append([]RuntimeDispatchRecord(nil), source.CompletedDispatches...)
	if source.ActiveDispatch != nil {
		dispatches = append(dispatches, *source.ActiveDispatch)
	}
	ret := make([]RuntimeOperationCloseout, 0)
	for _, dispatch := range dispatches {
		for _, execution := range dispatch.OperationExecutions {
			execution = execution.Normalized()
			outcome := RuntimeOperationCloseout{
				DispatchID: dispatch.ID, OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey,
				Fence:               execution.Fence,
				ExpectedEffectScope: execution.ExpectedEffectScope, ExecutionState: execution.State,
				ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: execution.OwnerReceiptHash,
			}
			if receipt, ok := RuntimeOperationReceiptForExecutionKeyFence(source.OperationReceipts, execution.ExecutionKey, execution.Fence); ok {
				outcome.ObservedEffectScope = receipt.ObservedEffectScope
				outcome.Outcome = receipt.Outcome
			}
			ret = append(ret, outcome.Normalized())
		}
	}
	return ret
}

func runtimeTerminalEvidenceRefs(facts RuntimeFacts) []string {
	facts = facts.Normalized()
	values := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values[value] = struct{}{}
		}
	}
	for _, ledger := range [][]EvidenceFact{facts.Evidence.Accepted, facts.Evidence.Rejected} {
		for _, evidence := range ledger {
			add(string(evidence.ID))
			add(evidence.SourceRef)
			for _, ref := range evidence.EvidenceRefs {
				add(ref)
			}
		}
	}
	for _, assessment := range facts.Assessments {
		for _, ref := range assessment.EvidenceRefs {
			add(ref)
		}
		for _, item := range assessment.Items {
			for _, ref := range item.EvidenceRefs {
				add(ref)
			}
		}
		if assessment.Gap != nil {
			for _, ref := range assessment.Gap.EvidenceRefs {
				add(ref)
			}
		}
	}
	for _, item := range facts.Coverage {
		for _, ref := range item.EvidenceRefs {
			add(ref)
		}
	}
	return runtimeTerminalStringsFromSet(values)
}

func runtimeTerminalArtifactRefs(facts RuntimeFacts) []common.ArtifactID {
	values := map[common.ArtifactID]struct{}{}
	for _, assessment := range facts.Normalized().Assessments {
		for _, ref := range assessment.ArtifactRefs {
			id := common.ArtifactID(strings.TrimSpace(ref))
			if id != "" {
				values[id] = struct{}{}
			}
		}
		for _, item := range assessment.Items {
			for _, ref := range item.ArtifactRefs {
				id := common.ArtifactID(strings.TrimSpace(string(ref)))
				if id != "" {
					values[id] = struct{}{}
				}
			}
		}
	}
	ret := make([]common.ArtifactID, 0, len(values))
	for id := range values {
		ret = append(ret, id)
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i]) < string(ret[j]) })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeTerminalCoverageIDsFromSet(values map[common.CoverageWorkItemID]struct{}) []common.CoverageWorkItemID {
	if len(values) == 0 {
		return nil
	}
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for id := range values {
		ret = append(ret, id)
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i]) < string(ret[j]) })
	return ret
}

func runtimeTerminalStringsFromSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for value := range values {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

func runtimeCoverageItem(fact RuntimeAssessmentFact) workorchresultcontract.DoDCoverageItem {
	return workorchresultcontract.DoDCoverageItem{
		ID: fact.RequirementID, Item: fact.RequirementText, Requirement: fact.RequirementText,
		State: runtimeAssessmentState(fact.Disposition == AssessmentDispositionSatisfied), EvidenceRefs: append([]string(nil), fact.EvidenceRefs...),
	}
}

func runtimeAssessmentState(satisfied bool) string {
	if satisfied {
		return "satisfied"
	}
	return "failed"
}

func runtimeAssessmentFailureReason(fact RuntimeAssessmentFact) string {
	if fact.Disposition == AssessmentDispositionSatisfied {
		return ""
	}
	return fact.Summary
}
