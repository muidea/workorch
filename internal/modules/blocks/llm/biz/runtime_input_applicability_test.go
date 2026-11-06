package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateCorpusApplicabilityUsesSealedTaskAndPreservesDependencies(t *testing.T) {
	body := "complete implementation\r\n必要依赖"
	surface := rc.RuntimeValidationSurface{ID: "recipe", GenerationContext: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{{TargetRef: "dependency", Content: body, ContentHash: rc.RuntimeValidationInputContentHash(body)}}}, ProofRequirementBindings: []rc.RuntimeValidationProofRequirementBinding{{CoverageWorkItemID: "cwi", RequirementHash: "requirement", RequiredCheckRefs: []string{"check"}}}}
	catalog := rc.RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: "owner", SnapshotID: "snapshot", ValidationSurfaces: []rc.RuntimeValidationSurface{surface}}
	c := rc.RuntimeCandidateContext{Focus: rc.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}, Resources: rc.RuntimeResourceFacts{OperationAdmissibilities: []rc.RuntimeOperationAdmissibility{catalog}}}
	before, _ := json.Marshal(c)
	for _, state := range []rc.ConvergenceState{rc.ConvergenceStateNeedPrerequisite, rc.ConvergenceStateNeedStateChange, rc.ConvergenceStateNeedValidation, rc.ConvergenceStateNeedAssessment, rc.ConvergenceStateNeedGovernance} {
		c.RequiredState = state
		v := runtimeCandidateApplicableGenerationContexts(c, rc.RuntimeCandidateContractManifest{})
		if state == rc.ConvergenceStateNeedStateChange {
			if len(v) != 1 || v[0].Context.Files[0].Content != body || v[0].CoverageInstruction != runtimeCorpusObservationInstruction {
				t.Fatal("implementation lost source/dependency or received generation instruction")
			}
			v[0].Context.Files[0].Content = "caller mutation"
		} else if len(v) != 0 || len(runtimeCandidateApplicablePreparations(c)) != 0 {
			t.Fatal("unrelated task received generation materials", state)
		}
	}
	c.RequiredState = rc.ConvergenceStateNeedStateChange
	revision := rc.RuntimeCandidateContractManifest{ImplementationWorkingBinding: &rc.RuntimeImplementationWorkingBinding{InputRevision: &rc.RuntimeValidationInputRevision{Binding: rc.RuntimeValidationProofRequirementBinding{ValidationRecipeID: "recipe"}}}}
	revised := runtimeCandidateApplicableGenerationContexts(c, revision)
	if len(revised) != 1 || !strings.HasPrefix(revised[0].CoverageInstruction, "Prepare actual assertions") {
		t.Fatal("ready binding revision lost generation task")
	}
	revision.ImplementationWorkingBinding.InputRevision.Binding.ValidationRecipeID = "foreign-recipe"
	revised = runtimeCandidateApplicableGenerationContexts(c, revision)
	if len(revised) != 1 || revised[0].CoverageInstruction != runtimeCorpusObservationInstruction {
		t.Fatal("foreign recipe assigned generation task")
	}
	c.RequiredState = ""
	after, _ := json.Marshal(c)
	if string(after) != string(before) {
		t.Fatal("projection changed owner input")
	}
}

func TestCandidateFeedbackIsCausalAndDoesNotUseRecentCutoff(t *testing.T) {
	c := rc.RuntimeCandidateContext{Focus: rc.CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, RequiredEvidenceKinds: []string{rc.EvidenceKindValidation}}, Failure: &rc.FailureFact{Signature: "exact-failure"}}
	values := []rc.EvidenceFact{{ID: "causal", FailureSignature: "exact-failure"}, {ID: "related", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}}
	for i := 0; i < 100; i++ {
		values = append(values, rc.EvidenceFact{ID: common.EvidenceRecordID("other"), CoverageWorkItemIDs: []common.CoverageWorkItemID{"other-cwi"}})
	}
	got := runtimeCandidateRelevantEvidence(c, values)
	if len(got) != 2 || got[0].ID != "causal" || got[1].ID != "related" {
		t.Fatal("causal source lost or unrelated history retained")
	}
	c.Assessments = []rc.RuntimeAssessmentFact{{ID: "related", FocusID: "focus"}, {ID: "foreign", CoverageWorkItemIDs: []common.CoverageWorkItemID{"other-cwi"}}}
	if got := runtimeCandidateRelevantAssessments(c); len(got) != 1 || got[0].ID != "related" {
		t.Fatal("assessment scope was ignored")
	}
}

