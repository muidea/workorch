package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticSharedWorkReferencesExpandExactOriginalAndRejectTampering(t *testing.T) {
	target := rc.RuntimeSemanticWorkTarget{OperationID: "op-1", TargetRefs: []string{"a"}, ReceiptRef: "receipt-1", ReceiptHash: "receipt-hash", BeforeHash: "before", AfterHash: "after", Changed: true}
	h := rc.RuntimeSemanticExecutionHistory{Receipts: []rc.RuntimeSemanticReceiptObservation{{ReceiptSequence: 4, DispatchID: "dispatch", OperationID: target.OperationID, TargetRefs: target.TargetRefs, ReceiptRef: target.ReceiptRef, ReceiptHash: target.ReceiptHash, BeforeHash: target.BeforeHash, AfterHash: target.AfterHash, Changed: true}}}
	c := &rc.RuntimeSemanticWorkContext{PriorRounds: []rc.RuntimeSemanticWorkRound{{Ordinal: 1, DispatchID: "dispatch", ReceiptSetHash: "set", Targets: []rc.RuntimeSemanticWorkTarget{target, {OperationID: "not-in-suffix", TargetRefs: []string{"b"}, ReceiptRef: "other", ReceiptHash: "other-hash"}}}}}
	v := compactSemanticWorkContext(c, h)
	if v.PriorRounds[0].Targets[0].ReceiptSequence != 4 || v.PriorRounds[0].Targets[1].Value == nil {
		t.Fatal("lost exact reference or old inline receipt")
	}
	raw, _ := json.Marshal(v)
	var loaded runtimeSemanticWorkProjection
	json.Unmarshal(raw, &loaded)
	expanded, err := expandSemanticWorkContext(&loaded, h)
	if err != nil || !reflect.DeepEqual(c, expanded) {
		t.Fatal("lossless restart failed", err)
	}
	expanded.PriorRounds[0].Targets[0].TargetRefs[0] = "mutated"
	if h.Receipts[0].TargetRefs[0] != "a" {
		t.Fatal("expanded request aliases source")
	}
	for _, kind := range []string{"missing", "duplicate", "foreign-dispatch", "mixed", "bad-index"} {
		t.Run(kind, func(t *testing.T) {
			json.Unmarshal(raw, &loaded)
			history := h.Clone()
			switch kind {
			case "missing":
				history.Receipts = nil
			case "duplicate":
				history.Receipts = append(history.Receipts, history.Receipts[0])
			case "foreign-dispatch":
				history.Receipts[0].DispatchID = "another"
			case "mixed":
				loaded.PriorRounds[0].Targets[0].Value = &target
			case "bad-index":
				loaded.PriorRounds[0].Targets[0].ReceiptSequence = -1
			}
			if _, err := expandSemanticWorkContext(&loaded, history); err == nil {
				t.Fatal("bad local ref accepted")
			}
		})
	}
	changed := h.Clone()
	changed.Receipts[0].AfterHash = "different"
	if compactSemanticWorkContext(c, changed).PriorRounds[0].Targets[0].Value == nil {
		t.Fatal("hash/identity equality substituted for exact receipt fields")
	}
}

