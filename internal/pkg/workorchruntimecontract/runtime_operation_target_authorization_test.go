package workorchruntimecontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateRuntimeOperationTargetsAgainstAuthorizationTruthTable(t *testing.T) {
	exact := RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"go.mod", "main.go", "semantic_test.go"},
	}
	existingFinite := RuntimeOperationTargetAuthorization{
		Policy:               RuntimeOperationTargetPolicyExistingOwnerScope,
		OwnerScopeTargetRefs: []string{"go.mod", "main.go", "semantic_test.go"}, OwnerSelectorRefs: []string{"workspace-tree"},
	}
	existingSelectorOnly := RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"owner-validated-target"},
	}
	prospective := RuntimeOperationTargetAuthorization{
		Policy:            RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		OwnerSelectorRefs: []string{"workspace-relative-create"}, ProspectiveConstraintRefs: []string{"workspace-relative-path"},
		ProspectiveTargetRefs: []string{"main.go", "semantic_test.go"},
	}

	testCases := []struct {
		name           string
		authorization  RuntimeOperationTargetAuthorization
		targetRefs     []string
		requireTargets bool
		wantRule       string
	}{
		{name: "exact singleton subset", authorization: exact, targetRefs: []string{"main.go"}, requireTargets: true},
		{name: "exact multi member subset", authorization: exact, targetRefs: []string{"go.mod", "semantic_test.go"}, requireTargets: true},
		{name: "exact optional empty", authorization: exact, requireTargets: false},
		{name: "exact required empty", authorization: exact, requireTargets: true, wantRule: runtimeOperationTargetRuleRequired},
		{name: "exact outside", authorization: exact, targetRefs: []string{"outside.go"}, requireTargets: true, wantRule: runtimeOperationTargetRuleOutside},
		{name: "exact empty sealed set", authorization: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget}, requireTargets: true, wantRule: runtimeOperationTargetRuleInvalidAuthorization},
		{name: "existing finite singleton subset", authorization: existingFinite, targetRefs: []string{"main.go"}, requireTargets: true},
		{name: "existing finite multi subset", authorization: existingFinite, targetRefs: []string{"go.mod", "semantic_test.go"}, requireTargets: true},
		{name: "existing finite outside", authorization: existingFinite, targetRefs: []string{"outside.go"}, requireTargets: true, wantRule: runtimeOperationTargetRuleOutside},
		{name: "existing finite required empty", authorization: existingFinite, requireTargets: true, wantRule: runtimeOperationTargetRuleRequired},
		{name: "existing selector owner feasibility", authorization: existingSelectorOnly, targetRefs: []string{"owner-resolved-target"}, requireTargets: true},
		{name: "existing selector optional empty", authorization: existingSelectorOnly, requireTargets: false},
		{name: "existing selector required empty", authorization: existingSelectorOnly, requireTargets: true, wantRule: runtimeOperationTargetRuleRequired},
		{name: "existing missing scope and selector", authorization: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope}, requireTargets: false, wantRule: runtimeOperationTargetRuleInvalidAuthorization},
		{name: "prospective first singleton", authorization: prospective, targetRefs: []string{"main.go"}, requireTargets: true},
		{name: "prospective second singleton", authorization: prospective, targetRefs: []string{"semantic_test.go"}, requireTargets: true},
		{name: "prospective always requires target", authorization: prospective, requireTargets: false, wantRule: runtimeOperationTargetRuleRequired},
		{name: "prospective operation cannot consume complete set", authorization: prospective, targetRefs: []string{"main.go", "semantic_test.go"}, requireTargets: true, wantRule: runtimeOperationTargetRuleProspectiveSingleton},
		{name: "prospective outside", authorization: prospective, targetRefs: []string{"outside.go"}, requireTargets: true, wantRule: runtimeOperationTargetRuleOutside},
		{name: "prospective requires sealed set", authorization: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"workspace-relative-create"}}, targetRefs: []string{"main.go"}, requireTargets: true, wantRule: runtimeOperationTargetRuleInvalidAuthorization},
		{name: "unknown policy", authorization: RuntimeOperationTargetAuthorization{Policy: "unknown"}, wantRule: runtimeOperationTargetRuleInvalidAuthorization},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateRuntimeOperationTargetsAgainstAuthorization(testCase.authorization, testCase.targetRefs, testCase.requireTargets)
			if testCase.wantRule == "" {
				if err != nil {
					t.Fatalf("valid target authorization rejected: %v", err)
				}
				return
			}
			var finding *RuntimeOperationTargetAuthorizationError
			if !errors.As(err, &finding) || finding.Rule != testCase.wantRule {
				t.Fatalf("target authorization finding=%+v err=%v, want rule %q", finding, err, testCase.wantRule)
			}
		})
	}
}

