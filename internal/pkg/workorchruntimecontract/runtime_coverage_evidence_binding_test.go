package workorchruntimecontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCoverageEvidenceProofValidateRejectsCrossKindProofShapes(t *testing.T) {
	validState := CoverageEvidenceProof{
		EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		AllowedOperationKinds: []string{StepKindFileWrite},
	}
	validValidation := CoverageEvidenceProof{
		EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
		AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe/test",
	}
	cases := []struct {
		name  string
		proof CoverageEvidenceProof
	}{
		{name: "state proof cannot be non-durable", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, AllowedOperationKinds: []string{StepKindFileWrite}}},
		{name: "validation proof cannot be durable", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe/test"}},
		{name: "validation proof requires recipe", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}}},
		{name: "validation proof cannot use state operation", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, AllowedOperationKinds: []string{StepKindFileWrite}, ValidationRecipeID: "recipe/test"}},
		{name: "validation proof cannot use read-only diagnostic operation", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, AllowedOperationKinds: []string{StepKindFileRead}, ValidationRecipeID: "recipe/test"}},
		{name: "state proof cannot use validation operation", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}}},
		{name: "variant must belong to proof operation kind", proof: CoverageEvidenceProof{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{StepKindFileWrite}, AllowedOperationVariants: []RuntimeOperationVariantRef{{Kind: StepKindGitWrite, VariantID: "commit"}}}},
	}
	if err := validState.Validate(); err != nil {
		t.Fatalf("valid state proof rejected: %v", err)
	}
	if err := validValidation.Validate(); err != nil {
		t.Fatalf("valid validation proof rejected: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.proof.Validate(); err == nil {
				t.Fatalf("invalid proof accepted: %+v", tc.proof)
			}
		})
	}
}

func TestRuntimeOperationKindSupportsCoverageEvidenceRequiresOwnerProofCapability(t *testing.T) {
	if !RuntimeOperationKindSupportsCoverageEvidence(StepKindFileWrite, EvidenceKindStateChange) {
		t.Fatal("durable File write must remain an owner-verifiable state-change proof")
	}
	if !RuntimeOperationKindSupportsCoverageEvidence(RuntimeBlockKindManagedValidationSession, EvidenceKindValidation) {
		t.Fatal("managed validation must remain an owner-verifiable validation proof")
	}
	if !RuntimeOperationKindSupportsCoverageEvidence(StepKindMCPToolCall, EvidenceKindValidation) {
		t.Fatal("MCP tool call with an owner-attested server/tool recipe must be a validation proof")
	}
	for _, kind := range []string{StepKindFileRead, StepKindGitRead, RuntimeBlockKindPatchPreview, StepKindShellCommand, StepKindMCPResourceRead, StepKindMCPPromptGet} {
		if RuntimeOperationKindSupportsCoverageEvidence(kind, EvidenceKindValidation) {
			t.Fatalf("diagnostic operation %q must not be elevated to validation coverage proof", kind)
		}
	}
}

func TestCoverageEvidenceBindingSealsSelectorCandidateRefs(t *testing.T) {
	binding := CoverageEvidenceBinding{
		ID: "binding-selector", CoverageWorkItemID: "cwi-selector", FocusID: "focus-selector", DefinitionRevision: "definition-selector",
		Proofs: []CoverageEvidenceProof{{
			EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{StepKindFileWrite},
		}},
		TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: RuntimeModuleFile,
		TargetSelectorSubjectRef: "deliverable", TargetSelectorCandidateRefs: []string{"internal/main.go", "go.mod", "go.mod"},
		DeclarationRef: "profile/selector",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	if !SameRuntimeTargetSelectorCandidateRefs(binding.TargetSelectorCandidateRefs, []string{"go.mod", "internal/main.go"}) {
		t.Fatalf("normalized selector candidates = %#v", binding.TargetSelectorCandidateRefs)
	}
	cloned := binding.Clone()
	cloned.TargetSelectorCandidateRefs[0] = "mutated.go"
	if binding.TargetSelectorCandidateRefs[0] == "mutated.go" {
		t.Fatal("binding clone aliases selector candidate refs")
	}
	changed := binding.Clone()
	changed.TargetSelectorCandidateRefs = []string{"other.go"}
	if StableCoverageEvidenceBindingHash(changed) == binding.Hash {
		t.Fatal("binding hash did not retain selector candidate refs")
	}
	invalid := binding.Clone()
	invalid.TargetSelectorRef = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("selector candidates without a selector were accepted")
	}
}

