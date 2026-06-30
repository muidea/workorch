package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) NextActionForRun(ctx context.Context, runID common.RunID) (runtimemodel.NextAction, error) {
	if o == nil || runID == "" {
		return runtimemodel.NextAction{}, common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return runtimemodel.NextAction{}, err
	}
	if run.State == common.RunStateBlockedOnApproval && run.BlockingApprovalID != "" && o.approvals != nil {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		approval, err := o.approvals.Get(scopedCtx, run.BlockingApprovalID)
		if err != nil {
			return runtimemodel.NextAction{}, err
		}
		if action, ok := derivePendingApprovalNextAction(run, approval); ok {
			return action, nil
		}
	}
	if run.State == common.RunStateBlockedOnDecision && run.BlockingTicketID != "" && o.tickets != nil {
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		ticket, err := o.tickets.Get(scopedCtx, run.BlockingTicketID)
		if err != nil {
			return runtimemodel.NextAction{}, err
		}
		if action, ok := deriveOpenDecisionNextAction(run, ticket); ok {
			return action, nil
		}
	}
	if action, ok := deriveMissingGapBaselineNextAction(run); ok {
		return action, nil
	}
	if action, ok, err := o.missingStateChangeNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	if action, ok, err := o.missingValidationNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	if action, ok, err := o.missingAssessmentNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	if action, ok, err := o.allSatisfiedCompleteNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	if action, ok, err := o.recoverableDiagnosticNextAction(ctx, run); err != nil {
		return runtimemodel.NextAction{}, err
	} else if ok {
		return action, nil
	}
	return runtimemodel.NextAction{}, nil
}

func deriveMissingGapBaselineNextAction(run *runmodel.Run) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" {
		return runtimemodel.NextAction{}, false
	}
	if !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	if len(mapFromValue(run.ExecutionContext[gapBaselineKey])) > 0 {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeAssessGap,
		SubjectType: "run",
		SubjectID:   string(run.ID),
		Stage:       "gap_analysis",
		StageMode:   "baseline",
		RequiredOutput: map[string]any{
			"gapBaselineRequired": true,
		},
		AllowedActions:   []string{runtimemodel.NextActionTypeAssessGap},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "gap baseline missing",
	}.Normalized(), true
}

func runStateAllowsGapAssessment(state common.RunState) bool {
	switch state {
	case common.RunStateCreated, common.RunStateRunning, common.RunStateValidating:
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) missingStateChangeNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	return o.missingCoverageEvidenceNextAction(ctx, run, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeStateChange,
		EvidenceKind: runtimemodel.EvidenceKindStateChange,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeValidate,
			runtimemodel.NextActionTypeAssessment,
		},
		Reason: "missing state_change evidence",
	})
}

func (o *RuntimeOrchestrator) missingValidationNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	return o.missingCoverageEvidenceNextAction(ctx, run, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeValidate,
		EvidenceKind: runtimemodel.EvidenceKindValidation,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeAssessment,
			runtimemodel.NextActionTypeStateChange,
		},
		Reason: "missing validation evidence",
	})
}

func (o *RuntimeOrchestrator) missingAssessmentNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	if run == nil || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false, nil
	}
	if len(mapFromValue(run.ExecutionContext[gapBaselineKey])) == 0 {
		return runtimemodel.NextAction{}, false, nil
	}
	items, err := o.coverageItemsForNextAction(ctx, run)
	if err != nil {
		return runtimemodel.NextAction{}, false, err
	}
	if changed, _ := o.coverageLedger().closeDeterministicallySatisfiedReadyItems(run, ctx); changed {
		if o.runs != nil {
			if err := o.persistRunProgressUpdate(ctx, run); err != nil {
				return runtimemodel.NextAction{}, false, err
			}
		}
		items = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	}
	action, ok := deriveMissingAssessmentNextAction(run, items)
	return action, ok, nil
}

type missingCoverageEvidenceNextActionSpec struct {
	ActionType          string
	EvidenceKind        string
	ForbiddenActions    []string
	Reason              string
	RejectedDiagnostics []*runtimemodel.EvidenceRecord
}

func (o *RuntimeOrchestrator) missingCoverageEvidenceNextAction(ctx context.Context, run *runmodel.Run, spec missingCoverageEvidenceNextActionSpec) (runtimemodel.NextAction, bool, error) {
	if run == nil || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false, nil
	}
	if len(mapFromValue(run.ExecutionContext[gapBaselineKey])) == 0 {
		return runtimemodel.NextAction{}, false, nil
	}
	if spec.ActionType == runtimemodel.NextActionTypeStateChange {
		if err := o.hydrateCurrentTargetSnapshotsFromAcceptedEvidence(ctx, run); err != nil {
			return runtimemodel.NextAction{}, false, err
		}
	}
	items, err := o.coverageItemsForNextAction(ctx, run)
	if err != nil {
		return runtimemodel.NextAction{}, false, err
	}
	if len(items) > 0 && o != nil && o.evidence.EvidenceRecords != nil {
		changed, _, err := o.reconcileCoverageAcceptedEvidenceRecords(ctx, run)
		if err != nil {
			return runtimemodel.NextAction{}, false, err
		}
		if changed && o.runs != nil {
			if err := o.persistRunProgressUpdate(ctx, run); err != nil {
				return runtimemodel.NextAction{}, false, err
			}
		}
		if changed {
			items = mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
		}
		if spec.ActionType == runtimemodel.NextActionTypeValidate {
			records, err := o.evidence.EvidenceRecords.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), run.ID)
			if err != nil {
				if !errors.Is(err, common.ErrNotFound) {
					return runtimemodel.NextAction{}, false, err
				}
			} else {
				spec.RejectedDiagnostics = records
			}
		}
	}
	action, ok := deriveMissingCoverageEvidenceNextAction(run, items, spec)
	if ok && spec.ActionType == runtimemodel.NextActionTypeStateChange {
		if snapshotAction, snapshotOK := deriveAcquireTargetSnapshotNextAction(run, action); snapshotOK {
			return snapshotAction, true, nil
		}
	}
	return action, ok, nil
}

func (o *RuntimeOrchestrator) coverageItemsForNextAction(ctx context.Context, run *runmodel.Run) ([]map[string]any, error) {
	if run == nil || run.ID == "" {
		return nil, nil
	}
	scopedCtx := coverageRepositoryContext(run, ctx)
	if o != nil && o.coverageLedger() != nil {
		if _, err := o.coverageLedger().hydrateProjectionFromRepository(scopedCtx, run); err != nil {
			return nil, err
		}
	}
	if items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]); len(items) > 0 {
		return items, nil
	}
	if o == nil || o.evidence.CoverageWorkItems == nil {
		return nil, nil
	}
	items, err := o.evidence.CoverageWorkItems.ListActiveByRun(scopedCtx, run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return coverageWorkItemProjectionFromModels(items), nil
}

