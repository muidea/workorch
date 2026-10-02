package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func runtimeManifestBoundSessionForTest(t testing.TB, manifest RuntimeCandidateContractManifest) RuntimeCandidateRequestSession {
	t.Helper()
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	expected := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentOperations,
		ManifestHash: manifest.Hash, WireContractHash: wire.Hash,
		LogicalRequestID: "request-manifest-validation", StablePrefixHash: "stable-prefix",
		MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
	expected.Hash = StableRuntimeExpectedDocumentHash(expected)
	session := RuntimeCandidateRequestSession{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected,
		Transaction:      AuthorityTransaction{AuthoritySnapshotID: "authority-manifest-validation"},
		StablePrefixHash: expected.StablePrefixHash, ManifestHash: manifest.Hash, CandidateManifest: &manifest,
		State: RuntimeCandidateRequestSessionPrepared, NextSequence: 1,
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("合法封存 session fixture: %v", err)
	}
	return session
}

func runtimeSessionValidationManifestForTest(t *testing.T) RuntimeCandidateContractManifest {
	t.Helper()
	return mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds: []string{StepKindFileWrite}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-manifest-validation"}, TargetRefs: []string{"target.txt"},
		},
	})
}

func TestRuntimeCandidateSessionManifestValidationPreservesSourceAndIdentityGates(t *testing.T) {
	manifest := runtimeSessionValidationManifestForTest(t)
	manifest.Bindings[0].Proofs = append(manifest.Bindings[0].Proofs, testCoverageEvidenceBinding().Proofs[1].Clone())
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
	session := runtimeManifestBoundSessionForTest(t, manifest)
	for _, test := range []struct {
		name   string
		mutate func(*RuntimeCandidateContractManifest)
		rehash bool
	}{
		{"body_drift", func(m *RuntimeCandidateContractManifest) { m.Bindings[0].DeclarationRef = "changed" }, false},
		{"binding_hash_missing", func(m *RuntimeCandidateContractManifest) { m.Bindings[0].Hash = "" }, true},
		{"binding_hash_drift", func(m *RuntimeCandidateContractManifest) { m.Bindings[0].Hash = "changed" }, true},
		{"duplicate_proof", func(m *RuntimeCandidateContractManifest) {
			m.Bindings[0].Proofs = append(m.Bindings[0].Proofs, m.Bindings[0].Proofs[0].Clone())
		}, true},
		{"duplicate_check", func(m *RuntimeCandidateContractManifest) {
			proof := &m.Bindings[0].Proofs[1]
			proof.RequiredCheckRefs = append(proof.RequiredCheckRefs, proof.RequiredCheckRefs[0])
		}, true},
		{"aliased_check", func(m *RuntimeCandidateContractManifest) {
			proof := &m.Bindings[0].Proofs[1]
			proof.RequiredCheckRefs = append(proof.RequiredCheckRefs, " "+proof.RequiredCheckRefs[0]+" ")
		}, true},
		{"exclusion_without_source", func(m *RuntimeCandidateContractManifest) {
			m.Operations[0].Target.ExcludedTargetRefs = []string{"protected.txt"}
		}, true},
		{"repair_wrong_binding", func(m *RuntimeCandidateContractManifest) {
			m.RepairPurpose = RuntimeDispatchRepairPurposeConfirmedValidation
			m.RepairClosureID = "unproven-repair"
			for index := range m.Operations[0].BindingAuthorizations {
				m.Operations[0].BindingAuthorizations[index].AllowedBindingRefs = []string{"another-binding"}
			}
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := session.Clone()
			test.mutate(bad.CandidateManifest)
			if test.rehash {
				bad.CandidateManifest.Hash = StableRuntimeCandidateContractManifestHash(*bad.CandidateManifest)
				bad.ManifestHash = bad.CandidateManifest.Hash
				bad.ExpectedDocument.ManifestHash = bad.ManifestHash
				bad.ExpectedDocument.Hash = StableRuntimeExpectedDocumentHash(*bad.ExpectedDocument)
			}
			before := cloneRuntimeCandidateContractManifestRaw(*bad.CandidateManifest)
			if bad.Validate() == nil || ValidateRuntimeCandidateContractManifestRawCanonical(*bad.CandidateManifest) == nil {
				t.Fatal("篡改来源通过公开 session 或 raw manifest gate")
			}
			if _, err := BuildRuntimeCandidateWireContract(*bad.CandidateManifest); err == nil {
				t.Fatal("篡改来源通过公开 wire compiler")
			}
			if !reflect.DeepEqual(before, *bad.CandidateManifest) {
				t.Fatal("失败校验修改原始 manifest")
			}
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(*RuntimeCandidateRequestSession)
	}{
		{"missing_expected", func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument = nil }},
		{"expected_self_hash", func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.Hash = "changed" }},
		{"wrong_wire", func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.WireContractHash = "changed" }},
		{"wrong_request", func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.LogicalRequestID = "changed" }},
		{"wrong_prefix", func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.StablePrefixHash = "changed" }},
		{"wrong_manifest", func(s *RuntimeCandidateRequestSession) { s.ExpectedDocument.ManifestHash = "changed" }},
		{"wrong_kind", func(s *RuntimeCandidateRequestSession) { s.DocumentKind = RuntimeProviderDocumentSemanticPlan }},
		{"wrong_sequence", func(s *RuntimeCandidateRequestSession) { s.NextSequence++ }},
		{"wrong_provider_state", func(s *RuntimeCandidateRequestSession) { s.ProviderSubmitted = true }},
		{"wrong_repair_commitment", func(s *RuntimeCandidateRequestSession) { s.RepairCommitmentHash = "unproven" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := session.Clone()
			test.mutate(&bad)
			if bad.ExpectedDocument != nil && test.name != "expected_self_hash" {
				bad.ExpectedDocument.Hash = StableRuntimeExpectedDocumentHash(*bad.ExpectedDocument)
			}
			if bad.Validate() == nil {
				t.Fatal("session 接受重新 hash 后仍不匹配的请求身份")
			}
		})
	}
}

