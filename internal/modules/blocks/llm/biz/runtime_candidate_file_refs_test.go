package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The reference fixture exercises the three independent input roles without
// depending on local docs, a live workspace or provider calls. The full owner
// and renderer admission paths have their own contract fixtures.
func candidateBodyReferenceFixture(t *testing.T, structural bool) (models.RuntimeCandidateRequest, models.ProviderRequest, runtimeCandidateExecutionTailProjection) {
	t.Helper()
	semantic := runtimeSemanticProviderTestRequest(t)
	p, err := runtimeProviderRequest(semantic)
	if err != nil {
		t.Fatal(err)
	}
	file := func(target, body string) rc.RuntimeValidationInputReviewFile {
		body = t.Name() + "\r\n" + body
		return rc.RuntimeValidationInputReviewFile{TargetRef: target, ContentHash: rc.RuntimeValidationInputContentHash(body), Content: body}
	}
	files := []rc.RuntimeValidationInputReviewFile{
		file("generation", strings.Repeat("source corpus\r\n", 1000)),
		file("reviewed", strings.Repeat("完整输入\r\n", 1000)),
		file("work-only", strings.Repeat("work snapshot\n", 1500)),
		file("dependency", "complete unchanged dependency"),
	}
	revision := &rc.RuntimeValidationInputRevision{SourceLogicalRequestID: "original-review", SourceDocumentHash: "original-report", Binding: rc.RuntimeValidationProofRequirementBinding{InputReview: &rc.RuntimeValidationInputReviewSubject{Files: []rc.RuntimeValidationInputReviewFile{files[1]}}}}
	projectedRevision := inputRevisionForProvider(revision)
	if structural {
		// Structural maintenance uses the current owner observation, not the old
		// binding. Keep the old preimage intact and distinct in the request.
		projectedRevision.Binding.InputReview.Files[0] = file("reviewed", "different historical preimage")
		projectedRevision.Observation = &runtimeInputRevisionObservationProjection{ObservationID: "observation", Files: []rc.RuntimeValidationInputReviewFile{files[1]}}
	}
	source := &rc.RuntimeImplementationWorkContent{ProtocolRef: rc.RuntimeImplementationWorkContentV1, ManifestRevision: "tree"}
	for _, f := range files {
		source.Snapshots = append(source.Snapshots, rc.TargetSnapshot{TargetRef: f.TargetRef, ContentHash: f.ContentHash, Content: f.Content, ContentState: rc.SnapshotContentStateComplete, Size: int64(len(f.Content))})
	}
	c := rc.RuntimeCandidateContext{WorkspaceID: semantic.SemanticContext.WorkspaceID, RunID: semantic.SemanticContext.RunID, RequiredState: rc.ConvergenceStateNeedStateChange,
		Focus: rc.CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}, ImplementationWorkContent: source,
		ChangeReview: &rc.RuntimeChangeReviewContext{WorkspaceID: semantic.SemanticContext.WorkspaceID, RunID: semantic.SemanticContext.RunID, Current: files},
	}
	c.Resources.ProviderCeiling = semantic.SemanticContext.ProviderCeiling
	c.Resources.OperationAdmissibilities = []rc.RuntimeOperationAdmissibility{{PrerequisiteID: "catalog", SnapshotID: "snapshot", OwnerModuleID: "owner", ValidationSurfaces: []rc.RuntimeValidationSurface{{ID: "recipe", GenerationContext: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{files[0]}}, ProofRequirementBindings: []rc.RuntimeValidationProofRequirementBinding{{CoverageWorkItemID: "cwi", RequirementHash: "requirement", RequiredCheckRefs: []string{"check"}}}}}}}
	baseline := file("reviewed", "distinct original before image\r\n")
	op := rc.RuntimeOperation{ID: "original", Kind: rc.StepKindFileEdit, TargetRefs: []string{"reviewed"}, Parameters: []rc.OperationParameter{{Name: "before", String: &baseline.Content}, {Name: "after", String: &files[1].Content}}}
	c.ChangeReview.Changes = []rc.RuntimeChangeReviewOperation{{TargetRef: "reviewed", Operation: op, OperationHash: rc.StableRuntimeOperationHash(op), Baseline: &baseline}}
	m := rc.RuntimeCandidateContractManifest{Hash: "manifest", RequiredOperationSequence: []rc.RuntimeRequiredOperationVariant{{RequirementID: "one", TargetRef: "reviewed"}, {RequirementID: "two", TargetRef: "work-only"}},
		ImplementationWorkingBinding: &rc.RuntimeImplementationWorkingBinding{IntentBinding: &rc.RuntimeImplementationWorkIntentBinding{Source: rc.RuntimeImplementationWorkPurposeChoice{ScopeRequestID: "scope"}, Intent: &rc.RuntimeImplementationIntent{ContentHash: source.Hash()}}},
		Operations:                   []rc.RuntimeCandidateOperationManifest{{Kind: rc.StepKindFileEdit, InputRevision: revision}},
	}
	output := runtimeCandidateOutputContractProjection{OperationContracts: []runtimeCandidateOperationContractProjection{{Kind: rc.StepKindFileEdit, InputRevision: projectedRevision}}}
	authority, _ := json.Marshal(struct {
		Context  runtimeCandidateAuthorityProjection      `json:"context"`
		Output   runtimeCandidateOutputContractProjection `json:"outputContract"`
		Decision struct{ FocusRef string }                `json:"decision"`
	}{runtimeCandidateAuthorityContextProjection(c, m), output, struct{ FocusRef string }{"context.focus"}})
	tail := buildRuntimeCandidateExecutionTailProjection(c, 0, nil, m, nil)
	before, _ := json.Marshal(tail)
	compactRuntimeCandidateFiles(&tail, runtimeCandidateAuthorityBodySources(output, runtimeCandidateApplicableGenerationContexts(c, m)))
	raw, _ := json.Marshal(tail)
	p.PromptContext.Sections[3].ID, p.PromptContext.Sections[3].Content = "C3.runtime-authority", string(authority)
	p.PromptContext.Sections[4].ID, p.PromptContext.Sections[4].Content = "C4.execution-tail", string(raw)
	var original runtimeCandidateExecutionTailProjection
	if err := json.Unmarshal(before, &original); err != nil {
		t.Fatal(err)
	}
	r := models.RuntimeCandidateRequest{DocumentKind: rc.RuntimeProviderDocumentOperations, CandidateContext: c, CandidateManifest: m}
	// The semantic fixture stores its definition in the semantic context.
	r.DefinitionContext = semantic.SemanticContext.Definition
	return r, p, original
}