func (o *RuntimeOrchestrator) hydrateCurrentTargetSnapshotsFromAcceptedEvidence(ctx context.Context, run *runmodel.Run) error {
	if o == nil || o.evidence.EvidenceRecords == nil || o.evidence.Patches == nil || o.steps == nil || run == nil || run.ID == "" || run.WorkspaceID == "" {
		return nil
	}
	if len(mapSliceFromValue(mapFromValue(run.ExecutionContext["repairActionPackage"])["targetSnapshots"])) > 0 ||
		len(mapSliceFromValue(mapFromValue(run.ExecutionContext["repairActionPackage"])["currentTargetSnapshots"])) > 0 ||
		len(mapSliceFromValue(run.ExecutionContext["targetSnapshots"])) > 0 ||
		len(mapSliceFromValue(run.ExecutionContext["currentTargetSnapshots"])) > 0 {
		return nil
	}
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
	records, err := o.evidence.EvidenceRecords.ListByRun(scopedCtx, run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	patches, err := o.evidence.Patches.ListByRun(scopedCtx, run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			patches = nil
		} else {
			return err
		}
	}
	snapshots := acceptedTargetSnapshotsFromEvidenceRecords(scopedCtx, run, records, o.steps, patches)
	if len(snapshots) == 0 {
		return nil
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	pkg := mapFromValue(run.ExecutionContext["repairActionPackage"])
	if pkg == nil {
		pkg = map[string]any{}
	}
	pkg["targetSnapshots"] = mergeRepairTargetSnapshots(mapSliceFromValue(pkg["targetSnapshots"]), snapshots)
	pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(mapSliceFromValue(pkg["targetSnapshots"]))
	run.ExecutionContext["repairActionPackage"] = pkg
	run.ExecutionContext["targetSnapshots"] = mergeRepairTargetSnapshots(mapSliceFromValue(run.ExecutionContext["targetSnapshots"]), snapshots)
	run.ExecutionContext["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(mapSliceFromValue(run.ExecutionContext["targetSnapshots"]))
	return nil
}

func acceptedTargetSnapshotsFromEvidenceRecords(ctx context.Context, run *runmodel.Run, records []*runtimemodel.EvidenceRecord, steps repository.StepRepository, patches []*runmodel.Patch) []map[string]any {
	if run == nil || steps == nil || len(records) == 0 {
		return nil
	}
	superseded := supersededEvidenceRecordSet(records)
	seen := map[string]bool{}
	var ret []map[string]any
	for idx := len(records) - 1; idx >= 0; idx-- {
		record := records[idx]
		if !acceptedTargetSnapshotEvidenceRecordValidForRun(run, record, superseded) {
			continue
		}
		targetRef := strings.TrimSpace(record.SubjectID)
		if targetRef == "" || seen[targetRef] {
			continue
		}
		if targetSnapshotEvidenceStaleByAppliedPatch(record, patches) {
			continue
		}
		step, err := steps.Get(ctx, record.StepID)
		if err != nil || step == nil {
			continue
		}
		snapshot := targetSnapshotFromAcceptedEvidenceStep(record, step)
		if len(snapshot) == 0 {
			continue
		}
		targetRef = strings.TrimSpace(stringFromMap(snapshot, "targetRef"))
		if targetRef == "" || seen[targetRef] {
			continue
		}
		snapshot["source"] = "accepted_target_snapshot_evidence"
		snapshot["evidenceRecordId"] = string(record.ID)
		snapshot["sourceRef"] = record.SourceRef
		seen[targetRef] = true
		ret = append(ret, snapshot)
		if len(ret) >= maxLLMContextSliceItems {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func acceptedTargetSnapshotEvidenceRecordValidForRun(run *runmodel.Run, record *runtimemodel.EvidenceRecord, superseded map[common.EvidenceRecordID]bool) bool {
	if run == nil || record == nil ||
		record.WorkspaceID != run.WorkspaceID ||
		record.RunID != run.ID ||
		record.StepID == "" ||
		record.Kind != runtimemodel.EvidenceKindTargetSnapshot ||
		record.Status != runtimemodel.EvidenceStatusAccepted ||
		record.SourceType != evidenceRecordSourceTypeTargetSnapshot ||
		superseded[record.ID] {
		return false
	}
	if strings.TrimSpace(run.DefinitionRevision) != "" && strings.TrimSpace(record.DefinitionRevision) != strings.TrimSpace(run.DefinitionRevision) {
		return false
	}
	if strings.TrimSpace(record.SubjectID) == "" || strings.TrimSpace(record.SourceRef) == "" {
		return false
	}
	return true
}

func targetSnapshotEvidenceStaleByAppliedPatch(record *runtimemodel.EvidenceRecord, patches []*runmodel.Patch) bool {
	if record == nil || len(patches) == 0 {
		return false
	}
	targetRef := normalizeRepairPatchTargetRef(record.SubjectID)
	if targetRef == "" {
		return true
	}
	snapshotAt := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(record.Metadata, "capturedAt"),
		record.CreatedAt,
		record.UpdatedAt,
	))
	for _, patch := range patches {
		if patch == nil ||
			patch.WorkspaceID != record.WorkspaceID ||
			patch.RunID != record.RunID ||
			patch.State != common.PatchStateApplied ||
			normalizeRepairPatchTargetRef(patch.TargetRef) != targetRef {
			continue
		}
		if patch.StepID == record.StepID {
			continue
		}
		if strings.TrimSpace(patch.AppliedAt) == "" || snapshotAt == "" || runtimeTimestampAfter(patch.AppliedAt, snapshotAt) {
			return true
		}
	}
	return false
}

func runtimeTimestampAfter(value string, baseline string) bool {
	value = strings.TrimSpace(value)
	baseline = strings.TrimSpace(baseline)
	if value == "" || baseline == "" {
		return false
	}
	valueTime, valueErr := time.Parse(time.RFC3339Nano, value)
	baselineTime, baselineErr := time.Parse(time.RFC3339Nano, baseline)
	if valueErr != nil || baselineErr != nil {
		return value > baseline
	}
	return valueTime.After(baselineTime)
}

func targetSnapshotFromAcceptedEvidenceStep(record *runtimemodel.EvidenceRecord, step *runmodel.Step) map[string]any {
	if record == nil || step == nil || step.ID != record.StepID || step.State != common.StepStateCompleted {
		return nil
	}
	output := mapFromValue(step.Output)
	if len(output) == 0 ||
		strings.TrimSpace(stringFromMap(output, "stage")) != "file_read" ||
		!boolValue(output["exists"]) ||
		boolValue(output["isBinary"]) ||
		boolValue(output["truncated"]) ||
		boolValue(output["redacted"]) {
		return nil
	}
	targetRef := strings.TrimSpace(firstNonEmptyString(stringFromMap(output, "path"), stringFromMap(output, "targetRef")))
	if targetRef == "" || targetRef != strings.TrimSpace(record.SubjectID) {
		return nil
	}
	content := stringFromMap(output, "content")
	if strings.TrimSpace(content) == "" || containsRuntimeRedactionMarker(content) {
		return nil
	}
	contentHash := strings.TrimSpace(stringFromMap(output, "contentHash"))
	if contentHash == "" {
		contentHash = sha256HexBytes([]byte(content))
	}
	recordHash := strings.TrimSpace(stringFromMap(record.Metadata, "contentHash"))
	if recordHash != "" && recordHash != contentHash {
		return nil
	}
	snapshot := repairActionTargetSnapshot(map[string]any{
		"targetRef":        targetRef,
		"exists":           true,
		"contentHash":      contentHash,
		"content":          content,
		"size":             output["size"],
		"contentComplete":  true,
		"safeForFileWrite": true,
		"redacted":         false,
		"truncated":        false,
		"isBinary":         false,
	})
	if len(snapshot) == 0 || !boolValue(snapshot["contentComplete"]) || !boolValue(snapshot["safeForFileWrite"]) {
		return nil
	}
	return snapshot
}

func deriveMissingStateChangeNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	return deriveMissingCoverageEvidenceNextAction(run, items, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeStateChange,
		EvidenceKind: runtimemodel.EvidenceKindStateChange,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeValidate,
			runtimemodel.NextActionTypeAssessment,
		},
		Reason: "missing state_change evidence",
	})
}

func deriveMissingValidationNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	return deriveMissingCoverageEvidenceNextAction(run, items, missingCoverageEvidenceNextActionSpec{
		ActionType:   runtimemodel.NextActionTypeValidate,
		EvidenceKind: runtimemodel.EvidenceKindValidation,
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeAssessment,
			runtimemodel.NextActionTypeStateChange,
		},
		Reason: "missing validation evidence",
	})
}

func deriveMissingAssessmentNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	for _, item := range items {
		if !runtimemodel.CoverageWorkItemStatusActive(stringFromMap(item, "status")) {
			continue
		}
		state := deriveCoverageWorkItemState(item)
		if !state.NeedsAssessment() {
			continue
		}
		itemID := common.CoverageWorkItemID(firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "repositoryId")))
		if itemID == "" {
			continue
		}
		requiredOutput := map[string]any{
			"coverageWorkItemIds":        []string{string(itemID)},
			"acceptedEvidenceRefs":       compactUniqueStrings(state.AcceptedEvidenceRefs),
			"requiredEvidenceKinds":      compactUniqueStrings(state.RequiredEvidenceKinds),
			"mustAssessAcceptedEvidence": true,
			"mustNotMutateWorkspace":     true,
		}
		return runtimemodel.NextAction{
			WorkspaceID:          run.WorkspaceID,
			RunID:                run.ID,
			Type:                 runtimemodel.NextActionTypeAssessment,
			SubjectType:          "coverage",
			SubjectID:            string(itemID),
			FocusCoverageItemIDs: []common.CoverageWorkItemID{itemID},
			RequiredOutput:       requiredOutput,
			AllowedActions:       []string{runtimemodel.NextActionTypeAssessment},
			ForbiddenActions: []string{
				runtimemodel.NextActionTypeStateChange,
				runtimemodel.NextActionTypeValidate,
				runtimemodel.NextActionTypeComplete,
			},
			RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindAssessment},
			Stage:                 runtimemodel.NextActionTypeAssessment,
			StageMode:             "accepted_evidence",
			Reason:                "coverage work item evidence is ready for assessment",
		}.Normalized(), true
	}
	return runtimemodel.NextAction{}, false
}

func deriveMissingCoverageEvidenceNextAction(run *runmodel.Run, items []map[string]any, spec missingCoverageEvidenceNextActionSpec) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	for _, item := range items {
		if !runtimemodel.CoverageWorkItemStatusActive(stringFromMap(item, "status")) {
			continue
		}
		state := deriveCoverageWorkItemState(item)
		if !stringSliceContains(state.MissingEvidenceKinds, spec.EvidenceKind) {
			continue
		}
		itemID := common.CoverageWorkItemID(firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "repositoryId")))
		if itemID == "" {
			continue
		}
		requiredOutput := map[string]any{
			"coverageWorkItemIds":   []string{string(itemID)},
			"requiredEvidenceKinds": []string{spec.EvidenceKind},
			"mustBindCoverageFocus": true,
		}
		stageMode := "evidence"
		actionType := spec.ActionType
		requiredEvidenceKind := spec.EvidenceKind
		allowedActions := []string{spec.ActionType}
		forbiddenActions := append([]string(nil), spec.ForbiddenActions...)
		reason := spec.Reason
		if spec.ActionType == runtimemodel.NextActionTypeValidate {
			route := validationNextActionRouteForCoverageItem(run, item, spec.EvidenceKind)
			if diagnosticRoute := validationNextActionRouteForRejectedDiagnostics(run, item, spec.RejectedDiagnostics); diagnosticRoute.Route != "" {
				route = diagnosticRoute
			}
			route = validationRouteWithoutDirectDecisionNextAction(route)
			requiredOutput = applyValidationRouteToRequiredOutput(requiredOutput, route)
			stageMode = firstNonEmptyString(route.StageMode, stageMode)
			if route.ActionType != "" && route.ActionType != spec.ActionType {
				actionType = route.ActionType
				allowedActions = []string{route.ActionType}
			}
			if route.Reason != "" {
				reason = route.Reason
			}
			switch route.ActionType {
			case runtimemodel.NextActionTypeStateChange:
				requiredEvidenceKind = runtimemodel.EvidenceKindStateChange
				requiredOutput = validationFailureStateChangeRequiredOutput(requiredOutput, route)
				allowedActions = []string{runtimemodel.NextActionTypeStateChange}
				forbiddenActions = []string{
					runtimemodel.NextActionTypeComplete,
					runtimemodel.NextActionTypeValidate,
					runtimemodel.NextActionTypeAssessment,
				}
			case runtimemodel.NextActionTypeFail:
				requiredOutput = validationFailureFailRequiredOutput(requiredOutput, route)
				allowedActions = []string{runtimemodel.NextActionTypeFail}
				forbiddenActions = []string{
					runtimemodel.NextActionTypeStateChange,
					runtimemodel.NextActionTypeValidate,
					runtimemodel.NextActionTypeComplete,
				}
			}
			if route.ActionType == runtimemodel.NextActionTypeRequestDecision {
				forbiddenActions = append(forbiddenActions, runtimemodel.NextActionTypeValidate)
			}
		}
		return runtimemodel.NextAction{
			WorkspaceID:           run.WorkspaceID,
			RunID:                 run.ID,
			Type:                  actionType,
			SubjectType:           "coverage",
			SubjectID:             string(itemID),
			FocusCoverageItemIDs:  []common.CoverageWorkItemID{itemID},
			RequiredOutput:        requiredOutput,
			AllowedActions:        allowedActions,
			ForbiddenActions:      forbiddenActions,
			RequiredEvidenceKinds: []string{requiredEvidenceKind},
			Stage:                 actionType,
			StageMode:             stageMode,
			Reason:                reason,
		}.Normalized(), true
	}
	return runtimemodel.NextAction{}, false
}

type validationNextActionRoute struct {
	Route                 string
	ActionType            string
	StageMode             string
	PolicyRef             string
	ValidationSafetyClass string
	ManagedHarnessRef     string
	DeferredEvidenceKind  string
	GovernanceReason      string
	Reason                string
	DiagnosticEvidenceID  string
	DiagnosticClass       string
	ViolatedRule          string
	FailureSignature      string
	RetryAttempt          int
	RetryMaxAttempts      int
	EscalationReason      string
}

const validationNoDeltaLoopRejectedThreshold = 3

