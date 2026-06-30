package orchestrator

import (
	"context"
	"errors"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) reconcileCoverageAcceptedEvidenceRecords(ctx context.Context, run *runModel.Run) (bool, int, error) {
	if o == nil || o.evidence.EvidenceRecords == nil || run == nil || run.ID == "" || run.WorkspaceID == "" {
		return false, 0, nil
	}
	records, err := o.evidence.EvidenceRecords.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), run.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return false, 0, nil
		}
		return false, 0, err
	}
	changed, bound := o.coverageLedger().reconcileAcceptedEvidenceRecords(run, records, ctx)
	return changed, bound, nil
}

func (c *coverageSubsystem) reconcileAcceptedEvidenceRecords(run *runModel.Run, records []*runtimecontract.EvidenceRecord, contexts ...context.Context) (bool, int) {
	if run == nil || len(records) == 0 {
		return false, 0
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false, 0
	}
	superseded := supersededEvidenceRecordSet(records)
	changed := false
	bound := 0
	for _, record := range records {
		if !coverageReusableAcceptedEvidenceRecord(run, record, superseded) {
			continue
		}
		refs := coverageEvidenceRecordRefs(record)
		if len(refs) == 0 {
			continue
		}
		for _, item := range items {
			if !coverageEvidenceRecordCanSatisfyItem(record, item) {
				continue
			}
			matchedBy := coverageEvidenceRecordItemMatchReason(record, item)
			if matchedBy == "" {
				continue
			}
			beforeAccepted := coverageEvidenceCountForKind(record.Kind, mapStringSliceValue(item["acceptedEvidenceByKind"])[record.Kind])
			applied := coverageApplyWorkItemEvidenceWithOptions(
				item,
				[]string{record.Kind},
				refs,
				true,
				record.Kind == runtimecontract.EvidenceKindValidation,
				string(record.StepID),
				"reused accepted evidence record by structured coverage scope",
				coverageEvidenceAcceptanceOptionsFromEvidenceRecord(record),
			)
			afterAccepted := coverageEvidenceCountForKind(record.Kind, mapStringSliceValue(item["acceptedEvidenceByKind"])[record.Kind])
			acceptedBound := afterAccepted > beforeAccepted
			edgeChanged := false
			if acceptedBound {
				edgeChanged = coverageAppendEvidenceReuseEdge(item, record, matchedBy)
			}
			if applied || edgeChanged {
				changed = true
				if acceptedBound {
					bound++
				}
			}
		}
	}
	if !changed {
		return false, 0
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(
		revision,
		items,
		coverageExcludedSourcesFromContext(run.ExecutionContext),
		coverageHistoricalItemsFromContext(run.ExecutionContext),
	)
	synchronizeAcceptanceProjection(run.ExecutionContext, items, mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]))
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return true, bound
}

func coverageReusableAcceptedEvidenceRecord(run *runModel.Run, record *runtimecontract.EvidenceRecord, superseded map[common.EvidenceRecordID]bool) bool {
	if run == nil || record == nil ||
		record.WorkspaceID != run.WorkspaceID ||
		record.RunID != run.ID ||
		record.Kind == "" ||
		record.Status != runtimecontract.EvidenceStatusAccepted ||
		strings.TrimSpace(record.SourceRef) == "" ||
		superseded[record.ID] {
		return false
	}
	if strings.TrimSpace(run.DefinitionRevision) != "" &&
		strings.TrimSpace(record.DefinitionRevision) != "" &&
		strings.TrimSpace(record.DefinitionRevision) != strings.TrimSpace(run.DefinitionRevision) {
		return false
	}
	if record.Kind == runtimecontract.EvidenceKindValidation && !validationEvidenceCanSatisfyDoD(coverageEvidenceRecordQuality(record)) {
		return false
	}
	return true
}

func coverageEvidenceRecordCanSatisfyItem(record *runtimecontract.EvidenceRecord, item map[string]any) bool {
	if record == nil || len(item) == 0 {
		return false
	}
	required := stringSet(stringSliceFromValue(item["requiredEvidenceKinds"]))
	if !required[strings.TrimSpace(record.Kind)] {
		return false
	}
	state := deriveCoverageWorkItemState(item)
	if !stringSliceContains(state.MissingEvidenceKinds, record.Kind) {
		return false
	}
	return true
}

func coverageEvidenceRecordRefs(record *runtimecontract.EvidenceRecord) []string {
	if record == nil {
		return nil
	}
	return compactUniqueStrings([]string{
		strings.TrimSpace(record.SourceRef),
		strings.TrimSpace(string(record.ID)),
	})
}

