package readmodelprojection

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func MapAcceptanceProjection(run readmodelcore.Run) readmodelcore.AcceptanceProjection {
	if run.RuntimeState == nil {
		return emptyAcceptanceProjection()
	}
	facts := run.RuntimeState.Facts.Normalized()
	projection := readmodelcore.AcceptanceProjection{}
	byID := map[string]string{}
	for _, item := range facts.Coverage {
		byID[string(item.ID)] = string(item.State)
	}
	for _, slice := range facts.Slices {
		item := readmodelcore.AcceptanceSlice{
			ID: slice.ID, Title: slice.Title, Scope: slice.Scope, Priority: slice.Priority,
			RequiredEvidenceKinds: append([]string(nil), slice.RequiredEvidenceKinds...),
			CompletionPolicy:      slice.CompletionPolicy, ResultPolicy: slice.ResultPolicy,
		}
		for _, id := range slice.CoverageWorkItemIDs {
			item.CoverageWorkItemIDs = append(item.CoverageWorkItemIDs, string(id))
		}
		item.State = "accepted"
		for _, id := range item.CoverageWorkItemIDs {
			if byID[id] != "satisfied" {
				item.State = "pending"
				break
			}
		}
		if slice.ID == facts.Focus.SliceID {
			projection.ActiveSliceID = slice.ID
			if item.State != "accepted" {
				item.State = "active"
			}
		}
		projection.Slices = append(projection.Slices, item)
	}
	if len(projection.Slices) == 0 && facts.Focus.ID != "" {
		projection.ActiveSliceID = facts.Focus.SliceID
	}
	projection.CurrentFocus = &readmodelcore.CoverageFocus{
		ID: facts.Focus.ID, SliceID: facts.Focus.SliceID,
		RequiredEvidenceKinds: append([]string(nil), facts.Focus.RequiredEvidenceKinds...),
		TargetRefs:            append([]string(nil), facts.Focus.TargetRefs...), State: string(run.RuntimeState.Decision.State),
		Attempt: facts.Repair.RepairAttempts,
	}
	for _, id := range facts.Focus.CoverageWorkItemIDs {
		projection.CurrentFocus.CoverageWorkItemIDs = append(projection.CurrentFocus.CoverageWorkItemIDs, string(id))
	}
	if facts.Repair.ActiveGap != nil {
		projection.CurrentFocus.FailureClass = string(facts.Repair.ActiveGap.FailureClass)
		projection.CurrentFocus.FailureSignature = facts.Repair.ActiveGap.FailureSignature
	}
	projection.Budget = &readmodelcore.ConvergenceBudget{
		FocusID: facts.Focus.ID, SliceID: facts.Focus.SliceID, State: string(run.RuntimeState.Decision.State), Decision: string(run.RuntimeState.Decision.State),
		MaxRepairAttemptsPerFocus: facts.ConvergencePolicy.MaxRepairAttemptsPerFocus,
		MaxValidationOnlyRetries:  facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus,
		RepairAttempts:            facts.Repair.RepairAttempts, ValidationOnlyRetries: facts.Repair.ValidationOnlyRetries,
		RepeatedFailureSignatures: len(facts.Repair.SignatureCounts), NoCoverageDeltaRounds: facts.Repair.NoProgressCount,
		MaxProviderTransportAttempts: facts.RecoveryPolicy.MaxProviderTransportAttempts,
		ProviderTransportAttempts:    facts.Recovery.ProviderTransportAttempts,
		MaxOperationRecoveryAttempts: facts.RecoveryPolicy.MaxOperationRecoveryAttempts,
		OperationRecoveryAttempts:    facts.Recovery.OperationRecoveryAttempts,
		MaxCASReloadAttempts:         facts.RecoveryPolicy.MaxCASReloadAttempts, CASReloadAttempts: facts.Recovery.CASReloadAttempts,
		MaxTerminalDeliveryAttempts: facts.RecoveryPolicy.MaxTerminalDeliveryAttempts,
		TerminalDeliveryAttempts:    facts.Recovery.TerminalDeliveryAttempts,
		MaxPrerequisiteRefreshes:    facts.RecoveryPolicy.MaxPrerequisiteRefreshes,
		PrerequisiteRefreshes:       facts.Recovery.Used(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh),
	}
	if facts.RepairDirective != nil {
		directive := facts.RepairDirective.Normalized()
		projection.RepairDirective = &readmodelcore.RuntimeRepairDirective{
			ID: directive.ID, RequiredAction: string(directive.RequiredState),
			RequiredEffectScopes:  runtimecontract.RuntimeEffectScopesAsStrings(directive.RequiredEffectScopes),
			RequiredEvidenceKinds: append([]string(nil), directive.RequiredEvidenceKinds...),
			TargetRefs:            append([]string(nil), directive.TargetRefs...), RepairAttempt: directive.RepairAttempt,
		}
	}
	projection.Operations = runtimeOperationStatuses(*run.RuntimeState)
	projection.CapabilityStates = runtimeCapabilityStateStatuses(facts)
	projection.Governance = runtimeGovernanceStatuses(*run.RuntimeState)
	projection.Audit = MapRuntimeV2Audit(run)
	projection.Summary = summarizeAcceptanceSlices(projection.Slices)
	return projection
}

