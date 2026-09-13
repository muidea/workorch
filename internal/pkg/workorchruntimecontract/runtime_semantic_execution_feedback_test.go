package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSemanticExecutionFeedbackExactScopeAndClone(t *testing.T) {
	c := RuntimeSemanticPlanRequestContext{WorkspaceID: "ws", RunID: "run", PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}
	c.Definition.Revision, c.Definition.StartupInputHash = "definition", "startup"
	m := RuntimeSemanticPlanManifest{Hash: "manifest", ProofObligations: []RuntimeSemanticProofObligation{{CoverageWorkItemID: "cwi",
		ValidationFailureRepair: &RuntimeSemanticValidationFailureRepair{FailedFocusID: "focus", FailureSignature: "failure",
			TriggerOwnerModuleID: RuntimeModuleShell, TriggerOperationKind: RuntimeBlockKindManagedValidationSession}}}}
	f := RuntimeSemanticExecutionFeedback{WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.Definition.Revision,
		StartupInputHash: c.Definition.StartupInputHash, ManifestHash: m.Hash, CoverageWorkItemID: "cwi", FailedFocusID: "focus", FailureSignature: "failure",
		OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		EvidenceID: "evidence", EvidenceRevision: "revision", DispatchID: "dispatch", OperationID: "op", ReceiptRef: "receipt", ReceiptHash: "receipt-hash",
		Diagnostic: &RuntimeSemanticExecutionDiagnostic{FeedbackHash: "feedback-hash", OwnerModuleID: RuntimeModuleShell, OriginalClass: "contract", OriginalCode: "validation_failed", Signature: "failure", SummaryOnly: true},
		Validation: ProjectRuntimeValidationExecutionFeedback(&RuntimeValidationObservation{ExecutedChecks: []string{"check/a"}, FailedChecks: []string{"check/a"}})}
	f.Hash = StableRuntimeSemanticExecutionFeedbackHash(f)
	c.ExecutionFeedback = []RuntimeSemanticExecutionFeedback{f}
	if err := ValidateRuntimeSemanticExecutionFeedback(c, m); err != nil {
		t.Fatal(err)
	}
	cloned := c.Clone()
	cloned.ExecutionFeedback[0].Validation.FailedChecks[0] = "changed"
	if c.ExecutionFeedback[0].Validation.FailedChecks[0] != "check/a" {
		t.Fatal("context clone aliases source execution feedback")
	}
	for name, mutate := range map[string]func(*RuntimeSemanticExecutionFeedback){
		"workspace":                     func(v *RuntimeSemanticExecutionFeedback) { v.WorkspaceID = "other" },
		"run":                           func(v *RuntimeSemanticExecutionFeedback) { v.RunID = "other" },
		"definition":                    func(v *RuntimeSemanticExecutionFeedback) { v.DefinitionRevision = "other" },
		"startup":                       func(v *RuntimeSemanticExecutionFeedback) { v.StartupInputHash = "other" },
		"manifest":                      func(v *RuntimeSemanticExecutionFeedback) { v.ManifestHash = "other" },
		"cwi":                           func(v *RuntimeSemanticExecutionFeedback) { v.CoverageWorkItemID = "other" },
		"focus":                         func(v *RuntimeSemanticExecutionFeedback) { v.FailedFocusID = "other" },
		"failure":                       func(v *RuntimeSemanticExecutionFeedback) { v.FailureSignature = "other" },
		"owner":                         func(v *RuntimeSemanticExecutionFeedback) { v.OwnerModuleID = RuntimeModuleFile },
		"operation":                     func(v *RuntimeSemanticExecutionFeedback) { v.OperationKind = StepKindFileWrite },
		"receipt":                       func(v *RuntimeSemanticExecutionFeedback) { v.ReceiptHash = "" },
		"missing diagnostic":            func(v *RuntimeSemanticExecutionFeedback) { v.Diagnostic = nil },
		"summary disguised as complete": func(v *RuntimeSemanticExecutionFeedback) { v.Diagnostic.SummaryOnly = false },
		"diagnostic owner":              func(v *RuntimeSemanticExecutionFeedback) { v.Diagnostic.OwnerModuleID = RuntimeModuleFile },
		"oversize": func(v *RuntimeSemanticExecutionFeedback) {
			v.ReceiptRef = strings.Repeat("x", RuntimeSemanticExecutionFeedbackMaxBytes)
		},
		"hidden_missing_details": func(v *RuntimeSemanticExecutionFeedback) { v.Validation.FailureDetailsIncomplete = false },
	} {
		t.Run(name, func(t *testing.T) {
			changed := c.Clone()
			mutate(&changed.ExecutionFeedback[0])
			changed.ExecutionFeedback[0].Hash = StableRuntimeSemanticExecutionFeedbackHash(changed.ExecutionFeedback[0])
			if err := ValidateRuntimeSemanticExecutionFeedback(changed, m); err == nil {
				t.Fatal("invalid feedback accepted after self-hash recomputation")
			}
		})
	}
	for name, mutate := range map[string]func(*RuntimeSemanticPlanRequestContext){
		"missing": func(v *RuntimeSemanticPlanRequestContext) { v.ExecutionFeedback = nil },
		"duplicate": func(v *RuntimeSemanticPlanRequestContext) {
			v.ExecutionFeedback = append(v.ExecutionFeedback, f.Clone())
		},
		"ordering":   func(v *RuntimeSemanticPlanRequestContext) { v.PlanStep = RuntimeSemanticPlanStepCoverageOrder },
		"stale_hash": func(v *RuntimeSemanticPlanRequestContext) { v.ExecutionFeedback[0].EvidenceRevision = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := c.Clone()
			mutate(&changed)
			if err := ValidateRuntimeSemanticExecutionFeedback(changed, m); err == nil {
				t.Fatal("invalid batch feedback accepted")
			}
		})
	}
	missingObservation := c.Clone()
	missingObservation.ExecutionFeedback[0].Validation = nil
	missingObservation.ExecutionFeedback[0].Hash = StableRuntimeSemanticExecutionFeedbackHash(missingObservation.ExecutionFeedback[0])
	if err := ValidateRuntimeSemanticExecutionFeedback(missingObservation, m); err != nil {
		t.Fatalf("explicit missing observation is still a sourced fact: %v", err)
	}
	if !reflect.DeepEqual(f, c.ExecutionFeedback[0]) {
		t.Fatal("validation mutated feedback")
	}
}