func TestValidateCoverageEvidenceBindingsForFocusRejectsSharedSemanticProof(t *testing.T) {
	focus := CoverageFocus{
		ID: "focus-shared", SliceID: "slice-presentation", EvidenceDeclarationRef: "profile/dodItem/dod-001",
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-001", "cwi-002"},
		RequiredEvidenceKinds: []string{EvidenceKindValidation},
	}
	if err := ValidateRuntimeFocusEvidenceDeclaration(focus); err == nil || !strings.Contains(err.Error(), "exactly one CWI") {
		t.Fatalf("shared semantic focus validation = %v, want one-CWI rejection", err)
	}
	focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-001"}
	focus.EvidenceDeclarationRef = "profile/dodItem/*"
	if err := ValidateRuntimeFocusEvidenceDeclaration(focus); err == nil || !strings.Contains(err.Error(), "CWI-specific declaration") {
		t.Fatalf("wildcard semantic focus validation = %v, want declaration rejection", err)
	}
}

func TestMCPToolCoverageEvidenceBindingRequiresExactOwnerRecipe(t *testing.T) {
	recipeID := RuntimeMCPToolValidationRecipeID("mcp-verify", "repo.summary")
	if recipeID != "mcp/tool_call/mcp-verify/repo.summary" {
		t.Fatalf("MCP recipe identity = %q", recipeID)
	}
	if RuntimeMCPToolValidationRecipeID("", "repo.summary") != "" || RuntimeMCPToolValidationRecipeID("mcp-verify", "") != "" {
		t.Fatal("incomplete MCP tool identity must not manufacture a recipe")
	}
	binding := CoverageEvidenceBinding{
		ID: "binding/mcp-summary", CoverageWorkItemID: "cwi-mcp-summary", FocusID: "focus-mcp", DefinitionRevision: "definition-mcp",
		Proofs: []CoverageEvidenceProof{{
			EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
			AllowedOperationKinds: []string{StepKindMCPToolCall}, ValidationRecipeID: recipeID, RequiredCheckRefs: []string{recipeID},
		}},
		TargetSelectorRef: "mcp-server-selector", TargetSelectorOwnerRef: "blocks/mcp", TargetSelectorSubjectRef: "mcp-verify",
		DeclarationRef: "test/profile/mcp-summary",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	operation := RuntimeOperation{ID: "mcp-summary", Kind: StepKindMCPToolCall}
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/mcp-summary", Fence: 1, OperationID: operation.ID,
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectNone,
		Evidence: RuntimeOperationEvidence{Quality: common.EvidenceQualityValid, Validation: &RuntimeValidationObservation{
			RecipeID: recipeID, ExecutedChecks: []string{recipeID}, PassedChecks: []string{recipeID},
		}},
	}
	if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, operation, receipt); err != nil {
		t.Fatalf("exact MCP owner validation proof rejected: %v", err)
	}
	for _, mismatch := range []struct {
		name   string
		mutate func(*RuntimeOperationReceipt)
	}{
		{name: "server", mutate: func(actual *RuntimeOperationReceipt) {
			actual.Evidence.Validation.RecipeID = RuntimeMCPToolValidationRecipeID("mcp-other", "repo.summary")
		}},
		{name: "tool", mutate: func(actual *RuntimeOperationReceipt) {
			actual.Evidence.Validation.RecipeID = RuntimeMCPToolValidationRecipeID("mcp-verify", "repo.other")
		}},
		{name: "check", mutate: func(actual *RuntimeOperationReceipt) {
			actual.Evidence.Validation.PassedChecks = []string{"mcp/tool_call/mcp-verify/repo.other"}
		}},
		{name: "rejected receipt", mutate: func(actual *RuntimeOperationReceipt) { actual.Outcome = RuntimeOperationOutcomeRejected }},
		{name: "zero work", mutate: func(actual *RuntimeOperationReceipt) { actual.Evidence.ZeroWork = true }},
		{name: "inconclusive", mutate: func(actual *RuntimeOperationReceipt) { actual.Evidence.Quality = common.EvidenceQualityInconclusive }},
	} {
		t.Run(mismatch.name, func(t *testing.T) {
			actual := receipt
			observation := receipt.Evidence.Validation.Clone()
			actual.Evidence.Validation = &observation
			mismatch.mutate(&actual)
			if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, operation, actual); err == nil {
				t.Fatalf("MCP %s identity mismatch received coverage credit", mismatch.name)
			}
		})
	}
}