func validationNextActionRouteForRejectedDiagnostics(run *runmodel.Run, item map[string]any, records []*runtimemodel.EvidenceRecord) validationNextActionRoute {
	if run == nil || len(item) == 0 || len(records) == 0 {
		return validationNextActionRoute{}
	}
	latest := latestRejectedValidationDiagnosticForCoverageItem(run, item, records)
	if latest == nil {
		return validationNextActionRoute{}
	}
	class := validationDiagnosticClassFromEvidenceRecord(latest)
	if class == "" {
		return validationNextActionRoute{}
	}
	policy := validationDiagnosticPolicyForCoverageItem(run, item)
	if validationDiagnosticImpliesStateChange(class, policy) {
		return validationNextActionRoute{
			Route:                "validation_failure_state_change",
			ActionType:           runtimemodel.NextActionTypeStateChange,
			StageMode:            "implementation_gap",
			PolicyRef:            strings.TrimSpace(firstNonEmptyString(stringFromMap(policy, "policyRef"), stringFromMap(policy, "id"))),
			Reason:               "validation diagnostic requires state-changing closure",
			DiagnosticEvidenceID: string(latest.ID),
			DiagnosticClass:      class,
		}
	}
	if validationDiagnosticIsContractViolation(class, latest) {
		count := repeatedValidationDiagnosticCountForCoverageItem(run, item, records, latest)
		maxRetries := validationOnlyRetryMaxForPolicy(policy)
		if maxRetries > 0 && count >= maxRetries {
			route := validationNextActionRoute{
				Route:                "validation_contract_retry_exhausted",
				ActionType:           runtimemodel.NextActionTypeFail,
				StageMode:            "fail",
				PolicyRef:            strings.TrimSpace(firstNonEmptyString(stringFromMap(policy, "policyRef"), stringFromMap(policy, "id"))),
				Reason:               "validation contract retry exhausted",
				DiagnosticEvidenceID: string(latest.ID),
				DiagnosticClass:      class,
				ViolatedRule:         validationDiagnosticViolatedRule(latest),
				FailureSignature:     validationDiagnosticFailureSignature(latest),
				RetryAttempt:         count,
				RetryMaxAttempts:     maxRetries,
				EscalationReason:     "contract_retry_exhausted",
			}
			if validationDiagnosticManagedHarnessAvailable(policy) {
				route.Route = runtimemodel.ValidationRouteManagedHarness
				route.ActionType = runtimemodel.NextActionTypeValidate
				route.StageMode = "managed_harness"
				route.ManagedHarnessRef = strings.TrimSpace(stringFromMap(policy, "managedHarnessRef"))
				route.Reason = "validation contract retry exhausted; routing to managed harness"
			}
			return route
		}
	}
	if validationDiagnosticAllowsNoDeltaStateChange(class) {
		count := validationNoDeltaRejectedDiagnosticCountForCoverageItem(run, item, records)
		if count >= validationNoDeltaLoopRejectedThreshold {
			return validationNextActionRoute{
				Route:                "validation_loop_without_state_change",
				ActionType:           runtimemodel.NextActionTypeStateChange,
				StageMode:            "validation_no_delta",
				PolicyRef:            strings.TrimSpace(firstNonEmptyString(stringFromMap(policy, "policyRef"), stringFromMap(policy, "id"))),
				Reason:               "validation loop made no accepted evidence delta",
				DiagnosticEvidenceID: string(latest.ID),
				DiagnosticClass:      class,
				FailureSignature:     validationDiagnosticFailureSignature(latest),
				RetryAttempt:         count,
				RetryMaxAttempts:     validationNoDeltaLoopRejectedThreshold,
				EscalationReason:     "validation_loop_without_state_change",
			}
		}
	}
	return validationNextActionRoute{}
}

func validationNextActionRouteForCoverageItem(run *runmodel.Run, item map[string]any, evidenceKind string) validationNextActionRoute {
	defaultRoute := validationNextActionRoute{
		Route:      runtimemodel.ValidationRouteLLMGraph,
		ActionType: runtimemodel.NextActionTypeValidate,
		StageMode:  "evidence",
		Reason:     "missing validation evidence",
	}
	if run == nil || len(item) == 0 || strings.TrimSpace(evidenceKind) != runtimemodel.EvidenceKindValidation {
		return defaultRoute
	}
	for _, policy := range validationPoliciesForRun(run) {
		if !validationPolicyAppliesToCoverageItem(policy, item, evidenceKind) {
			continue
		}
		route := validationRouteFromPolicy(policy, defaultRoute)
		if route.Route != "" {
			return route
		}
	}
	return defaultRoute
}

func validationPoliciesForRun(run *runmodel.Run) []map[string]any {
	if run == nil || run.ExecutionContext == nil {
		return nil
	}
	policies := make([]map[string]any, 0)
	for _, key := range []string{"validationPolicies", "evidencePolicies"} {
		policies = append(policies, mapSliceFromValue(run.ExecutionContext[key])...)
	}
	if executionContract := mapFromValue(run.ExecutionContext["executionContract"]); len(executionContract) > 0 {
		for _, key := range []string{"validationPolicies", "evidencePolicies"} {
			policies = append(policies, mapSliceFromValue(executionContract[key])...)
		}
	}
	return policies
}

func validationDiagnosticPolicyForCoverageItem(run *runmodel.Run, item map[string]any) map[string]any {
	for _, policy := range validationPoliciesForRun(run) {
		if validationPolicyAppliesToCoverageItem(policy, item, runtimemodel.EvidenceKindValidation) {
			return policy
		}
	}
	return nil
}

func latestRejectedValidationDiagnosticForCoverageItem(run *runmodel.Run, item map[string]any, records []*runtimemodel.EvidenceRecord) *runtimemodel.EvidenceRecord {
	if run == nil || len(item) == 0 || len(records) == 0 {
		return nil
	}
	superseded := supersededEvidenceRecordSet(records)
	for idx := len(records) - 1; idx >= 0; idx-- {
		record := records[idx]
		if record == nil ||
			record.WorkspaceID != run.WorkspaceID ||
			record.RunID != run.ID ||
			record.Kind != runtimemodel.EvidenceKindDiagnostic ||
			record.Status != runtimemodel.EvidenceStatusRejected ||
			superseded[record.ID] {
			continue
		}
		if validationDiagnosticEvidenceMatchesCoverageItem(record, item) {
			return record
		}
	}
	return nil
}

func validationDiagnosticEvidenceMatchesCoverageItem(record *runtimemodel.EvidenceRecord, item map[string]any) bool {
	if record == nil || len(item) == 0 {
		return false
	}
	itemID := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "repositoryId")))
	if itemID == "" {
		return false
	}
	refs := compactUniqueStrings(append([]string{}, record.CoverageItemRefs...))
	refs = append(refs, stringSliceFromValue(record.Metadata["coverageWorkItemIds"])...)
	refs = append(refs, stringSliceFromValue(record.Metadata["coverageItemRefs"])...)
	refs = append(refs, stringSliceFromValue(mapFromValue(record.Metadata["budgetScope"])["coverageWorkItemIds"])...)
	refs = append(refs, stringSliceFromValue(mapFromValue(record.Metadata["contractViolation"])["coverageWorkItemIds"])...)
	refs = append(refs, stringSliceFromValue(mapFromValue(mapFromValue(record.Metadata["contractViolation"])["budgetScope"])["coverageWorkItemIds"])...)
	if len(refs) == 0 {
		for _, focusID := range []string{
			stringFromMap(record.Metadata, "focusId"),
			stringFromMap(mapFromValue(record.Metadata["budgetScope"]), "focusId"),
			stringFromMap(mapFromValue(record.Metadata["contractViolation"]), "focusId"),
			stringFromMap(mapFromValue(mapFromValue(record.Metadata["contractViolation"])["budgetScope"]), "focusId"),
		} {
			focusID = strings.TrimSpace(focusID)
			if focusID != "" && dispatchFocusAligned(map[string]any{"id": focusID}, item) {
				return true
			}
		}
		return false
	}
	return stringSliceContains(compactUniqueStrings(refs), itemID)
}