func TestSemanticSharedWorkV5IntegrityAndStagePrefix(t *testing.T) {
	request := runtimeSemanticWorkProviderRequestForTest(t, true)
	w := request.SemanticManifest.ProofObligations[0].ImplementationWork
	body := strings.Repeat("完整当前业务正文\n", 300)
	hash := sha256.Sum256([]byte(body))
	w.Scope.Content = &rc.RuntimeImplementationWorkContent{ProtocolRef: rc.RuntimeImplementationWorkContentV1, ManifestRevision: w.Scope.Selection.ObservedRevision, Snapshots: []rc.TargetSnapshot{{TargetRef: w.Scope.Selection.TargetRefs[0], Content: body, ContentHash: hex.EncodeToString(hash[:]), ContentState: rc.SnapshotContentStateComplete, Size: int64(len(body))}}}
	request.SemanticManifest.Hash = rc.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	request.SemanticContext.WorkContext = &rc.RuntimeSemanticWorkContext{Content: w.Scope.Content.Clone()}
	request.SemanticContext.CoverageProgress.ManifestHash = request.SemanticManifest.Hash
	request.SemanticContext.CoverageProgress.Hash = rc.StableRuntimeSemanticCoverageProgressHash(request.SemanticContext.CoverageProgress)
	request.SemanticContext.ExecutionHistory.ManifestHash = request.SemanticManifest.Hash
	request.SemanticContext.ExecutionHistory.Hash = rc.StableRuntimeSemanticExecutionHistoryHash(request.SemanticContext.ExecutionHistory)
	request.SemanticContext.ExecutionBudget.ManifestHash = request.SemanticManifest.Hash
	request.SemanticContext.ExecutionBudget.Hash = rc.StableRuntimeSemanticExecutionBudgetHash(request.SemanticContext.ExecutionBudget)
	request.ExpectedDocument = &rc.RuntimeExpectedDocumentContract{SemanticContextEncoding: rc.RuntimeSemanticContextEncoding}
	legacy, err := prepareRuntimeSemanticPlanRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	v4, err := buildRuntimeSemanticPrompt(request, legacy)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedDocument.SemanticContextEncoding = rc.RuntimeSemanticContextEncoding
	prepared, err := prepareRuntimeSemanticPlanRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	v5, err := buildRuntimeSemanticPrompt(request, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if v4.definition != v5.definition || v4.capability != v5.capability || prepared.stablePrefix != legacy.stablePrefix {
		t.Fatal("stable source prefix changed")
	}
	if strings.Contains(v5.tail, "完整当前业务正文") || !strings.Contains(v5.authority, "完整当前业务正文") || !strings.Contains(v5.tail, `"workContextRef":"C3.workContext"`) {
		t.Fatal("current source omitted or duplicated")
	}
	provider := newRuntimeSemanticProviderRequest(request, prepared, v5)
	if err := validateRuntimeSemanticContextIntegrity(prepared.context, prepared.manifest, 0, nil, nil, provider.ContextBudget, provider.PromptContext); err != nil {
		t.Fatal(err)
	}
	// Same exact WorkContext precedes planStep, schema and manifest identity.
	other := prepared
	other.context = prepared.context.Clone()
	other.context.PlanStep = rc.RuntimeSemanticPlanStepProofBatch
	other.context.BatchID = "next-stage"
	other.context.BatchCoverageWorkItemIDs = nil // overwritten below by manifest IDs
	for _, item := range prepared.manifest.CoverageItems {
		other.context.BatchCoverageWorkItemIDs = append(other.context.BatchCoverageWorkItemIDs, item.ID)
	}
	next, err := buildRuntimeSemanticPrompt(request, other)
	if err != nil {
		t.Fatal(err)
	}
	lcp := 0
	for lcp < len(v5.authority) && lcp < len(next.authority) && v5.authority[lcp] == next.authority[lcp] {
		lcp++
	}
	if lcp < len(body) {
		t.Fatalf("common current source still follows volatile stage: prefix=%d body=%d", lcp, len(body))
	}
	t.Logf("v5 cross-stage exact common prefix bytes=%d", lcp)
	for _, kind := range []string{"wrong-ref", "dropped-content", "altered-content", "unsealed-version"} {
		t.Run(kind, func(t *testing.T) {
			p := newRuntimeSemanticProviderRequest(request, prepared, v5)
			for n := range p.PromptContext.Sections {
				s := &p.PromptContext.Sections[n]
				if strings.HasPrefix(s.ID, "C4.") && kind == "wrong-ref" {
					s.Content = strings.ReplaceAll(s.Content, semanticWorkContextRef, "another-context")
				}
				if strings.HasPrefix(s.ID, "C3.") && (kind == "dropped-content" || kind == "altered-content") {
					var a runtimeSemanticAuthorityProjection
					json.Unmarshal([]byte(s.Content), &a)
					if kind == "dropped-content" {
						a.WorkContext.Content = nil
					} else {
						a.WorkContext.Content.Snapshots[0].Content = "changed"
					}
					raw, _ := json.Marshal(a)
					s.Content = string(raw)
				}
			}
			if kind == "unsealed-version" {
				p.ContextBudget.RenderContractRef = "unknown"
			}
			if validateRuntimeSemanticContextIntegrity(prepared.context, prepared.manifest, 0, nil, nil, p.ContextBudget, p.PromptContext) == nil {
				t.Fatal("tampered context passed integrity")
			}
		})
	}
}

func TestSemanticSharedWorkConcurrentProjectIsolation(t *testing.T) {
	for _, project := range []string{"workspace-alpha/profile-a", "workspace-beta/profile-b"} {
		t.Run(project, func(t *testing.T) {
			t.Parallel()
			c := &rc.RuntimeSemanticWorkContext{Content: &rc.RuntimeImplementationWorkContent{ManifestRevision: project, Snapshots: []rc.TargetSnapshot{{TargetRef: "same-name", Content: project}}}}
			for n := 0; n < 30; n++ {
				v := compactSemanticWorkContext(c, rc.RuntimeSemanticExecutionHistory{})
				out, err := expandSemanticWorkContext(v, rc.RuntimeSemanticExecutionHistory{})
				if err != nil || !reflect.DeepEqual(c, out) {
					t.Fatal("cross-project contamination", err)
				}
			}
		})
	}
}