func TestCoverageEvidenceBindingReceiptMatchDoesNotBorrowAcrossProofKinds(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	stateOperation := RuntimeOperation{ID: "write-main", Kind: StepKindFileWrite, TargetRefs: []string{"main.go"}}
	stateReceipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/write-main", Fence: 1, OperationID: "write-main", Outcome: RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"main.go"},
		Evidence: RuntimeOperationEvidence{Summary: "wrote main.go"},
	}
	if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, stateOperation, stateReceipt); err != nil {
		t.Fatalf("state proof must not require validation facts: %v", err)
	}

	validationOperation := RuntimeOperation{ID: "validate-main", Kind: RuntimeBlockKindManagedValidationSession, TargetRefs: []string{"main.go"}}
	validationReceipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/validate-main", Fence: 1, OperationID: "validate-main", Outcome: RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: RuntimeEffectNone, TargetRefs: []string{"main.go"},
		Evidence: RuntimeOperationEvidence{
			Summary: "validated main.go", EvidenceRefs: []string{"artifact/test-log"}, EvidenceTags: []string{"metric/tests-pass"},
			Validation: &RuntimeValidationObservation{RecipeID: "recipe/test-main", ExecutedChecks: []string{"unit/main"}, PassedChecks: []string{"unit/main"}},
		},
	}
	if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, validationOperation, validationReceipt); err != nil {
		t.Fatalf("exact validation proof rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*RuntimeOperationReceipt)
		message string
	}{
		{name: "target", mutate: func(receipt *RuntimeOperationReceipt) { receipt.TargetRefs = []string{"other.go"} }, message: "targets"},
		{name: "effect", mutate: func(receipt *RuntimeOperationReceipt) { receipt.ObservedEffectScope = RuntimeEffectEphemeralSession }, message: "effect"},
		{name: "recipe", mutate: func(receipt *RuntimeOperationReceipt) { receipt.Evidence.Validation.RecipeID = "recipe/other" }, message: "recipe"},
		{name: "check", mutate: func(receipt *RuntimeOperationReceipt) {
			receipt.Evidence.Validation.PassedChecks = []string{"unit/other"}
		}, message: "passed check"},
		{name: "metric", mutate: func(receipt *RuntimeOperationReceipt) { receipt.Evidence.EvidenceTags = nil }, message: "metric constraint"},
		{name: "artifact", mutate: func(receipt *RuntimeOperationReceipt) { receipt.Evidence.EvidenceRefs = nil }, message: "artifact ref"},
		{name: "zero work", mutate: func(receipt *RuntimeOperationReceipt) { receipt.Evidence.ZeroWork = true }, message: "zero-work"},
		{name: "invalid quality", mutate: func(receipt *RuntimeOperationReceipt) { receipt.Evidence.Quality = common.EvidenceQualityInvalid }, message: "weak owner evidence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			receipt := validationReceipt
			observation := validationReceipt.Evidence.Validation.Clone()
			receipt.Evidence.Validation = &observation
			tc.mutate(&receipt)
			err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, validationOperation, receipt)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("%s mismatch err=%v, want %q", tc.name, err, tc.message)
			}
		})
	}

	unchangedStateReceipt := stateReceipt
	unchangedStateReceipt.Changed = false
	if err := MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindStateChange, stateOperation, unchangedStateReceipt); err == nil || !strings.Contains(err.Error(), "changed owner receipt") {
		t.Fatalf("unchanged durable receipt received state-change credit: %v", err)
	}
}