func runtimeOperationStatuses(state runtimecontract.RuntimeStateRecord) []readmodelcore.RuntimeOperationStatus {
	dispatches := append([]runtimecontract.RuntimeDispatchRecord(nil), state.CompletedDispatches...)
	if state.ActiveDispatch != nil {
		dispatches = append(dispatches, state.ActiveDispatch.Clone())
	}
	ret := make([]readmodelcore.RuntimeOperationStatus, 0)
	for _, dispatch := range dispatches {
		operations := runtimeDispatchOperations(dispatch.Candidate)
		for _, rawExecution := range dispatch.OperationExecutions {
			execution := rawExecution.Normalized()
			status := readmodelcore.RuntimeOperationStatus{
				DispatchID: dispatch.ID, OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
				CapabilityStateHash: dispatch.Transaction.CapabilityStateHash,
				ExpectedEffectScope: string(execution.ExpectedEffectScope), ExecutionState: string(execution.State),
				ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: execution.OwnerReceiptHash,
				OutcomeUnknown: execution.State == runtimecontract.OperationExecutionOutcomeUnknown,
			}
			if operation, ok := operations[execution.OperationID]; ok {
				status.Kind = operation.Kind
				status.RequiredOperationRef = operation.RequiredOperationRef
				if contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind); found {
					if variant, err := runtimecontract.MatchRuntimeOperationVariant(operation, contract); err == nil {
						status.VariantID = variant.ID
					}
				}
			}
			if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence); ok {
				status.ObservedEffectScope = string(receipt.ObservedEffectScope)
				status.Outcome = string(receipt.Outcome)
			}
			for _, rawBinding := range state.OperationGovernanceBindings {
				binding := rawBinding.Normalized()
				if binding.ExecutionKey != execution.ExecutionKey || (binding.Fence != execution.Fence && (binding.Grant == nil || binding.Grant.ResumeFence != execution.Fence)) {
					continue
				}
				status.GovernanceState = string(binding.State)
				status.GovernanceTicketID = string(binding.TicketID)
			}
			ret = append(ret, status)
		}
	}
	return ret
}

func runtimeDispatchOperations(candidate runtimecontract.RuntimeCandidate) map[string]runtimecontract.RuntimeOperation {
	ret := map[string]runtimecontract.RuntimeOperation{}
	if candidate.StateChange != nil {
		for _, item := range candidate.StateChange.Operations {
			operation := item.Operation.Normalized()
			if operation.ID != "" {
				ret[operation.ID] = operation
			}
		}
	}
	if candidate.Validation != nil {
		for _, item := range candidate.Validation.Operations {
			operation := item.Operation.Normalized()
			if operation.ID != "" {
				ret[operation.ID] = operation
			}
		}
	}
	return ret
}

func runtimeCapabilityStateStatuses(facts runtimecontract.RuntimeFacts) []readmodelcore.RuntimeCapabilityState {
	ret := make([]readmodelcore.RuntimeCapabilityState, 0, len(facts.Resources.CapabilityStates))
	for _, snapshot := range facts.Resources.CapabilityStates {
		state := readmodelcore.RuntimeCapabilityState{
			RequirementID: snapshot.RequirementID, SnapshotID: snapshot.SnapshotID, AcquisitionKey: snapshot.AcquisitionKey,
			OwnerModuleID: snapshot.OwnerModuleID, CapabilityStateKind: snapshot.CapabilityStateKind, SubjectRef: snapshot.SubjectRef,
			ObservedRevision: snapshot.ObservedRevision, ContentHash: snapshot.ContentHash, Disposition: string(snapshot.Disposition),
			QueryReceiptRef: snapshot.QueryReceiptRef, ObservedAt: snapshot.ObservedAt,
			InvalidatedByReceiptRefs: append([]string(nil), snapshot.InvalidatedByReceiptRefs...),
		}
		for _, requirement := range snapshot.RequiredOperationSequence {
			state.RequiredOperationSequence = append(state.RequiredOperationSequence, readmodelcore.RuntimeRequiredOperationState{
				RequirementID: requirement.RequirementID, Kind: requirement.Kind, VariantID: requirement.VariantID,
				DependsOnRequirementIDs: append([]string(nil), requirement.DependsOnRequirementIDs...), Reason: requirement.Reason,
			})
		}
		ret = append(ret, state)
	}
	return ret
}

