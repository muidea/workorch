package workorchprojectioncontract

import "testing"

func TestCloneRuntimeAuditDetachesSealedSoulMetadata(t *testing.T) {
	input := &RuntimeAudit{StartupInputHash: "sealed-input", SealedProfileContractsHash: "sealed-profile", SealedSoulBindings: []RuntimeAuditSoulBinding{{Stage: "execution", SoulID: "soul-a", Revision: 3, ContentHash: "sealed-content"}}}
	cloned := CloneRuntimeAudit(input)
	cloned.SealedSoulBindings[0].Revision = 4
	if input.SealedSoulBindings[0].Revision != 3 || cloned.StartupInputHash != input.StartupInputHash || cloned.SealedProfileContractsHash != input.SealedProfileContractsHash {
		t.Fatal("audit clone lost or aliased sealed metadata")
	}
}

func TestCloneRuntimeAuditEvidenceDetachesCollections(t *testing.T) {
	input := []RuntimeAuditEvidence{{
		ID: "evidence-1", TargetRefs: []string{"a.go"}, BindingRefs: []string{"binding-1"},
		CoverageWorkItemIDs: []string{"cwi-1"}, EvidenceRefs: []string{"ref-1"}, ExecutedChecks: []string{"test-a"},
	}}
	cloned := CloneRuntimeAuditEvidence(input)
	cloned[0].TargetRefs[0] = "b.go"
	cloned[0].BindingRefs[0] = "binding-2"
	cloned[0].CoverageWorkItemIDs[0] = "cwi-2"
	cloned[0].EvidenceRefs[0] = "ref-2"
	cloned[0].ExecutedChecks[0] = "test-b"
	if input[0].TargetRefs[0] != "a.go" || input[0].BindingRefs[0] != "binding-1" || input[0].CoverageWorkItemIDs[0] != "cwi-1" || input[0].EvidenceRefs[0] != "ref-1" || input[0].ExecutedChecks[0] != "test-a" {
		t.Fatalf("clone mutated source: %+v", input)
	}
}

func TestCloneRuntimeAuditCoverageBindingsDetachesNestedProofs(t *testing.T) {
	input := []RuntimeAuditCoverageBinding{{
		ID: "binding-1", TargetRefs: []string{"a.go"}, TargetSelectorCandidateRefs: []string{"candidate.go"},
		ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRefs: []string{"new.go"},
		PrerequisiteRefs: []string{"manifest-1"},
		Proofs: []RuntimeAuditCoverageProof{{
			RequiredCheckRefs: []string{"test-a"}, ArtifactRefs: []string{"artifact-a"},
			ValidationCheckAttestations: []RuntimeAuditValidationCheckAttestation{{
				RequiredCheckRef: "test-a", ObservationProtocolRef: "protocol-a", NativeCheckRef: "native-a",
			}},
		}},
	}}
	cloned := CloneRuntimeAuditCoverageBindings(input)
	cloned[0].TargetRefs[0] = "b.go"
	cloned[0].TargetSelectorCandidateRefs[0] = "other-candidate.go"
	cloned[0].ProspectiveTargetAllOfConstraintRefs[0] = "other-constraint"
	cloned[0].ProspectiveTargetRefs[0] = "other-new.go"
	cloned[0].PrerequisiteRefs[0] = "manifest-2"
	cloned[0].Proofs[0].RequiredCheckRefs[0] = "test-b"
	cloned[0].Proofs[0].ValidationCheckAttestations[0].NativeCheckRef = "native-b"
	cloned[0].Proofs[0].ArtifactRefs[0] = "artifact-b"
	if input[0].TargetRefs[0] != "a.go" || input[0].TargetSelectorCandidateRefs[0] != "candidate.go" ||
		input[0].ProspectiveTargetAllOfConstraintRefs[0] != "workspace-relative-path" || input[0].ProspectiveTargetRefs[0] != "new.go" ||
		input[0].PrerequisiteRefs[0] != "manifest-1" ||
		input[0].Proofs[0].RequiredCheckRefs[0] != "test-a" ||
		input[0].Proofs[0].ValidationCheckAttestations[0].NativeCheckRef != "native-a" ||
		input[0].Proofs[0].ArtifactRefs[0] != "artifact-a" {
		t.Fatalf("coverage binding clone mutated source: %+v", input)
	}
}

func TestCloneRuntimeAuditAssessmentsDetachesItemsAndGap(t *testing.T) {
	input := []RuntimeAuditAssessment{{
		ID: "assessment-1", EvidenceRefs: []string{"evidence-a"},
		Items: []RuntimeAuditAssessmentItem{{CoverageWorkItemID: "cwi-1", EvidenceRefs: []string{"item-evidence-a"}}},
		Gap:   &RuntimeAuditAssessmentGap{TargetRefs: []string{"a.go"}, MissingEvidenceKinds: []string{"validation"}},
	}}
	cloned := CloneRuntimeAuditAssessments(input)
	cloned[0].EvidenceRefs[0] = "evidence-b"
	cloned[0].Items[0].EvidenceRefs[0] = "item-evidence-b"
	cloned[0].Gap.TargetRefs[0] = "b.go"
	cloned[0].Gap.MissingEvidenceKinds[0] = "state_change"
	if input[0].EvidenceRefs[0] != "evidence-a" || input[0].Items[0].EvidenceRefs[0] != "item-evidence-a" ||
		input[0].Gap.TargetRefs[0] != "a.go" || input[0].Gap.MissingEvidenceKinds[0] != "validation" {
		t.Fatalf("assessment clone mutated source: %+v", input)
	}
}
