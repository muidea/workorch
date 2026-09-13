package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeSemanticExecutionFeedbackMaxBytes = 64 * 1024

// RuntimeSemanticExecutionFeedback is a detached factual C4 value, not a
// second planning ledger. Its authority references come from the sealed repair
// obligation; execution references come from the matching committed receipt.
// No raw diagnostics, operation parameters, next action or credit travel here.
type RuntimeSemanticExecutionFeedback struct {
	WorkspaceID        common.WorkspaceID                  `json:"workspaceId"`
	RunID              common.RunID                        `json:"runId"`
	DefinitionRevision string                              `json:"definitionRevision"`
	StartupInputHash   string                              `json:"startupInputHash"`
	ManifestHash       string                              `json:"manifestHash"`
	CoverageWorkItemID common.CoverageWorkItemID           `json:"coverageWorkItemId"`
	FailedFocusID      string                              `json:"failedFocusId"`
	FailureSignature   string                              `json:"failureSignature"`
	OwnerModuleID      string                              `json:"ownerModuleId"`
	OperationKind      string                              `json:"operationKind"`
	EvidenceID         common.EvidenceRecordID             `json:"evidenceId"`
	EvidenceRevision   string                              `json:"evidenceRevision"`
	DispatchID         string                              `json:"dispatchId"`
	OperationID        string                              `json:"operationId"`
	ReceiptRef         string                              `json:"receiptRef"`
	ReceiptHash        string                              `json:"receiptHash"`
	Validation         *RuntimeValidationExecutionFeedback `json:"validation"`
	Diagnostic         *RuntimeSemanticExecutionDiagnostic `json:"diagnostic"`
	Hash               string                              `json:"hash"`
}

func (f RuntimeSemanticExecutionFeedback) Clone() RuntimeSemanticExecutionFeedback {
	f.Diagnostic = f.Diagnostic.Clone()
	if f.Validation != nil {
		v := f.Validation.Clone()
		f.Validation = &v
	}
	return f
}

func StableRuntimeSemanticExecutionFeedbackHash(f RuntimeSemanticExecutionFeedback) string {
	f.Hash = ""
	data, _ := json.Marshal(f)
	sum := sha256.Sum256(data)
	return "runtime-semantic-execution-feedback:" + hex.EncodeToString(sum[:])
}

// ValidateRuntimeSemanticExecutionFeedback requires exactly one causal fact
// per visible repair CWI. Coverage ordering and ordinary proof requests cannot
// receive unrelated historical failures. Missing/duplicate feedback is not an
// invitation for the provider to reconstruct it from a signature or prose.
func ValidateRuntimeSemanticExecutionFeedback(c RuntimeSemanticPlanRequestContext, m RuntimeSemanticPlanManifest) error {
	expected := map[common.CoverageWorkItemID]RuntimeSemanticValidationFailureRepair{}
	if c.PlanStep == RuntimeSemanticPlanStepProofBatch {
		for _, obligation := range m.ProofObligations {
			if obligation.ValidationFailureRepair != nil && containsCoverageWorkItemID(c.BatchCoverageWorkItemIDs, obligation.CoverageWorkItemID) {
				expected[obligation.CoverageWorkItemID] = *obligation.ValidationFailureRepair
			}
		}
	}
	if len(c.ExecutionFeedback) != len(expected) {
		return fmt.Errorf("semantic execution feedback does not cover the exact repair batch")
	}
	data, err := json.Marshal(c.ExecutionFeedback)
	if err != nil || len(data) > RuntimeSemanticExecutionFeedbackMaxBytes {
		return fmt.Errorf("semantic execution feedback exceeds bounded context capacity")
	}
	for _, f := range c.ExecutionFeedback {
		r, found := expected[f.CoverageWorkItemID]
		if !found || f.WorkspaceID != c.WorkspaceID || f.RunID != c.RunID || f.DefinitionRevision != c.Definition.Revision ||
			f.StartupInputHash != c.Definition.StartupInputHash || f.ManifestHash != m.Hash || f.FailedFocusID != r.FailedFocusID ||
			f.FailureSignature != r.FailureSignature || f.OwnerModuleID != r.TriggerOwnerModuleID || f.OperationKind != r.TriggerOperationKind {
			return fmt.Errorf("semantic execution feedback scope or repair origin mismatch")
		}
		descriptor, known := RuntimeOperationDescriptorForKind(f.OperationKind)
		if !known || descriptor.OwnerModuleID != f.OwnerModuleID || f.EvidenceID == "" || f.EvidenceRevision == "" ||
			f.DispatchID == "" || f.OperationID == "" || f.ReceiptRef == "" || f.ReceiptHash == "" || f.Hash != StableRuntimeSemanticExecutionFeedbackHash(f) {
			return fmt.Errorf("semantic execution feedback has incomplete or changed provenance")
		}
		if f.Validation != nil {
			if err := f.Validation.Validate(); err != nil {
				return err
			}
		}
		if f.Diagnostic == nil {
			return fmt.Errorf("semantic repair execution feedback requires the causal owner diagnostic")
		}
		if err := f.Diagnostic.Validate(f.OwnerModuleID); err != nil {
			return err
		}
		delete(expected, f.CoverageWorkItemID)
	}
	return nil
}