func validationDiagnosticClassFromEvidenceRecord(record *runtimemodel.EvidenceRecord) string {
	if record == nil {
		return ""
	}
	for _, source := range []map[string]any{
		mapFromValue(record.Metadata["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(record.Metadata["runtimeDiagnostics"])["validationFailureDiagnostic"]),
		record.Metadata,
	} {
		if class := allowedValidationDiagnosticClass(firstNonEmptyString(
			stringFromMap(source, "class"),
			stringFromMap(source, "diagnosticClass"),
			stringFromMap(source, "failureClass"),
		)); class != "" {
			return class
		}
	}
	if validationDiagnosticViolatedRule(record) != "" {
		return runtimemodel.ValidationDiagnosticContractViolation
	}
	return ""
}

func validationDiagnosticViolatedRule(record *runtimemodel.EvidenceRecord) string {
	if record == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(record.Metadata, "violatedRule"),
		stringFromMap(mapFromValue(record.Metadata["contractViolation"]), "violatedRule"),
	))
}

func validationDiagnosticFailureSignature(record *runtimemodel.EvidenceRecord) string {
	if record == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(record.Metadata, "failureSignature"),
		stringFromMap(mapFromValue(record.Metadata["contractViolation"]), "failureSignature"),
		stringFromMap(mapFromValue(record.Metadata["contractViolation"]), "signature"),
	))
}

func validationDiagnosticImpliesStateChange(class string, policy map[string]any) bool {
	class = allowedValidationDiagnosticClass(class)
	if class == runtimemodel.ValidationDiagnosticImplementationGap {
		return true
	}
	if class == "" || len(policy) == 0 || !boolValue(policy["validationFailureImpliesStateChange"]) {
		return false
	}
	classes := compactUniqueStrings(append(
		stringSliceFromValue(policy["implementationGapDiagnosticClasses"]),
		stringSliceFromValue(policy["validationFailureStateChangeDiagnosticClasses"])...,
	))
	return len(classes) > 0 && stringSliceContains(classes, class)
}

func validationDiagnosticIsContractViolation(class string, record *runtimemodel.EvidenceRecord) bool {
	if allowedValidationDiagnosticClass(class) == runtimemodel.ValidationDiagnosticContractViolation {
		return true
	}
	return validationDiagnosticViolatedRule(record) != ""
}

func validationDiagnosticAllowsNoDeltaStateChange(class string) bool {
	switch allowedValidationDiagnosticClass(class) {
	case runtimemodel.ValidationDiagnosticBuildFailed,
		runtimemodel.ValidationDiagnosticTestFailed,
		runtimemodel.ValidationDiagnosticOutputInconclusive,
		runtimemodel.ValidationDiagnosticEvidenceBindingMissing,
		runtimemodel.ValidationDiagnosticEvidenceMetadataMissing:
		return true
	default:
		return false
	}
}

func validationOnlyRetryMaxForPolicy(policy map[string]any) int {
	maxRetries := int(int64FromValue(policy["maxValidationOnlyRetriesPerFocus"]))
	if maxRetries < 0 {
		return 0
	}
	return maxRetries
}

func repeatedValidationDiagnosticCountForCoverageItem(run *runmodel.Run, item map[string]any, records []*runtimemodel.EvidenceRecord, latest *runtimemodel.EvidenceRecord) int {
	if run == nil || latest == nil {
		return 0
	}
	latestClass := validationDiagnosticClassFromEvidenceRecord(latest)
	latestRule := validationDiagnosticViolatedRule(latest)
	latestSignature := validationDiagnosticFailureSignature(latest)
	count := 0
	for _, record := range records {
		if record == nil ||
			record.WorkspaceID != run.WorkspaceID ||
			record.RunID != run.ID ||
			record.Kind != runtimemodel.EvidenceKindDiagnostic ||
			record.Status != runtimemodel.EvidenceStatusRejected ||
			!validationDiagnosticEvidenceMatchesCoverageItem(record, item) {
			continue
		}
		if latestSignature != "" && validationDiagnosticFailureSignature(record) == latestSignature {
			count++
			continue
		}
		if latestRule != "" && validationDiagnosticViolatedRule(record) == latestRule {
			count++
			continue
		}
		if latestRule == "" && latestSignature == "" && validationDiagnosticClassFromEvidenceRecord(record) == latestClass {
			count++
		}
	}
	return count
}

func validationNoDeltaRejectedDiagnosticCountForCoverageItem(run *runmodel.Run, item map[string]any, records []*runtimemodel.EvidenceRecord) int {
	if run == nil || len(item) == 0 || len(records) == 0 {
		return 0
	}
	latestAcceptedIndex := -1
	latestAcceptedTime := ""
	for idx, record := range records {
		if !acceptedEvidenceDeltaMatchesCoverageItem(run, item, record) {
			continue
		}
		latestAcceptedIndex = idx
		if ts := evidenceRecordTimestamp(record); ts != "" {
			latestAcceptedTime = ts
		}
	}
	count := 0
	for idx, record := range records {
		if record == nil ||
			record.WorkspaceID != run.WorkspaceID ||
			record.RunID != run.ID ||
			record.Kind != runtimemodel.EvidenceKindDiagnostic ||
			record.Status != runtimemodel.EvidenceStatusRejected ||
			!validationDiagnosticEvidenceMatchesCoverageItem(record, item) {
			continue
		}
		if latestAcceptedTime != "" {
			if !runtimeTimestampAfter(evidenceRecordTimestamp(record), latestAcceptedTime) {
				continue
			}
		} else if idx <= latestAcceptedIndex {
			continue
		}
		if !validationDiagnosticAllowsNoDeltaStateChange(validationDiagnosticClassFromEvidenceRecord(record)) {
			continue
		}
		count++
	}
	return count
}

func acceptedEvidenceDeltaMatchesCoverageItem(run *runmodel.Run, item map[string]any, record *runtimemodel.EvidenceRecord) bool {
	if run == nil || record == nil ||
		record.WorkspaceID != run.WorkspaceID ||
		record.RunID != run.ID ||
		record.Status != runtimemodel.EvidenceStatusAccepted {
		return false
	}
	switch record.Kind {
	case runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation:
	default:
		return false
	}
	return validationDiagnosticEvidenceMatchesCoverageItem(record, item)
}

func evidenceRecordTimestamp(record *runtimemodel.EvidenceRecord) string {
	if record == nil {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(record.UpdatedAt, record.CreatedAt))
}

func removeStringsFromSlice(values []string, removals ...string) []string {
	if len(values) == 0 || len(removals) == 0 {
		return compactUniqueStrings(values)
	}
	removeSet := stringSet(removals)
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if removeSet[strings.TrimSpace(value)] {
			continue
		}
		ret = append(ret, value)
	}
	return compactUniqueStrings(ret)
}

func validationDiagnosticManagedHarnessAvailable(policy map[string]any) bool {
	if len(policy) == 0 || strings.TrimSpace(stringFromMap(policy, "managedHarnessRef")) == "" {
		return false
	}
	return stringSet(stringSliceFromValue(policy["allowedExecutionModes"]))["managed_harness"]
}

