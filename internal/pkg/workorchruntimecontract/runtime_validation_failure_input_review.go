package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"hash"
)

// RuntimeValidationFailureInputReview binds a new input-semantics transaction
// to the original failed execution. It grants neither repair authority nor
// acceptance credit. The exact input body remains in the owner catalog.
type RuntimeValidationFailureInputReview struct {
	BindingRef         RuntimeCWIBindingRef
	SubjectHash        string
	FailureFingerprint string
	EvidenceHash       string
	Feedback           RuntimeSemanticExecutionFeedback
}

func (s *RuntimeValidationFailureInputReview) Clone() *RuntimeValidationFailureInputReview {
	if s == nil {
		return nil
	}
	v := *s
	v.Feedback = s.Feedback.Clone()
	return &v
}

func (s RuntimeValidationFailureInputReview) Validate() error {
	f := s.Feedback
	if err := s.BindingRef.Validate(); err != nil {
		return err
	}
	if s.SubjectHash == "" || s.FailureFingerprint == "" || s.EvidenceHash == "" || f.Hash != StableRuntimeSemanticExecutionFeedbackHash(f) ||
		f.ManifestHash != s.BindingRef.SemanticManifestHash || f.WorkspaceID == "" || f.RunID == "" || f.CoverageWorkItemID == "" ||
		f.DefinitionRevision == "" || f.StartupInputHash == "" || f.FailedFocusID == "" || f.FailureSignature == "" ||
		f.DispatchID == "" || f.OperationID == "" || f.EvidenceID == "" || f.EvidenceRevision == "" || f.ReceiptRef == "" || f.ReceiptHash == "" {
		return fmt.Errorf("failure input review requires exact original binding/input/receipt provenance")
	}
	if f.PreviousInputReview == nil || f.PreviousInputReview.Validate() != nil || f.PreviousInputReview.Report.SubjectHash != s.SubjectHash || f.PreviousInputReview.CoverageWorkItemID != f.CoverageWorkItemID {
		return fmt.Errorf("failure input review requires the exact previous formal report")
	}
	data, err := json.Marshal([]RuntimeSemanticExecutionFeedback{f})
	if err != nil || len(data) > RuntimeSemanticExecutionFeedbackMaxBytes {
		return fmt.Errorf("failure input review exceeds bounded execution feedback capacity")
	}
	descriptor, ok := RuntimeOperationDescriptorForKind(f.OperationKind)
	if !ok || descriptor.OwnerModuleID != f.OwnerModuleID || f.Validation == nil || f.Validation.Validate() != nil || f.Validation.FailureDetailsIncomplete || f.Validation.ChecksTruncated || len(f.Validation.FailedChecks) == 0 {
		return fmt.Errorf("failure input review requires complete source-bound native diagnostics")
	}
	if f.Diagnostic == nil || f.Diagnostic.Validate(f.OwnerModuleID) != nil {
		return fmt.Errorf("failure input review requires exact owner diagnostic")
	}
	for _, check := range f.Validation.FailedChecks {
		found := false
		for _, detail := range f.Validation.CheckFailures {
			if detail.CheckRef == check && detail.AssertionText != "" {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("failure input review requires every native failed check body")
		}
	}
	return nil
}

func writeRuntimeFailureInputReviewHash(h hash.Hash, s *RuntimeValidationFailureInputReview) {
	if s == nil {
		return
	}
	writeHashStrings(h, []string{"validation-failure-input-review", s.BindingRef.BindingID, s.BindingRef.BindingHash, s.BindingRef.FocusID,
		s.BindingRef.SemanticManifestHash, s.SubjectHash, s.FailureFingerprint, s.EvidenceHash, StableRuntimeSemanticExecutionFeedbackHash(s.Feedback)})
}

func validateRuntimeFailureInputReviewManifest(m RuntimeSemanticPlanManifest) error {
	s := m.FailureInputReview
	if s == nil {
		return nil
	}
	if err := s.Validate(); err != nil {
		return err
	}
	f := s.Feedback
	if m.WorkspaceID != f.WorkspaceID || m.RunID != f.RunID || m.DefinitionRevision != f.DefinitionRevision || len(m.CoverageItems) != 1 || m.CoverageItems[0].ID != f.CoverageWorkItemID || len(m.ProofObligations) != 1 || m.ProofObligations[0].ValidationFailureRepair != nil || m.ProofObligations[0].ImplementationWork != nil {
		return fmt.Errorf("failure input review must retain one original Definition obligation")
	}
	matches := 0
	for _, c := range m.OwnerCatalogs {
		for _, surface := range c.ValidationSurfaces {
			for _, b := range surface.ProofRequirementBindings {
				if b.CoverageWorkItemID == f.CoverageWorkItemID && b.InputReview != nil && b.InputReview.Hash == s.SubjectHash {
					matches++
				}
			}
		}
	}
	if matches != 1 {
		return fmt.Errorf("failure input review requires unique unchanged owner input subject")
	}
	return nil
}

func runtimeFailureInputReviewDocument(l RuntimeSemanticPlanningLedger) (RuntimeSemanticPlanDocument, bool) {
	if l.FailureInputReview == nil || l.Manifest == nil || l.Phase != RuntimeSemanticPlanningProofBatchDocumentAccepted {
		return RuntimeSemanticPlanDocument{}, false
	}
	for _, d := range l.BatchDocuments {
		if d.BatchID != l.ActiveBatchID {
			continue
		}
		count := 0
		for _, p := range d.Proposals {
			if p.CoverageWorkItemID == l.FailureInputReview.Feedback.CoverageWorkItemID && p.ValidationInputReview != nil && p.ValidationInputReview.SubjectHash == l.FailureInputReview.SubjectHash {
				count++
			}
		}
		if count == 1 {
			return d, true
		}
	}
	return RuntimeSemanticPlanDocument{}, false
}
