package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func semanticProposalHashFullEncodingReference(proposal RuntimeSemanticProofProposal) string {
	proposal = proposal.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-semantic-proof-proposal")
	writeHashString(h, string(proposal.CoverageWorkItemID))
	writeHashString(h, proposal.OwnerModuleID)
	writeHashString(h, proposal.OperationKind)
	writeHashString(h, proposal.OperationVariantID)
	writeRuntimeSemanticTargetHash(h, proposal.Target)
	writeHashStrings(h, proposal.RequiredEvidenceKinds)
	writeHashStrings(h, RuntimeEffectScopesAsStrings(proposal.RequiredEffectScopes))
	writeHashString(h, proposal.ValidationRecipeID)
	writeHashStrings(h, proposal.RequiredCheckRefs)
	writeHashString(h, proposal.DeclarationRef)
	writeRuntimeValidationInputReviewHash(h, proposal.ValidationInputReview)
	return "runtime-semantic-proposal-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func semanticManifestHashFullEncodingReference(manifest RuntimeSemanticPlanManifest) string {
	manifest = manifest.Normalized()
	h := sha256.New()
	writeRuntimeSemanticInputReviewInformationHash(h, manifest.InputReviewInformation)
	writeHashString(h, RuntimeSemanticPlanManifestSchemaCurrent)
	writeHashString(h, string(manifest.WorkspaceID))
	writeHashString(h, string(manifest.RunID))
	writeHashString(h, manifest.DefinitionRevision)
	writeHashString(h, manifest.DefinitionSeedHash)
	writeHashString(h, manifest.OperationContractRevision)
	writeRuntimeSemanticCoverageItemsHash(h, manifest.CoverageItems)
	writeHashStrings(h, manifest.AllowedOwnerModuleIDs)
	writeHashStrings(h, manifest.AllowedOperationKinds)
	for _, policy := range manifest.AllowedTargetPolicies {
		writeHashString(h, string(policy))
	}
	writeRuntimeSemanticProofObligationsHash(h, manifest.ProofObligations)
	for _, catalog := range manifest.OwnerCatalogs {
		writeHashString(h, catalog.SnapshotID)
	}
	for _, catalog := range manifest.OwnerFactDependencyCatalogs {
		writeHashString(h, catalog.SnapshotID)
	}
	return "runtime-semantic-manifest-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func semanticDocumentHashFullEncodingReference(document RuntimeSemanticPlanDocument) string {
	document = document.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeSemanticPlanDocumentSchemaCurrent)
	writeHashString(h, document.ManifestHash)
	writeHashString(h, document.DefinitionSeedHash)
	writeHashString(h, string(document.PlanStep))
	writeHashString(h, document.BatchID)
	writeHashString(h, fmt.Sprintf("%d", document.BatchSequence))
	for _, id := range document.BatchCoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	for _, id := range document.CoverageOrder {
		writeHashString(h, string(id))
	}
	if document.ImplementationPurpose != "" {
		writeHashString(h, "implementation-purpose")
		writeHashString(h, string(document.ImplementationPurpose))
	}
	for _, proposal := range document.Proposals {
		writeHashString(h, semanticProposalHashFullEncodingReference(proposal))
	}
	return "runtime-semantic-document-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func assertSemanticBufferedHashIdentity(t testing.TB, proposal RuntimeSemanticProofProposal, manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument) {
	t.Helper()
	type hashInputs struct {
		Proposal RuntimeSemanticProofProposal
		Manifest RuntimeSemanticPlanManifest
		Document RuntimeSemanticPlanDocument
	}
	before, err := json.Marshal(hashInputs{proposal, manifest, document})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if got, want := StableRuntimeSemanticProofProposalHash(proposal), semanticProposalHashFullEncodingReference(proposal); got != want {
			t.Fatal("proposal 原完整 hash 编码漂移")
		}
		if got, want := StableRuntimeSemanticPlanManifestHash(manifest), semanticManifestHashFullEncodingReference(manifest); got != want {
			t.Fatal("manifest 原完整 hash 编码漂移")
		}
		if got, want := StableRuntimeSemanticPlanDocumentHash(document), semanticDocumentHashFullEncodingReference(document); got != want {
			t.Fatal("document 原完整 hash 编码漂移")
		}
	}
	after, _ := json.Marshal(hashInputs{proposal, manifest, document})
	if string(before) != string(after) {
		t.Fatal("hash 改写原 semantic 输入")
	}
}