func TestOrdinaryOrderOmitsHistoryButRejectsItsInjection(t *testing.T) {
	r := runtimeSemanticProviderTestRequest(t)
	r.SemanticContext.PlanStep = rc.RuntimeSemanticPlanStepCoverageOrder
	r.SemanticContext.BatchID = ""
	r.SemanticContext.BatchCoverageWorkItemIDs = nil
	refreshSemanticProgressForTest(t, &r)
	p, err := runtimeProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range p.PromptContext.Sections {
		if s.ID != "C4.semantic-tail" {
			continue
		}
		if strings.Contains(s.Content, `"executionHistory"`) {
			t.Fatal("ordering exposed receipt history")
		}
		var tail runtimeSemanticTailProjection
		if err := json.Unmarshal([]byte(s.Content), &tail); err != nil {
			t.Fatal(err)
		}
		tail.ExecutionHistory = rc.RuntimeSemanticExecutionHistory{Hash: "unrelated-injected-history"}
		raw, _ := json.Marshal(tail)
		p.PromptContext.Sections[i].Content = string(raw)
	}
	if err := validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, p.ContextBudget, p.PromptContext); err == nil || !strings.Contains(err.Error(), "semantic_execution_history") {
		t.Fatal("unrelated receipt injection passed", err)
	}
}

func TestCandidateLocalFilesExpandExactlyAndRejectWrongSource(t *testing.T) {
	body := strings.Repeat("完整源文\r\n", 1000)
	hash := rc.RuntimeValidationInputContentHash(body)
	contexts := []runtimeValidationGenerationContextProjection{{Context: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{{TargetRef: "a", ContentHash: hash, Content: body}}}}}
	source := runtimeCandidateExecutionTailProjection{ImplementationWorkContent: &rc.RuntimeCandidateWorkContent{Snapshots: []rc.TargetSnapshot{{TargetRef: "a", ContentHash: hash, Content: body}}}}
	makePrompt := func() *models.PromptContext {
		var p struct {
			Context runtimeCandidateAuthorityProjection `json:"context"`
		}
		p.Context.ValidationGenerationContexts = contexts
		raw, _ := json.Marshal(p)
		return &models.PromptContext{Sections: []models.PromptSection{{ID: "C3.runtime-authority", Content: string(raw)}}}
	}
	clone := func() runtimeCandidateExecutionTailProjection {
		raw, _ := json.Marshal(source)
		var tail runtimeCandidateExecutionTailProjection
		json.Unmarshal(raw, &tail)
		return tail
	}
	for _, change := range []string{"none", "missing", "duplicate", "scope", "hash", "text", "mixed", "rule"} {
		t.Run(change, func(t *testing.T) {
			tail := clone()
			compactRuntimeCandidateWorkFiles(&tail, contexts)
			if len(tail.WorkFileRefs) != 1 || tail.ImplementationWorkContent.Snapshots[0].Content != "" {
				t.Fatal("exact duplicate not shared")
			}
			prompt := makePrompt()
			switch change {
			case "missing":
				prompt.Sections = nil
			case "duplicate":
				tail.WorkFileRefs = append(tail.WorkFileRefs, tail.WorkFileRefs[0])
			case "scope":
				tail.WorkFileRefs[0].Generation = 1
			case "hash":
				tail.ImplementationWorkContent.Snapshots[0].ContentHash = "foreign"
			case "text":
				prompt.Sections[0].Content = strings.Replace(prompt.Sections[0].Content, "完整源文", "篡改源文", 1)
			case "mixed":
				tail.ImplementationWorkContent.Snapshots[0].Content = "mixed"
			case "rule":
				tail.ReferenceRule = "claimed rule"
			}
			err := expandRuntimeCandidateWorkFiles(&tail, prompt)
			if change == "none" {
				tail.WorkFileRefs = nil
				tail.ReferenceRule = ""
				if err != nil || !reflect.DeepEqual(tail, source) {
					t.Fatal("expansion changed original", err)
				}
			} else if err == nil {
				t.Fatal("bad reference accepted", change)
			}
		})
	}
	if contexts[0].Context.Files[0].Content != body || source.ImplementationWorkContent.Snapshots[0].Content != body {
		t.Fatal("compaction changed source")
	}
	tail := clone()
	tail.ImplementationWorkContent.Snapshots[0].Content = "different before image"
	compactRuntimeCandidateWorkFiles(&tail, contexts)
	if len(tail.WorkFileRefs) != 0 {
		t.Fatal("different before image shared by target/hash alone")
	}
	tail = clone()
	compactRuntimeCandidateWorkFiles(&tail, contexts)
	raw, _ := json.Marshal(tail)
	prompt := makePrompt()
	section := models.PromptSection{ID: "C4.execution-tail", Content: string(raw)}
	prompt.Sections = append(prompt.Sections, section)
	m := runtimeProviderContextSectionMetrics(section, prompt)
	if m.LocalReferences != 1 || m.ExpandedBytes <= m.BodyBytes {
		t.Fatal("exact reference accounting lost", m)
	}
}

