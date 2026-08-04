package workorchprojectioncontract

// RuntimeAuditEvidence is the immutable, owner-neutral projection of one
// already-judged Runtime evidence fact. It carries no reducer instruction or
// evidence-admission authority.
type RuntimeAuditEvidence struct {
	ID                  string   `json:"id,omitempty"`
	Kind                string   `json:"kind,omitempty"`
	AuthoritySnapshotID string   `json:"authoritySnapshotId,omitempty"`
	DispatchID          string   `json:"dispatchId,omitempty"`
	OperationID         string   `json:"operationId,omitempty"`
	Revision            string   `json:"revision,omitempty"`
	FailureClass        string   `json:"failureClass,omitempty"`
	FailureCode         string   `json:"failureCode,omitempty"`
	FailureSignature    string   `json:"failureSignature,omitempty"`
	TargetRefs          []string `json:"targetRefs,omitempty"`
	BindingRefs         []string `json:"bindingRefs,omitempty"`
	CoverageWorkItemIDs []string `json:"coverageWorkItemIds,omitempty"`
	Quality             string   `json:"quality,omitempty"`
	ZeroWork            bool     `json:"zeroWork,omitempty"`
	EvidenceReason      string   `json:"evidenceReason,omitempty"`
	EvidenceRefs        []string `json:"evidenceRefs,omitempty"`
	EvidenceTags        []string `json:"evidenceTags,omitempty"`
	ValidationRecipeID  string   `json:"validationRecipeId,omitempty"`
	ExecutedChecks      []string `json:"executedChecks,omitempty"`
	PassedChecks        []string `json:"passedChecks,omitempty"`
	FailedChecks        []string `json:"failedChecks,omitempty"`
	SkippedChecks       []string `json:"skippedChecks,omitempty"`
}

func CloneRuntimeAuditEvidence(values []RuntimeAuditEvidence) []RuntimeAuditEvidence {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeAuditEvidence, 0, len(values))
	for _, value := range values {
		value.TargetRefs = append([]string(nil), value.TargetRefs...)
		value.BindingRefs = append([]string(nil), value.BindingRefs...)
		value.CoverageWorkItemIDs = append([]string(nil), value.CoverageWorkItemIDs...)
		value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
		value.EvidenceTags = append([]string(nil), value.EvidenceTags...)
		value.ExecutedChecks = append([]string(nil), value.ExecutedChecks...)
		value.PassedChecks = append([]string(nil), value.PassedChecks...)
		value.FailedChecks = append([]string(nil), value.FailedChecks...)
		value.SkippedChecks = append([]string(nil), value.SkippedChecks...)
		ret = append(ret, value)
	}
	return ret
}
