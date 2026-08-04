package readmodelprojection

import (
	"fmt"
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

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
