package readmodelprojection

import (
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// MapRuntimeAudit projects persisted Runtime metadata for observation. It
// intentionally never returns raw snapshot content, candidate parameters or
// owner-private tool payloads, and it does not infer transitions from history.
func MapRuntimeAudit(run readmodelcore.Run) *readmodelcore.RuntimeAudit {
	if run.RuntimeState == nil {
		return nil
	}
	state := run.RuntimeState.Clone()
	facts := state.Facts.Normalized()
	ret := &readmodelcore.RuntimeAudit{
		WorkspaceID: string(facts.WorkspaceID), RunID: string(facts.RunID), SchemaVersion: state.SchemaVersion,
		StateRevision: state.StateRevision, EvidenceSequence: state.EvidenceSequence,
		DefinitionRevision: facts.DefinitionRevision, EvidenceRevision: facts.EvidenceRevision,
		Focus: runtimeAuditFocus(facts.Focus), Coverage: runtimeAuditCoverage(facts.Coverage),
		RemainingFocusIDs: runtimeAuditFocusIDs(facts.RemainingFocuses), CompletedFocusIDs: runtimeAuditFocusIDs(facts.CompletedFocuses),
		CoverageBindings: runtimeAuditAllCoverageBindings(facts),
		AcceptedEvidence: runtimeAuditEvidence(facts.Evidence.Accepted), RejectedEvidence: runtimeAuditEvidence(facts.Evidence.Rejected),
		Grounding: runtimeAuditGrounding(facts.Prerequisites, facts.Resources), Authority: runtimeAuditAuthority(state.Decision.AuthoritySnapshot),
		CandidateAttempts: runtimeAuditCandidateAttempts(state.CandidateAttempts), Dispatches: runtimeAuditDispatches(state),
		OperationReceipts: runtimeAuditOperationReceipts(state.OperationReceipts),
		Repair:            runtimeAuditRepair(facts), Assessments: runtimeAuditAssessments(facts.Assessments),
		RepairDirective: runtimeRepairDirectiveProjection(facts.RepairDirective),
	}
	if facts.Failure != nil {
		ret.Failure = runtimeAuditFailure(*facts.Failure)
	}
	if state.Decision.Terminal != nil || state.Termination != nil {
		terminal := readmodelcore.RuntimeAuditTerminal{TerminalDeliveryAttempts: facts.Recovery.TerminalDeliveryAttempts}
		if state.Decision.Terminal != nil {
			terminal.Code = state.Decision.Terminal.Code
			terminal.Summary = state.Decision.Terminal.Summary
		}
		if state.Termination != nil {
			terminal.AdministrativeKind = string(state.Termination.Kind)
			terminal.AdministrativeRevision = state.Termination.Revision
		}
		ret.Terminal = &terminal
	}
	return ret
}

func runtimeAuditFocusIDs(values []runtimecontract.CoverageFocus) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value = value.Normalized(); value.ID != "" {
			ret = append(ret, value.ID)
		}
	}
	return ret
}

func runtimeAuditEvidence(values []runtimecontract.EvidenceFact) []readmodelcore.RuntimeAuditEvidence {
	ret := make([]readmodelcore.RuntimeAuditEvidence, 0, len(values))
	for _, raw := range values {
		value := raw
		item := readmodelcore.RuntimeAuditEvidence{
			ID: string(value.ID), Kind: value.Kind, AuthoritySnapshotID: value.AuthoritySnapshotID, DispatchID: value.DispatchID,
			OperationID: value.OperationID, Revision: value.Revision, FailureClass: string(value.FailureClass), FailureCode: value.FailureCode,
			FailureSignature: value.FailureSignature, TargetRefs: append([]string(nil), value.TargetRefs...),
			BindingRefs: append([]string(nil), value.BindingRefs...), CoverageWorkItemIDs: coverageIDs(value.CoverageWorkItemIDs),
			Quality: string(value.Quality), ZeroWork: value.ZeroWork, EvidenceReason: value.EvidenceReason,
			EvidenceRefs: append([]string(nil), value.EvidenceRefs...), EvidenceTags: append([]string(nil), value.EvidenceTags...),
		}
		if value.Validation != nil {
			observation := value.Validation.Normalized()
			item.ValidationRecipeID = observation.RecipeID
			item.ExecutedChecks = append([]string(nil), observation.ExecutedChecks...)
			item.PassedChecks = append([]string(nil), observation.PassedChecks...)
			item.FailedChecks = append([]string(nil), observation.FailedChecks...)
			item.SkippedChecks = append([]string(nil), observation.SkippedChecks...)
		}
		ret = append(ret, item)
	}
	return ret
}