func runtimeGovernanceStatuses(state runtimecontract.RuntimeStateRecord) []readmodelcore.RuntimeGovernanceState {
	resolutions := make(map[string]runtimecontract.RuntimeGovernanceResolutionRecord, len(state.GovernanceResolutions))
	for _, resolution := range state.GovernanceResolutions {
		resolutions[string(resolution.TicketID)] = resolution
	}
	ret := make([]readmodelcore.RuntimeGovernanceState, 0, len(state.GovernanceRequests))
	for _, request := range state.GovernanceRequests {
		value := readmodelcore.RuntimeGovernanceState{
			TicketID: string(request.TicketID), AuthoritySnapshotID: request.AuthoritySnapshotID,
			DispatchID: request.DispatchID, DependencyKind: request.DependencyKind, SubjectRef: request.SubjectRef,
		}
		if resolution, found := resolutions[value.TicketID]; found {
			value.ResolutionID = string(resolution.ResolutionID)
			value.Outcome = string(resolution.Outcome)
		}
		ret = append(ret, value)
	}
	sort.Slice(ret, func(left, right int) bool {
		if ret[left].TicketID != ret[right].TicketID {
			return ret[left].TicketID < ret[right].TicketID
		}
		return ret[left].AuthoritySnapshotID < ret[right].AuthoritySnapshotID
	})
	return ret
}

func MapWorkspaceAcceptanceProjection(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.AcceptanceProjection {
	if snapshot.Run != nil {
		return MapAcceptanceProjection(*snapshot.Run)
	}
	if run := selectWorkspaceAcceptanceRun(snapshot); run != nil {
		return MapAcceptanceProjection(*run)
	}
	return emptyAcceptanceProjection()
}

func selectWorkspaceAcceptanceRun(snapshot readmodelcore.WorkspaceSnapshot) *readmodelcore.Run {
	for _, runID := range []string{
		strings.TrimSpace(snapshot.Workspace.ActiveRunID),
		strings.TrimSpace(snapshot.TaskProgress.RunID),
		strings.TrimSpace(snapshot.Outcome.RunID),
	} {
		if runID == "" {
			continue
		}
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == runID {
				return &snapshot.Runs[idx]
			}
		}
	}
	if len(snapshot.Runs) == 0 {
		return nil
	}
	return &snapshot.Runs[len(snapshot.Runs)-1]
}

func emptyAcceptanceProjection() readmodelcore.AcceptanceProjection {
	return readmodelcore.AcceptanceProjection{Summary: readmodelcore.AcceptanceSummary{}}
}

func mergeAcceptanceProjectionSource(parent map[string]any, nested map[string]any) map[string]any {
	ret := map[string]any{}
	for key, value := range parent {
		ret[key] = value
	}
	for key, value := range nested {
		ret[key] = value
	}
	return ret
}

func summarizeAcceptanceSlices(items []readmodelcore.AcceptanceSlice) readmodelcore.AcceptanceSummary {
	var ret readmodelcore.AcceptanceSummary
	for _, item := range items {
		switch strings.TrimSpace(item.State) {
		case "accepted":
			ret.AcceptedSlices++
		case "active":
			ret.ActiveSlices++
		case "failed":
			ret.FailedSlices++
		case "", "pending", "blocked":
			ret.OpenSlices++
		default:
			ret.OpenSlices++
		}
	}
	return ret
}

