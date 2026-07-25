package readmodelprojection

import (
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// MapRuntimeV2Audit projects persisted Runtime V2 metadata for observation. It
// intentionally never returns raw snapshot content, candidate parameters or
// owner-private tool payloads, and it does not infer transitions from history.
func MapRuntimeV2Audit(run readmodelcore.Run) *readmodelcore.RuntimeV2Audit {
	if run.RuntimeState == nil {
		return nil
	}
	state := run.RuntimeState.Clone()
	facts := state.Facts.Normalized()
	ret := &readmodelcore.RuntimeV2Audit{
		WorkspaceID: string(facts.WorkspaceID), RunID: string(facts.RunID), SchemaVersion: state.SchemaVersion,
		StateRevision: state.StateRevision, EvidenceSequence: state.EvidenceSequence,
		DefinitionRevision: facts.DefinitionRevision, EvidenceRevision: facts.EvidenceRevision,
		Focus: runtimeV2AuditFocus(facts.Focus), Coverage: runtimeV2AuditCoverage(facts.Coverage),
		RemainingFocusIDs: runtimeV2AuditFocusIDs(facts.RemainingFocuses), CompletedFocusIDs: runtimeV2AuditFocusIDs(facts.CompletedFocuses),
		AcceptedEvidence: runtimeV2AuditEvidence(facts.Evidence.Accepted), RejectedEvidence: runtimeV2AuditEvidence(facts.Evidence.Rejected),
		Grounding: runtimeV2AuditGrounding(facts.Prerequisites, facts.Resources), Authority: runtimeV2AuditAuthority(state.Decision.AuthoritySnapshot),
		CandidateAttempts: runtimeV2AuditCandidateAttempts(state.CandidateAttempts), Dispatches: runtimeV2AuditDispatches(state),
		Repair: runtimeV2AuditRepair(facts), Assessments: runtimeV2AuditAssessments(facts.Assessments),
	}
	if state.Decision.Terminal != nil || state.Termination != nil {
		terminal := readmodelcore.RuntimeV2AuditTerminal{TerminalDeliveryAttempts: facts.Recovery.TerminalDeliveryAttempts}
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

func runtimeV2AuditFocusIDs(values []runtimecontract.CoverageFocus) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value = value.Normalized(); value.ID != "" {
			ret = append(ret, value.ID)
		}
	}
	return ret
}

func runtimeV2AuditEvidence(values []runtimecontract.EvidenceFact) []readmodelcore.RuntimeV2AuditEvidence {
	ret := make([]readmodelcore.RuntimeV2AuditEvidence, 0, len(values))
	for _, raw := range values {
		value := raw
		ret = append(ret, readmodelcore.RuntimeV2AuditEvidence{ID: string(value.ID), Kind: value.Kind, AuthoritySnapshotID: value.AuthoritySnapshotID, DispatchID: value.DispatchID, OperationID: value.OperationID, Revision: value.Revision, FailureClass: string(value.FailureClass), FailureCode: value.FailureCode, FailureSignature: value.FailureSignature, TargetRefs: append([]string(nil), value.TargetRefs...), CoverageWorkItemIDs: coverageIDs(value.CoverageWorkItemIDs)})
	}
	return ret
}

func runtimeV2AuditFocus(focus runtimecontract.CoverageFocus) readmodelcore.RuntimeV2AuditFocus {
	focus = focus.Normalized()
	return readmodelcore.RuntimeV2AuditFocus{
		ID: focus.ID, SliceID: focus.SliceID, CoverageWorkItemIDs: coverageIDs(focus.CoverageWorkItemIDs),
		TargetRefs: append([]string(nil), focus.TargetRefs...), TargetRole: focus.TargetRole, RequireAssessment: focus.RequireAssessment,
	}
}

func runtimeV2AuditCoverage(values []runtimecontract.RuntimeCoverageItemFact) []readmodelcore.RuntimeV2AuditCoverage {
	values = runtimecontract.NormalizeRuntimeCoverageFacts(values)
	ret := make([]readmodelcore.RuntimeV2AuditCoverage, 0, len(values))
	for _, value := range values {
		ret = append(ret, readmodelcore.RuntimeV2AuditCoverage{
			ID: string(value.ID), SliceID: value.SliceID, State: string(value.State), EvidenceRefs: append([]string(nil), value.EvidenceRefs...),
			LastAssessmentID: value.LastAssessmentID, ReopenedByEvidenceRefs: append([]string(nil), value.ReopenedByEvidenceRefs...),
		})
	}
	return ret
}

func runtimeV2AuditGrounding(prerequisites runtimecontract.PrerequisiteState, resources runtimecontract.RuntimeResourceFacts) readmodelcore.RuntimeV2AuditGrounding {
	resources = resources.Normalized()
	ret := readmodelcore.RuntimeV2AuditGrounding{
		TargetSnapshots:                 runtimeV2AuditTargetSnapshots(resources.TargetSnapshots),
		ActivePrerequisites:             runtimeV2AuditPrerequisites(prerequisites.Pending, prerequisites.Satisfied),
		RetiredPrerequisites:            runtimeV2AuditPrerequisites(prerequisites.Retired),
		PrerequisiteObservations:        runtimeV2AuditPrerequisiteObservations(resources.PrerequisiteObservations),
		RetiredPrerequisiteObservations: runtimeV2AuditPrerequisiteObservations(resources.RetiredPrerequisiteObservations),
		OperationAdmissibilities:        runtimeV2AuditOperationAdmissibilities(resources.OperationAdmissibilities),
	}
	if resources.TargetSelection != nil {
		selection := resources.TargetSelection.Normalized()
		ret.TargetSelection = &readmodelcore.RuntimeV2AuditTargetSelection{
			PrerequisiteID: selection.PrerequisiteID, SnapshotID: selection.SnapshotID, OwnerModuleID: selection.OwnerModuleID,
			TargetRole: selection.TargetRole, SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.SelectorOwnerRef,
			SelectorSubjectRef: selection.SelectorSubjectRef, SelectorCandidateRefs: append([]string(nil), selection.SelectorCandidateRefs...),
			Disposition: string(selection.Disposition), TargetRefs: append([]string(nil), selection.TargetRefs...),
			ProspectiveTargetRef: selection.ProspectiveTargetRef, ObservedRevision: selection.ObservedRevision,
		}
	}
	if resources.FailureTarget != nil {
		failureTarget := *resources.FailureTarget
		item := readmodelcore.RuntimeV2AuditFailureTarget{Signature: failureTarget.Signature, TargetRefs: append([]string(nil), failureTarget.TargetRefs...)}
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
		ret.ProviderCeiling = &readmodelcore.RuntimeV2AuditProviderCeiling{
			ProviderID: ceiling.Target.ProviderID, ModelID: ceiling.Target.ModelID,
			ContextWindowTokens: ceiling.ContextWindowTokens, ProviderMaxOutputTokens: ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens, MaxInputTokens: ceiling.MaxInputTokens,
			MaxBytes: ceiling.MaxBytes, CapabilityRevision: ceiling.CapabilityRevision, ConfigurationRevision: ceiling.ConfigurationRevision,
		}
	}
	return ret
}

func runtimeV2AuditPrerequisites(groups ...[]runtimecontract.RuntimePrerequisite) []readmodelcore.RuntimeV2AuditPrerequisite {
	values := make([]runtimecontract.RuntimePrerequisite, 0)
	for _, group := range groups {
		values = append(values, group...)
	}
	seen := make(map[string]struct{}, len(values))
	ret := make([]readmodelcore.RuntimeV2AuditPrerequisite, 0, len(values))
	for _, raw := range values {
		value := raw
		if value.ID == "" {
			continue
		}
		if _, found := seen[value.ID]; found {
			continue
		}
		seen[value.ID] = struct{}{}
		ret = append(ret, readmodelcore.RuntimeV2AuditPrerequisite{
			ID: value.ID, Kind: string(value.Kind), Scope: string(value.Scope), FocusID: value.FocusID, OwnerModuleID: value.OwnerModuleID,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeV2AuditTargetSnapshots(values []runtimecontract.TargetSnapshot) []readmodelcore.RuntimeV2AuditTargetSnapshot {
	ret := make([]readmodelcore.RuntimeV2AuditTargetSnapshot, 0, len(values))
	for _, raw := range values {
		value := raw
		ret = append(ret, readmodelcore.RuntimeV2AuditTargetSnapshot{
			TargetRef: value.TargetRef, ContentHash: value.ContentHash, ContentState: string(value.ContentState), Size: value.Size, ObservedAt: value.ObservedAt,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].TargetRef < ret[j].TargetRef })
	return ret
}

func runtimeV2AuditPrerequisiteObservations(values []runtimecontract.RuntimePrerequisiteObservation) []readmodelcore.RuntimeV2AuditPrerequisiteObservation {
	values = runtimecontract.NormalizeRuntimePrerequisiteObservations(values)
	ret := make([]readmodelcore.RuntimeV2AuditPrerequisiteObservation, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := readmodelcore.RuntimeV2AuditPrerequisiteObservation{
			PrerequisiteID: value.PrerequisiteID, Kind: string(value.Kind), OwnerModuleID: value.OwnerModuleID,
			ObservationID: value.ObservationID, ContentHash: value.ContentHash,
		}
		for _, dependency := range value.Dependencies {
			item.Dependencies = append(item.Dependencies, readmodelcore.RuntimeV2AuditFreshnessDependency{
				Domain: string(dependency.Domain), OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, Revision: dependency.Revision,
			})
		}
		ret = append(ret, item)
	}
	return ret
}

func runtimeV2AuditOperationAdmissibilities(values []runtimecontract.RuntimeOperationAdmissibility) []readmodelcore.RuntimeV2AuditOperationAdmissibility {
	ret := make([]readmodelcore.RuntimeV2AuditOperationAdmissibility, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		ret = append(ret, readmodelcore.RuntimeV2AuditOperationAdmissibility{
			PrerequisiteID: value.PrerequisiteID, SnapshotID: value.SnapshotID, AcquisitionBinding: value.AcquisitionBinding,
			OwnerModuleID: value.OwnerModuleID, OperationKinds: append([]string(nil), value.OperationKinds...),
			TargetRefs: append([]string(nil), value.TargetRefs...), SelectorRefs: append([]string(nil), value.SelectorRefs...),
			ObservedRevision: value.ObservedRevision,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].PrerequisiteID < ret[j].PrerequisiteID })
	return ret
}

func runtimeV2AuditAuthority(value runtimecontract.AuthoritySnapshot) readmodelcore.RuntimeV2AuditAuthority {
	return readmodelcore.RuntimeV2AuditAuthority{
		SnapshotID: value.ID, State: string(value.State), Action: string(value.Action), FocusID: value.FocusID,
		RequiredOutputHash: value.RequiredOutputHash, DefinitionRevision: value.DefinitionRevision, EvidenceRevision: value.EvidenceRevision,
		CapabilityStateHash: value.CapabilityStateHash, PrerequisiteFreshnessHash: value.PrerequisiteFreshnessHash,
		OperationContractRevision: value.OperationContractRevision, Attempt: value.Attempt,
	}
}

func runtimeV2AuditCandidateAttempts(values []runtimecontract.RuntimeCandidateAttemptRecord) []readmodelcore.RuntimeV2AuditCandidateAttempt {
	ret := make([]readmodelcore.RuntimeV2AuditCandidateAttempt, 0, len(values))
	for _, raw := range values {
		value := raw
		item := readmodelcore.RuntimeV2AuditCandidateAttempt{
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

func runtimeV2AuditDispatches(state runtimecontract.RuntimeStateRecord) []readmodelcore.RuntimeV2AuditDispatch {
	dispatches := append([]runtimecontract.RuntimeDispatchRecord(nil), state.CompletedDispatches...)
	if state.ActiveDispatch != nil {
		dispatches = append(dispatches, state.ActiveDispatch.Clone())
	}
	ret := make([]readmodelcore.RuntimeV2AuditDispatch, 0, len(dispatches))
	for _, dispatch := range dispatches {
		ret = append(ret, readmodelcore.RuntimeV2AuditDispatch{
			ID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID, State: string(dispatch.State),
			CandidateHash: dispatch.CandidateHash, CompletedOperations: len(dispatch.CompletedOperationIDs),
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func runtimeV2AuditRepair(facts runtimecontract.RuntimeFacts) readmodelcore.RuntimeV2AuditRepair {
	ledger := facts.Repair.Normalized()
	ret := readmodelcore.RuntimeV2AuditRepair{
		FocusID: ledger.FocusID, SliceID: ledger.SliceID, RepairAttempts: ledger.RepairAttempts,
		ValidationOnlyRetries: ledger.ValidationOnlyRetries, NoProgressCount: ledger.NoProgressCount,
		NoProgressOperationShapeCount: len(ledger.NoProgressOperationShapes), LastProgressRevision: ledger.LastProgressRevision,
		Exhausted: ledger.Exhausted, TerminalDeliveryAttempts: facts.Recovery.Normalized().TerminalDeliveryAttempts,
	}
	if ledger.ActiveGap != nil {
		gap := ledger.ActiveGap.Normalized()
		ret.ActiveGapCoverageWorkItemIDs = coverageIDs(gap.CoverageWorkItemIDs)
		ret.ActiveGapMissingEvidenceKinds = append([]string(nil), gap.MissingEvidenceKinds...)
		ret.ActiveGapTargetRefs = append([]string(nil), gap.TargetRefs...)
		ret.ActiveGapFailureSignature = gap.FailureSignature
	}
	return ret
}

func runtimeV2AuditAssessments(values []runtimecontract.RuntimeAssessmentFact) []readmodelcore.RuntimeV2AuditAssessment {
	ret := make([]readmodelcore.RuntimeV2AuditAssessment, 0, len(values))
	for _, raw := range values {
		value := raw
		item := readmodelcore.RuntimeV2AuditAssessment{
			ID: value.ID, FocusID: value.FocusID, SliceID: value.SliceID, Disposition: string(value.Disposition),
			CoverageWorkItemIDs: coverageIDs(value.CoverageWorkItemIDs),
		}
		if value.Gap != nil {
			gap := value.Gap.Normalized()
			item.GapCoverageWorkItemIDs = coverageIDs(gap.CoverageWorkItemIDs)
			item.GapMissingEvidenceKinds = append([]string(nil), gap.MissingEvidenceKinds...)
			item.GapTargetRefs = append([]string(nil), gap.TargetRefs...)
			item.GapFailureSignature = gap.FailureSignature
		}
		ret = append(ret, item)
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