func TestSemanticBufferedHashMatchesFullOriginalEncoding(t *testing.T) {
	for _, value := range []string{"", " 原判据\r\n\x00 ", string([]byte{0xff, 0x80}), strings.Repeat("x", 120), strings.Repeat("x", 121), strings.Repeat("原", 32768)} {
		proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: "cwi", OwnerModuleID: "owner", OperationKind: "kind", OperationVariantID: "variant", DeclarationRef: value,
			Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{" b ", "a", "a"}}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession}, ValidationRecipeID: "recipe", RequiredCheckRefs: []string{"check-b", "check-a"}}
		manifest := RuntimeSemanticPlanManifest{WorkspaceID: "ws", RunID: "run", DefinitionRevision: value, DefinitionSeedHash: "seed", OperationContractRevision: "contract", AllowedOwnerModuleIDs: []string{"owner"}, AllowedOperationKinds: []string{"kind"}, AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget},
			CoverageItems: []RuntimeCoverageItem{{ID: "cwi", TargetText: value}}, OwnerCatalogs: []RuntimeOperationAdmissibility{{SnapshotID: "catalog", OwnerModuleID: "owner"}}}
		document := RuntimeSemanticPlanDocument{ManifestHash: "manifest", DefinitionSeedHash: "seed", PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: value, BatchSequence: 2, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, Proposals: []RuntimeSemanticProofProposal{proposal}}
		assertSemanticBufferedHashIdentity(t, proposal, manifest, document)
		body, _ := json.Marshal(document)
		var reloaded RuntimeSemanticPlanDocument
		if err := json.Unmarshal(body, &reloaded); err != nil {
			t.Fatal(err)
		}
		if StableRuntimeSemanticPlanDocumentHash(reloaded) != semanticDocumentHashFullEncodingReference(reloaded) {
			t.Fatal("JSON 重载 hash 漂移")
		}
	}
	proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: "cwi", OwnerModuleID: "owner", OperationKind: "kind", DeclarationRef: "declaration"}
	original := testing.AllocsPerRun(50, func() { _ = semanticProposalHashFullEncodingReference(proposal) })
	current := testing.AllocsPerRun(50, func() { _ = StableRuntimeSemanticProofProposalHash(proposal) })
	if current >= original {
		t.Fatalf("短字段分配未减少：current=%v original=%v", current, original)
	}
}

func TestSemanticBufferedHashValidSealedLedgerRetainsIdentityAfterReload(t *testing.T) {
	ledger := semanticSealedLedgerFixture(t)
	if ledger.Manifest == nil || ledger.Document == nil || len(ledger.Document.Proposals) == 0 {
		t.Fatal("缺少有效封存前像")
	}
	assertSemanticBufferedHashIdentity(t, ledger.Document.Proposals[0], *ledger.Manifest, *ledger.Document)
	body, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("原封存 semantic ledger 重载身份漂移：%v", err)
	}
	assertSemanticBufferedHashIdentity(t, reloaded.Document.Proposals[0], *reloaded.Manifest, *reloaded.Document)
}

func FuzzSemanticBufferedHashEncoding(f *testing.F) {
	f.Add("原判据\r\n", "target", "recipe")
	f.Add(string([]byte{0xff, 0x80}), "", "")
	f.Fuzz(func(t *testing.T, declaration, target, recipe string) {
		proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: "cwi", OwnerModuleID: "owner", OperationKind: "kind", DeclarationRef: declaration, ValidationRecipeID: recipe, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{target}}}
		manifest := RuntimeSemanticPlanManifest{WorkspaceID: "ws", RunID: "run", DefinitionRevision: declaration, AllowedOwnerModuleIDs: []string{target}, AllowedOperationKinds: []string{recipe}}
		document := RuntimeSemanticPlanDocument{ManifestHash: target, DefinitionSeedHash: recipe, BatchID: declaration, Proposals: []RuntimeSemanticProofProposal{proposal}}
		assertSemanticBufferedHashIdentity(t, proposal, manifest, document)
	})
}

func BenchmarkSemanticBufferedProposalHash(b *testing.B) {
	for _, declaration := range []string{"declaration", strings.Repeat("原", 32768)} {
		proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: "cwi", OwnerModuleID: "owner", OperationKind: "kind", DeclarationRef: declaration}
		for _, mode := range []string{"reference", "current"} {
			b.Run(fmt.Sprintf("bytes=%d/%s", len(declaration), mode), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if mode == "reference" {
						_ = semanticProposalHashFullEncodingReference(proposal)
					} else {
						_ = StableRuntimeSemanticProofProposalHash(proposal)
					}
				}
			})
		}
	}
}