func runtimeAuditFocus(focus runtimecontract.CoverageFocus) readmodelcore.RuntimeAuditFocus {
	focus = focus.Normalized()
	return readmodelcore.RuntimeAuditFocus{
		ID: focus.ID, SliceID: focus.SliceID, CoverageWorkItemIDs: coverageIDs(focus.CoverageWorkItemIDs),
		TargetRefs: append([]string(nil), focus.TargetRefs...), TargetRole: focus.TargetRole, RequireAssessment: focus.RequireAssessment,
		CoverageBindings: runtimeAuditCoverageBindings(focus.CoverageEvidenceBindings),
	}
}

// runtimeAuditAllCoverageBindings retains the immutable declaration for the
// complete Run. The current focus alone is not sufficient after focus advance:
// a verifier must still be able to inspect the proof that closed an earlier
// CWI. This is a pure projection; it never derives or changes coverage state.
func runtimeAuditAllCoverageBindings(facts runtimecontract.RuntimeFacts) []readmodelcore.RuntimeAuditCoverageBinding {
	values := append([]runtimecontract.CoverageEvidenceBinding(nil), facts.Focus.CoverageEvidenceBindings...)
	for _, focus := range facts.RemainingFocuses {
		values = append(values, focus.CoverageEvidenceBindings...)
	}
	for _, focus := range facts.CompletedFocuses {
		values = append(values, focus.CoverageEvidenceBindings...)
	}
	return runtimeAuditCoverageBindings(values)
}

func runtimeAuditCoverageBindings(values []runtimecontract.CoverageEvidenceBinding) []readmodelcore.RuntimeAuditCoverageBinding {
	values = runtimecontract.NormalizeCoverageEvidenceBindings(values)
	ret := make([]readmodelcore.RuntimeAuditCoverageBinding, 0, len(values))
	for _, raw := range values {
		binding := raw.Normalized()
		item := readmodelcore.RuntimeAuditCoverageBinding{
			ID: binding.ID, CoverageWorkItemID: string(binding.CoverageWorkItemID), FocusID: binding.FocusID,
			DefinitionRevision: binding.DefinitionRevision, Hash: binding.Hash, TargetRefs: append([]string(nil), binding.TargetRefs...),
			TargetSelectorRef: binding.TargetSelectorRef, TargetSelectorOwnerRef: binding.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef: binding.TargetSelectorSubjectRef, ProspectiveTargetRef: binding.ProspectiveTargetRef,
			DeclarationRef: binding.DeclarationRef, PrerequisiteRefs: append([]string(nil), binding.PrerequisiteRefs...),
		}
		for _, rawProof := range binding.Proofs {
			proof := rawProof.Normalized()
			variants := make([]string, 0, len(proof.AllowedOperationVariants))
			for _, variant := range proof.AllowedOperationVariants {
				variants = append(variants, variant.Kind+":"+variant.VariantID)
			}
			item.Proofs = append(item.Proofs, readmodelcore.RuntimeAuditCoverageProof{
				EvidenceKind: proof.EvidenceKind, RequiredEffectScopes: runtimecontract.RuntimeEffectScopesAsStrings(proof.RequiredEffectScopes),
				AllowedOperationKinds: append([]string(nil), proof.AllowedOperationKinds...), AllowedOperationVariants: variants,
				ValidationRecipeID: proof.ValidationRecipeID, RequiredCheckRefs: append([]string(nil), proof.RequiredCheckRefs...),
				MetricConstraints: append([]string(nil), proof.MetricConstraints...), ArtifactRefs: append([]string(nil), proof.ArtifactRefs...),
			})
		}
		ret = append(ret, item)
	}
	return ret
}