func TestCandidateTargetValidatorsShareExactTargetMembership(t *testing.T) {
	decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:         []string{StepKindFileWrite},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:            []string{"go.mod", "main.go", "semantic_test.go"},
		},
	})
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	validPayload := []byte(`{"operations":[{"kind":"file/write","targetRefs":["main.go"],"parameters":[{"name":"content","string":"package main"}]}]}`)
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, validPayload)
	if err != nil {
		t.Fatalf("strict decoder rejected exact-target singleton subset: %v", err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		t.Fatalf("semantic validator rejected exact-target singleton subset: %v", err)
	}

	outsidePayload := []byte(`{"operations":[{"kind":"file/write","targetRefs":["outside.go"],"parameters":[{"name":"content","string":"package main"}]}]}`)
	_, err = DecodeRuntimeCandidatePayloadWithManifest(manifest, outsidePayload)
	var strictFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &strictFinding) || strictFinding.Rule != runtimeOperationTargetRuleOutside {
		t.Fatalf("strict target finding=%+v err=%v", strictFinding, err)
	}

	outsideCandidate := cloneRuntimeCandidate(candidate)
	outsideCandidate.StateChange.Operations[0].Operation.TargetRefs = []string{"outside.go"}
	outsideCandidate, err = DeriveRuntimeCandidateOperationAuthority(manifest, outsideCandidate)
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateRuntimeCandidateAgainstManifest(manifest, outsideCandidate)
	var semanticFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &semanticFinding) || semanticFinding.Rule != runtimeOperationTargetRuleOutside {
		t.Fatalf("semantic target finding=%+v err=%v", semanticFinding, err)
	}

	emptyPayload := []byte(`{"operations":[{"kind":"file/write","targetRefs":[],"parameters":[{"name":"content","string":"package main"}]}]}`)
	_, err = DecodeRuntimeCandidatePayloadWithManifest(manifest, emptyPayload)
	var emptyFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &emptyFinding) || emptyFinding.Rule != runtimeOperationTargetRuleRequired {
		t.Fatalf("strict empty-target finding=%+v err=%v", emptyFinding, err)
	}
}

func TestCoverageEvidenceBindingReceiptUsesOperationTargetSubset(t *testing.T) {
	binding := targetAuthorizationStateChangeBinding(
		"binding/exact-multi",
		[]string{StepKindFileWrite, StepKindShellCommand},
		[]string{"go.mod", "main.go", "semantic_test.go"},
		nil,
	)
	operation := RuntimeOperation{ID: "write-main", Kind: StepKindFileWrite, TargetRefs: []string{"main.go"}}
	receipt := targetAuthorizationAcceptedReceipt(operation, RuntimeEffectWorkspaceDurable)
	if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, operation, receipt); err != nil {
		t.Fatalf("multi-target exact binding rejected matching singleton operation/receipt: %v", err)
	}

	outsideOperation := operation
	outsideOperation.TargetRefs = []string{"outside.go"}
	outsideReceipt := targetAuthorizationAcceptedReceipt(outsideOperation, RuntimeEffectWorkspaceDurable)
	err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, outsideOperation, outsideReceipt)
	var outsideFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &outsideFinding) || outsideFinding.Rule != runtimeOperationTargetRuleOutside {
		t.Fatalf("outside receipt finding=%+v err=%v", outsideFinding, err)
	}

	emptyOperation := operation
	emptyOperation.TargetRefs = nil
	emptyReceipt := targetAuthorizationAcceptedReceipt(emptyOperation, RuntimeEffectWorkspaceDurable)
	err = MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, emptyOperation, emptyReceipt)
	var emptyFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &emptyFinding) || emptyFinding.Rule != runtimeOperationTargetRuleRequired {
		t.Fatalf("empty receipt finding=%+v err=%v", emptyFinding, err)
	}

	driftedReceipt := receipt
	driftedReceipt.TargetRefs = []string{"semantic_test.go"}
	if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, operation, driftedReceipt); err == nil || !strings.Contains(err.Error(), "receipt targets do not match sealed operation") {
		t.Fatalf("receipt-operation target drift accepted: %v", err)
	}
}

