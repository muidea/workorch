package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestChangeReviewLocalReferencesAreExactAndRejectMissingForeignAndDuplicateSources(t *testing.T) {
	body := "完整当前事实\r\n"
	f := rc.RuntimeValidationInputReviewFile{TargetRef: "resource", Content: body, ContentHash: rc.RuntimeValidationInputContentHash(body)}
	context := &rc.RuntimeChangeReviewContext{WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition", Current: []rc.RuntimeValidationInputReviewFile{f}}
	sources := []runtimeChangeReviewBodySource{{"C3.workContext.content.snapshots[0].Content", f}}
	for _, name := range []string{"exact", "missing", "duplicate", "foreign", "changed-body", "changed-target"} {
		t.Run(name, func(t *testing.T) {
			c := context.Clone()
			refs := compactChangeReviewFiles(c, sources)
			if len(refs) != 1 || c.Current[0].Content != "" || context.Current[0].Content != body {
				t.Fatal("not lossless/detached")
			}
			src := append([]runtimeChangeReviewBodySource(nil), sources...)
			switch name {
			case "missing":
				src = nil
			case "duplicate":
				refs = append(refs, refs[0])
			case "foreign":
				refs[0].Source = "another-request/source"
			case "changed-body":
				src[0].file.Content += "tampered"
			case "changed-target":
				src[0].file.TargetRef = "foreign"
			}
			err := expandChangeReviewFiles(c, refs, src)
			if name == "exact" {
				if err != nil || !reflect.DeepEqual(c, context) {
					t.Fatal("lossy reference", err)
				}
			} else if err == nil {
				t.Fatal("invalid reference reached provider")
			}
		})
	}
}

func TestChangeReviewRenderedIntegrityAndFrozenRecovery(t *testing.T) {
	r, base := frozenProviderFixture(t)
	c := &rc.RuntimeChangeReviewContext{WorkspaceID: r.SemanticContext.WorkspaceID, RunID: r.SemanticContext.RunID, DefinitionRevision: r.SemanticContext.Definition.Revision}
	for _, s := range r.SemanticContext.WorkContext.Content.Snapshots {
		c.Current = append(c.Current, rc.RuntimeValidationInputReviewFile{TargetRef: s.TargetRef, ContentHash: s.ContentHash, Content: s.Content})
	}
	preimage := rc.RuntimeValidationInputReviewFile{TargetRef: c.Current[0].TargetRef, Content: strings.Repeat("exact auxiliary preimage\r\n", 100)}
	preimage.ContentHash = rc.RuntimeValidationInputContentHash(preimage.Content)
	body := c.Current[0].Content
	op := rc.RuntimeOperation{ID: "auxiliary", Kind: rc.StepKindFileEdit, TargetRefs: []string{preimage.TargetRef}, Parameters: []rc.OperationParameter{{Name: "oldText", String: &preimage.Content}, {Name: "newText", String: &body}}}
	c.Changes = []rc.RuntimeChangeReviewOperation{{Operation: op, OperationHash: rc.StableRuntimeOperationHash(op), ExecutionKey: "original-key", Fence: 1, DispatchID: "original-dispatch", OperationID: op.ID, OwnerModuleID: rc.RuntimeModuleFile, ReceiptRef: "runtime-receipt/original-key", ReceiptHash: "original-receipt-hash", TargetRef: preimage.TargetRef, BeforeHash: preimage.ContentHash, AfterHash: c.Current[0].ContentHash, Baseline: &preimage}}
	r.SemanticContext.ChangeReview = c
	p, err := runtimeProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 2; i++ {
		if base.PromptContext.Sections[i].Content != p.PromptContext.Sections[i].Content {
			t.Fatalf("volatile change facts moved into C%d", i)
		}
	}
	metrics := runtimeProviderContextSectionMetrics(p.PromptContext.Sections[4], p.PromptContext)
	var expanded runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &expanded); err != nil {
		t.Fatal(err)
	}
	if err := expandChangeReviewFiles(expanded.ChangeReview, expanded.ChangeReviewFileRefs, changeReviewBodySources(r.SemanticContext.WorkContext.Content, "C3.workContext.content", nil, "")); err != nil {
		t.Fatal(err)
	}
	if err := expandChangeReviewValues(expanded.ChangeReview, expanded.ChangeReviewValueRefs); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expanded.ChangeReview, c) {
		t.Fatal("rendered projection lost original auxiliary preimage or operation")
	}
	refCount := len(expanded.ChangeReviewFileRefs) + len(expanded.ChangeReviewValueRefs)
	expanded.ChangeReviewFileRefs = nil
	expanded.ChangeReviewValueRefs = nil
	expandedJSON, _ := json.Marshal(expanded)
	if metrics.LocalReferences != refCount || metrics.BodyBytes != len(p.PromptContext.Sections[4].Content) || metrics.ExpandedBytes != len(expandedJSON) {
		t.Fatal("actual/expanded accounting is not exact", metrics)
	}
	var tail runtimeSemanticTailProjection
	if json.Unmarshal([]byte(p.PromptContext.Sections[4].Content), &tail) != nil || len(tail.ChangeReviewFileRefs) != len(c.Current) || len(tail.ChangeReviewValueRefs) == 0 {
		t.Fatal("body duplicated instead of exactly referenced")
	}
	for _, mutate := range []string{"omitted", "foreign", "changed", "lost-ref", "lost-value-ref", "foreign-value-ref", "mixed-value", "instruction"} {
		candidate := p
		prompt := *p.PromptContext
		prompt.Sections = append(prompt.Sections[:0:0], p.PromptContext.Sections...)
		candidate.PromptContext = &prompt
		var v runtimeSemanticTailProjection
		json.Unmarshal([]byte(prompt.Sections[4].Content), &v)
		switch mutate {
		case "omitted":
			v.ChangeReview = nil
		case "foreign":
			v.ChangeReview.WorkspaceID = "other-workspace"
		case "changed":
			v.ChangeReview.Current[0].ContentHash = "another-version"
		case "lost-ref":
			v.ChangeReviewFileRefs = nil
		case "lost-value-ref":
			v.ChangeReviewValueRefs = nil
		case "foreign-value-ref":
			v.ChangeReviewValueRefs[0].SourceIndex = len(v.ChangeReview.Changes)
		case "mixed-value":
			v.ChangeReview.Changes[0].Operation.Parameters[0].String = &body
		case "instruction":
			prompt.Sections[3].Content = strings.Replace(prompt.Sections[3].Content, "changeReview contains", "changeReview does not contain", 1)
		}
		if mutate != "instruction" {
			raw, _ := json.Marshal(v)
			prompt.Sections[4].Content = string(raw)
		}
		if validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, candidate.ContextBudget, candidate.PromptContext) == nil {
			t.Fatal("corrupt source/instruction admitted", mutate)
		}
	}
	frozen, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = frozen, ref
	original, _ := json.Marshal(sectionMessages(p, false))
	r.SemanticContext.ChangeReview.Current[0].Content = "new current fact must wait for a new request"
	r.SemanticContext.ChangeReview.Changes[0].Baseline.Content = "new preimage must wait for a new request"
	replay, err := runtimeProviderRequestForSubmission(r)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := json.Marshal(sectionMessages(replay, false))
	if !bytes.Equal(original, actual) {
		t.Fatal("recovery rewrote frozen source or requested fresh model sampling")
	}
}