func validationPolicyAppliesToCoverageItem(policy map[string]any, item map[string]any, evidenceKind string) bool {
	if len(policy) == 0 || len(item) == 0 {
		return false
	}
	if kinds := compactUniqueStrings(stringSliceFromValue(policy["appliesToEvidenceKinds"])); len(kinds) > 0 && !stringSliceContains(kinds, evidenceKind) {
		return false
	}
	if ids := compactUniqueStrings(stringSliceFromValue(policy["sourceRequirementIds"])); len(ids) > 0 &&
		!coverageStructuredIDsIntersect(ids, coverageWorkItemSourceRequirementIDs(item)) {
		return false
	}
	if ids := compactUniqueStrings(stringSliceFromValue(policy["coverageTargetIds"])); len(ids) > 0 &&
		!coverageStructuredIDsIntersect(ids, coverageWorkItemCoverageTargetIDs(item)) {
		return false
	}
	if refs := compactUniqueStrings(stringSliceFromValue(policy["targetRefs"])); len(refs) > 0 &&
		!coverageTargetRefsIntersect(refs, coverageWorkItemTargetRefs(item)) {
		return false
	}
	requirements := mapSliceFromValue(policy["coverageRequirements"])
	if len(requirements) == 0 {
		return true
	}
	for _, requirement := range requirements {
		if len(compactUniqueStrings(stringSliceFromValue(requirement["requiredEvidenceKinds"]))) > 0 &&
			!stringSliceContains(stringSliceFromValue(requirement["requiredEvidenceKinds"]), evidenceKind) {
			continue
		}
		if ids := compactUniqueStrings(stringSliceFromValue(requirement["sourceRequirementIds"])); len(ids) > 0 &&
			!coverageStructuredIDsIntersect(ids, coverageWorkItemSourceRequirementIDs(item)) {
			continue
		}
		if ids := compactUniqueStrings(stringSliceFromValue(requirement["coverageTargetIds"])); len(ids) > 0 &&
			!coverageStructuredIDsIntersect(ids, coverageWorkItemCoverageTargetIDs(item)) {
			continue
		}
		return true
	}
	return false
}

func validationRouteFromPolicy(policy map[string]any, fallback validationNextActionRoute) validationNextActionRoute {
	route := fallback
	route.PolicyRef = strings.TrimSpace(firstNonEmptyString(stringFromMap(policy, "policyRef"), stringFromMap(policy, "id")))
	route.ValidationSafetyClass = strings.TrimSpace(stringFromMap(policy, "validationSafetyClass"))
	route.ManagedHarnessRef = strings.TrimSpace(stringFromMap(policy, "managedHarnessRef"))
	route.DeferredEvidenceKind = strings.TrimSpace(stringFromMap(policy, "deferredEvidenceKind"))
	route.GovernanceReason = strings.TrimSpace(stringFromMap(policy, "governanceReason"))
	modes := stringSet(stringSliceFromValue(policy["allowedExecutionModes"]))
	switch {
	case modes["runtime_action"] && len(mapFromValue(policy["runtimeAction"])) > 0:
		route.Route = runtimemodel.ValidationRouteRuntimeAction
		route.StageMode = "runtime_action"
		route.Reason = "validation policy routes to deterministic runtime action"
	case modes["managed_harness"] && route.ManagedHarnessRef != "":
		route.Route = runtimemodel.ValidationRouteManagedHarness
		route.StageMode = "managed_harness"
		route.Reason = "validation policy routes to managed harness"
	case modes["deferred"]:
		route.Route = runtimemodel.ValidationRouteDeferred
		route.StageMode = "deferred"
		route.Reason = "validation policy routes to deferred evidence"
	case modes["decision"]:
		route.Route = runtimemodel.ValidationRouteRequestDecision
		route.ActionType = runtimemodel.NextActionTypeRequestDecision
		route.StageMode = "governance"
		route.Reason = firstNonEmptyString(route.GovernanceReason, "validation policy requires governance")
	case modes["fail"]:
		route.Route = runtimemodel.ValidationRouteFail
		route.ActionType = runtimemodel.NextActionTypeFail
		route.StageMode = "fail"
		route.Reason = "validation policy disallows automatic validation"
	case validationSafetyClassRequiresNonLLMRoute(route.ValidationSafetyClass):
		route = validationRouteForUnsafePolicy(route, modes)
	case modes["llm_graph"] || len(modes) == 0:
		route.Route = runtimemodel.ValidationRouteLLMGraph
		route.StageMode = "evidence"
	}
	return route
}

func validationRouteWithoutDirectDecisionNextAction(route validationNextActionRoute) validationNextActionRoute {
	if route.ActionType != runtimemodel.NextActionTypeRequestDecision {
		return route
	}
	route.ActionType = ""
	return route
}

func validationSafetyClassRequiresNonLLMRoute(value string) bool {
	switch strings.TrimSpace(value) {
	case "destructive", "external", "ci_only", "manual":
		return true
	default:
		return false
	}
}

func validationRouteForUnsafePolicy(route validationNextActionRoute, modes map[string]bool) validationNextActionRoute {
	switch {
	case modes["managed_harness"] && route.ManagedHarnessRef != "":
		route.Route = runtimemodel.ValidationRouteManagedHarness
		route.StageMode = "managed_harness"
		route.Reason = "unsafe validation policy routes to managed harness"
	case modes["deferred"] || route.DeferredEvidenceKind != "":
		route.Route = runtimemodel.ValidationRouteDeferred
		route.StageMode = "deferred"
		route.Reason = "unsafe validation policy routes to deferred evidence"
	case modes["decision"]:
		route.Route = runtimemodel.ValidationRouteRequestDecision
		route.ActionType = runtimemodel.NextActionTypeRequestDecision
		route.StageMode = "governance"
		route.Reason = firstNonEmptyString(route.GovernanceReason, "unsafe validation policy requires governance")
	default:
		route.Route = runtimemodel.ValidationRouteFail
		route.ActionType = runtimemodel.NextActionTypeFail
		route.StageMode = "fail"
		route.Reason = "unsafe validation policy has no allowed non-LLM route"
	}
	return route
}

func applyValidationRouteToRequiredOutput(requiredOutput map[string]any, route validationNextActionRoute) map[string]any {
	next := cloneStepPayload(requiredOutput)
	if next == nil {
		next = map[string]any{}
	}
	next["validationRoute"] = firstNonEmptyString(route.Route, runtimemodel.ValidationRouteLLMGraph)
	next["validationRouteOwner"] = "runtime_next_action"
	next["mustProduceConcreteValidationEvidence"] = true
	next["mustNotMutateWorkspace"] = true
	if route.PolicyRef != "" {
		next["validationPolicyRef"] = route.PolicyRef
	}
	if route.ValidationSafetyClass != "" {
		next["validationSafetyClass"] = route.ValidationSafetyClass
	}
	if route.ManagedHarnessRef != "" {
		next["managedHarnessRef"] = route.ManagedHarnessRef
	}
	if route.DeferredEvidenceKind != "" {
		next["deferredEvidenceKind"] = route.DeferredEvidenceKind
	}
	if route.GovernanceReason != "" {
		next["governanceReason"] = route.GovernanceReason
	}
	switch route.Route {
	case runtimemodel.ValidationRouteRuntimeAction:
		next["mustUseRuntimeAction"] = true
	case runtimemodel.ValidationRouteManagedHarness:
		next["mustUseManagedHarness"] = true
	case runtimemodel.ValidationRouteDeferred:
		next["mustRecordDeferredEvidenceRequirement"] = true
	case runtimemodel.ValidationRouteRequestDecision:
		next["mustRequestGovernance"] = true
	case runtimemodel.ValidationRouteFail:
		next["mustFailFast"] = true
	}
	return next
}