func TestCandidateRequestLocalBodiesRoundtripAndAccounting(t *testing.T) {
	for _, structural := range []bool{false, true} {
		t.Run(map[bool]string{false: "semantic-review", true: "owner-observation"}[structural], func(t *testing.T) {
			t.Parallel()
			r, p, original := candidateBodyReferenceFixture(t, structural)
			var tail runtimeCandidateExecutionTailProjection
			json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &tail)
			if len(tail.WorkFileRefs) != 1 || len(tail.ChangeReviewFileRefs) != 3 || len(tail.ChangeReviewValueRefs) == 0 {
				t.Fatal("three input roles did not share exact bodies", tail.WorkFileRefs, tail.ChangeReviewFileRefs)
			}
			if len(tail.ImplementationWorkContent.Snapshots) != 2 || len(tail.ChangeReview.Current) != 4 || len(r.CandidateContext.ImplementationWorkContent.Snapshots) != 4 {
				t.Fatal("selected work targets changed review/dependency scope")
			}
			if err := expandRuntimeCandidateFiles(&tail, p.PromptContext); err != nil {
				t.Fatal(err)
			}
			for _, ch := range tail.ChangeReview.Changes {
				if rc.StableRuntimeOperationHash(ch.Operation) != ch.OperationHash {
					t.Fatal("historical operation or preimage changed")
				}
			}
			count := len(tail.WorkFileRefs) + len(tail.ChangeReviewFileRefs) + len(tail.ChangeReviewValueRefs)
			tail.WorkFileRefs, tail.ChangeReviewFileRefs, tail.ChangeReviewValueRefs, tail.ReferenceRule = nil, nil, nil, ""
			if !reflect.DeepEqual(tail, original) {
				t.Fatal("local reuse changed full original values")
			}
			expanded, _ := json.Marshal(original)
			metrics := runtimeProviderContextSectionMetrics(p.PromptContext.Sections[4], p.PromptContext)
			if metrics.LocalReferences != count || metrics.ExpandedBytes != len(expanded) || metrics.BodyBytes != len(p.PromptContext.Sections[4].Content) || metrics.ExpandedBytes <= metrics.BodyBytes {
				t.Fatal("request accounting did not use the same expansion", metrics)
			}
			if _, err := runtimeCandidateExecutionTailForIntegrity(r.CandidateContext, r.CandidateManifest, p.PromptContext); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCandidateRequestLocalBodiesRejectBrokenReferences(t *testing.T) {
	for _, name := range []string{"missing-C3", "duplicate-C3", "missing-source", "wrong-target", "wrong-hash", "changed-source", "work-cycle", "review-cycle", "duplicate-work", "duplicate-review", "mixed-work", "lost-work-ref", "lost-review-ref", "lost-value-ref", "foreign-source", "unknown-rule"} {
		t.Run(name, func(t *testing.T) {
			r, p, _ := candidateBodyReferenceFixture(t, false)
			var tail runtimeCandidateExecutionTailProjection
			json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &tail)
			switch name {
			case "missing-C3":
				p.PromptContext.Sections[3].ID = "absent"
			case "duplicate-C3":
				p.PromptContext.Sections = append(p.PromptContext.Sections, p.PromptContext.Sections[3])
			case "missing-source":
				p.PromptContext.Sections[3].Content = `{}`
			case "wrong-target":
				tail.ImplementationWorkContent.Snapshots[0].TargetRef = "foreign"
			case "wrong-hash":
				tail.ImplementationWorkContent.Snapshots[0].ContentHash = "foreign"
			case "changed-source":
				p.PromptContext.Sections[3].Content = strings.ReplaceAll(p.PromptContext.Sections[3].Content, "完整输入", "篡改输入")
			case "work-cycle":
				tail.WorkFileRefs[0].Source = "C4.implementationWorkContent.snapshots[0].Content"
			case "review-cycle":
				tail.ChangeReviewFileRefs[0].Source = "C4.changeReview.current[0].content"
			case "duplicate-work":
				tail.WorkFileRefs = append(tail.WorkFileRefs, tail.WorkFileRefs[0])
			case "duplicate-review":
				tail.ChangeReviewFileRefs = append(tail.ChangeReviewFileRefs, tail.ChangeReviewFileRefs[0])
			case "mixed-work":
				tail.ImplementationWorkContent.Snapshots[0].Content = "also inline"
			case "lost-work-ref":
				tail.WorkFileRefs, tail.ReferenceRule = nil, ""
			case "lost-review-ref":
				tail.ChangeReviewFileRefs = nil
			case "lost-value-ref":
				tail.ChangeReviewValueRefs = nil
			case "foreign-source":
				tail.WorkFileRefs[0].Source = "other-workspace/C3/input"
			case "unknown-rule":
				tail.ReferenceRule = "unrecognized"
			}
			raw, _ := json.Marshal(tail)
			p.PromptContext.Sections[4].Content = string(raw)
			if validateRuntimeCandidateContextIntegrity(r.DefinitionContext, r.CandidateContext, r.CandidateManifest, 0, nil, p.ContextBudget, p.PromptContext) == nil {
				t.Fatal("corrupt encoding admitted", name)
			}
		})
	}
}

func TestCandidateRevisionSourcesRejectRehashedOrForeignOriginals(t *testing.T) {
	r, p, _ := candidateBodyReferenceFixture(t, false)
	if err := validateRuntimeCandidateContextIntegrity(r.DefinitionContext, r.CandidateContext, r.CandidateManifest, 0, nil, p.ContextBudget, p.PromptContext); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"body-and-hash", "report", "recipe", "logical", "scope", "owner", "unexpected"} {
		t.Run(name, func(t *testing.T) {
			var authority struct {
				Output runtimeCandidateOutputContractProjection `json:"outputContract"`
			}
			json.Unmarshal([]byte(p.PromptContext.Sections[3].Content), &authority)
			v := authority.Output.OperationContracts[0].InputRevision
			switch name {
			case "body-and-hash":
				v.Binding.InputReview.Files[0].Content += "new"
				v.Binding.InputReview.Files[0].ContentHash = rc.RuntimeValidationInputContentHash(v.Binding.InputReview.Files[0].Content)
			case "report":
				v.Report = &rc.RuntimeValidationInputReview{Checks: []rc.RuntimeValidationInputCheckReview{{Reason: "changed semantic finding"}}}
			case "recipe":
				v.Recipe = &rc.RuntimeValidationRecipe{TimeoutMillis: 600000}
			case "logical":
				v.SourceLogicalRequestID = "foreign-review"
			case "scope":
				v.InputTargetRefs = []string{"another-input"}
			case "owner":
				authority.Output.OperationContracts[0].OwnerModuleID = "foreign-owner"
			case "unexpected":
				authority.Output.OperationContracts = append(authority.Output.OperationContracts, authority.Output.OperationContracts[0])
			}
			if runtimeCandidateInputRevisionSourcesMatch(r.CandidateManifest, authority.Output.OperationContracts) {
				t.Fatal("rendered original replaced sealed source", name)
			}
		})
	}
}

func TestCandidateLocalReferencesFreezeCorrectionAndRecovery(t *testing.T) {
	r, p, _ := candidateBodyReferenceFixture(t, false)
	r.StablePrefixHash = rc.StableRuntimeCandidatePrefixHashWithExecutionContract(r.DefinitionContext, r.CandidateContext.Activation.ExecutionContract)
	e := rc.RuntimeExpectedDocumentContract{SchemaVersion: rc.RuntimeExpectedDocumentSchemaCurrent, DocumentKind: r.DocumentKind, OperationsContextEncoding: rc.RuntimeOperationsContextEncoding, ManifestHash: r.CandidateManifest.Hash, WireContractHash: "sealed-wire", LogicalRequestID: "request", StablePrefixHash: r.StablePrefixHash, MaxOutputTokens: r.CandidateContext.Resources.ProviderCeiling.EffectiveMaxOutputTokens, MaxRecoveryAttempts: 1}
	e.Hash = rc.StableRuntimeExpectedDocumentHash(e)
	r.ExpectedDocument, r.RequestIdentity.LogicalRequestID = &e, e.LogicalRequestID
	small := r
	ceiling := *r.CandidateContext.Resources.ProviderCeiling
	ceiling.MaxBytes = 1000
	small.CandidateContext.Resources.ProviderCeiling = &ceiling
	if _, _, err := freezeRuntimeProviderInput(small, p); err == nil || !strings.Contains(err.Error(), "exceeds sealed ceiling") {
		t.Fatal("genuine capacity failure was relaxed", err)
	}
	raw, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = raw, ref
	first, err := runtimeProviderRequestForSubmission(r)
	if err != nil {
		t.Fatal(err)
	}
	stale := r
	newContract := e.Clone()
	newContract.ManifestHash = "new-revision-manifest"
	newContract.Hash = rc.StableRuntimeExpectedDocumentHash(newContract)
	stale.ExpectedDocument = &newContract
	if _, err := runtimeProviderRequestForSubmission(stale); err == nil {
		t.Fatal("old frozen bytes accepted for a new manifest revision")
	}
	// Recovery consumes only the original frozen wire; current observations
	// cannot replace its source, despite a matching path in the new workspace.
	for _, foreign := range []bool{false, true} {
		current := r
		current.CandidateContext.ChangeReview = r.CandidateContext.ChangeReview.Clone()
		current.CandidateContext.ChangeReview.Current[0].Content = "new observation"
		if foreign {
			current.CandidateContext.WorkspaceID = "foreign"
		}
		restored, err := runtimeProviderRequestForSubmission(current)
		if foreign {
			if err == nil {
				t.Fatal("foreign frozen request admitted")
			}
		} else if err != nil || !bytes.Equal(responsesWireInputForRequest(&restored), responsesWireInputForRequest(&first)) {
			t.Fatal("recovery re-rendered body references", err)
		}
	}
	r.BaseFrozenInput, r.BaseFrozenInputRef = raw, ref
	e = e.Clone()
	e.CorrectionAttempt = 1
	e.LogicalRequestID = "correction"
	e.Hash = rc.StableRuntimeExpectedDocumentHash(e)
	r.ExpectedDocument, r.RequestIdentity.LogicalRequestID = &e, e.LogicalRequestID
	// Correcting C4 composes with the actual old C3. A new source layout must
	// not silently replace that prefix, and refs into it must still resolve.
	p.PromptContext.Sections[3].Content = `{}`
	corrected, correctedRef, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = corrected, correctedRef
	restored, err := runtimeProviderRequestForSubmission(r)
	if err != nil || restored.PromptContext.Sections[3].Content != first.PromptContext.Sections[3].Content || restored.PromptContext.Sections[4].Content != first.PromptContext.Sections[4].Content {
		t.Fatal("correction did not preserve local source composition", err)
	}
	var broken runtimeCandidateExecutionTailProjection
	json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &broken)
	broken.WorkFileRefs[0].Source = "C3.outputContract.operationContracts[1].inputRevision.Binding.InputReview.Files[0].content"
	bad, _ := json.Marshal(broken)
	p.PromptContext.Sections[4].Content = string(bad)
	if _, _, err := freezeRuntimeProviderInput(r, p); err == nil {
		t.Fatal("correction reference into fresh/foreign source admitted")
	}
}