func decodeAcceptanceSlices(value any) []readmodelcore.AcceptanceSlice {
	switch typed := value.(type) {
	case []readmodelcore.AcceptanceSlice:
		return append([]readmodelcore.AcceptanceSlice(nil), typed...)
	case []any:
		ret := make([]readmodelcore.AcceptanceSlice, 0, len(typed))
		for _, item := range typed {
			if mapped := decodeAcceptanceSlice(item); !isEmptyAcceptanceSlice(mapped) {
				ret = append(ret, mapped)
			}
		}
		return ret
	case []map[string]any:
		ret := make([]readmodelcore.AcceptanceSlice, 0, len(typed))
		for _, item := range typed {
			if mapped := decodeAcceptanceSlice(item); !isEmptyAcceptanceSlice(mapped) {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

func decodeCoverageFocusPtr(value any) *readmodelcore.CoverageFocus {
	ret := decodeCoverageFocus(value)
	if isEmptyCoverageFocus(ret) {
		return nil
	}
	return &ret
}

func decodeConvergenceBudgetPtr(value any) *readmodelcore.ConvergenceBudget {
	ret := decodeConvergenceBudget(value)
	if isEmptyConvergenceBudget(ret) {
		return nil
	}
	return &ret
}

func decodeAcceptanceSlice(value any) readmodelcore.AcceptanceSlice {
	switch typed := value.(type) {
	case readmodelcore.AcceptanceSlice:
		return typed
	case map[string]any:
		return readmodelcore.AcceptanceSlice{
			ID:                    stringValue(typed, "id", "ID"),
			Title:                 stringValue(typed, "title", "Title"),
			State:                 stringValue(typed, "state", "State"),
			Priority:              readModelIntValue(valueByKey(typed, "priority", "Priority")),
			Scope:                 stringValue(typed, "scope", "Scope"),
			CoverageWorkItemIDs:   readModelStringSliceValue(valueByKey(typed, "coverageWorkItemIds", "coverageWorkItemIDs", "CoverageWorkItemIDs")),
			RequiredEvidenceKinds: readModelStringSliceValue(valueByKey(typed, "requiredEvidenceKinds", "RequiredEvidenceKinds")),
			CompletionPolicy:      stringValue(typed, "completionPolicy", "CompletionPolicy"),
			ResultPolicy:          stringValue(typed, "resultPolicy", "ResultPolicy"),
			CreatedAt:             stringValue(typed, "createdAt", "CreatedAt"),
			ActivatedAt:           stringValue(typed, "activatedAt", "ActivatedAt"),
			ClosedAt:              stringValue(typed, "closedAt", "ClosedAt"),
		}
	default:
		return readmodelcore.AcceptanceSlice{}
	}
}

func decodeCoverageFocus(value any) readmodelcore.CoverageFocus {
	switch typed := value.(type) {
	case readmodelcore.CoverageFocus:
		return typed
	case map[string]any:
		return readmodelcore.CoverageFocus{
			ID:                    stringValue(typed, "id", "ID"),
			SliceID:               stringValue(typed, "sliceId", "sliceID", "SliceID"),
			CoverageWorkItemIDs:   readModelStringSliceValue(valueByKey(typed, "coverageWorkItemIds", "coverageWorkItemIDs", "CoverageWorkItemIDs")),
			FailureClass:          stringValue(typed, "failureClass", "FailureClass"),
			FailureSignature:      stringValue(typed, "failureSignature", "FailureSignature"),
			RequiredEvidenceKinds: readModelStringSliceValue(valueByKey(typed, "requiredEvidenceKinds", "RequiredEvidenceKinds")),
			AcceptedEvidenceRefs:  readModelStringSliceValue(valueByKey(typed, "acceptedEvidenceRefs", "AcceptedEvidenceRefs")),
			RejectedEvidenceRefs:  readModelStringSliceValue(valueByKey(typed, "rejectedEvidenceRefs", "RejectedEvidenceRefs")),
			TargetRefs:            readModelStringSliceValue(valueByKey(typed, "targetRefs", "TargetRefs")),
			State:                 stringValue(typed, "state", "State"),
			Attempt:               readModelIntValue(valueByKey(typed, "attempt", "Attempt")),
		}
	default:
		return readmodelcore.CoverageFocus{}
	}
}

func decodeConvergenceBudget(value any) readmodelcore.ConvergenceBudget {
	switch typed := value.(type) {
	case readmodelcore.ConvergenceBudget:
		return typed
	case map[string]any:
		return readmodelcore.ConvergenceBudget{
			FocusID:                     stringValue(typed, "focusId", "focusID", "FocusID"),
			SliceID:                     stringValue(typed, "sliceId", "sliceID", "SliceID"),
			State:                       stringValue(typed, "state", "State"),
			Decision:                    stringValue(typed, "decision", "Decision"),
			MaxRepairAttemptsPerFocus:   readModelIntValue(valueByKey(typed, "maxRepairAttemptsPerFocus", "MaxRepairAttemptsPerFocus")),
			MaxLLMRoundsPerSlice:        readModelIntValue(valueByKey(typed, "maxLLMRoundsPerSlice", "MaxLLMRoundsPerSlice")),
			MaxRepeatedFailureSignature: readModelIntValue(valueByKey(typed, "maxRepeatedFailureSignature", "MaxRepeatedFailureSignature")),
			MaxValidationOnlyRetries:    readModelIntValue(valueByKey(typed, "maxValidationOnlyRetries", "MaxValidationOnlyRetries")),
			MaxNoCoverageDeltaRounds:    readModelIntValue(valueByKey(typed, "maxNoCoverageDeltaRounds", "MaxNoCoverageDeltaRounds")),
			MaxContextBytesPerRepair:    readModelIntValue(valueByKey(typed, "maxContextBytesPerRepair", "MaxContextBytesPerRepair")),
			RepairAttempts:              readModelIntValue(valueByKey(typed, "repairAttempts", "RepairAttempts")),
			LLMRounds:                   readModelIntValue(valueByKey(typed, "llmRounds", "LLMRounds")),
			RepeatedFailureSignatures:   readModelIntValue(valueByKey(typed, "repeatedFailureSignatures", "RepeatedFailureSignatures")),
			ValidationOnlyRetries:       readModelIntValue(valueByKey(typed, "validationOnlyRetries", "ValidationOnlyRetries")),
			NoCoverageDeltaRounds:       readModelIntValue(valueByKey(typed, "noCoverageDeltaRounds", "NoCoverageDeltaRounds")),
			LastNoDeltaReason:           stringValue(typed, "lastNoDeltaReason", "LastNoDeltaReason"),
			LastDecisionReason:          stringValue(typed, "lastDecisionReason", "LastDecisionReason"),
		}
	default:
		return readmodelcore.ConvergenceBudget{}
	}
}

func isEmptyAcceptanceSlice(item readmodelcore.AcceptanceSlice) bool {
	return strings.TrimSpace(item.ID) == "" &&
		strings.TrimSpace(item.Title) == "" &&
		strings.TrimSpace(item.State) == "" &&
		len(item.CoverageWorkItemIDs) == 0 &&
		len(item.RequiredEvidenceKinds) == 0
}

func isEmptyCoverageFocus(item readmodelcore.CoverageFocus) bool {
	return strings.TrimSpace(item.ID) == "" &&
		strings.TrimSpace(item.SliceID) == "" &&
		len(item.CoverageWorkItemIDs) == 0 &&
		strings.TrimSpace(item.FailureClass) == "" &&
		strings.TrimSpace(item.FailureSignature) == "" &&
		len(item.RequiredEvidenceKinds) == 0 &&
		len(item.AcceptedEvidenceRefs) == 0 &&
		len(item.RejectedEvidenceRefs) == 0 &&
		len(item.TargetRefs) == 0 &&
		strings.TrimSpace(item.State) == "" &&
		item.Attempt == 0
}

func isEmptyConvergenceBudget(item readmodelcore.ConvergenceBudget) bool {
	return strings.TrimSpace(item.FocusID) == "" &&
		strings.TrimSpace(item.SliceID) == "" &&
		strings.TrimSpace(item.State) == "" &&
		strings.TrimSpace(item.Decision) == "" &&
		item.MaxRepairAttemptsPerFocus == 0 &&
		item.MaxLLMRoundsPerSlice == 0 &&
		item.MaxRepeatedFailureSignature == 0 &&
		item.MaxValidationOnlyRetries == 0 &&
		item.MaxNoCoverageDeltaRounds == 0 &&
		item.MaxContextBytesPerRepair == 0 &&
		item.RepairAttempts == 0 &&
		item.LLMRounds == 0 &&
		item.RepeatedFailureSignatures == 0 &&
		item.ValidationOnlyRetries == 0 &&
		item.NoCoverageDeltaRounds == 0 &&
		strings.TrimSpace(item.LastNoDeltaReason) == "" &&
		strings.TrimSpace(item.LastDecisionReason) == ""
}

func valueByKey(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func stringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func mapValue(values map[string]any, key string) map[string]any {
	value, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}