func validationFailureStateChangeRequiredOutput(requiredOutput map[string]any, route validationNextActionRoute) map[string]any {
	next := cloneStepPayload(requiredOutput)
	if next == nil {
		next = map[string]any{}
	}
	for _, key := range []string{
		"validationRoute",
		"validationRouteOwner",
		"mustProduceConcreteValidationEvidence",
		"mustNotMutateWorkspace",
		"mustUseRuntimeAction",
		"mustUseManagedHarness",
		"mustRecordDeferredEvidenceRequirement",
		"mustRequestGovernance",
		"mustFailFast",
	} {
		delete(next, key)
	}
	next["requiredEvidenceKinds"] = []string{runtimemodel.EvidenceKindStateChange}
	next["mustIncludeStateChangingNode"] = true
	next["acceptedKinds"] = stateChangingRepairAcceptedKinds()
	next["forbiddenKinds"] = removeStringsFromSlice(stringSliceFromValue(next["forbiddenKinds"]), stateChangingRepairAcceptedKinds()...)
	next["mustAddressValidationFailureDiagnostic"] = true
	next["mustNotRepeatRejectedValidationOnlyOutput"] = true
	next["validationFailureToStateChange"] = true
	if route.Route == "validation_loop_without_state_change" {
		next["validationNoDeltaReason"] = route.Route
		next["validationNoDeltaRejectedAttempts"] = route.RetryAttempt
	}
	if route.DiagnosticEvidenceID != "" {
		next["diagnosticEvidenceId"] = route.DiagnosticEvidenceID
	}
	if route.DiagnosticClass != "" {
		next["validationFailureDiagnosticClass"] = route.DiagnosticClass
	}
	if route.PolicyRef != "" {
		next["validationPolicyRef"] = route.PolicyRef
	}
	return next
}

func validationFailureFailRequiredOutput(requiredOutput map[string]any, route validationNextActionRoute) map[string]any {
	next := cloneStepPayload(requiredOutput)
	if next == nil {
		next = map[string]any{}
	}
	for _, key := range []string{
		"mustProduceConcreteValidationEvidence",
		"mustNotMutateWorkspace",
		"mustIncludeStateChangingNode",
		"mustUseRuntimeAction",
		"mustUseManagedHarness",
		"mustRecordDeferredEvidenceRequirement",
		"mustRequestGovernance",
	} {
		delete(next, key)
	}
	next["mustFailFast"] = true
	next["failureReason"] = firstNonEmptyString(route.EscalationReason, "validation_failure_not_recoverable")
	if route.DiagnosticEvidenceID != "" {
		next["diagnosticEvidenceId"] = route.DiagnosticEvidenceID
	}
	if route.DiagnosticClass != "" {
		next["validationFailureDiagnosticClass"] = route.DiagnosticClass
	}
	if route.ViolatedRule != "" {
		next["violatedRule"] = route.ViolatedRule
	}
	if route.FailureSignature != "" {
		next["failureSignature"] = route.FailureSignature
	}
	if route.RetryAttempt > 0 {
		next["retryAttempt"] = route.RetryAttempt
	}
	if route.RetryMaxAttempts > 0 {
		next["retryMaxAttempts"] = route.RetryMaxAttempts
	}
	return next
}

func deriveAcquireTargetSnapshotNextAction(run *runmodel.Run, stateChangeAction runtimemodel.NextAction) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || stateChangeAction.Type != runtimemodel.NextActionTypeStateChange {
		return runtimemodel.NextAction{}, false
	}
	input := mapFromValue(run.ExecutionContext)
	if len(input) == 0 {
		return runtimemodel.NextAction{}, false
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if !stateChangeRequiresTargetSnapshotAcquisition(input, pkg) {
		return runtimemodel.NextAction{}, false
	}
	required := cloneStepPayload(stateChangeAction.RequiredOutput)
	if required == nil {
		required = map[string]any{}
	}
	applyTargetSnapshotAcquisitionRequiredOutputContract(required)
	if targetSet := repairActionTargetSet(input, pkg); len(targetSet) > 0 {
		required["repairTargetSet"] = targetSet
		required["mustTargetRepairTargetSet"] = true
	}
	required["parentNextActionKind"] = runtimemodel.NextActionTypeStateChange
	required["prerequisiteForNextAction"] = runtimemodel.NextActionTypeStateChange
	required["finiteExitRule"] = "accepted_target_snapshot_evidence_or_scoped_blocker_or_internal_fail_fast"
	return runtimemodel.NextAction{
		WorkspaceID:          run.WorkspaceID,
		RunID:                run.ID,
		Type:                 runtimemodel.NextActionTypeStateChange,
		SubjectType:          stateChangeAction.SubjectType,
		SubjectID:            stateChangeAction.SubjectID,
		FocusCoverageItemIDs: append([]common.CoverageWorkItemID(nil), stateChangeAction.FocusCoverageItemIDs...),
		RequiredOutput:       required,
		AllowedActions:       []string{runtimemodel.NextActionTypeStateChange},
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeAcquireTargetSnapshot,
			runtimemodel.NextActionTypeComplete,
			runtimemodel.NextActionTypeRequestDecision,
		},
		RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindDiagnostic},
		Stage:                 runtimemodel.NextActionTypeStateChange,
		StageMode:             "target_snapshot_acquisition",
		Reason:                "state_change requires current target snapshot prerequisite",
	}.Normalized(), true
}

func (o *RuntimeOrchestrator) recoverableDiagnosticNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	if run == nil || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false, nil
	}
	if o == nil || o.evidence.EvidenceRecords == nil {
		return runtimemodel.NextAction{}, false, nil
	}
	records, err := o.evidence.EvidenceRecords.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return runtimemodel.NextAction{}, false, nil
		}
		return runtimemodel.NextAction{}, false, err
	}
	action, ok := deriveRecoverableDiagnosticNextAction(run, records)
	return action, ok, nil
}

func deriveRecoverableDiagnosticNextAction(run *runmodel.Run, records []*runtimemodel.EvidenceRecord) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false
	}
	superseded := supersededEvidenceRecordSet(records)
	for idx := len(records) - 1; idx >= 0; idx-- {
		record := records[idx]
		if record == nil ||
			record.WorkspaceID != run.WorkspaceID ||
			record.RunID != run.ID ||
			record.Kind != runtimemodel.EvidenceKindDiagnostic ||
			record.Status != runtimemodel.EvidenceStatusRejected ||
			superseded[record.ID] {
			continue
		}
		recoverability := recoverabilityFromDiagnosticEvidence(record)
		if !contractViolationRecoverabilityCanRetry(recoverability) {
			continue
		}
		required := map[string]any{
			"diagnosticEvidenceId":           string(record.ID),
			"diagnosticSourceRef":            record.SourceRef,
			"recoverability":                 recoverability,
			"mustUseRejectedDiagnostic":      true,
			"mustNotPromoteRejectedEvidence": true,
		}
		if failureClass := strings.TrimSpace(stringFromMap(record.Metadata, "failureClass")); failureClass != "" {
			required["failureClass"] = failureClass
		}
		if failureSignature := strings.TrimSpace(stringFromMap(record.Metadata, "failureSignature")); failureSignature != "" {
			required["failureSignature"] = failureSignature
		}
		return runtimemodel.NextAction{
			WorkspaceID:    run.WorkspaceID,
			RunID:          run.ID,
			Type:           runtimemodel.NextActionTypeRepair,
			SubjectType:    "diagnostic",
			SubjectID:      string(record.ID),
			RequiredOutput: required,
			AllowedActions: []string{runtimemodel.NextActionTypeRepair},
			ForbiddenActions: []string{
				runtimemodel.NextActionTypeRequestDecision,
				runtimemodel.NextActionTypeComplete,
			},
			RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindDiagnostic},
			Stage:                 runtimemodel.NextActionTypeRepair,
			StageMode:             "diagnostic_retry",
			Reason:                "recoverable rejected diagnostic",
		}.Normalized(), true
	}
	return runtimemodel.NextAction{}, false
}

