package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func terminalResultCompilerSource() RuntimeStateRecord {
	facts := testRuntimeFacts()
	facts.Coverage[0].SourceScope = "dodItem"
	facts.Coverage[0].SourceID = "requirement-1"
	facts.Coverage[0].TargetText = "原验收要求"
	return RuntimeStateRecord{
		ResultFeedbackVersion: RuntimeResultFeedbackVersionCurrent, StateRevision: 7,
		Definition: RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision, GoalID: "goal-1", DoDID: "dod-1"},
		Facts:      facts,
		Decision:   RuntimeDecision{State: ConvergenceStateFailed, AuthoritySnapshot: AuthoritySnapshot{ID: "authority-1"}, Terminal: &RuntimeTerminalDecision{Code: "bounded_failure", Summary: "有界终态"}},
	}
}

func TestRuntimeTerminalResultCompilerTerminalKindsAndVersionGuard(t *testing.T) {
	for _, kind := range []TerminalOutcomeKind{TerminalOutcomeKindCompleted, TerminalOutcomeKindFailed, TerminalOutcomeKindCancelled, TerminalOutcomeKindSuperseded} {
		t.Run(string(kind), func(t *testing.T) {
			source := terminalResultCompilerSource()
			switch kind {
			case TerminalOutcomeKindCompleted:
				source.Decision.State = ConvergenceStateComplete
			case TerminalOutcomeKindCancelled, TerminalOutcomeKindSuperseded:
				source.Decision.State = ConvergenceStateNeedStateChange
				source.Decision.Terminal = nil
				terminationKind := RuntimeTerminationCancelled
				if kind == TerminalOutcomeKindSuperseded {
					terminationKind = RuntimeTerminationSuperseded
				}
				source.Termination = &RuntimeTerminationRecord{Kind: terminationKind, RequestedBy: "user", Reason: "显式终止", Revision: 1}
			}
			before := source.Clone()
			material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
			if err != nil {
				t.Fatal(err)
			}
			if material.RuntimeCloseout.TerminalOutcome.Kind != kind || material.DoDAssessment.Satisfied != (kind == TerminalOutcomeKindCompleted) || material.RuntimeCloseout.RuntimeStateRevision != source.StateRevision {
				t.Fatalf("wrong terminal material: %+v", material)
			}
			if !reflect.DeepEqual(before, source) {
				t.Fatal("compiler mutated source")
			}
		})
	}
	for _, version := range []uint32{0, 1, 2, 3, 4, RuntimeResultFeedbackVersionCurrent + 1} {
		source := terminalResultCompilerSource()
		source.ResultFeedbackVersion = version
		before := source.Clone()
		if _, err := DeriveRuntimeTerminalResultMaterialFromState(source); err == nil {
			t.Fatalf("unsupported feedback version %d was accepted", version)
		}
		if !reflect.DeepEqual(before, source) {
			t.Fatal("version rejection mutated source")
		}
	}
	source := terminalResultCompilerSource()
	source.Decision.State = ConvergenceStateNeedStateChange
	source.Decision.Terminal = nil
	if _, err := DeriveRuntimeTerminalResultMaterialFromState(source); err == nil {
		t.Fatal("nonterminal source was accepted")
	}
	source.Decision.Terminal = &RuntimeTerminalDecision{Code: "competing", Summary: "非法终态"}
	if _, err := DeriveRuntimeTerminalResultMaterialFromState(source); err == nil {
		t.Fatal("nonterminal decision with terminal detail was accepted")
	}
}

func TestRuntimeTerminalResultCompilerArtifactsDiagnosticsAndRestart(t *testing.T) {
	source := terminalResultCompilerSource()
	source.Facts.Assessments = []RuntimeAssessmentFact{{
		ID: "assessment-1", FocusID: "focus-1", Scope: "dodItem", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Disposition: AssessmentDispositionTerminalRejection,
		ArtifactRefs: []string{"artifact-top"}, Items: []RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", ArtifactRefs: []common.ArtifactID{"artifact-item"}}},
	}}
	diagnostic := &RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "owner/test", OriginalClass: "owner_failure", OriginalCode: "exact_cause", OriginalMessage: "  原始诊断\n含完整内容  ", Signature: "owner/exact", TargetRefs: []string{"target-2", "target-1", "target-2"}}
	source.Decision.Terminal.OriginalDiagnostic = diagnostic
	before := source.Clone()
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	wantArtifacts := []common.ArtifactID{"artifact-item", "artifact-top"}
	if !reflect.DeepEqual(material.ArtifactRefs, wantArtifacts) || !reflect.DeepEqual(material.SummaryDetail.ArtifactRefs, wantArtifacts) || !reflect.DeepEqual(material.RuntimeCloseout.ArtifactRefs, wantArtifacts) {
		t.Fatal("assessment item artifact refs were lost")
	}
	if !reflect.DeepEqual(material.RuntimeCloseout.OriginalDiagnostic, diagnostic) {
		t.Fatal("owner diagnostic was rewritten")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	repeated, err := DeriveRuntimeTerminalResultMaterialFromState(reloaded)
	if err != nil || !reflect.DeepEqual(material, repeated) {
		t.Fatalf("same-version restart changed material: %v", err)
	}
	if !reflect.DeepEqual(source, before) {
		t.Fatal("compiler mutated source")
	}
	material.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "changed"
	if diagnostic.TargetRefs[0] != "target-2" {
		t.Fatal("material aliases owner diagnostic")
	}
}

