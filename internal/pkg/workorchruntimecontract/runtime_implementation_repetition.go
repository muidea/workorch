package workorchruntimecontract

import "slices"

// RuntimeImplementationRepeatedWorkStates counts visits to the same COMPLETE
// resource state in the original CWI. It is not a count of successful effects,
// semantic progress, or consecutive partial reviews. A new revision, request,
// focus, explanation or process cannot erase an earlier visit. Only completed
// rounds participate; receipt validity is independently checked by the ledger.
// This conservative cycle guard cannot detect semantic wheel-spinning through
// different states; the original provider/token/capability budgets still apply.
func RuntimeImplementationRepeatedWorkStates(preview RuntimeImplementationWorkScopePreview, rounds []RuntimeImplementationWorkRound) int {
	if preview.Observation == nil || preview.Observation.Content == nil {
		return 0
	}
	policyHash := StableRuntimeImplementationWorkPolicyHash(preview.Request.Policy)
	count := 0
	for _, round := range rounds {
		if round.WorkspaceID != preview.Request.WorkspaceID || round.RunID != preview.Request.RunID || round.CoverageWorkItemID != preview.Request.CoverageWorkItemID || round.PolicyHash != policyHash || round.CompletedReceiptSetHash == "" || round.FailedReceiptHash != "" || round.ScopeObservation == nil || round.ScopeObservation.Content == nil {
			continue
		}
		if sameImplementationResourceState(*preview.Observation.Content, *round.ScopeObservation.Content) {
			count++
		}
	}
	return count
}

func sameImplementationResourceState(a, b RuntimeImplementationWorkContent) bool {
	// Observed tree revisions intentionally differ after a real successor write.
	// Compare original target identities and FULL bytes, not revision-sensitive
	// content hashes or a model's assertion of progress.
	return slices.EqualFunc(a.Snapshots, b.Snapshots, func(a, b TargetSnapshot) bool {
		return a.TargetRef == b.TargetRef && a.Content == b.Content && a.Size == b.Size && a.ContentHash == b.ContentHash
	})
}