func TestCoverageEvidenceBindingManifestOnlyExposesCurrentEvidenceKind(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	focus := CoverageFocus{
		ID: "focus-main", SliceID: "slice-main", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-main"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}, TargetRefs: []string{"main.go"},
		CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
	}
	stateDecision := testCoverageBindingDecision(focus, ConvergenceStateNeedStateChange, []string{StepKindFileWrite}, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, EvidenceKindStateChange)
	stateManifest, err := BuildRuntimeCandidateContractManifest(stateDecision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatalf("state manifest: %v", err)
	}
	if len(stateManifest.Operations) != 1 || !sameStrings(stateManifest.Operations[0].AllowedBindingRefs, []string{binding.ID}) {
		t.Fatalf("state manifest binding refs = %+v", stateManifest.Operations)
	}

	validationDecision := testCoverageBindingDecision(focus, ConvergenceStateNeedValidation, []string{RuntimeBlockKindManagedValidationSession}, []RuntimeEffectScope{RuntimeEffectNone}, EvidenceKindValidation)
	validationManifest, err := BuildRuntimeCandidateContractManifest(validationDecision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatalf("validation manifest: %v", err)
	}
	if len(validationManifest.Operations) != 1 || !sameStrings(validationManifest.Operations[0].AllowedBindingRefs, []string{binding.ID}) {
		t.Fatalf("validation manifest binding refs = %+v", validationManifest.Operations)
	}
	if refs := CoverageEvidenceBindingRefsForOperationEvidenceKind([]CoverageEvidenceBinding{binding}, StepKindFileWrite, EvidenceKindValidation); len(refs) != 0 {
		t.Fatalf("state operation leaked into validation proof: %v", refs)
	}
	if refs := CoverageEvidenceBindingRefsForOperationEvidenceKind([]CoverageEvidenceBinding{binding}, RuntimeBlockKindManagedValidationSession, EvidenceKindStateChange); len(refs) != 0 {
		t.Fatalf("validation operation leaked into state proof: %v", refs)
	}
}

func TestCurrentBindingManifestRejectsRawCoverageIDsAndUnknownBindingRefs(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	focus := CoverageFocus{
		ID: "focus-main", SliceID: "slice-main", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-main"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}, TargetRefs: []string{"main.go"},
		CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(testCoverageBindingDecision(
		focus, ConvergenceStateNeedStateChange, []string{StepKindFileWrite}, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, EvidenceKindStateChange,
	), RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatalf("state manifest: %v", err)
	}
	if len(manifest.Bindings) == 0 {
		t.Fatalf("current binding manifest must expose sealed binding authority: %+v", manifest)
	}
	cases := []struct {
		name    string
		payload string
		path    string
		actual  string
	}{
		{
			name:    "raw coverage is forbidden",
			payload: `{"operations":[{"id":"write-main","kind":"file/write","coverageWorkItemIds":["cwi-main"],"targetRefs":["main.go"],"parameters":[{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].coverageWorkItemIds", actual: "coverageWorkItemIds",
		},
		{
			name:    "unknown binding is forbidden",
			payload: `{"operations":[{"id":"write-main","kind":"file/write","bindingRefs":["binding/unknown"],"targetRefs":["main.go"],"parameters":[{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].bindingRefs", actual: "binding/unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(tc.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Path != tc.path || diagnostic.Actual != tc.actual {
				t.Fatalf("diagnostic = %+v, err=%v; want path=%q actual=%q", diagnostic, err, tc.path, tc.actual)
			}
		})
	}
	valid := []byte(`{"operations":[{"id":"write-main","kind":"file/write","bindingRefs":["binding/cwi-main"],"targetRefs":["main.go"],"parameters":[{"name":"content","string":"package main"}]}]}`)
	if _, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, valid); err != nil {
		t.Fatalf("sealed binding reference rejected: %v", err)
	}
}

func TestCurrentBindingManifestRejectsMultipleBindingRefsForOneOperation(t *testing.T) {
	first := testCoverageEvidenceBinding()
	first.ID = "binding/cwi-first"
	first.CoverageWorkItemID = "cwi-first"
	first.FocusID = "focus-multi"
	first.Proofs = []CoverageEvidenceProof{first.Proofs[0]}
	first.Hash = StableCoverageEvidenceBindingHash(first)
	second := first
	second.ID = "binding/cwi-second"
	second.CoverageWorkItemID = "cwi-second"
	second.Hash = StableCoverageEvidenceBindingHash(second)
	focus := CoverageFocus{
		ID: "focus-multi", SliceID: "slice-presentation", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-first", "cwi-second"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, TargetRefs: []string{"main.go"},
		CoverageEvidenceBindings: []CoverageEvidenceBinding{first, second},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(testCoverageBindingDecision(
		focus, ConvergenceStateNeedStateChange, []string{StepKindFileWrite}, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, EvidenceKindStateChange,
	), RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatalf("state manifest: %v", err)
	}
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "write-main", Kind: StepKindFileWrite, BindingRefs: []string{first.ID, second.ID}, TargetRefs: []string{"main.go"},
		Parameters: []OperationParameter{{Name: "content", String: stringPtr("package main")}},
	}}}}}
	err = ValidateRuntimeCandidateAgainstManifest(manifest, candidate)
	var diagnostic *RuntimeCandidateContractDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.operations[0].bindingRefs" || !strings.Contains(diagnostic.Error(), "exactly one bindingRef") {
		t.Fatalf("multi-binding candidate diagnostic=%+v err=%v", diagnostic, err)
	}
}