func supersededEvidenceRecordSet(records []*runtimemodel.EvidenceRecord) map[common.EvidenceRecordID]bool {
	ret := map[common.EvidenceRecordID]bool{}
	for _, record := range records {
		if record == nil {
			continue
		}
		for _, id := range record.Supersedes {
			if id != "" {
				ret[id] = true
			}
		}
	}
	return ret
}

func recoverabilityFromDiagnosticEvidence(record *runtimemodel.EvidenceRecord) string {
	if record == nil {
		return ""
	}
	if value := strings.TrimSpace(stringFromMap(record.Metadata, "recoverability")); value != "" {
		return value
	}
	return strings.TrimSpace(stringFromMap(mapFromValue(record.Metadata["contractViolation"]), "recoverability"))
}

func contractViolationRecoverabilityCanRetry(value string) bool {
	switch strings.TrimSpace(value) {
	case runtimemodel.ContractViolationRecoverabilityRetrySameContract,
		runtimemodel.ContractViolationRecoverabilityNarrowContract,
		runtimemodel.ContractViolationRecoverabilityNormalizeThenRetry:
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) allSatisfiedCompleteNextAction(ctx context.Context, run *runmodel.Run) (runtimemodel.NextAction, bool, error) {
	if run == nil || !runStateAllowsGapAssessment(run.State) {
		return runtimemodel.NextAction{}, false, nil
	}
	if len(mapFromValue(run.ExecutionContext[gapBaselineKey])) == 0 {
		return runtimemodel.NextAction{}, false, nil
	}
	items, err := o.coverageItemsForNextAction(ctx, run)
	if err != nil {
		return runtimemodel.NextAction{}, false, err
	}
	action, ok := deriveAllSatisfiedCompleteNextAction(run, items)
	return action, ok, nil
}

func deriveAllSatisfiedCompleteNextAction(run *runmodel.Run, items []map[string]any) (runtimemodel.NextAction, bool) {
	if run == nil || run.ID == "" || run.WorkspaceID == "" || !runStateAllowsGapAssessment(run.State) || len(items) == 0 {
		return runtimemodel.NextAction{}, false
	}
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(items))
	requiredKinds := make([]string, 0)
	for _, item := range items {
		itemID := common.CoverageWorkItemID(firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "repositoryId")))
		if itemID == "" || !coverageWorkItemCompletionReady(item) {
			return runtimemodel.NextAction{}, false
		}
		state := deriveCoverageWorkItemState(item)
		if len(state.MissingEvidenceKinds) > 0 {
			return runtimemodel.NextAction{}, false
		}
		coverageIDs = append(coverageIDs, itemID)
		requiredKinds = append(requiredKinds, state.RequiredEvidenceKinds...)
	}
	coverageIDStrings := make([]string, 0, len(coverageIDs))
	for _, id := range coverageIDs {
		coverageIDStrings = append(coverageIDStrings, string(id))
	}
	return runtimemodel.NextAction{
		WorkspaceID:          run.WorkspaceID,
		RunID:                run.ID,
		Type:                 runtimemodel.NextActionTypeComplete,
		SubjectType:          "run",
		SubjectID:            string(run.ID),
		FocusCoverageItemIDs: coverageIDs,
		RequiredOutput: map[string]any{
			"completionReady":     true,
			"coverageWorkItemIds": coverageIDStrings,
			"mustFreezeEvidence":  true,
		},
		AllowedActions: []string{runtimemodel.NextActionTypeComplete},
		ForbiddenActions: []string{
			runtimemodel.NextActionTypeStateChange,
			runtimemodel.NextActionTypeValidate,
			runtimemodel.NextActionTypeRepair,
			runtimemodel.NextActionTypeRequestDecision,
			runtimemodel.NextActionTypeRequestApproval,
		},
		RequiredEvidenceKinds: compactUniqueStrings(requiredKinds),
		Stage:                 runtimemodel.NextActionTypeComplete,
		StageMode:             "accepted_evidence",
		Reason:                "all coverage work items satisfied",
	}.Normalized(), true
}

func coverageWorkItemCompletionStatusSatisfied(item map[string]any) bool {
	switch strings.TrimSpace(stringFromMap(item, "status")) {
	case "satisfied", "closed", "completed":
		return true
	default:
		return false
	}
}

func coverageWorkItemCompletionReady(item map[string]any) bool {
	if len(item) == 0 || !coverageWorkItemHasAcceptedEvidence(item) {
		return false
	}
	if !coverageWorkItemCompletionStatusSatisfied(item) {
		return false
	}
	state := deriveCoverageWorkItemState(item)
	return len(state.MissingEvidenceKinds) == 0 && coverageWorkItemRequiredEvidenceComplete(item)
}

func deriveOpenDecisionNextAction(run *runmodel.Run, ticket *decisionmodel.Ticket) (runtimemodel.NextAction, bool) {
	if run == nil || ticket == nil {
		return runtimemodel.NextAction{}, false
	}
	if run.ID == "" || run.WorkspaceID == "" || run.State != common.RunStateBlockedOnDecision || run.BlockingTicketID == "" {
		return runtimemodel.NextAction{}, false
	}
	if ticket.ID != run.BlockingTicketID || ticket.WorkspaceID != run.WorkspaceID || ticket.RunID != run.ID || ticket.State != common.DecisionTicketStateOpen {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeRequestDecision,
		SubjectType: "decision",
		SubjectID:   string(ticket.ID),
		Stage:       "decision",
		StageMode:   "governance",
		RequiredOutput: map[string]any{
			"ticketId": string(ticket.ID),
		},
		AllowedActions:   []string{"resolve_decision"},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "open decision blocks run",
	}.Normalized(), true
}

func derivePendingApprovalNextAction(run *runmodel.Run, approval *approvalmodel.Approval) (runtimemodel.NextAction, bool) {
	if run == nil || approval == nil {
		return runtimemodel.NextAction{}, false
	}
	if run.ID == "" || run.WorkspaceID == "" || run.State != common.RunStateBlockedOnApproval || run.BlockingApprovalID == "" {
		return runtimemodel.NextAction{}, false
	}
	if approval.ID != run.BlockingApprovalID || approval.WorkspaceID != run.WorkspaceID || approval.RunID != run.ID || approval.State != common.ApprovalStatePending {
		return runtimemodel.NextAction{}, false
	}
	return runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeRequestApproval,
		SubjectType: "approval",
		SubjectID:   string(approval.ID),
		Stage:       "approval",
		StageMode:   "governance",
		RequiredOutput: map[string]any{
			"approvalId": string(approval.ID),
			"action":     approval.Action,
		},
		AllowedActions:   []string{"approve", "reject"},
		ForbiddenActions: []string{runtimemodel.NextActionTypeStateChange},
		Reason:           "pending approval blocks run",
	}.Normalized(), true
}