func TestCandidateStageGateRejectsInapplicableCorpusAndUnrelatedEvidence(t *testing.T) {
	semantic := runtimeSemanticProviderTestRequest(t)
	provider, err := runtimeProviderRequest(semantic)
	if err != nil {
		t.Fatal(err)
	}
	c := rc.RuntimeCandidateContext{WorkspaceID: semantic.SemanticContext.WorkspaceID, RunID: semantic.SemanticContext.RunID, RequiredState: rc.ConvergenceStateNeedAssessment, Focus: rc.CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}}
	c.Resources.ProviderCeiling = semantic.SemanticContext.ProviderCeiling
	for _, change := range []string{"none", "corpus", "evidence"} {
		prompt := *provider.PromptContext
		prompt.Sections = append([]models.PromptSection(nil), prompt.Sections...)
		actual := runtimeCandidateAuthorityContextProjection(c, rc.RuntimeCandidateContractManifest{})
		if change == "corpus" {
			actual.ValidationGenerationContexts = []runtimeValidationGenerationContextProjection{{OwnerModuleID: "injected"}}
		}
		raw, _ := json.Marshal(struct {
			Context  runtimeCandidateAuthorityProjection `json:"context"`
			Decision struct{ FocusRef string }           `json:"decision"`
		}{actual, struct{ FocusRef string }{"context.focus"}})
		prompt.Sections[3].Content = string(raw)
		tail := buildRuntimeCandidateExecutionTailProjection(c, 0, nil, rc.RuntimeCandidateContractManifest{}, nil)
		if change == "evidence" {
			tail.AcceptedEvidence = []runtimeCandidateProviderSafeEvidenceProjection{{ID: "foreign"}}
		}
		raw, _ = json.Marshal(tail)
		prompt.Sections[4].Content = string(raw)
		budget := *provider.ContextBudget
		err := validateRuntimeCandidateContextIntegrity(semantic.SemanticContext.Definition, c, rc.RuntimeCandidateContractManifest{}, 0, nil, &budget, &prompt)
		if (err == nil) != (change == "none") {
			t.Fatal("stage gate did not enforce exact selection", change, err)
		}
	}
}
