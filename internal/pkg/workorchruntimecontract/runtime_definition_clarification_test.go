package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
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

func runtimeDefinitionGapPrerequisiteClaimForTest(coverageID common.CoverageWorkItemID) RuntimeDefinitionGapPrerequisiteClaim {
	return RuntimeDefinitionGapPrerequisiteClaim{
		CoverageWorkItemID: coverageID,
		OwnerModuleID:      RuntimeModuleShell,
		FactKind:           RuntimeOwnerFactSelectableValidationSurface,
		OperationKind:      RuntimeBlockKindManagedValidationSession,
	}
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

	undeclared := document.Clone()
	undeclared.GapKind = RuntimeDefinitionGapUndeclaredPrerequisite
	undeclared.DiagnosticMessage = "The sealed item omits one exact prerequisite."
	missingClaimPayload, _ := json.Marshal(undeclared)
	var missingClaimDiagnostic *RuntimeCandidatePayloadDiagnostic
	if _, err := DecodeRuntimeDefinitionGapPayloadWithManifest(manifest, expected, missingClaimPayload); !errors.As(err, &missingClaimDiagnostic) ||
		!sameStrings(missingClaimDiagnostic.Required, []string{"prerequisiteClaims"}) {
		t.Fatalf("missing prerequisite claim did not produce an exact required-field diagnostic: diagnostic=%+v err=%v", missingClaimDiagnostic, err)
	}
	undeclared.PrerequisiteClaims = []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest(manifest.CoverageItems[0].ID)}
	undeclaredPayload, _ := json.Marshal(undeclared)
	var nestedUnknown map[string]any
	if err := json.Unmarshal(undeclaredPayload, &nestedUnknown); err != nil {
		t.Fatal(err)
	}
	claims := nestedUnknown["prerequisiteClaims"].([]any)
	claims[0].(map[string]any)["transition"] = "need_governance"
	nestedUnknownPayload, _ := json.Marshal(nestedUnknown)
	if _, err := DecodeRuntimeDefinitionGapPayloadWithManifest(manifest, expected, nestedUnknownPayload); err == nil {
		t.Fatal("provider-authored transition inside a prerequisite claim was accepted")
	}
}

func TestEvaluateRuntimeDefinitionGapAdmissionAcceptsManifestBoundGapWithoutDeclaredAcquisition(t *testing.T) {
	manifest, expected, document := runtimeDefinitionGapDocumentForTest(t)
	document.GapKind = RuntimeDefinitionGapUndeclaredPrerequisite
	document.PrerequisiteClaims = []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest(manifest.CoverageItems[0].ID)}
	document.DiagnosticMessage = "The sealed item omits a prerequisite needed to form its proof."
	decision := EvaluateRuntimeDefinitionGapAdmission(manifest, expected, document)
	if !decision.Admitted() || decision.Code != "gap_admitted" {
		t.Fatalf("Definition gap admission = %+v", decision)
	}
}

func TestEvaluateRuntimeDefinitionGapAdmissionRejectsUndeclaredPrerequisiteWithSealedOwnerFactAcquisition(t *testing.T) {
	manifest, semanticDocument, _ := semanticMissingOwnerFactAcquisitionFixture(t)
	expected := RuntimeExpectedDocumentContract{
		SchemaVersion:            RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:             RuntimeProviderDocumentSemanticPlan,
		PlanStep:                 RuntimeSemanticPlanStepProofBatch,
		ManifestHash:             manifest.Hash,
		DefinitionSeedHash:       manifest.DefinitionSeedHash,
		LogicalRequestID:         "semantic-request-owner-fact-gap",
		StablePrefixHash:         "stable-prefix-owner-fact-gap",
		BatchID:                  semanticDocument.BatchID,
		BatchSequence:            semanticDocument.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), semanticDocument.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        "coverage-order-owner-fact-gap",
		MaxOutputTokens:          4096,
		MaxRecoveryAttempts:      1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	document := RuntimeDefinitionGapProviderDocument{
		DocumentKind:             RuntimeProviderDocumentSemanticPlan,
		SchemaVersion:            RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:             manifest.Hash,
		DefinitionSeedHash:       manifest.DefinitionSeedHash,
		ResultKind:               RuntimeDefinitionGapResultKind,
		PlanStep:                 RuntimeSemanticPlanStepProofBatch,
		BatchID:                  expected.BatchID,
		BatchSequence:            &expected.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), expected.BatchCoverageWorkItemIDs...),
		GapKind:                  RuntimeDefinitionGapUndeclaredPrerequisite,
		OffendingItemRefs:        []string{string(expected.BatchCoverageWorkItemIDs[0])},
		PrerequisiteClaims:       []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest(expected.BatchCoverageWorkItemIDs[0])},
		DiagnosticMessage:        "No selectable validation surface is currently published.",
	}
	if err := document.ValidateAgainst(manifest, expected); err != nil {
		t.Fatalf("structurally valid gap fixture: %v", err)
	}
	decision := EvaluateRuntimeDefinitionGapAdmission(manifest, expected, document)
	if decision.Disposition != RuntimeDefinitionGapAdmissionRejected || decision.Code != "owner_fact_acquisition_declared" {
		t.Fatalf("false undeclared-prerequisite admission = %+v", decision)
	}
}

