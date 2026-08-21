package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func runtimeDefinitionGapDocumentForTest(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeExpectedDocumentContract, RuntimeDefinitionGapProviderDocument) {
	t.Helper()
	manifest := semanticContractTestManifest(t)
	expected := RuntimeExpectedDocumentContract{
		SchemaVersion:       RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:        RuntimeProviderDocumentSemanticPlan,
		PlanStep:            RuntimeSemanticPlanStepCoverageOrder,
		ManifestHash:        manifest.Hash,
		DefinitionSeedHash:  manifest.DefinitionSeedHash,
		LogicalRequestID:    "semantic-request-gap",
		StablePrefixHash:    "stable-prefix-gap",
		MaxOutputTokens:     4096,
		MaxRecoveryAttempts: 1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	document := RuntimeDefinitionGapProviderDocument{
		DocumentKind:       RuntimeProviderDocumentSemanticPlan,
		SchemaVersion:      RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:       manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash,
		ResultKind:         RuntimeDefinitionGapResultKind,
		PlanStep:           RuntimeSemanticPlanStepCoverageOrder,
		GapKind:            RuntimeDefinitionGapImplicitOneOf,
		OffendingItemRefs:  []string{string(manifest.CoverageItems[0].ID)},
		DiagnosticMessage:  "The sealed item implies alternatives and cannot be represented as one complete all-of proof.",
	}
	if err := document.ValidateAgainst(manifest, expected); err != nil {
		t.Fatalf("Definition gap fixture: %v", err)
	}
	return manifest, expected, document
}

func TestRuntimeDefinitionGapDecoderIsManifestBoundAndStrict(t *testing.T) {
	manifest, expected, document := runtimeDefinitionGapDocumentForTest(t)
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeDefinitionGapPayloadWithManifest(manifest, expected, payload)
	if err != nil {
		t.Fatalf("decode Definition gap: %v", err)
	}
	if decoded.GapKind != RuntimeDefinitionGapImplicitOneOf || len(decoded.OffendingItemRefs) != 1 || !RuntimeSemanticPayloadIsDefinitionGap(payload) {
		t.Fatalf("decoded Definition gap = %+v", decoded)
	}

	unknownField := append(payload[:len(payload)-1], []byte(`,"ticketId":"provider-ticket"}`)...)
	if _, err := DecodeRuntimeDefinitionGapPayloadWithManifest(manifest, expected, unknownField); err == nil {
		t.Fatal("provider-authored ticket identity was accepted")
	}
	outside := document.Clone()
	outside.OffendingItemRefs = []string{"not-in-current-manifest"}
	outsidePayload, _ := json.Marshal(outside)
	if _, err := DecodeRuntimeDefinitionGapPayloadWithManifest(manifest, expected, outsidePayload); err == nil {
		t.Fatal("offending ref outside the sealed manifest was accepted")
	}
	overBudget := document.Clone()
	overBudget.DiagnosticMessage = strings.Repeat("x", RuntimeDefinitionClarificationMaxDiagnosticBytes+1)
	overBudgetPayload, _ := json.Marshal(overBudget)
	if _, err := DecodeRuntimeDefinitionGapPayloadWithManifest(manifest, expected, overBudgetPayload); err == nil {
		t.Fatal("over-budget diagnostic was accepted")
	}
}

func TestRuntimeDefinitionClarificationIdentityIsStableAndBounded(t *testing.T) {
	_, _, document := runtimeDefinitionGapDocumentForTest(t)
	diagnostic := RuntimeOriginalDiagnosticFromDefinitionGap(document)
	observation := RuntimeDefinitionGapObservation{
		SchemaVersion: RuntimeDefinitionClarificationSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", StartupInputHash: "startup-1",
		AuthoritySnapshotID: "authority-1", FocusRef: "focus-1", LogicalRequestID: "semantic-request-1",
		SemanticDocumentHash: "semantic-document-1", GapKind: document.GapKind,
		OffendingItemRefs: append([]string(nil), document.OffendingItemRefs...), OriginalDiagnostic: diagnostic,
	}
	observation.ObservationID = StableRuntimeDefinitionGapObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatalf("validate observation: %v", err)
	}
	dispatch, err := CompileRuntimeDefinitionClarificationDispatch(observation, "clarification-authority-1")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := CompileRuntimeDefinitionClarificationDispatch(observation.Clone(), "clarification-authority-1")
	if err != nil || duplicate.DispatchID != dispatch.DispatchID || duplicate.CanonicalGapSignatureHash != dispatch.CanonicalGapSignatureHash {
		t.Fatalf("stable dispatch mismatch: first=%+v duplicate=%+v err=%v", dispatch, duplicate, err)
	}
	drift := observation.Clone()
	drift.SemanticDocumentHash = "semantic-document-drift"
	drift.ObservationID = StableRuntimeDefinitionGapObservationID(drift)
	if err := dispatch.ValidateAgainst(drift); err == nil {
		t.Fatal("dispatch accepted a different persisted semantic document")
	}
}