func TestCoverageEvidenceBindingProspectiveReceiptRemainsPerOperationSingleton(t *testing.T) {
	prospectiveTargets := []string{"main.go", "semantic_test.go"}
	binding := targetAuthorizationStateChangeBinding("binding/prospective-multi", []string{StepKindFileWrite}, nil, prospectiveTargets)
	for _, targetRef := range prospectiveTargets {
		operation := RuntimeOperation{ID: "write-" + targetRef, Kind: StepKindFileWrite, TargetRefs: []string{targetRef}}
		receipt := targetAuthorizationAcceptedReceipt(operation, RuntimeEffectWorkspaceDurable)
		if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, operation, receipt); err != nil {
			t.Fatalf("prospective singleton %q rejected: %v", targetRef, err)
		}
	}

	multiOperation := RuntimeOperation{ID: "write-all", Kind: StepKindFileWrite, TargetRefs: prospectiveTargets}
	multiReceipt := targetAuthorizationAcceptedReceipt(multiOperation, RuntimeEffectWorkspaceDurable)
	err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, multiOperation, multiReceipt)
	var singletonFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &singletonFinding) || singletonFinding.Rule != runtimeOperationTargetRuleProspectiveSingleton {
		t.Fatalf("prospective multi-target operation finding=%+v err=%v", singletonFinding, err)
	}

	outsideOperation := RuntimeOperation{ID: "write-outside", Kind: StepKindFileWrite, TargetRefs: []string{"outside.go"}}
	outsideReceipt := targetAuthorizationAcceptedReceipt(outsideOperation, RuntimeEffectWorkspaceDurable)
	err = MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, outsideOperation, outsideReceipt)
	var outsideFinding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &outsideFinding) || outsideFinding.Rule != runtimeOperationTargetRuleOutside {
		t.Fatalf("prospective outside finding=%+v err=%v", outsideFinding, err)
	}
}

func TestRuntimeOperationTargetAuthorizationProofProjectionDoesNotInventConstraint(t *testing.T) {
	binding := targetAuthorizationStateChangeBinding(
		"binding/prospective-projection", []string{StepKindFileWrite}, nil, []string{"main.go", "semantic_test.go"},
	)
	identity, err := DeriveCoverageEvidenceProofIdentity(binding, EvidenceKindStateChange)
	if err != nil {
		t.Fatal(err)
	}
	fromIdentity, err := RuntimeOperationTargetAuthorizationFromProofIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	fromBinding, err := RuntimeOperationTargetAuthorizationFromCoverageEvidenceBinding(binding)
	if err != nil {
		t.Fatal(err)
	}
	for name, authorization := range map[string]RuntimeOperationTargetAuthorization{
		"proof identity": fromIdentity,
		"binding":        fromBinding,
	} {
		t.Run(name, func(t *testing.T) {
			if authorization.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
				len(authorization.ProspectiveConstraintRefs) != 0 ||
				!sameStrings(authorization.OwnerSelectorRefs, []string{binding.TargetSelectorRef}) ||
				!sameStrings(authorization.ProspectiveTargetRefs, binding.ProspectiveTargetRefs) {
				t.Fatalf("prospective target projection invented or lost proof facts: %+v", authorization)
			}
			if err := ValidateRuntimeOperationTargetsAgainstAuthorization(authorization, []string{"main.go"}, true); err != nil {
				t.Fatalf("projected singleton membership rejected: %v", err)
			}
		})
	}
}

func targetAuthorizationStateChangeBinding(id string, operationKinds, exactTargetRefs, prospectiveTargetRefs []string) CoverageEvidenceBinding {
	binding := CoverageEvidenceBinding{
		ID: id, CoverageWorkItemID: "cwi-target-auth", FocusID: "focus-target-auth", DefinitionRevision: "definition-target-auth",
		Proofs: []CoverageEvidenceProof{{
			EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: operationKinds,
		}},
		TargetRefs: exactTargetRefs, DeclarationRef: "declaration/target-auth",
	}
	if len(prospectiveTargetRefs) != 0 {
		binding.TargetRefs = nil
		binding.TargetSelectorRef = "file/workspace-relative-create"
		binding.TargetSelectorOwnerRef = RuntimeModuleFile
		binding.TargetSelectorSubjectRef = "workspace-deliverable"
		binding.TargetSelectorCandidateRefs = append([]string(nil), prospectiveTargetRefs...)
		binding.ProspectiveTargetRefs = append([]string(nil), prospectiveTargetRefs...)
	}
	binding = binding.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	return binding
}