func coverageEvidenceRecordQuality(record *runtimecontract.EvidenceRecord) runtimeEvidenceQuality {
	if record == nil {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "evidence record is missing"}
	}
	metadata := record.Metadata
	quality := common.EvidenceQuality(strings.TrimSpace(firstNonEmptyString(
		stringFromMap(metadata, "evidenceQuality"),
		stringFromMap(metadata, "quality"),
	)))
	return runtimeEvidenceQuality{
		Quality:  quality,
		Reason:   strings.TrimSpace(firstNonEmptyString(stringFromMap(metadata, "evidenceReason"), stringFromMap(metadata, "reason"))),
		ZeroWork: boolValue(metadata["zeroWork"]),
		Tags:     stringSliceFromValue(metadata["evidenceTags"]),
	}
}

func coverageEvidenceRecordItemMatchReason(record *runtimecontract.EvidenceRecord, item map[string]any) string {
	if record == nil || len(item) == 0 {
		return ""
	}
	if coverageStructuredIDsIntersect(coverageEvidenceRecordCoverageRefs(record), coverageWorkItemStructuredIDs(item)) {
		return "coverage_item_ref"
	}
	if coverageStructuredIDsIntersect(coverageEvidenceRecordSourceRequirementRefs(record), coverageWorkItemSourceRequirementIDs(item)) {
		return "source_requirement_id"
	}
	if coverageStructuredIDsIntersect(coverageEvidenceRecordCoverageTargetRefs(record), coverageWorkItemCoverageTargetIDs(item)) {
		return "coverage_target_id"
	}
	if coverageStrictTargetRefsIntersect(coverageEvidenceRecordTargetRefs(record), coverageWorkItemTargetRefs(item)) {
		return "target_ref"
	}
	return ""
}

func coverageEvidenceRecordCoverageRefs(record *runtimecontract.EvidenceRecord) []string {
	if record == nil {
		return nil
	}
	refs := append([]string{}, record.CoverageItemRefs...)
	metadata := record.Metadata
	for _, key := range []string{"coverageItemRefs", "coverageWorkItemIds", "coverageWorkItemIDs"} {
		refs = append(refs, stringSliceFromValue(metadata[key])...)
	}
	if strings.TrimSpace(record.SubjectType) == "coverage" || strings.TrimSpace(record.SubjectType) == "coverage_work_item" {
		refs = append(refs, record.SubjectID)
	}
	return compactUniqueStrings(refs)
}

func coverageEvidenceRecordSourceRequirementRefs(record *runtimecontract.EvidenceRecord) []string {
	if record == nil {
		return nil
	}
	return compactUniqueStrings(stringSliceFromValue(record.Metadata["sourceRequirementIds"]))
}

func coverageEvidenceRecordCoverageTargetRefs(record *runtimecontract.EvidenceRecord) []string {
	if record == nil {
		return nil
	}
	refs := make([]string, 0)
	for _, key := range []string{"coverageTargetIds"} {
		refs = append(refs, stringSliceFromValue(record.Metadata[key])...)
	}
	return compactUniqueStrings(refs)
}

func coverageEvidenceAcceptanceOptionsFromEvidenceRecord(record *runtimecontract.EvidenceRecord) coverageEvidenceAcceptanceOptions {
	if record == nil {
		return coverageEvidenceAcceptanceOptions{}
	}
	metadata := record.Metadata
	quality := coverageEvidenceRecordQuality(record)
	tags := stringSliceFromValue(metadata["evidenceTags"])
	tags = append(tags, stringSliceFromValue(metadata["tags"])...)
	ret := coverageEvidenceAcceptanceOptions{
		EnforceProofStrength: strings.TrimSpace(record.Kind) == runtimecontract.EvidenceKindValidation,
		Quality:              quality,
		TargetRefs:           compactUniqueStrings(coverageEvidenceRecordTargetRefs(record)),
		ObservedClaims:       coverageEvidenceRecordObservedClaims(record),
		ProofKind:            firstNonEmptyString(stringFromMap(metadata, "proofKind"), stringFromMap(metadata, "mode")),
		CoverageAlignment:    stringFromMap(metadata, "coverageAlignment"),
		Tags:                 compactUniqueStrings(tags),
		Reason:               firstNonEmptyString(quality.Reason, stringFromMap(metadata, "evidenceReason"), stringFromMap(metadata, "reason")),
	}
	if coverageEvidenceTagsRequireProofStrength(ret.Tags) {
		ret.EnforceProofStrength = true
	}
	return ret
}

func coverageEvidenceRecordObservedClaims(record *runtimecontract.EvidenceRecord) []string {
	if record == nil {
		return nil
	}
	metadata := record.Metadata
	claims := stringSliceFromValue(metadata["observedClaims"])
	claims = append(claims, stringSliceFromValue(metadata["claims"])...)
	for _, item := range mapSliceFromValue(metadata["observedClaims"]) {
		for _, key := range []string{"claim", "observed", "observation", "description"} {
			if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
				claims = append(claims, value)
				break
			}
		}
	}
	if claim := strings.TrimSpace(stringFromMap(metadata, "observedClaim")); claim != "" {
		claims = append(claims, claim)
	}
	return compactUniqueStrings(claims)
}