func TestRuntimeCandidateSessionManifestValidationRetainsOwnerInputPreimages(t *testing.T) {
	decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds: []string{StepKindFileWrite}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"acceptance.fixture", "implementation"},
		},
	})
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
		OperationAdmissibilities: []RuntimeOperationAdmissibility{inputProtectionCatalog(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	session := runtimeManifestBoundSessionForTest(t, manifest)
	for _, test := range []struct {
		name   string
		mutate func(*RuntimeCandidateContractManifest)
	}{
		{"missing_exclusion", func(m *RuntimeCandidateContractManifest) { m.Operations[0].Target.ExcludedTargetRefs = nil }},
		{"missing_catalog", func(m *RuntimeCandidateContractManifest) { m.MutationInputCatalogs = nil }},
		{"catalog_input_drift", func(m *RuntimeCandidateContractManifest) {
			m.MutationInputCatalogs[0].ValidationSurfaces[0].InputTargetRefs = []string{"different-input"}
		}},
		{"catalog_snapshot_drift", func(m *RuntimeCandidateContractManifest) { m.MutationInputCatalogs[0].SnapshotID = "changed" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := session.Clone()
			test.mutate(bad.CandidateManifest)
			bad.CandidateManifest.Hash = StableRuntimeCandidateContractManifestHash(*bad.CandidateManifest)
			bad.ManifestHash = bad.CandidateManifest.Hash
			bad.ExpectedDocument.ManifestHash = bad.ManifestHash
			bad.ExpectedDocument.Hash = StableRuntimeExpectedDocumentHash(*bad.ExpectedDocument)
			if bad.Validate() == nil || ValidateRuntimeCandidateContractManifestRawCanonical(*bad.CandidateManifest) == nil {
				t.Fatal("重新 hash 掩盖 owner input 前像或排除集合漂移")
			}
			if _, err := BuildRuntimeCandidateWireContract(*bad.CandidateManifest); err == nil {
				t.Fatal("公开 wire compiler 遗漏 owner input 来源校验")
			}
		})
	}
}

func TestRuntimeCandidateSessionManifestValidationIsDetachedAndFresh(t *testing.T) {
	session := runtimeManifestBoundSessionForTest(t, runtimeSessionValidationManifestForTest(t))
	before := session.Clone()
	for range 2 {
		if err := session.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, session) {
		t.Fatal("重复校验修改调用者 session")
	}
	validated, err := validateRuntimeCandidateManifestRawCanonical(*session.CandidateManifest)
	if err != nil {
		t.Fatal(err)
	}
	validated.manifest.Bindings[0].TargetRefs[0] = "detached.txt"
	validated.manifest.Operations[0].AllowedOperationFields[0] = "detached-field"
	if !reflect.DeepEqual(before, session) {
		t.Fatal("私有校验产物共享调用者的可变 backing array")
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeCandidateRequestSession
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil || !reflect.DeepEqual(reloaded, session) {
		t.Fatalf("同版本重载改变 session 身份: %v", err)
	}
	wire, err := BuildRuntimeCandidateWireContract(*reloaded.CandidateManifest)
	if err != nil || wire.Hash != session.ExpectedDocument.WireContractHash {
		t.Fatalf("重载 wire identity 漂移: %v", err)
	}
	reloaded.CandidateManifest.Bindings[0].DeclarationRef = "changed-after-success"
	if err := reloaded.Validate(); err == nil {
		t.Fatal("先前成功结果掩盖 fresh-load 来源漂移")
	}
}
