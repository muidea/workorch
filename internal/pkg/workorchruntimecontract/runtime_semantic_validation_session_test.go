package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestRuntimeSemanticValidationSessionReusesOnlyExactRawManifest(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	session := newRuntimeSemanticValidationSession()
	validated, err := validateRuntimeSemanticPlanManifestInSession(manifest, session)
	if err != nil {
		t.Fatal(err)
	}
	if validated.manifest.Hash != manifest.Hash || stableRuntimeSemanticPlanManifestHashCanonical(validated.manifest) != manifest.Hash {
		t.Fatalf("validated manifest hash drifted: got %q want %q", validated.manifest.Hash, manifest.Hash)
	}
	if _, err := validateRuntimeSemanticPlanManifestInSession(manifest, session); err != nil {
		t.Fatalf("exact manifest replay failed: %v", err)
	}

	forged := manifest.Clone()
	forged.OwnerCatalogs[0].TargetRefs = append(forged.OwnerCatalogs[0].TargetRefs, "forged.go")
	// Retain both caller-supplied identities. Reuse must still compare the raw
	// body and perform the complete catalog/hash validation.
	forged.OwnerCatalogs[0].SnapshotID = manifest.OwnerCatalogs[0].SnapshotID
	forged.Hash = manifest.Hash
	if _, err := validateRuntimeSemanticPlanManifestInSession(forged, session); err == nil || !strings.Contains(err.Error(), "snapshot identity") {
		t.Fatalf("same-hash forged catalog was accepted: %v", err)
	}
}

func TestRuntimeSemanticValidationSessionKeepsRawCatalogCollectionFailClosed(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	session := newRuntimeSemanticValidationSession()
	if _, err := validateRuntimeSemanticPlanManifestInSession(manifest, session); err != nil {
		t.Fatal(err)
	}

	duplicate := manifest.Clone()
	duplicate.OwnerCatalogs = append(duplicate.OwnerCatalogs, duplicate.OwnerCatalogs[0])
	duplicate.Hash = StableRuntimeSemanticPlanManifestHash(duplicate)
	if _, err := validateRuntimeSemanticPlanManifestInSession(duplicate, session); err == nil || !strings.Contains(err.Error(), "duplicates prerequisite") {
		t.Fatalf("cached duplicate raw catalog was accepted: %v", err)
	}
}