func TestRuntimeTerminalResultCompilerCurrentCoverageAndChineseFeedback(t *testing.T) {
	source := terminalResultCompilerSource()
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(material.DoDAssessment.Items) != 1 || material.DoDAssessment.Items[0].State != string(RuntimeRequirementUnassessed) || !strings.Contains(material.DoDAssessment.Items[0].FailureReason, "尚未记录") || !strings.Contains(material.SummaryDetail.VerificationSummary, "尚未记录验证证据") {
		t.Fatal("unassessed requirement was not reported faithfully in Chinese")
	}
	if !reflect.DeepEqual(material.DoDAssessment.Items, material.SummaryDetail.DoDItems) || len(material.SummaryDetail.KnownLimitations) == 0 || len(material.SummaryDetail.ResidualRisks) == 0 || len(material.SummaryDetail.NextActions) == 0 {
		t.Fatal("terminal feedback is incomplete")
	}
	source.Facts.Assessments = []RuntimeAssessmentFact{{ID: "historical-pass", FocusID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Disposition: AssessmentDispositionSatisfied}}
	outcomes := DeriveRuntimeTerminalRequirementOutcomesFromState(source, RuntimeOwnerRevisionSet{})
	if len(outcomes) != 1 || outcomes[0].AssessmentState != RuntimeRequirementIndeterminate || outcomes[0].AssessmentReasonCode != "current_coverage_not_satisfied" {
		t.Fatal("historical assessment restored current coverage credit")
	}
}

func TestRuntimeTerminalResultCompilerPreservesFailureDiagnosticAcrossReload(t *testing.T) {
	source := terminalResultCompilerSource()
	diagnostic := &RuntimeOriginalDiagnostic{
		SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleOrchestrator,
		OriginalClass: "semantic_plan_rejection", OriginalCode: "semantic_plan_invalid",
		OriginalMessage: "  完整原始失败\n不得规范化  ", Signature: "semantic-plan/invalid-document",
		SemanticViolations: []RuntimeSemanticContractViolation{{
			Kind: RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].target.prospectiveTargetRefs",
			CoverageWorkItemID: "cwi-1", Rule: RuntimeSemanticConstraintRelational, Actual: " target ", Required: []string{"every target absent from sealed existing target set"},
		}},
	}
	source.Facts.Failure = &FailureFact{Class: FailureClassCandidateExhausted, Code: "semantic_contract_failed", Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage, OriginalDiagnostic: diagnostic}
	before := source.Clone()
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(material.RuntimeCloseout.OriginalDiagnostic, diagnostic) {
		t.Fatal("facts normalization replaced original diagnostic bytes or nil collection")
	}
	wire, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(wire, &reloaded); err != nil {
		t.Fatal(err)
	}
	repeated, err := DeriveRuntimeTerminalResultMaterialFromState(reloaded)
	if err != nil || !reflect.DeepEqual(material, repeated) {
		t.Fatalf("same original authority changed delivery material after reload: %v", err)
	}
	if !reflect.DeepEqual(source, before) {
		t.Fatal("terminal compiler mutated original authority")
	}
	material.RuntimeCloseout.OriginalDiagnostic.SemanticViolations[0].Required[0] = "changed"
	if !reflect.DeepEqual(source, before) {
		t.Fatal("terminal diagnostic aliases original failure")
	}
}

func TestRuntimeTerminalResultMaterialRawClonePreservesNilEmptyAndNestedValues(t *testing.T) {
	material := RuntimeTerminalResultMaterial{
		WorkspaceID: "workspace", RunID: "run", GoalID: "goal", DoDID: "dod", Summary: "summary",
		EvidenceRefs: []string{}, ArtifactRefs: []common.ArtifactID{},
		DoDAssessment: workorchresultcontract.DoDAssessment{Items: []workorchresultcontract.DoDAssessmentItem{{EvidenceRefs: []string{}}}, GoalCoverage: &workorchresultcontract.DoDCoverage{Items: []workorchresultcontract.DoDCoverageItem{{ArtifactRefs: []common.ArtifactID{"artifact"}}}}},
		SummaryDetail: workorchresultcontract.ResultSummary{
			KnownLimitations: []string{}, Diagnostics: workorcheventpayload.Payload{Fields: []workorcheventpayload.Field{{Name: "raw", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: []workorcheventpayload.Value{{Kind: workorcheventpayload.ValueObject, Object: []workorcheventpayload.Field{{Name: "text", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: "  原始文本  "}}}}}}}}},
		},
	}
	clone := material.CloneRaw()
	if !reflect.DeepEqual(clone, material) || clone.EvidenceRefs == nil || clone.ArtifactRefs == nil || clone.DoDAssessment.Items[0].EvidenceRefs == nil || clone.SummaryDetail.KnownLimitations == nil {
		t.Fatal("raw clone changed nil/empty identity")
	}
	clone.DoDAssessment.GoalCoverage.Items[0].ArtifactRefs[0] = "changed"
	clone.SummaryDetail.Diagnostics.Fields[0].Value.Array[0].Object[0].Value.String = "changed"
	if material.DoDAssessment.GoalCoverage.Items[0].ArtifactRefs[0] != "artifact" || material.SummaryDetail.Diagnostics.Fields[0].Value.Array[0].Object[0].Value.String != "  原始文本  " {
		t.Fatal("raw clone aliases nested material")
	}
	if err := material.ValidateRaw(); err == nil {
		t.Fatal("raw gate normalized a noncanonical empty ref set")
	}
	material = RuntimeTerminalResultMaterial{WorkspaceID: "workspace", RunID: "run", GoalID: "goal", DoDID: "dod", Summary: "summary"}
	if err := material.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
}