func coverageEvidenceRecordTargetRefs(record *runtimecontract.EvidenceRecord) []string {
	if record == nil {
		return nil
	}
	refs := append([]string{}, coverageStructuredBindingRefsFromValue(record.Metadata["structuredTargetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(record.Metadata["targetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(record.Metadata["sourceRequirementIds"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(record.Metadata["coverageTargetIds"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(record.Metadata["coverageTargets"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(record.Metadata["coverageWorkItemIds"])...)
	if ref := strings.TrimSpace(stringFromMap(record.Metadata, "targetRef")); ref != "" {
		if coverageBindingRefLooksStructured(ref) {
			refs = append(refs, ref)
		}
	}
	if strings.TrimSpace(record.SubjectType) == "target" || strings.TrimSpace(record.SubjectType) == "resource" {
		if coverageBindingRefLooksStructured(record.SubjectID) {
			refs = append(refs, record.SubjectID)
		}
	}
	return compactUniqueStrings(refs)
}

func coverageWorkItemStructuredIDs(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	ids := []string{
		stringFromMap(item, "id"),
		stringFromMap(item, "repositoryId"),
		stringFromMap(item, "coverageWorkItemId"),
		stringFromMap(item, "normalizedSourceKey"),
		stringFromMap(item, "sourceRequirementId"),
	}
	ids = append(ids, stringSliceFromValue(item["coverageWorkItemIds"])...)
	return compactUniqueStrings(ids)
}

func coverageWorkItemSourceRequirementIDs(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	ids := []string{stringFromMap(item, "sourceRequirementId")}
	ids = append(ids, stringSliceFromValue(item["sourceRequirementAliases"])...)
	return compactUniqueStrings(ids)
}

func coverageWorkItemCoverageTargetIDs(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	ids := []string{
		stringFromMap(item, "id"),
		stringFromMap(item, "coverageTargetId"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "normalizedSourceKey"),
	}
	ids = append(ids, stringSliceFromValue(item["coverageTargetIds"])...)
	return compactUniqueStrings(ids)
}

func coverageWorkItemTargetRefs(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	refs := []string{
		stringFromMap(item, "targetRef"),
	}
	refs = append(refs, coverageStructuredBindingRefsFromValue(item["structuredTargetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(item["targetRefs"])...)
	return compactUniqueStrings(refs)
}

func coverageStructuredIDsIntersect(left []string, right []string) bool {
	leftSet := map[string]bool{}
	for _, value := range left {
		if normalized := normalizeCoverageStructuredID(value); normalized != "" {
			leftSet[normalized] = true
		}
	}
	if len(leftSet) == 0 {
		return false
	}
	for _, value := range right {
		if leftSet[normalizeCoverageStructuredID(value)] {
			return true
		}
	}
	return false
}

func normalizeCoverageStructuredID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "focus-")
	return value
}

func coverageTargetRefsIntersect(left []string, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	normalizedRight := map[string]bool{}
	for _, value := range right {
		if normalized := normalizeRepairPatchTargetRef(value); normalized != "" {
			normalizedRight[normalized] = true
		}
	}
	for _, value := range left {
		normalized := normalizeRepairPatchTargetRef(value)
		if normalized != "" && normalizedRight[normalized] {
			return true
		}
	}
	return false
}

func coverageStrictTargetRefsIntersect(left []string, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	normalizedRight := map[string]bool{}
	for _, value := range right {
		if normalized := normalizeCoverageStructuredID(value); normalized != "" {
			normalizedRight[normalized] = true
		}
	}
	if len(normalizedRight) == 0 {
		return false
	}
	for _, value := range left {
		normalized := normalizeCoverageStructuredID(value)
		if normalized != "" && normalizedRight[normalized] {
			return true
		}
	}
	return false
}

func coverageAppendEvidenceReuseEdge(item map[string]any, record *runtimecontract.EvidenceRecord, matchedBy string) bool {
	if len(item) == 0 || record == nil || record.ID == "" {
		return false
	}
	edges := mapSliceFromValue(item["evidenceReuseRefs"])
	key := string(record.ID) + "|" + strings.TrimSpace(matchedBy)
	for _, edge := range edges {
		if stringFromMap(edge, "evidenceRecordId")+"|"+stringFromMap(edge, "matchedBy") == key {
			return false
		}
	}
	edge := map[string]any{
		"evidenceRecordId": string(record.ID),
		"sourceRef":        record.SourceRef,
		"kind":             record.Kind,
		"matchedBy":        matchedBy,
	}
	if record.StepID != "" {
		edge["stepId"] = string(record.StepID)
	}
	item["evidenceReuseRefs"] = append(edges, edge)
	return true
}
