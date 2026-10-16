package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Original encoders, preserving every field, prefix, raw byte and collection.
func referenceObligationCheckUnbufferedHash(revision string, id common.CoverageWorkItemID, value RuntimeSourceObligationCheck) string {
	h := sha256.New()
	for _, field := range []string{"runtime-source-obligation-check/v1", revision, string(id), value.InventoryHash, value.ObligationID, value.RequirementID, value.VerificationID, value.Criterion, value.Declaration} {
		writeHashString(h, field)
	}
	return "workorch-obligation-check-" + hex.EncodeToString(h.Sum(nil))
}

func referenceProofRequirementUnbufferedHash(r RuntimeValidationProofRequirement) string {
	r = r.Normalized()
	h := sha256.New()
	for _, field := range []string{RuntimeValidationProofRequirementSchemaCurrent, r.DefinitionRevision, string(r.CoverageWorkItemID), r.SourceScope, r.SourceID, r.SourceTextHash, r.DeclarationRef, r.ValidationRecipeID} {
		writeHashString(h, field)
	}
	writeHashStrings(h, r.RequiredCheckRefs)
	if len(r.SourceObligationChecks) != 0 {
		writeRuntimeSourceObligationChecksHash(h, r.SourceObligationChecks)
	}
	if len(r.LineageAssertions) != 0 {
		payload, _ := json.Marshal(r.LineageAssertions)
		writeHashString(h, "validation-proof-composition/v1")
		writeHashString(h, string(payload))
	}
	return RuntimeValidationProofRequirementHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

func obligationHashFixture(raw string) RuntimeValidationProofRequirement {
	return RuntimeValidationProofRequirement{SchemaVersion: RuntimeValidationProofRequirementSchemaCurrent,
		DefinitionRevision: "definition-1", CoverageWorkItemID: "cwi-1", SourceScope: "dodItem", SourceID: "dod-1", SourceTextHash: "source-hash",
		DeclarationRef: "definition/dodItem/dod-1", ValidationRecipeID: raw, RequiredCheckRefs: []string{"check-1", "check-1", raw, ""},
		SourceObligationChecks: []RuntimeSourceObligationCheck{{InventoryHash: raw, ObligationID: "obligation-1", RequirementID: "requirement-1", VerificationID: "verification-1", Criterion: raw, Declaration: raw, CheckRef: "check-1"}},
		LineageAssertions:      []RuntimeLineageAssertion{{DeclarationRef: raw, SourceScope: raw, SourceID: raw}},
	}
}

func assertObligationBufferedHashes(t testing.TB, r RuntimeValidationProofRequirement, raw string) {
	t.Helper()
	before, _ := json.Marshal(r)
	for range 2 {
		if got, want := StableRuntimeValidationProofRequirementHash(r), referenceProofRequirementUnbufferedHash(r); got != want {
			t.Fatal("requirement original hash encoding changed")
		}
		for _, check := range r.SourceObligationChecks {
			if got, want := StableRuntimeSourceObligationCheckRef(raw, r.CoverageWorkItemID, check), referenceObligationCheckUnbufferedHash(raw, r.CoverageWorkItemID, check); got != want {
				t.Fatal("source obligation original hash encoding changed")
			}
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("hash changed original input")
	}
	var loaded RuntimeValidationProofRequirement
	if err := json.Unmarshal(before, &loaded); err != nil {
		t.Fatal(err)
	}
	if StableRuntimeValidationProofRequirementHash(loaded) != referenceProofRequirementUnbufferedHash(loaded) {
		t.Fatal("reloaded hash differs from original encoder")
	}
}

func TestObligationBufferedHashesRetainOriginalEncoding(t *testing.T) {
	assertObligationBufferedHashes(t, RuntimeValidationProofRequirement{}, "")
	for _, raw := range []string{"", " 原\x00文\xff ", strings.Repeat("s", 120), strings.Repeat("s", 121), strings.Repeat("l", 8191), strings.Repeat("l", 8192), strings.Repeat("长原文\x00", 4096)} {
		r := obligationHashFixture(raw)
		assertObligationBufferedHashes(t, r, raw)
		r.RequiredCheckRefs = nil
		r.LineageAssertions = nil
		assertObligationBufferedHashes(t, r, raw)
	}
}

func TestObligationBufferedHashRetainsIndependentValidationAndStaleRejection(t *testing.T) {
	r := runtimeValidationProofRequirementForTest(t, "definition-1", RuntimeCoverageItem{ID: "cwi-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "original criterion"}, "", "", nil)
	if r.Hash != referenceProofRequirementUnbufferedHash(r) || r.Validate() != nil {
		t.Fatal("valid original proof identity changed")
	}
	oldHash := r.Hash
	r.RequiredCheckRefs = append(r.RequiredCheckRefs, "different-exact-check")
	if r.Validate() == nil || StableRuntimeValidationProofRequirementHash(r) == oldHash {
		t.Fatal("old hash accepted changed original check requirements")
	}
	r.Hash = referenceProofRequirementUnbufferedHash(r)
	if err := r.Validate(); err != nil {
		t.Fatal("independent original encoder no longer authorizes the same valid body", err)
	}
	r.RequiredCheckRefs = append(r.RequiredCheckRefs, r.RequiredCheckRefs[0])
	r.Hash = referenceProofRequirementUnbufferedHash(r)
	if r.Validate() == nil {
		t.Fatal("recomputed hash bypassed original duplicate raw-check gate")
	}
}

func TestObligationBufferedHashReadOnlyArchiveOriginalIdentity(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		visit := func(m *RuntimeSemanticPlanManifest) {
			if m == nil {
				return
			}
			for _, obligation := range m.ProofObligations {
				r := obligation.ValidationProofRequirement
				if r == nil {
					continue
				}
				if r.Hash != referenceProofRequirementUnbufferedHash(*r) {
					t.Fatal("archive proof hash differs from original encoder")
				}
				assertObligationBufferedHashes(t, *r, r.DefinitionRevision)
				for _, check := range r.SourceObligationChecks {
					if check.CheckRef != referenceObligationCheckUnbufferedHash(r.DefinitionRevision, r.CoverageWorkItemID, check) {
						t.Fatal("archive exact source check ref differs from original encoder")
					}
				}
			}
		}
		visit(run.RuntimeState.SemanticPlanning.Manifest)
		for _, generation := range run.RuntimeState.SemanticPlanning.History {
			visit(generation.Snapshot.Manifest)
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzObligationBufferedHashesMatchOriginal(f *testing.F) {
	for _, raw := range []string{"", "原\x00文\xff", strings.Repeat("x", 121), strings.Repeat("x", 8192)} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 32768 {
			t.Skip()
		}
		assertObligationBufferedHashes(t, obligationHashFixture(raw), raw)
	})
}

func TestObligationBufferedHashesReduceCompleteEncodingAllocations(t *testing.T) {
	r := obligationHashFixture("short original criterion")
	check := r.SourceObligationChecks[0]
	for _, requirement := range []bool{false, true} {
		old := testing.AllocsPerRun(20, func() {
			if requirement {
				_ = referenceProofRequirementUnbufferedHash(r)
			} else {
				_ = referenceObligationCheckUnbufferedHash(r.DefinitionRevision, r.CoverageWorkItemID, check)
			}
		})
		current := testing.AllocsPerRun(20, func() {
			if requirement {
				_ = StableRuntimeValidationProofRequirementHash(r)
			} else {
				_ = StableRuntimeSourceObligationCheckRef(r.DefinitionRevision, r.CoverageWorkItemID, check)
			}
		})
		if current >= old {
			t.Fatalf("complete encoding allocations did not fall: %v >= %v", current, old)
		}
		t.Logf("requirement=%v allocations old=%.0f buffered=%.0f", requirement, old, current)
	}
}