func TestCoverageEvidenceAttributionRequiresBindingForCurrentSchemaEvidence(t *testing.T) {
	first := testCoverageEvidenceBinding()
	first.ID = "binding/cwi-first"
	first.CoverageWorkItemID = "cwi-first"
	first.FocusID = "focus-multi"
	first.Hash = StableCoverageEvidenceBindingHash(first)
	second := first
	second.ID = "binding/cwi-second"
	second.CoverageWorkItemID = "cwi-second"
	second.Hash = StableCoverageEvidenceBindingHash(second)

	facts := RuntimeFacts{
		WorkspaceID: "ws-binding", RunID: "run-binding", DefinitionRevision: "definition-main", EvidenceRevision: "evidence-1",
		Focus: CoverageFocus{
			ID: "focus-multi", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-first", "cwi-second"},
			RequiredEvidenceKinds:    []string{EvidenceKindStateChange, EvidenceKindValidation},
			CoverageEvidenceBindings: []CoverageEvidenceBinding{first, second},
		},
		Evidence: EvidenceLedger{Accepted: []EvidenceFact{
			// An unbound validation receipt must not stand in for the second CWI,
			// even when this is the final focus.
			{ID: "second-unbound-validation", Kind: EvidenceKindValidation, DispatchID: "dispatch-second", OperationID: "validate-second", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-second"}},
		}},
	}.Normalized()

	if EvidenceAppliesToCoverageWorkItem(facts.Focus, facts.Evidence.Accepted[0], "cwi-second") {
		t.Fatalf("unbound current-schema evidence was eligible for CWI credit: %+v", facts.Evidence.Accepted[0])
	}
	if missing := MissingRequiredEvidenceKinds(facts); len(missing) != 2 || missing[0] != EvidenceKindStateChange || missing[1] != EvidenceKindValidation {
		t.Fatalf("unbound CWI evidence must not satisfy any proof kind: %#v", missing)
	}

	facts.Evidence.Accepted[0].BindingRefs = []string{second.ID}
	facts = facts.Normalized()
	if !EvidenceAppliesToCoverageWorkItem(facts.Focus, facts.Evidence.Accepted[0], "cwi-second") {
		t.Fatalf("binding-derived current evidence was not attributed to CWI: %+v", facts.Evidence.Accepted[0])
	}
	if missing := MissingRequiredEvidenceKinds(facts); len(missing) != 1 || missing[0] != EvidenceKindStateChange {
		t.Fatalf("binding-derived validation changed non-credit classification unexpectedly: %#v", missing)
	}
}

func testCoverageEvidenceBinding() CoverageEvidenceBinding {
	binding := CoverageEvidenceBinding{
		ID: "binding/cwi-main", CoverageWorkItemID: "cwi-main", FocusID: "focus-main", DefinitionRevision: "definition-main",
		Proofs: []CoverageEvidenceProof{
			{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{StepKindFileWrite}},
			{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe/test-main", RequiredCheckRefs: []string{"unit/main"}, MetricConstraints: []string{"metric/tests-pass"}, ArtifactRefs: []string{"artifact/test-log"}},
		},
		TargetRefs: []string{"main.go"}, DeclarationRef: "test/profile/main",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	return binding
}

func testCoverageBindingDecision(focus CoverageFocus, state ConvergenceState, kinds []string, scopes []RuntimeEffectScope, evidenceKind string) RuntimeDecision {
	return RuntimeDecision{
		State: state, Focus: focus,
		AuthoritySnapshot: AuthoritySnapshot{DefinitionRevision: "definition-main"},
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds: kinds, AcceptedEffectScopes: scopes, RequiredEffectScopes: scopes,
			RequiredEvidenceKinds: []string{evidenceKind}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-main"}, TargetRefs: []string{"main.go"},
		},
	}
}
