package readmodelprojection

import (
	"fmt"
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: result_quality_dod.go

func checkResultQualityDoDAssessment(quality *projectioncontract.ResultQualityTrace, item projectioncontract.Result, requiredDoDLeafItems []string, evidenceIndex map[string]struct{}, weakValidationRefs map[string]bool, evidenceRefs []string) {
	if item.DoDAssessment == nil || len(item.DoDAssessment.Items) == 0 {
		appendResultQualityIssue(quality, "missing_dod_item_assessment", "warning", "result is missing DoD item-level assessment", "attach DoD item states and evidence refs", evidenceRefs...)
		return
	}
	if missingLeaf := missingRequiredDoDLeafItems(requiredDoDLeafItems, item.DoDAssessment.Items); len(missingLeaf) > 0 {
		appendResultQualityIssue(
			quality,
			"missing_dod_leaf_coverage",
			"error",
			fmt.Sprintf("result dodAssessment does not cover required DoD leaf items: %s", strings.Join(missingLeaf, "; ")),
			"include each missing DoD leaf item in dodAssessment.items with concrete evidenceRefs",
			evidenceRefs...,
		)
	}
	for _, assessmentItem := range item.DoDAssessment.Items {
		checkResultQualityDoDAssessmentItem(quality, assessmentItem, evidenceIndex, weakValidationRefs)
	}
}

// source: result_quality_dod_item_issues.go

func appendResultQualityDoDMissingEvidenceIssue(quality *projectioncontract.ResultQualityTrace, itemName string) {
	appendResultQualityIssue(
		quality,
		"dod_item_missing_evidence_refs",
		"error",
		fmt.Sprintf("satisfied DoD item %q has no evidence refs", itemName),
		"attach concrete validation/patch/tool evidence refs for every satisfied DoD item",
	)
}

func appendResultQualityDoDUnlinkedEvidenceIssue(quality *projectioncontract.ResultQualityTrace, itemName string, missingRefs []string, refs []string) {
	appendResultQualityIssue(
		quality,
		"dod_item_unlinked_evidence_refs",
		"error",
		fmt.Sprintf("satisfied DoD item %q references unlinked evidence: %s", itemName, strings.Join(missingRefs, ", ")),
		"use evidence refs that exist in runtime steps, validations, patches, tool calls, checkpoints, or artifacts",
		refs...,
	)
}

func appendResultQualityDoDWeakValidationIssue(quality *projectioncontract.ResultQualityTrace, itemName string, refs []string) {
	appendResultQualityIssue(
		quality,
		"dod_item_weak_verification_evidence",
		"error",
		fmt.Sprintf("satisfied DoD item %q relies only on weak verification evidence", itemName),
		"add effective assertions/tests or mark the item failed/skipped with impact and residual risk",
		refs...,
	)
}