func runtimeAuditCoverage(values []runtimecontract.RuntimeCoverageItemFact) []readmodelcore.RuntimeAuditCoverage {
	values = runtimecontract.NormalizeRuntimeCoverageFacts(values)
	ret := make([]readmodelcore.RuntimeAuditCoverage, 0, len(values))
	for _, value := range values {
		ret = append(ret, readmodelcore.RuntimeAuditCoverage{
			ID: string(value.ID), SliceID: value.SliceID, State: string(value.State), EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
			LastAssessmentID: value.LastAssessmentID, ReopenedByEvidenceRefs: append([]string(nil), value.ReopenedByEvidenceRefs...),
		})
	}
	return ret
}

func runtimeAuditGrounding(prerequisites runtimecontract.PrerequisiteState, resources runtimecontract.RuntimeResourceFacts) readmodelcore.RuntimeAuditGrounding {
	resources = resources.Normalized()
	ret := readmodelcore.RuntimeAuditGrounding{
		TargetSnapshots:                 runtimeAuditTargetSnapshots(resources.TargetSnapshots),
		ActivePrerequisites:             runtimeAuditPrerequisites(prerequisites.Pending, prerequisites.Satisfied),
		RetiredPrerequisites:            runtimeAuditPrerequisites(prerequisites.Retired),
		PrerequisiteObservations:        runtimeAuditPrerequisiteObservations(resources.PrerequisiteObservations),
		RetiredPrerequisiteObservations: runtimeAuditPrerequisiteObservations(resources.RetiredPrerequisiteObservations),
		OperationAdmissibilities:        runtimeAuditOperationAdmissibilities(resources.OperationAdmissibilities),
	}
	if resources.TargetSelection != nil {
		selection := resources.TargetSelection.Normalized()
		ret.TargetSelection = &readmodelcore.RuntimeAuditTargetSelection{
			PrerequisiteID: selection.PrerequisiteID, SnapshotID: selection.SnapshotID, OwnerModuleID: selection.OwnerModuleID,
			TargetRole: selection.TargetRole, SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.SelectorOwnerRef,
			SelectorSubjectRef: selection.SelectorSubjectRef, SelectorCandidateRefs: append([]string(nil), selection.SelectorCandidateRefs...),
			Disposition: string(selection.Disposition), TargetRefs: append([]string(nil), selection.TargetRefs...),
			ProspectiveTargetRef: selection.ProspectiveTargetRef, ObservedRevision: selection.ObservedRevision,
		}
	}
	if resources.FailureTarget != nil {
		failureTarget := *resources.FailureTarget
		item := readmodelcore.RuntimeAuditFailureTarget{Signature: failureTarget.Signature, TargetRefs: append([]string(nil), failureTarget.TargetRefs...)}
		if failureTarget.TargetObservation != nil {
			observation := failureTarget.TargetObservation.Normalized()
			item.OwnerModuleID = observation.OwnerModuleID
			item.SubjectRef = observation.SubjectRef
			item.TargetRole = observation.TargetRole
			item.TargetRefs = append([]string(nil), observation.TargetRefs...)
			item.ObservedRevision = observation.ObservedRevision
			item.DiagnosticClass = observation.DiagnosticClass
			item.EvidenceRefs = append([]string(nil), observation.EvidenceRefs...)
			item.ObservationHash = observation.ContentHash
		}
		ret.FailureTarget = &item
	}
	if resources.ProviderCeiling != nil {
		ceiling := resources.ProviderCeiling.Normalized()
		ret.ProviderCeiling = &readmodelcore.RuntimeAuditProviderCeiling{
			ProviderID: ceiling.Target.ProviderID, ModelID: ceiling.Target.ModelID,
			ContextWindowTokens: ceiling.ContextWindowTokens, ProviderMaxOutputTokens: ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens, MaxInputTokens: ceiling.MaxInputTokens,
			MaxBytes: ceiling.MaxBytes, CapabilityRevision: ceiling.CapabilityRevision, ConfigurationRevision: ceiling.ConfigurationRevision,
		}
	}
	return ret
}

