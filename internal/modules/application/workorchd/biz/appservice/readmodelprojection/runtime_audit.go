package readmodelprojection

import (
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// mapRuntimeAudit projects persisted Runtime metadata for observation. It
// intentionally never returns raw snapshot content, candidate parameters or
// owner-private tool payloads, and it does not infer transitions from history.
func mapRuntimeAudit(run appmodelcore.Run) *projectioncontract.RuntimeAudit {
	if run.RuntimeState == nil {
		return nil
	}
	state := run.RuntimeState.Clone()
	facts := state.Facts.Normalized()
	ret := &projectioncontract.RuntimeAudit{
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
		terminal := projectioncontract.RuntimeAuditTerminal{TerminalDeliveryAttempts: facts.Recovery.TerminalDeliveryAttempts}
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

func runtimeAuditEvidence(values []runtimecontract.EvidenceFact) []projectioncontract.RuntimeAuditEvidence {
	ret := make([]projectioncontract.RuntimeAuditEvidence, 0, len(values))
	for _, raw := range values {
		value := raw
		item := projectioncontract.RuntimeAuditEvidence{
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

func runtimeAuditFocus(focus runtimecontract.CoverageFocus) projectioncontract.RuntimeAuditFocus {
	focus = focus.Normalized()
	return projectioncontract.RuntimeAuditFocus{
		ID: focus.ID, SliceID: focus.SliceID, CoverageWorkItemIDs: coverageIDs(focus.CoverageWorkItemIDs),
		TargetRefs: append([]string(nil), focus.TargetRefs...), TargetRole: focus.TargetRole, RequireAssessment: focus.RequireAssessment,
		CoverageBindings: runtimeAuditCoverageBindings(focus.CoverageEvidenceBindings),
	}
}

// runtimeAuditAllCoverageBindings retains the immutable declaration for the
// complete Run. The current focus alone is not sufficient after focus advance:
// a verifier must still be able to inspect the proof that closed an earlier
// CWI. This is a pure projection; it never derives or changes coverage state.
func runtimeAuditAllCoverageBindings(facts runtimecontract.RuntimeFacts) []projectioncontract.RuntimeAuditCoverageBinding {
	values := append([]runtimecontract.CoverageEvidenceBinding(nil), facts.Focus.CoverageEvidenceBindings...)
	for _, focus := range facts.RemainingFocuses {
		values = append(values, focus.CoverageEvidenceBindings...)
	}
	for _, focus := range facts.CompletedFocuses {
		values = append(values, focus.CoverageEvidenceBindings...)
	}
	return runtimeAuditCoverageBindings(values)
}

func runtimeAuditCoverageBindings(values []runtimecontract.CoverageEvidenceBinding) []projectioncontract.RuntimeAuditCoverageBinding {
	values = runtimecontract.NormalizeCoverageEvidenceBindings(values)
	ret := make([]projectioncontract.RuntimeAuditCoverageBinding, 0, len(values))
	for _, raw := range values {
		binding := raw.Normalized()
		item := projectioncontract.RuntimeAuditCoverageBinding{
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
			item.Proofs = append(item.Proofs, projectioncontract.RuntimeAuditCoverageProof{
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

func runtimeAuditCoverage(values []runtimecontract.RuntimeCoverageItemFact) []projectioncontract.RuntimeAuditCoverage {
	values = runtimecontract.NormalizeRuntimeCoverageFacts(values)
	ret := make([]projectioncontract.RuntimeAuditCoverage, 0, len(values))
	for _, value := range values {
		ret = append(ret, projectioncontract.RuntimeAuditCoverage{
			ID: string(value.ID), SliceID: value.SliceID, State: string(value.State), EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
			LastAssessmentID: value.LastAssessmentID, ReopenedByEvidenceRefs: append([]string(nil), value.ReopenedByEvidenceRefs...),
		})
	}
	return ret
}

func runtimeAuditGrounding(prerequisites runtimecontract.PrerequisiteState, resources runtimecontract.RuntimeResourceFacts) projectioncontract.RuntimeAuditGrounding {
	resources = resources.Normalized()
	ret := projectioncontract.RuntimeAuditGrounding{
		TargetSnapshots:                 runtimeAuditTargetSnapshots(resources.TargetSnapshots),
		ActivePrerequisites:             runtimeAuditPrerequisites(prerequisites.Pending, prerequisites.Satisfied),
		RetiredPrerequisites:            runtimeAuditPrerequisites(prerequisites.Retired),
		PrerequisiteObservations:        runtimeAuditPrerequisiteObservations(resources.PrerequisiteObservations),
		RetiredPrerequisiteObservations: runtimeAuditPrerequisiteObservations(resources.RetiredPrerequisiteObservations),
		OperationAdmissibilities:        runtimeAuditOperationAdmissibilities(resources.OperationAdmissibilities),
	}
	if resources.TargetSelection != nil {
		selection := resources.TargetSelection.Normalized()
		ret.TargetSelection = &projectioncontract.RuntimeAuditTargetSelection{
			PrerequisiteID: selection.PrerequisiteID, SnapshotID: selection.SnapshotID, OwnerModuleID: selection.OwnerModuleID,
			TargetRole: selection.TargetRole, SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.SelectorOwnerRef,
			SelectorSubjectRef: selection.SelectorSubjectRef, SelectorCandidateRefs: append([]string(nil), selection.SelectorCandidateRefs...),
			Disposition: string(selection.Disposition), TargetRefs: append([]string(nil), selection.TargetRefs...),
			ProspectiveTargetRef: selection.ProspectiveTargetRef, ObservedRevision: selection.ObservedRevision,
		}
	}
	if resources.FailureTarget != nil {
		failureTarget := *resources.FailureTarget
		item := projectioncontract.RuntimeAuditFailureTarget{Signature: failureTarget.Signature, TargetRefs: append([]string(nil), failureTarget.TargetRefs...)}
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
		ret.ProviderCeiling = &projectioncontract.RuntimeAuditProviderCeiling{
			ProviderID: ceiling.Selection.Target.ProviderID, ModelID: ceiling.Selection.Target.ModelID,
			ContextWindowTokens: ceiling.ContextWindowTokens, ProviderMaxOutputTokens: ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens, MaxInputTokens: ceiling.MaxInputTokens,
			MaxBytes: ceiling.MaxBytes, CapabilityRevision: ceiling.CapabilityRevision, ConfigurationRevision: ceiling.ConfigurationRevision,
		}
	}
	return ret
}

func runtimeAuditPrerequisites(groups ...[]runtimecontract.RuntimePrerequisite) []projectioncontract.RuntimeAuditPrerequisite {
	values := make([]runtimecontract.RuntimePrerequisite, 0)
	for _, group := range groups {
		values = append(values, group...)
	}
	seen := make(map[string]struct{}, len(values))
	ret := make([]projectioncontract.RuntimeAuditPrerequisite, 0, len(values))
	for _, raw := range values {
		value := raw
		if value.ID == "" {
			continue
		}
		if _, found := seen[value.ID]; found {
			continue
		}
		seen[value.ID] = struct{}{}
		ret = append(ret, projectioncontract.RuntimeAuditPrerequisite{
			ID: value.ID, Kind: string(value.Kind), Scope: string(value.Scope), FocusID: value.FocusID, OwnerModuleID: value.OwnerModuleID,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeAuditTargetSnapshots(values []runtimecontract.TargetSnapshot) []projectioncontract.RuntimeAuditTargetSnapshot {
	ret := make([]projectioncontract.RuntimeAuditTargetSnapshot, 0, len(values))
	for _, raw := range values {
		value := raw
		ret = append(ret, projectioncontract.RuntimeAuditTargetSnapshot{
			TargetRef: value.TargetRef, ContentHash: value.ContentHash, ContentState: string(value.ContentState), Size: value.Size, ObservedAt: value.ObservedAt,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].TargetRef < ret[j].TargetRef })
	return ret
}

func runtimeAuditPrerequisiteObservations(values []runtimecontract.RuntimePrerequisiteObservation) []projectioncontract.RuntimeAuditPrerequisiteObservation {
	values = runtimecontract.NormalizeRuntimePrerequisiteObservations(values)
	ret := make([]projectioncontract.RuntimeAuditPrerequisiteObservation, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := projectioncontract.RuntimeAuditPrerequisiteObservation{
			PrerequisiteID: value.PrerequisiteID, Kind: string(value.Kind), OwnerModuleID: value.OwnerModuleID,
			ObservationID: value.ObservationID, ContentHash: value.ContentHash,
		}
		for _, dependency := range value.Dependencies {
			item.Dependencies = append(item.Dependencies, projectioncontract.RuntimeAuditFreshnessDependency{
				Domain: string(dependency.Domain), OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, Revision: dependency.Revision,
			})
		}
		ret = append(ret, item)
	}
	return ret
}

func runtimeAuditOperationAdmissibilities(values []runtimecontract.RuntimeOperationAdmissibility) []projectioncontract.RuntimeAuditOperationAdmissibility {
	ret := make([]projectioncontract.RuntimeAuditOperationAdmissibility, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		surfaces := make([]projectioncontract.RuntimeAuditValidationSurface, 0, len(value.ValidationSurfaces))
		for _, surface := range value.ValidationSurfaces {
			surfaces = append(surfaces, projectioncontract.RuntimeAuditValidationSurface{
				ID: surface.ID, OwnerModuleID: surface.OwnerModuleID, OperationKind: surface.OperationKind,
				Readiness: string(surface.Readiness), ReadinessReason: surface.ReadinessReason,
				InputSchemaHash: surface.InputSchemaHash, InputBindingRef: surface.InputBindingRef,
				PrerequisiteRef: surface.PrerequisiteRef, CheckRefs: append([]string(nil), surface.CheckRefs...),
				EffectScope: string(surface.EffectScope), IdempotencyClass: string(surface.IdempotencyClass),
			})
		}
		ret = append(ret, projectioncontract.RuntimeAuditOperationAdmissibility{
			PrerequisiteID: value.PrerequisiteID, SnapshotID: value.SnapshotID, AcquisitionBinding: value.AcquisitionBinding,
			OwnerModuleID: value.OwnerModuleID, OperationKinds: append([]string(nil), value.OperationKinds...),
			TargetRefs: append([]string(nil), value.TargetRefs...), SelectorRefs: append([]string(nil), value.SelectorRefs...),
			ValidationRecipeRefs: append([]string(nil), value.ValidationRecipeRefs...), ValidationSurfaces: surfaces,
			ObservedRevision: value.ObservedRevision,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].PrerequisiteID < ret[j].PrerequisiteID })
	return ret
}

func runtimeAuditAuthority(value runtimecontract.AuthoritySnapshot) projectioncontract.RuntimeAuditAuthority {
	return projectioncontract.RuntimeAuditAuthority{
		SnapshotID: value.ID, ConvergenceState: string(value.State), FocusID: value.FocusID,
		RequiredOutputHash: value.RequiredOutputHash, DefinitionRevision: value.DefinitionRevision, EvidenceRevision: value.EvidenceRevision,
		CapabilityStateHash: value.CapabilityStateHash, PrerequisiteFreshnessHash: value.PrerequisiteFreshnessHash,
		OperationContractRevision: value.OperationContractRevision, Attempt: value.Attempt,
	}
}

func runtimeAuditOperationReceipts(values []runtimecontract.RuntimeOperationReceipt) []projectioncontract.RuntimeAuditOperationReceipt {
	ret := make([]projectioncontract.RuntimeAuditOperationReceipt, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := projectioncontract.RuntimeAuditOperationReceipt{
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

func runtimeAuditCandidateAttempts(values []runtimecontract.RuntimeCandidateAttemptRecord) []projectioncontract.RuntimeAuditCandidateAttempt {
	ret := make([]projectioncontract.RuntimeAuditCandidateAttempt, 0, len(values))
	for _, raw := range values {
		value := raw
		item := projectioncontract.RuntimeAuditCandidateAttempt{
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

func runtimeAuditDispatches(state runtimecontract.RuntimeStateRecord) []projectioncontract.RuntimeAuditDispatch {
	dispatches := append([]runtimecontract.RuntimeDispatchRecord(nil), state.CompletedDispatches...)
	if state.ActiveDispatch != nil {
		dispatches = append(dispatches, state.ActiveDispatch.Clone())
	}
	ret := make([]projectioncontract.RuntimeAuditDispatch, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ret = append(ret, projectioncontract.RuntimeAuditDispatch{
			ID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID, State: string(dispatch.State),
			CandidateHash: dispatch.CandidateHash, CompletedOperations: len(dispatch.CompletedOperationIDs),
			Operations: runtimeAuditSealedOperations(dispatch.Candidate),
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeAuditSealedOperations(candidate runtimecontract.RuntimeCandidate) []projectioncontract.RuntimeAuditSealedOperation {
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
	ret := make([]projectioncontract.RuntimeAuditSealedOperation, 0, len(operations))
	for _, raw := range operations {
		operation := raw.Normalized()
		if operation.ID == "" {
			continue
		}
		ret = append(ret, projectioncontract.RuntimeAuditSealedOperation{
			ID: operation.ID, Kind: operation.Kind, BindingRefs: append([]string(nil), operation.BindingRefs...),
			TargetRefs: append([]string(nil), operation.TargetRefs...),
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeAuditRepair(facts runtimecontract.RuntimeFacts) projectioncontract.RuntimeAuditRepair {
	ledger := facts.Repair.Normalized()
	ret := projectioncontract.RuntimeAuditRepair{
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

func runtimeAuditFailure(raw runtimecontract.FailureFact) *projectioncontract.RuntimeAuditFailure {
	failure := raw.Normalized()
	ret := &projectioncontract.RuntimeAuditFailure{
		Class: string(failure.Class), Code: failure.Code, Signature: failure.Signature,
		Message: failure.Message, TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if diagnostic := failure.OriginalDiagnostic; diagnostic != nil {
		ret.OriginalDiagnostic = diagnostic.CloneOriginal()
	}
	return ret
}

func runtimeAuditAssessments(values []runtimecontract.RuntimeAssessmentFact) []projectioncontract.RuntimeAuditAssessment {
	ret := make([]projectioncontract.RuntimeAuditAssessment, 0, len(values))
	for _, raw := range values {
		value := raw
		item := projectioncontract.RuntimeAuditAssessment{
			ID: value.ID, FocusID: value.FocusID, SliceID: value.SliceID, Scope: value.Scope, RequirementID: value.RequirementID,
			RequirementText: value.RequirementText, ClaimedDisposition: string(value.ClaimedDisposition), CanonicalDisposition: string(value.Disposition), CoverageWorkItemIDs: coverageIDs(value.CoverageWorkItemIDs),
			Summary: value.Summary, EvidenceRefs: append([]string(nil), value.EvidenceRefs...), ArtifactRefs: append([]string(nil), value.ArtifactRefs...),
		}
		for _, rawItem := range value.Items {
			assessmentItem := rawItem.Normalized()
			item.Items = append(item.Items, projectioncontract.RuntimeAuditAssessmentItem{
				CoverageWorkItemID: string(assessmentItem.CoverageWorkItemID), Satisfied: assessmentItem.Satisfied, Reason: assessmentItem.Reason,
				EvidenceRefs: append([]string(nil), assessmentItem.EvidenceRefs...), ArtifactRefs: artifactIDs(assessmentItem.ArtifactRefs),
			})
		}
		if value.Gap != nil {
			gap := value.Gap.Normalized()
			item.Gap = &projectioncontract.RuntimeAuditAssessmentGap{
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