func TestEvaluateRuntimeDefinitionGapAdmissionRejectsSatisfiedDeclaredPrerequisite(t *testing.T) {
	manifest, semanticDocument, _, source := semanticIndependentProofSetFixture(t)
	requirement := runtimeDefinitionGapPrerequisiteClaimForTest("cwi-001").Requirement()
	source.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{requirement}
	source.OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: requirement, RequirePreexisting: true}}
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	expected := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentSemanticPlan,
		PlanStep: RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		LogicalRequestID: "semantic-request-satisfied-owner-fact-gap", StablePrefixHash: "stable-prefix-satisfied-owner-fact-gap",
		BatchID: semanticDocument.BatchID, BatchSequence: semanticDocument.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), semanticDocument.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        "coverage-order-satisfied-owner-fact-gap", MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	document := RuntimeDefinitionGapProviderDocument{
		DocumentKind: RuntimeProviderDocumentSemanticPlan, SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, ResultKind: RuntimeDefinitionGapResultKind,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: expected.BatchID, BatchSequence: &expected.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), expected.BatchCoverageWorkItemIDs...),
		GapKind:                  RuntimeDefinitionGapUndeclaredPrerequisite, OffendingItemRefs: []string{"cwi-001"},
		PrerequisiteClaims: []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest("cwi-001")},
		DiagnosticMessage:  "No selectable validation surface is currently published.",
	}
	decision := EvaluateRuntimeDefinitionGapAdmission(manifest, expected, document)
	if decision.Disposition != RuntimeDefinitionGapAdmissionRejected || decision.Code != "owner_fact_requirement_satisfied" {
		t.Fatalf("satisfied prerequisite gap admission = %+v", decision)
	}
}

func TestRuntimeDefinitionGapUndeclaredPrerequisiteRequiresExactTypedClaims(t *testing.T) {
	manifest, expected, document := runtimeDefinitionGapDocumentForTest(t)
	document.GapKind = RuntimeDefinitionGapUndeclaredPrerequisite
	document.DiagnosticMessage = "The sealed item omits a prerequisite needed to form its proof."
	if err := document.ValidateAgainst(manifest, expected); err == nil || !strings.Contains(err.Error(), "prerequisite claims") {
		t.Fatalf("undeclared prerequisite without typed claim was accepted: %v", err)
	}

	document.PrerequisiteClaims = []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest(manifest.CoverageItems[0].ID)}
	duplicate := document.Clone()
	duplicate.PrerequisiteClaims = append(duplicate.PrerequisiteClaims, duplicate.PrerequisiteClaims[0])
	if err := duplicate.ValidateAgainst(manifest, expected); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate prerequisite claim was accepted: %v", err)
	}

	nonPrerequisite := document.Clone()
	nonPrerequisite.GapKind = RuntimeDefinitionGapSemanticAmbiguity
	if err := nonPrerequisite.ValidateAgainst(manifest, expected); err == nil || !strings.Contains(err.Error(), "cannot carry prerequisite claims") {
		t.Fatalf("non-prerequisite gap carried typed claims: %v", err)
	}
}