// RuntimeValidationExecutionFeedback preserves bounded owner-reported check
// facts. A nil value explicitly means the receipt has no typed observation;
// locations/hashes alone never imply a complete assertion or an accepted DoD.
type RuntimeValidationExecutionFeedback struct {
	RecipeID                 string                          `json:"recipeId,omitempty"`
	ObservationProtocolRef   string                          `json:"observationProtocolRef,omitempty"`
	FailureDetailProtocolRef string                          `json:"failureDetailProtocolRef,omitempty"`
	ExecutedChecks           []string                        `json:"executedChecks,omitempty"`
	PassedChecks             []string                        `json:"passedChecks,omitempty"`
	FailedChecks             []string                        `json:"failedChecks,omitempty"`
	SkippedChecks            []string                        `json:"skippedChecks,omitempty"`
	IncompleteChecks         []string                        `json:"incompleteChecks,omitempty"`
	CheckFailures            []RuntimeValidationCheckFailure `json:"checkFailures,omitempty"`
	FailureDetailsIncomplete bool                            `json:"failureDetailsIncomplete"`
	ChecksTruncated          bool                            `json:"checksTruncated,omitempty"`
}

func (f RuntimeValidationExecutionFeedback) Clone() RuntimeValidationExecutionFeedback {
	f.ExecutedChecks = append([]string(nil), f.ExecutedChecks...)
	f.PassedChecks = append([]string(nil), f.PassedChecks...)
	f.FailedChecks = append([]string(nil), f.FailedChecks...)
	f.SkippedChecks = append([]string(nil), f.SkippedChecks...)
	f.IncompleteChecks = append([]string(nil), f.IncompleteChecks...)
	f.CheckFailures = append([]RuntimeValidationCheckFailure(nil), f.CheckFailures...)
	return f
}

func (f RuntimeValidationExecutionFeedback) Validate() error {
	for _, checks := range [][]string{f.ExecutedChecks, f.PassedChecks, f.FailedChecks, f.SkippedChecks, f.IncompleteChecks} {
		if len(checks) > RuntimeValidationFailureDetailLimit {
			return fmt.Errorf("validation feedback check inventory exceeds capacity")
		}
	}
	o := RuntimeValidationObservation{RecipeID: f.RecipeID, ObservationProtocolRef: f.ObservationProtocolRef,
		FailureDetailProtocolRef: f.FailureDetailProtocolRef, ExecutedChecks: f.ExecutedChecks, PassedChecks: f.PassedChecks,
		FailedChecks: f.FailedChecks, SkippedChecks: f.SkippedChecks, IncompleteChecks: f.IncompleteChecks, CheckFailures: f.CheckFailures}
	if err := o.ValidateFailureDetails(); err != nil {
		return err
	}
	canonical := ProjectRuntimeValidationExecutionFeedback(&o)
	if !f.FailureDetailsIncomplete && canonical.FailureDetailsIncomplete {
		return fmt.Errorf("validation feedback hides incomplete failure details")
	}
	canonical.FailureDetailsIncomplete, canonical.ChecksTruncated = f.FailureDetailsIncomplete, f.ChecksTruncated
	if !reflect.DeepEqual(f, *canonical) {
		return fmt.Errorf("validation feedback is not canonical")
	}
	return nil
}
