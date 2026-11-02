package llm

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func frozenProviderFixture(t *testing.T) (models.RuntimeCandidateRequest, models.ProviderRequest) {
	t.Helper()
	r := runtimeSemanticWorkProviderRequestForTest(t, true)
	logical := r.RequestIdentity.LogicalRequestID
	if logical == "" {
		logical = "frozen-logical"
		r.RequestIdentity.LogicalRequestID = logical
	}
	e := rc.RuntimeExpectedDocumentContract{SchemaVersion: rc.RuntimeExpectedDocumentSchemaCurrent, DocumentKind: r.DocumentKind, PlanStep: r.SemanticContext.PlanStep, SemanticContextEncoding: rc.RuntimeSemanticContextEncoding, ManifestHash: r.SemanticManifest.Hash, DefinitionSeedHash: r.SemanticManifest.DefinitionSeedHash, LogicalRequestID: logical, StablePrefixHash: r.StablePrefixHash, MaxOutputTokens: r.SemanticContext.ProviderCeiling.EffectiveMaxOutputTokens, MaxRecoveryAttempts: 1}
	e.Hash = rc.StableRuntimeExpectedDocumentHash(e)
	r.ExpectedDocument = &e
	p, err := runtimeProviderRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	return r, p
}
func TestFrozenProviderInputRestoresExactMessagesWithoutRerender(t *testing.T) {
	r, p := frozenProviderFixture(t)
	raw, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput = raw
	r.FrozenInputRef = ref
	original, _ := json.Marshal(sectionMessages(p, false))
	// A new mutable observation must not enter an existing request, even if a
	// current renderer would correctly reject its incomplete metadata.
	r.SemanticContext.WorkContext.Content.Snapshots[0].Content = "different mutable bytes"
	replay, err := runtimeProviderRequestForSubmission(r)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ContextBudget.CachePlan.StablePrefixHash != r.StablePrefixHash || replay.ContextBudget.CachePlan.RenderedStablePrefixHash != p.ContextBudget.CachePlan.RenderedStablePrefixHash {
		t.Fatal("freeze changed Runtime commitment or rendered prefix audit identity")
	}
	if replay.Model != r.SemanticContext.ProviderCeiling.Selection.Target.ModelID || replay.MaxOutputTokens != r.ExpectedDocument.MaxOutputTokens {
		t.Fatal("freeze did not bind actual provider model/output ceiling")
	}
	if !bytes.Equal(original, responsesWireInputForRequest(&replay)) {
		t.Fatal("restart changed exact provider message bytes")
	}
	replay.PromptContext.Sections[0].Content = "caller mutation"
	second, err := runtimeProviderRequestForSubmission(r)
	if err != nil || second.PromptContext.Sections[0].Content == "caller mutation" {
		t.Fatal("frozen source aliases return values", err)
	}
	for _, kind := range []string{"missing", "corrupt", "size", "workspace", "logical", "expected", "ceiling"} {
		t.Run(kind, func(t *testing.T) {
			bad := r
			bad.SemanticContext = r.SemanticContext.Clone()
			bad.FrozenInputRef = ref.Clone()
			switch kind {
			case "missing":
				bad.FrozenInput = nil
			case "corrupt":
				bad.FrozenInput = append(append([]byte(nil), raw...), byte(' '))
			case "size":
				bad.FrozenInputRef.Bytes++
			case "workspace":
				bad.SemanticContext.WorkspaceID = "foreign"
			case "logical":
				bad.RequestIdentity.LogicalRequestID = "foreign"
			case "expected":
				e := bad.ExpectedDocument.Clone()
				e.Hash = "foreign"
				bad.ExpectedDocument = &e
			case "ceiling":
				bad.SemanticContext.ProviderCeiling.MaxBytes++
			}
			if _, err := runtimeProviderRequestForSubmission(bad); err == nil {
				t.Fatal("missing/foreign frozen input accepted")
			}
		})
	}
}
func TestFrozenCorrectionPreservesBasePrefixWireAndSchema(t *testing.T) {
	r, p := frozenProviderFixture(t)
	raw, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.BaseFrozenInput = raw
	r.BaseFrozenInputRef = ref
	e := r.ExpectedDocument.Clone()
	e.CorrectionAttempt = 1
	e.LogicalRequestID = "correction"
	e.Hash = rc.StableRuntimeExpectedDocumentHash(e)
	r.ExpectedDocument = &e
	r.RequestIdentity.LogicalRequestID = e.LogicalRequestID
	// Simulate changed built-in rendering between the original submission and
	// a permitted correction. Only the new C4/C5 input is selected for correction.
	p.PromptContext.Sections[0].Content = "new built-in prose"
	p.PromptContext.Sections[3].Content = "new layout"
	p.PromptContext.Sections[4].Content = `{"correction":"exact original finding"}`
	p.Schema = `{"changed":true}`
	corrected, correctedRef, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	r.FrozenInput, r.FrozenInputRef = corrected, correctedRef
	replay, err := runtimeProviderRequestForSubmission(r)
	if err != nil {
		t.Fatal(err)
	}
	var base runtimeFrozenProviderInput
	json.Unmarshal(raw, &base)
	if replay.Schema != base.Request.Schema || replay.FrozenCacheKey != base.CacheKey {
		t.Fatal("correction changed sealed schema/routing identity")
	}
	var actual, prior []responsesInputMessage
	json.Unmarshal(replay.FrozenSectionMessages, &actual)
	json.Unmarshal(base.Messages, &prior)
	a, _ := json.Marshal(actual[:4])
	b, _ := json.Marshal(prior[:4])
	if !bytes.Equal(a, b) {
		t.Fatal("correction changed C0-C3 physical bytes")
	}
	if !strings.Contains(actual[4].Content[0].Text, "exact original finding") {
		t.Fatal("correction input lost")
	}
}