func runtimeAuditPrerequisites(groups ...[]runtimecontract.RuntimePrerequisite) []readmodelcore.RuntimeAuditPrerequisite {
	values := make([]runtimecontract.RuntimePrerequisite, 0)
	for _, group := range groups {
		values = append(values, group...)
	}
	seen := make(map[string]struct{}, len(values))
	ret := make([]readmodelcore.RuntimeAuditPrerequisite, 0, len(values))
	for _, raw := range values {
		value := raw
		if value.ID == "" {
			continue
		}
		if _, found := seen[value.ID]; found {
			continue
		}
		seen[value.ID] = struct{}{}
		ret = append(ret, readmodelcore.RuntimeAuditPrerequisite{
			ID: value.ID, Kind: string(value.Kind), Scope: string(value.Scope), FocusID: value.FocusID, OwnerModuleID: value.OwnerModuleID,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeAuditTargetSnapshots(values []runtimecontract.TargetSnapshot) []readmodelcore.RuntimeAuditTargetSnapshot {
	ret := make([]readmodelcore.RuntimeAuditTargetSnapshot, 0, len(values))
	for _, raw := range values {
		value := raw
		ret = append(ret, readmodelcore.RuntimeAuditTargetSnapshot{
			TargetRef: value.TargetRef, ContentHash: value.ContentHash, ContentState: string(value.ContentState), Size: value.Size, ObservedAt: value.ObservedAt,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].TargetRef < ret[j].TargetRef })
	return ret
}

func runtimeAuditPrerequisiteObservations(values []runtimecontract.RuntimePrerequisiteObservation) []readmodelcore.RuntimeAuditPrerequisiteObservation {
	values = runtimecontract.NormalizeRuntimePrerequisiteObservations(values)
	ret := make([]readmodelcore.RuntimeAuditPrerequisiteObservation, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := readmodelcore.RuntimeAuditPrerequisiteObservation{
			PrerequisiteID: value.PrerequisiteID, Kind: string(value.Kind), OwnerModuleID: value.OwnerModuleID,
			ObservationID: value.ObservationID, ContentHash: value.ContentHash,
		}
		for _, dependency := range value.Dependencies {
			item.Dependencies = append(item.Dependencies, readmodelcore.RuntimeAuditFreshnessDependency{
				Domain: string(dependency.Domain), OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, Revision: dependency.Revision,
			})
		}
		ret = append(ret, item)
	}
	return ret
}

func runtimeAuditOperationAdmissibilities(values []runtimecontract.RuntimeOperationAdmissibility) []readmodelcore.RuntimeAuditOperationAdmissibility {
	ret := make([]readmodelcore.RuntimeAuditOperationAdmissibility, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		ret = append(ret, readmodelcore.RuntimeAuditOperationAdmissibility{
			PrerequisiteID: value.PrerequisiteID, SnapshotID: value.SnapshotID, AcquisitionBinding: value.AcquisitionBinding,
			OwnerModuleID: value.OwnerModuleID, OperationKinds: append([]string(nil), value.OperationKinds...),
			TargetRefs: append([]string(nil), value.TargetRefs...), SelectorRefs: append([]string(nil), value.SelectorRefs...),
			ObservedRevision: value.ObservedRevision,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].PrerequisiteID < ret[j].PrerequisiteID })
	return ret
}

func runtimeAuditAuthority(value runtimecontract.AuthoritySnapshot) readmodelcore.RuntimeAuditAuthority {
	return readmodelcore.RuntimeAuditAuthority{
		SnapshotID: value.ID, ConvergenceState: string(value.State), FocusID: value.FocusID,
		RequiredOutputHash: value.RequiredOutputHash, DefinitionRevision: value.DefinitionRevision, EvidenceRevision: value.EvidenceRevision,
		CapabilityStateHash: value.CapabilityStateHash, PrerequisiteFreshnessHash: value.PrerequisiteFreshnessHash,
		OperationContractRevision: value.OperationContractRevision, Attempt: value.Attempt,
	}
}

func runtimeAuditOperationReceipts(values []runtimecontract.RuntimeOperationReceipt) []readmodelcore.RuntimeAuditOperationReceipt {
	ret := make([]readmodelcore.RuntimeAuditOperationReceipt, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := readmodelcore.RuntimeAuditOperationReceipt{
			ExecutionKey: value.ExecutionKey, Fence: value.Fence, OperationID: value.OperationID,
			Outcome: string(value.Outcome), ObservedEffectScope: string(value.ObservedEffectScope), Changed: value.Changed,
			TargetRefs: append([]string(nil), value.TargetRefs...), EvidenceRefs: append([]string(nil), value.Evidence.EvidenceRefs...),
			EvidenceTags: append([]string(nil), value.Evidence.EvidenceTags...),
		}
		if value.Evidence.Validation != nil {
			observation := value.Evidence.Validation.Normalized()
			item.ValidationRecipeID = observation.RecipeID
			item.ExecutedChecks = append([]string(nil), observation.ExecutedChecks...)
			item.PassedChecks = append([]string(nil), observation.PassedChecks...)
			item.FailedChecks = append([]string(nil), observation.FailedChecks...)
			item.SkippedChecks = append([]string(nil), observation.SkippedChecks...)
		}
		if value.Feedback != nil {
			feedback := value.Feedback.CloneOriginal()
			item.FeedbackDomain = string(feedback.Domain)
			item.StableCategory = string(feedback.StableCategory)
			if diagnostic := feedback.OriginalDiagnostic; diagnostic != nil {
				item.OriginalDiagnostic = diagnostic.CloneOriginal()
			}
		}
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].ExecutionKey != ret[j].ExecutionKey {
			return ret[i].ExecutionKey < ret[j].ExecutionKey
		}
		return ret[i].Fence < ret[j].Fence
	})
	return ret
}

func runtimeAuditCandidateAttempts(values []runtimecontract.RuntimeCandidateAttemptRecord) []readmodelcore.RuntimeAuditCandidateAttempt {
	ret := make([]readmodelcore.RuntimeAuditCandidateAttempt, 0, len(values))
	for _, raw := range values {
		value := raw
		item := readmodelcore.RuntimeAuditCandidateAttempt{
			AuthorityContractID: value.AuthorityContractID, AuthoritySnapshotID: value.Transaction.AuthoritySnapshotID,
			State: string(value.State), Attempt: value.Attempt, StablePrefixHash: value.StablePrefixHash,
			ProposalPersisted: value.Proposal != nil, FeasibilityCount: len(value.Feasibilities),
		}
		if value.Failure != nil {
			item.FailureClass = string(value.Failure.Class)
			item.FailureCode = value.Failure.Code
			item.FailureSignature = value.Failure.Signature
		}
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].AuthoritySnapshotID != ret[j].AuthoritySnapshotID {
			return ret[i].AuthoritySnapshotID < ret[j].AuthoritySnapshotID
		}
		return ret[i].Attempt < ret[j].Attempt
	})
	return ret
}