func targetAuthorizationAcceptedReceipt(operation RuntimeOperation, effect RuntimeEffectScope) RuntimeOperationReceipt {
	operation = operation.Normalized()
	return RuntimeOperationReceipt{
		ExecutionKey: "execution/target-auth/" + operation.ID, Fence: 1, OperationID: operation.ID,
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: effect, Changed: true,
		TargetRefs: append([]string(nil), operation.TargetRefs...),
		Evidence:   RuntimeOperationEvidence{Quality: common.EvidenceQualityValid},
	}
}

func TestRuntimeOperationRequiresTargetRefsParity(t *testing.T) {
	testCases := []struct {
		name              string
		kind              string
		target            RuntimeOperationTargetAuthorization
		evidenceKind      string
		want              bool
		checkSM3          bool
		skipBindingLookup bool
	}{
		{
			name:         "managed_validation_exact_target_non_empty",
			kind:         RuntimeBlockKindManagedValidationSession,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"tests/input_test.go"}},
			evidenceKind: EvidenceKindValidation,
			want:         true,
			checkSM3:     true,
		},
		{
			name:              "managed_validation_exact_target_empty",
			kind:              RuntimeBlockKindManagedValidationSession,
			target:            RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget},
			evidenceKind:      EvidenceKindValidation,
			want:              false,
			skipBindingLookup: true,
		},
		{
			name:         "managed_validation_existing_owner_scope_finite",
			kind:         RuntimeBlockKindManagedValidationSession,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"tests/input_test.go"}, OwnerSelectorRefs: []string{"shell/validation"}},
			evidenceKind: EvidenceKindValidation,
			want:         true,
		},
		{
			name:         "managed_validation_existing_owner_scope_selector_only",
			kind:         RuntimeBlockKindManagedValidationSession,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"shell/validation"}},
			evidenceKind: EvidenceKindValidation,
			want:         false,
			checkSM3:     true,
		},
		{
			name:         "git_write_selector_only_existing_owner_scope",
			kind:         StepKindGitWrite,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"workspace-repository"}},
			evidenceKind: EvidenceKindStateChange,
			want:         false,
		},
		{
			name:         "mcp_tool_call_selector_only_existing_owner_scope",
			kind:         StepKindMCPToolCall,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"server-a"}},
			evidenceKind: EvidenceKindValidation,
			want:         false,
		},
		{
			name:         "file_write_always_requires_target",
			kind:         StepKindFileWrite,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"file/workspace_deliverable"}},
			evidenceKind: EvidenceKindStateChange,
			want:         true,
		},
		{
			name:         "patch_apply_exact_target",
			kind:         RuntimeBlockKindPatchApply,
			target:       RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"}},
			evidenceKind: EvidenceKindStateChange,
			want:         true,
			checkSM3:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			contract, ok := RuntimeOperationContractForKind(tc.kind)
			if !ok {
				t.Fatalf("missing contract for %q", tc.kind)
			}
			got := RuntimeOperationRequiresTargetRefs(contract, tc.target)
			if got != tc.want {
				t.Fatalf("RuntimeOperationRequiresTargetRefs = %t, want %t", got, tc.want)
			}

			effect := RuntimeEffectWorkspaceDurable
			state := ConvergenceStateNeedStateChange
			var validationObs *RuntimeValidationObservation
			recipe := RuntimeValidationRecipe{ID: "recipe/parity", Command: "sh", Args: []string{"-c", "true"}}
			declarationRef := CanonicalRuntimeDefinitionEvidenceDeclarationRef("test", "cwi-parity")
			proof := CoverageEvidenceProof{
				EvidenceKind:          tc.evidenceKind,
				AllowedOperationKinds: []string{tc.kind},
				RequiredEffectScopes:  []RuntimeEffectScope{effect},
			}
			if tc.evidenceKind == EvidenceKindValidation {
				effect = RuntimeEffectNone
				state = ConvergenceStateNeedValidation
				proof.RequiredEffectScopes = []RuntimeEffectScope{effect}
				proof.ValidationRecipeID = recipe.ID
				proof.RequiredCheckRefs = []string{recipe.ID}
				validationObs = &RuntimeValidationObservation{
					RecipeID:       recipe.ID,
					ExecutedChecks: []string{recipe.ID},
					PassedChecks:   []string{recipe.ID},
				}
				if tc.kind == RuntimeBlockKindManagedValidationSession {
					item := RuntimeCoverageItem{ID: "cwi-parity", SourceScope: "test", SourceID: "cwi-parity", TargetText: "parity test proof"}
					requirement := runtimeValidationProofRequirementForTest(t, "def-parity", item, declarationRef, recipe.ID, nil)
					attestations := runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)
					proof.ValidationProofRequirementHash = requirement.Hash
					proof.RequiredCheckRefs = append([]string(nil), requirement.RequiredCheckRefs...)
					proof.ValidationCheckAttestations = attestations
					declarationRef = requirement.DeclarationRef
					validationObs = &RuntimeValidationObservation{
						RecipeID:               recipe.ID,
						ObservationProtocolRef: attestations[0].ObservationProtocolRef,
						ExecutedChecks:         []string{attestations[0].NativeCheckRef},
						PassedChecks:           []string{attestations[0].NativeCheckRef},
					}
				}
			}

			binding := CoverageEvidenceBinding{
				ID:                 "binding/" + tc.name,
				CoverageWorkItemID: "cwi-parity",
				FocusID:            "focus-parity",
				DefinitionRevision: "def-parity",
				DeclarationRef:     declarationRef,
				Proofs:             []CoverageEvidenceProof{proof},
			}
			switch tc.target.Policy {
			case RuntimeOperationTargetPolicyExactTarget:
				binding.TargetRefs = append([]string(nil), tc.target.ExactTargetRefs...)
			case RuntimeOperationTargetPolicyExistingOwnerScope:
				binding.TargetSelectorRef = tc.target.OwnerSelectorRefs[0]
				binding.TargetSelectorOwnerRef = RuntimeModuleForOperationKind(tc.kind)
				binding.TargetSelectorSubjectRef = "subject-parity"
				binding.TargetRefs = append([]string(nil), tc.target.OwnerScopeTargetRefs...)
			}
			binding = binding.Normalized()
			binding.Hash = StableCoverageEvidenceBindingHash(binding)

			if tc.checkSM3 {
				decision := RuntimeDecision{
					State:             state,
					AuthoritySnapshot: AuthoritySnapshot{DefinitionRevision: "def-parity"},
					Focus: CoverageFocus{
						ID:                       "focus-parity",
						CoverageWorkItemIDs:      []common.CoverageWorkItemID{"cwi-parity"},
						RequiredEvidenceKinds:    []string{tc.evidenceKind},
						TargetRefs:               append([]string(nil), binding.TargetRefs...),
						TargetSelectorRef:        binding.TargetSelectorRef,
						TargetSelectorOwnerRef:   binding.TargetSelectorOwnerRef,
						TargetSelectorSubjectRef: binding.TargetSelectorSubjectRef,
						CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
					},
					RequiredOutput: RequiredOutputContract{
						AcceptedKinds:         []string{tc.kind},
						AcceptedEffectScopes:  []RuntimeEffectScope{effect},
						RequiredEffectScopes:  []RuntimeEffectScope{effect},
						RequiredEvidenceKinds: []string{tc.evidenceKind},
						CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-parity"},
						TargetRefs:            append([]string(nil), tc.target.ExactTargetRefs...),
					},
				}
				var ownerFacts RuntimeCandidateManifestOwnerFacts
				if tc.kind == RuntimeBlockKindManagedValidationSession {
					ownerFacts.ValidationRecipes = []RuntimeValidationRecipe{recipe}
				}
				manifest, err := BuildRuntimeCandidateContractManifest(decision, ownerFacts)
				if err != nil {
					t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
				}
				if len(manifest.Operations) != 1 || manifest.Operations[0].RequireTargetRefs != tc.want {
					t.Fatalf("SM-3 manifest RequireTargetRefs = %t, want %t (%+v)", manifest.Operations[0].RequireTargetRefs, tc.want, manifest.Operations)
				}
			}

			if tc.skipBindingLookup {
				if err := ValidateRuntimeOperationTargetsAgainstAuthorization(tc.target, nil, got); err != nil {
					t.Fatalf("empty exact target with requireTargets=%t rejected: %v", got, err)
				}
				return
			}

			emptyOp := RuntimeOperation{ID: "op-empty", Kind: tc.kind}
			emptyReceipt := RuntimeOperationReceipt{
				ExecutionKey: "execution/parity/" + emptyOp.ID, Fence: 1, OperationID: emptyOp.ID,
				Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: effect, Changed: true,
				Evidence: RuntimeOperationEvidence{Quality: common.EvidenceQualityValid, Validation: validationObs},
			}
			err := MatchCoverageEvidenceBindingReceipt(binding, tc.evidenceKind, emptyOp, emptyReceipt)
			if tc.want {
				if err == nil {
					t.Fatalf("SM-2 receipt matcher unexpectedly accepted empty targetRefs when RequireTargetRefs=true")
				}
			} else if err != nil {
				t.Fatalf("SM-2 receipt matcher rejected empty targetRefs when RequireTargetRefs=false: %v", err)
			}
		})
	}
}