func TestRuntimeDefinitionGapSourceRefClaimsCoverEveryDerivedCWI(t *testing.T) {
	manifest, expected, document := runtimeDefinitionGapDocumentForTest(t)
	sharedSourceID := manifest.CoverageItems[0].SourceID
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{
		ID: "cwi-002", SliceID: "slice-2", SourceScope: manifest.CoverageItems[0].SourceScope,
		SourceID: sharedSourceID, TargetText: "second proof", Order: 2,
	})
	manifest = sealSemanticTestManifest(t, manifest)
	expected.ManifestHash = manifest.Hash
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	document.ManifestHash = manifest.Hash
	document.GapKind = RuntimeDefinitionGapUndeclaredPrerequisite
	document.OffendingItemRefs = []string{sharedSourceID}
	document.PrerequisiteClaims = []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest("cwi-001")}
	document.DiagnosticMessage = "The shared source omits an exact prerequisite."
	if err := document.ValidateAgainst(manifest, expected); err == nil || !strings.Contains(err.Error(), `coverage "cwi-002" has no exact prerequisite claim`) {
		t.Fatalf("shared source ref did not require claims for every derived CWI: %v", err)
	}
	document.PrerequisiteClaims = append(document.PrerequisiteClaims, runtimeDefinitionGapPrerequisiteClaimForTest("cwi-002"))
	if err := document.ValidateAgainst(manifest, expected); err != nil {
		t.Fatalf("complete shared-source prerequisite claims were rejected: %v", err)
	}
	batchExpected := expected.Clone()
	batchExpected.PlanStep = RuntimeSemanticPlanStepProofBatch
	batchExpected.BatchID = "batch-shared-source"
	batchExpected.BatchSequence = 0
	batchExpected.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-001"}
	batchExpected.CoverageOrderHash = "coverage-order-shared-source"
	batchExpected.Hash = StableRuntimeExpectedDocumentHash(batchExpected)
	batchDocument := document.Clone()
	batchDocument.PlanStep = RuntimeSemanticPlanStepProofBatch
	batchDocument.BatchID = batchExpected.BatchID
	batchDocument.BatchSequence = &batchExpected.BatchSequence
	batchDocument.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batchExpected.BatchCoverageWorkItemIDs...)
	if err := batchDocument.ValidateAgainst(manifest, batchExpected); err == nil || !strings.Contains(err.Error(), `coverage "cwi-002" is outside the sealed batch`) {
		t.Fatalf("shared source ref escaped proof-batch scope: %v", err)
	}
	schema, err := RuntimeSemanticPlanJSONSchemaForRequest(manifest, RuntimeSemanticPlanStepProofBatch, batchExpected.BatchID, batchExpected.BatchSequence, batchExpected.BatchCoverageWorkItemIDs)
	if err != nil {
		t.Fatal(err)
	}
	var decodedSchema map[string]any
	if err := json.Unmarshal([]byte(schema), &decodedSchema); err != nil {
		t.Fatal(err)
	}
	refEnum := decodedSchema["properties"].(map[string]any)["offendingItemRefs"].(map[string]any)["items"].(map[string]any)["enum"].([]any)
	if len(refEnum) != 1 || refEnum[0] != "cwi-001" {
		t.Fatalf("proof-batch schema exposed a shared source ref that escapes the batch: %v", refEnum)
	}
}

func TestEvaluateRuntimeDefinitionGapAdmissionClassifiesStaleIdentity(t *testing.T) {
	manifest, expected, document := runtimeDefinitionGapDocumentForTest(t)
	t.Run("provider document", func(t *testing.T) {
		stale := document.Clone()
		stale.ManifestHash = "runtime-semantic-manifest-stale"
		decision := EvaluateRuntimeDefinitionGapAdmission(manifest, expected, stale)
		if decision.Disposition != RuntimeDefinitionGapAdmissionStaleConflict || decision.Code != "document_identity_stale" || decision.Admitted() {
			t.Fatalf("stale Definition gap admission = %+v", decision)
		}
	})
	t.Run("expected document", func(t *testing.T) {
		stale := expected.Clone()
		stale.ManifestHash = "runtime-semantic-manifest-stale"
		stale.Hash = StableRuntimeExpectedDocumentHash(stale)
		decision := EvaluateRuntimeDefinitionGapAdmission(manifest, stale, document)
		if decision.Disposition != RuntimeDefinitionGapAdmissionStaleConflict || decision.Code != "expected_document_stale" || decision.Admitted() {
			t.Fatalf("stale expected document admission = %+v", decision)
		}
	})
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

func TestRuntimeDefinitionGapProviderHashBindsTypedPrerequisiteClaims(t *testing.T) {
	manifest, _, document := runtimeDefinitionGapDocumentForTest(t)
	document.GapKind = RuntimeDefinitionGapUndeclaredPrerequisite
	document.PrerequisiteClaims = []RuntimeDefinitionGapPrerequisiteClaim{runtimeDefinitionGapPrerequisiteClaimForTest(manifest.CoverageItems[0].ID)}
	document.DiagnosticMessage = "The sealed item omits one exact prerequisite."
	baseline := StableRuntimeDefinitionGapProviderDocumentHash(document)
	drift := document.Clone()
	drift.PrerequisiteClaims[0].OwnerModuleID = RuntimeModuleMCP
	drift.PrerequisiteClaims[0].OperationKind = StepKindMCPToolCall
	if changed := StableRuntimeDefinitionGapProviderDocumentHash(drift); changed == baseline {
		t.Fatalf("typed prerequisite claim drift did not change provider document identity: %q", changed)
	}
}