func runtimeAuditDispatches(state runtimecontract.RuntimeStateRecord) []readmodelcore.RuntimeAuditDispatch {
	dispatches := append([]runtimecontract.RuntimeDispatchRecord(nil), state.CompletedDispatches...)
	if state.ActiveDispatch != nil {
		dispatches = append(dispatches, state.ActiveDispatch.Clone())
	}
	ret := make([]readmodelcore.RuntimeAuditDispatch, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ret = append(ret, readmodelcore.RuntimeAuditDispatch{
			ID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID, State: string(dispatch.State),
			CandidateHash: dispatch.CandidateHash, CompletedOperations: len(dispatch.CompletedOperationIDs),
			Operations: runtimeAuditSealedOperations(dispatch.Candidate),
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeAuditSealedOperations(candidate runtimecontract.RuntimeCandidate) []readmodelcore.RuntimeAuditSealedOperation {
	operations := make([]runtimecontract.RuntimeOperation, 0)
	if candidate.StateChange != nil {
		for _, item := range candidate.StateChange.Operations {
			operations = append(operations, item.Operation)
		}
	}
	if candidate.Validation != nil {
		for _, item := range candidate.Validation.Operations {
			operations = append(operations, item.Operation)
		}
	}
	ret := make([]readmodelcore.RuntimeAuditSealedOperation, 0, len(operations))
	for _, raw := range operations {
		operation := raw.Normalized()
		if operation.ID == "" {
			continue
		}
		ret = append(ret, readmodelcore.RuntimeAuditSealedOperation{
			ID: operation.ID, Kind: operation.Kind, BindingRefs: append([]string(nil), operation.BindingRefs...),
			TargetRefs: append([]string(nil), operation.TargetRefs...),
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeAuditRepair(facts runtimecontract.RuntimeFacts) readmodelcore.RuntimeAuditRepair {
	ledger := facts.Repair.Normalized()
	ret := readmodelcore.RuntimeAuditRepair{
		FocusID: ledger.FocusID, SliceID: ledger.SliceID, RepairAttempts: ledger.RepairAttempts,
		ValidationOnlyRetries: ledger.ValidationOnlyRetries, NoProgressCount: ledger.NoProgressCount,
		NoProgressOperationShapeCount: len(ledger.NoProgressOperationShapes), LastProgressRevision: ledger.LastProgressRevision,
		Exhausted: ledger.Exhausted, TerminalDeliveryAttempts: facts.Recovery.Normalized().TerminalDeliveryAttempts,
	}
	if ledger.ActiveGap != nil {
		gap := ledger.ActiveGap.Normalized()
		ret.ActiveGapCoverageWorkItemIDs = coverageIDs(gap.CoverageWorkItemIDs)
		ret.ActiveGapFailureClass = string(gap.FailureClass)
		ret.ActiveGapFailureCode = gap.FailureCode
		ret.ActiveGapFailureSignature = gap.FailureSignature
	}
	return ret
}

func runtimeAuditFailure(raw runtimecontract.FailureFact) *readmodelcore.RuntimeAuditFailure {
	failure := raw.Normalized()
	ret := &readmodelcore.RuntimeAuditFailure{
		Class: string(failure.Class), Code: failure.Code, Signature: failure.Signature,
		Message: failure.Message, TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if diagnostic := failure.OriginalDiagnostic; diagnostic != nil {
		ret.OriginalDiagnostic = diagnostic.CloneOriginal()
	}
	return ret
}

func runtimeAuditAssessments(values []runtimecontract.RuntimeAssessmentFact) []readmodelcore.RuntimeAuditAssessment {
	ret := make([]readmodelcore.RuntimeAuditAssessment, 0, len(values))
	for _, raw := range values {
		value := raw
		item := readmodelcore.RuntimeAuditAssessment{
			ID: value.ID, FocusID: value.FocusID, SliceID: value.SliceID, Scope: value.Scope, RequirementID: value.RequirementID,
			RequirementText: value.RequirementText, ClaimedDisposition: string(value.ClaimedDisposition), CanonicalDisposition: string(value.Disposition), CoverageWorkItemIDs: coverageIDs(value.CoverageWorkItemIDs),
			Summary: value.Summary, EvidenceRefs: append([]string(nil), value.EvidenceRefs...), ArtifactRefs: append([]string(nil), value.ArtifactRefs...),
		}
		for _, rawItem := range value.Items {
			assessmentItem := rawItem.Normalized()
			item.Items = append(item.Items, readmodelcore.RuntimeAuditAssessmentItem{
				CoverageWorkItemID: string(assessmentItem.CoverageWorkItemID), Satisfied: assessmentItem.Satisfied, Reason: assessmentItem.Reason,
				EvidenceRefs: append([]string(nil), assessmentItem.EvidenceRefs...), ArtifactRefs: artifactIDs(assessmentItem.ArtifactRefs),
			})
		}
		if value.Gap != nil {
			gap := value.Gap.Normalized()
			item.Gap = &readmodelcore.RuntimeAuditAssessmentGap{
				CoverageWorkItemIDs: coverageIDs(gap.CoverageWorkItemIDs), MissingEvidenceKinds: append([]string(nil), gap.MissingEvidenceKinds...),
				TargetRefs: append([]string(nil), gap.TargetRefs...), EvidenceRefs: append([]string(nil), gap.EvidenceRefs...), Reason: gap.Reason,
				FailureClass: string(gap.FailureClass), FailureCode: gap.FailureCode, FailureSignature: gap.FailureSignature,
			}
		}
		ret = append(ret, item)
	}
	return ret
}

func artifactIDs(values []common.ArtifactID) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, string(value))
	}
	return ret
}

func coverageIDs(values []common.CoverageWorkItemID) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, string(value))
	}
	return ret
}
