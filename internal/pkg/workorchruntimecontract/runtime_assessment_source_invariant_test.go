package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDefinitionFocusAssessmentSourceIsExactSeedProjection(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "behavior is verified"}
	seed := RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{item}}
	valid := RuntimeDefinitionFocus{
		ID: "definition-focus-1", SliceID: item.SliceID,
		AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, []RuntimeDefinitionFocus{valid}); err != nil {
		t.Fatalf("valid exact source projection: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeDefinitionFocus)
	}{
		{name: "scope", mutate: func(value *RuntimeDefinitionFocus) { value.AssessmentScope = "goal" }},
		{name: "id", mutate: func(value *RuntimeDefinitionFocus) { value.AssessmentID = "goal-1" }},
		{name: "text", mutate: func(value *RuntimeDefinitionFocus) { value.AssessmentText = "rewritten" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drifted := valid.Clone()
			tc.mutate(&drifted)
			if err := ValidateRuntimeDefinitionFocuses(seed, []RuntimeDefinitionFocus{drifted}); err == nil {
				t.Fatal("definition focus source drift was accepted")
			}
		})
	}
}

func TestRuntimeAssessmentAuthoritySourceIsSeedRootedForActiveDispatch(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "behavior is verified"}
	facts := RuntimeFacts{
		DefinitionSeed: RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{item}},
		DefinitionFocuses: []RuntimeDefinitionFocus{{
			ID: "definition-focus-1", SliceID: item.SliceID,
			AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
			EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
		}},
	}
	dispatch := RuntimeDispatchRecord{
		ID: "dispatch-1", Transaction: AuthorityTransaction{FocusID: "semantic-focus-1"},
		Candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{FocusID: "semantic-focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}}},
		AssessmentAuthorityInput: &RuntimeAssessmentAuthorityInput{
			SourceSliceID: item.SliceID, SourceScope: item.SourceScope, SourceID: item.SourceID, SourceText: item.TargetText,
		},
	}
	if err := ValidateRuntimeAssessmentAuthoritySources(facts, &dispatch, nil); err != nil {
		t.Fatalf("valid active assessment authority source: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeAssessmentAuthorityInput)
	}{
		{name: "scope", mutate: func(value *RuntimeAssessmentAuthorityInput) { value.SourceScope = "goal" }},
		{name: "id", mutate: func(value *RuntimeAssessmentAuthorityInput) { value.SourceID = "goal-1" }},
		{name: "text", mutate: func(value *RuntimeAssessmentAuthorityInput) { value.SourceText = "rewritten" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drifted := dispatch.Clone()
			tc.mutate(drifted.AssessmentAuthorityInput)
			if err := ValidateRuntimeAssessmentAuthoritySources(facts, &drifted, nil); err == nil {
				t.Fatal("active assessment authority source drift was accepted")
			}
		})
	}
}

func TestRuntimeAssessmentAuthorityAndAdjudicationRejectRawNormalizationLaundering(t *testing.T) {
	focus := CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}.Normalized()
	identity := RuntimeEquivalentAuthorityInputIdentity{
		DefinitionRevision: "definition-1", FocusHash: StableRuntimeAuthorityFocusHash(focus),
		EvidenceRevision: "1", CapabilityStateHash: "capability-1",
		CatalogRevisionHash: StableRuntimeOperationAdmissibilityCatalogRevisionHash(nil),
	}.Normalized()
	transaction := AuthorityTransaction{
		FocusID: "focus-1", State: ConvergenceStateNeedAssessment,
		AuthorityInputID: StableRuntimeEquivalentAuthorityInputID(identity),
	}
	authority := RuntimeAssessmentAuthorityInput{
		AuthorityInput: identity, Purpose: RuntimeAssessmentPurposeDefinitionProof,
		SourceSliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", SourceText: "behavior is verified",
		Focus: focus,
	}.Normalized()
	if err := authority.Validate(transaction, nil); err != nil {
		t.Fatalf("valid assessment authority input: %v", err)
	}

	t.Run("authority whitespace", func(t *testing.T) {
		drifted := authority
		drifted.SourceText += " "
		if err := drifted.Validate(transaction, nil); err == nil || !strings.Contains(err.Error(), "raw canonical") {
			t.Fatalf("authority whitespace error = %v", err)
		}
	})
	t.Run("authority nil-empty", func(t *testing.T) {
		drifted := authority
		drifted.OperationAdmissibilities = []RuntimeOperationAdmissibility{}
		if err := drifted.Validate(transaction, nil); err == nil || !strings.Contains(err.Error(), "raw canonical") {
			t.Fatalf("authority nil-empty error = %v", err)
		}
	})

	proof := RuntimeAssessmentAdjudicationProof{
		CoverageWorkItemID: "cwi-1", EvidenceKind: EvidenceKindValidation,
		EvidenceFactID: "evidence-1", EvidenceFactHash: "fact-hash-1",
	}
	adjudication := RuntimeAssessmentAdjudication{
		DispatchID: "dispatch-1", AuthorityInputID: transaction.AuthorityInputID, FocusID: transaction.FocusID,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Proofs: []RuntimeAssessmentAdjudicationProof{proof},
		Disposition: AssessmentDispositionSatisfied,
	}.Normalized()
	if err := adjudication.ValidateRaw(); err != nil {
		t.Fatalf("valid assessment adjudication: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeAssessmentAdjudication)
	}{
		{name: "proof whitespace", mutate: func(value *RuntimeAssessmentAdjudication) { value.Proofs[0].EvidenceFactHash += " " }},
		{name: "proof duplicate", mutate: func(value *RuntimeAssessmentAdjudication) { value.Proofs = append(value.Proofs, value.Proofs[0]) }},
		{name: "coverage duplicate", mutate: func(value *RuntimeAssessmentAdjudication) {
			value.CoverageWorkItemIDs = append(value.CoverageWorkItemIDs, "cwi-1")
		}},
		{name: "proof nil-empty", mutate: func(value *RuntimeAssessmentAdjudication) { value.Proofs = []RuntimeAssessmentAdjudicationProof{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drifted := adjudication.Clone()
			tc.mutate(&drifted)
			if err := drifted.ValidateRaw(); err == nil {
				t.Fatal("non-canonical raw assessment adjudication was accepted")
			}
		})
	}
	t.Run("proof order", func(t *testing.T) {
		ordered := adjudication.Clone()
		ordered.Proofs = append(ordered.Proofs, RuntimeAssessmentAdjudicationProof{
			CoverageWorkItemID: "cwi-1", EvidenceKind: EvidenceKindStateChange,
			EvidenceFactID: "evidence-2", EvidenceFactHash: "fact-hash-2",
		})
		ordered = ordered.Normalized()
		ordered.Proofs[0], ordered.Proofs[1] = ordered.Proofs[1], ordered.Proofs[0]
		if err := ordered.ValidateRaw(); err == nil {
			t.Fatal("non-canonical assessment proof order was accepted")
		}
	})
}

func TestRuntimeAssessmentPreSealBindingLineageIsExactAcrossManifestAttemptDispatchAndAuthority(t *testing.T) {
	decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:         []string{StepKindFileWrite},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-assessment"},
		},
	})
	decision.State = ConvergenceStateNeedAssessment
	decision.RequiredOutput = RequiredOutputContract{}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
		AssessmentPurpose:             RuntimeAssessmentPurposeOwnerFactAcquisition,
		AssessmentPurposeManifestHash: "semantic-manifest/assessment-owner-fact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Bindings) != 1 {
		t.Fatalf("assessment manifest bindings = %+v", manifest.Bindings)
	}

	focus := decision.Focus.Normalized()
	identity := RuntimeEquivalentAuthorityInputIdentity{
		DefinitionRevision:  decision.AuthoritySnapshot.DefinitionRevision,
		FocusHash:           StableRuntimeAuthorityFocusHash(focus),
		BindingSetHash:      StableRuntimeCoverageEvidenceBindingSnapshotHash(manifest.Bindings),
		EvidenceRevision:    "runtime-evidence-00000000000000000001",
		CapabilityStateHash: "capability-assessment",
		CatalogRevisionHash: StableRuntimeOperationAdmissibilityCatalogRevisionHash(nil),
	}.Normalized()
	transaction := AuthorityTransaction{
		FocusID: focus.ID, State: ConvergenceStateNeedAssessment,
		AuthorityInputID: StableRuntimeEquivalentAuthorityInputID(identity),
	}
	authority := RuntimeAssessmentAuthorityInput{
		AuthorityInput: identity, Purpose: RuntimeAssessmentPurposeOwnerFactAcquisition,
		SourceSliceID: "slice-assessment", SourceScope: "dodItem", SourceID: "dod-assessment", SourceText: "assessment target",
		Focus: focus,
	}.Normalized()
	if err := authority.Validate(transaction, manifest.Bindings); err != nil {
		t.Fatalf("assessment authority input does not match manifest binding: %v", err)
	}

	candidate := RuntimeCandidate{Assessment: &AssessmentCandidate{
		FocusID: focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-assessment"},
		Disposition: AssessmentDispositionSatisfied, Summary: "assessment observation",
	}}
	candidateHash := StableRuntimeCandidateHash(candidate)
	dispatch := RuntimeDispatchRecord{
		ID: "dispatch-assessment", Transaction: transaction,
		CandidateHash: candidateHash, Candidate: candidate, CandidateManifestHash: manifest.Hash,
		CoverageBindings:             CloneCoverageEvidenceBindings(manifest.Bindings),
		CoverageBindingHash:          StableRuntimeCoverageEvidenceBindingSnapshotHash(manifest.Bindings),
		AssessmentAuthorityInputHash: StableRuntimeAssessmentAuthorityInputHash(authority),
		AssessmentAuthorityInput:     &authority,
	}
	logicalRequestID := "request-assessment"
	stablePrefixHash := "prefix-assessment"
	session := RuntimeCandidateRequestSession{
		LogicalRequestID: logicalRequestID, DocumentKind: RuntimeProviderDocumentOperations,
		Transaction: transaction, StablePrefixHash: stablePrefixHash, ManifestHash: manifest.Hash,
		CandidateManifest: &manifest,
	}
	attempt := RuntimeCandidateAttemptRecord{
		Transaction: transaction, State: RuntimeCandidateAttemptStateAccepted,
		LogicalRequestID: logicalRequestID, StablePrefixHash: stablePrefixHash, ManifestHash: manifest.Hash,
		CandidateHash: candidateHash, DispatchID: dispatch.ID,
		TargetAuthorizationHash:      StableRuntimeDispatchOperationTargetAuthorizationsHash(nil),
		AdmissionCoverageBindings:    CloneCoverageEvidenceBindings(manifest.Bindings),
		AdmissionCoverageBindingHash: StableRuntimeCoverageEvidenceBindingSnapshotHash(manifest.Bindings),
	}
	dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
	attempt.DispatchAuthorityHash = dispatch.DispatchAuthorityHash
	if err := ValidateRuntimeDispatchAdmissionLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err != nil {
		t.Fatalf("exact assessment attempt/dispatch admission lineage: %v", err)
	}
	if err := ValidateRuntimeAcceptedCandidateManifestLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{attempt}, []RuntimeCandidateRequestSession{session}); err != nil {
		t.Fatalf("exact assessment manifest/attempt/dispatch lineage: %v", err)
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(&dispatch, nil, nil, nil); err == nil {
		t.Fatal("assessment dispatch without accepted attempt was accepted")
	}
	if !RuntimeCoverageEvidenceBindingProjectionsEqual(manifest.Bindings, attempt.AdmissionCoverageBindings) ||
		!RuntimeCoverageEvidenceBindingProjectionsEqual(manifest.Bindings, dispatch.CoverageBindings) ||
		!RuntimeCoverageEvidenceBindingProjectionsEqual(manifest.Bindings, authority.Focus.CoverageEvidenceBindings) {
		t.Fatal("assessment pre-seal binding projection is not exact across all authority copies")
	}

	t.Run("attempt drift", func(t *testing.T) {
		drifted := attempt.Clone()
		drifted.AdmissionCoverageBindings[0].DeclarationRef = "declaration/drifted-attempt"
		drifted.AdmissionCoverageBindings[0].Hash = StableCoverageEvidenceBindingHash(drifted.AdmissionCoverageBindings[0])
		drifted.AdmissionCoverageBindingHash = StableRuntimeCoverageEvidenceBindingSnapshotHash(drifted.AdmissionCoverageBindings)
		if err := ValidateRuntimeAcceptedCandidateManifestLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{drifted}, []RuntimeCandidateRequestSession{session}); err == nil {
			t.Fatal("assessment attempt binding drift was accepted")
		}
	})
	t.Run("duplicate request session", func(t *testing.T) {
		if err := ValidateRuntimeAcceptedCandidateManifestLineage(
			&dispatch,
			nil,
			[]RuntimeCandidateAttemptRecord{attempt},
			[]RuntimeCandidateRequestSession{session, session},
		); err == nil {
			t.Fatal("duplicate pre-seal request session was accepted")
		}
	})
	t.Run("unrelated raw manifest remains fail closed", func(t *testing.T) {
		malformed := session.Clone()
		malformed.LogicalRequestID = "unrelated-request"
		manifestCopy := malformed.CandidateManifest.Clone()
		manifestCopy.Bindings = append(manifestCopy.Bindings, manifestCopy.Bindings[0])
		malformed.CandidateManifest = &manifestCopy
		if err := ValidateRuntimeAcceptedCandidateManifestLineage(
			&dispatch,
			nil,
			[]RuntimeCandidateAttemptRecord{attempt},
			[]RuntimeCandidateRequestSession{session, malformed},
		); err == nil {
			t.Fatal("unrelated non-canonical pre-seal manifest was accepted")
		}
	})
	t.Run("dispatch drift", func(t *testing.T) {
		drifted := dispatch.Clone()
		drifted.CoverageBindings[0].DeclarationRef = "declaration/drifted-dispatch"
		drifted.CoverageBindings[0].Hash = StableCoverageEvidenceBindingHash(drifted.CoverageBindings[0])
		drifted.CoverageBindingHash = StableRuntimeCoverageEvidenceBindingSnapshotHash(drifted.CoverageBindings)
		if err := ValidateRuntimeAcceptedCandidateManifestLineage(&drifted, nil, []RuntimeCandidateAttemptRecord{attempt}, []RuntimeCandidateRequestSession{session}); err == nil {
			t.Fatal("assessment dispatch binding drift was accepted")
		}
	})
	t.Run("authority input drift", func(t *testing.T) {
		drifted := authority.Clone()
		drifted.Focus.CoverageEvidenceBindings[0].DeclarationRef = "declaration/drifted-authority"
		drifted.Focus.CoverageEvidenceBindings[0].Hash = StableCoverageEvidenceBindingHash(drifted.Focus.CoverageEvidenceBindings[0])
		drifted.AuthorityInput.FocusHash = StableRuntimeAuthorityFocusHash(drifted.Focus)
		if err := drifted.Validate(transaction, dispatch.CoverageBindings); err == nil {
			t.Fatal("assessment authority-input binding drift was accepted")
		}
	})
}