func TestFrozenInputRejectsForeignCorrectionBaseAndChangedWire(t *testing.T) {
	r, p := frozenProviderFixture(t)
	raw, ref, err := freezeRuntimeProviderInput(r, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"workspace", "manifest", "batch", "ceiling", "attempt"} {
		t.Run("correction/"+field, func(t *testing.T) {
			bad := r
			e := r.ExpectedDocument.Clone()
			e.CorrectionAttempt = 1
			e.LogicalRequestID = "correction"
			e.Hash = rc.StableRuntimeExpectedDocumentHash(e)
			bad.ExpectedDocument = &e
			bad.RequestIdentity.LogicalRequestID = e.LogicalRequestID
			bad.BaseFrozenInput = raw
			bad.BaseFrozenInputRef = ref
			var base runtimeFrozenProviderInput
			json.Unmarshal(raw, &base)
			switch field {
			case "workspace":
				base.Request.WorkspaceID = "foreign"
			case "manifest":
				base.ManifestHash = "foreign"
			case "batch":
				base.Expected.BatchCoverageWorkItemIDs = append(base.Expected.BatchCoverageWorkItemIDs, "foreign")
			case "ceiling":
				base.Ceiling.MaxBytes++
			case "attempt":
				base.Expected.CorrectionAttempt = 1
			}
			bad.BaseFrozenInput, _ = json.Marshal(base)
			bad.BaseFrozenInputRef = &rc.RuntimeProviderInputRef{Bytes: len(bad.BaseFrozenInput), Hash: rc.RuntimeProviderInputHash(bad.BaseFrozenInput)}
			if _, _, err := freezeRuntimeProviderInput(bad, p); err == nil {
				t.Fatal("foreign correction base accepted")
			}
		})
	}
	for _, field := range []string{"text", "role", "block", "unknown", "trailing", "expected"} {
		t.Run("wire/"+field, func(t *testing.T) {
			bad := r
			var f runtimeFrozenProviderInput
			json.Unmarshal(raw, &f)
			var messages []responsesInputMessage
			json.Unmarshal(f.Messages, &messages)
			switch field {
			case "text":
				messages[0].Content[0].Text += "changed"
			case "role":
				messages[0].Role = "user"
			case "block":
				messages[0].Content[0].Type = "unknown"
			case "expected":
				f.Expected.MaxOutputTokens++
			}
			f.Messages, _ = json.Marshal(messages)
			bad.FrozenInput, _ = json.Marshal(f)
			if field == "unknown" {
				bad.FrozenInput = append(bad.FrozenInput[:len(bad.FrozenInput)-1], []byte(`,"unrecognized":true}`)...)
			}
			if field == "trailing" {
				bad.FrozenInput = append(bad.FrozenInput, []byte(` {}`)...)
			}
			bad.FrozenInputRef = &rc.RuntimeProviderInputRef{Bytes: len(bad.FrozenInput), Hash: rc.RuntimeProviderInputHash(bad.FrozenInput)}
			if _, err := runtimeProviderRequestForSubmission(bad); err == nil {
				t.Fatal("changed frozen wire accepted")
			}
		})
	}
}