func TestRuntimeAssessmentPreSealBindingRejectsExplicitNoBindingForm(t *testing.T) {
	decision := RuntimeDecision{
		State:             ConvergenceStateNeedAssessment,
		Focus:             CoverageFocus{ID: "focus-no-binding", SliceID: "slice-no-binding", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-no-binding"}},
		AuthoritySnapshot: AuthoritySnapshot{DefinitionRevision: "definition-no-binding"},
	}
	if _, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
		AssessmentPurpose:             RuntimeAssessmentPurposeDefinitionProof,
		AssessmentPurposeManifestHash: "semantic-manifest/no-binding",
	}); err == nil {
		t.Fatal("assessment manifest without a semantic coverage binding was accepted")
	}
}

func TestRuntimeAssessmentSourceProjectionRejectsCoordinatedFocusAndFactRewrite(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "behavior is verified"}
	facts := RuntimeFacts{
		DefinitionSeed: RuntimeDefinitionSeed{CoverageItems: []RuntimeCoverageItem{item}},
		DefinitionFocuses: []RuntimeDefinitionFocus{{
			ID: "definition-focus-1", SliceID: item.SliceID,
			AssessmentScope: "goal", AssessmentID: "goal-1", AssessmentText: "coordinated rewrite",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		}},
		Assessments: []RuntimeAssessmentFact{{
			ID: "assessment-1", FocusID: "semantic-focus-1", SliceID: item.SliceID,
			Scope: "goal", RequirementID: "goal-1", RequirementText: "coordinated rewrite",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID},
		}},
	}
	if _, err := RuntimeAssessmentSourceProjection(facts, "semantic-focus-1", []common.CoverageWorkItemID{item.ID}); err == nil {
		t.Fatal("coordinated DefinitionFocus/assessment source rewrite escaped the seed root")
	}
}
