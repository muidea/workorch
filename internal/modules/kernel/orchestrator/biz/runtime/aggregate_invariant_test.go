package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateWireDiagnosticRequiresExactPersistedManifestPreimage(t *testing.T) {
	t.Parallel()
	aggregate, _ := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	secondOperation := manifest.Operations[0].Normalized()
	secondOperation.Kind = runtimecontract.RuntimeBlockKindPatchPreview
	previewContract, _ := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchPreview)
	secondOperation.ValueRules = previewContract.ValueRules
	secondOperation.Variants = previewContract.Variants
	manifest.Operations = append(manifest.Operations, secondOperation)
	manifest.Hash = ""
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("wire diagnostic fixture has no patch/apply rule")
	}
	wireRule, err := runtimecontract.RuntimeCandidateWireRuleIdentityForOperation(wire, rule, 0)
	if err != nil {
		t.Fatal(err)
	}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "parameters must be an array",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].parameters", Actual: "object", Allowed: []string{"array"}, Required: []string{"parameters"},
			WireRule: wireRule, ManifestHash: manifest.Hash,
		},
	}
	sealedFailure, correction, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealedFailure == nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce {
		t.Fatalf("wire diagnostic fixture did not seal: failure=%+v correction=%+v err=%v", sealedFailure, correction, err)
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	attempt := runtimecontract.RuntimeCandidateAttemptRecord{
		Transaction: transaction, Attempt: 0, StablePrefixHash: "wire-prefix", ManifestHash: manifest.Hash,
		LogicalRequestID: "wire-request", State: runtimecontract.RuntimeCandidateAttemptStateFailed,
		Failure: sealedFailure, Correction: correction,
	}
	manifestCopy := manifest.Clone()
	session := runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: attempt.LogicalRequestID, Transaction: transaction, StablePrefixHash: attempt.StablePrefixHash,
		ManifestHash: manifest.Hash, State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, CandidateManifest: &manifestCopy,
	}
	if err := validateRuntimeCandidateAttemptSessionBindings([]runtimecontract.RuntimeCandidateAttemptRecord{attempt}, []runtimecontract.RuntimeCandidateRequestSession{session}); err != nil {
		t.Fatalf("exact persisted wire diagnostic preimage was rejected: %v", err)
	}

	drifted := attempt.Clone()
	drifted.Failure.Diagnostic.WireRule.WireContractHash = "wire-contract-drift"
	if err := validateRuntimeCandidateAttemptSessionBindings([]runtimecontract.RuntimeCandidateAttemptRecord{drifted}, []runtimecontract.RuntimeCandidateRequestSession{session}); err == nil || !strings.Contains(err.Error(), "wire diagnostic lineage") {
		t.Fatalf("wire diagnostic drift survived fresh-load binding: %v", err)
	}
	missingPreimage := session.Clone()
	missingPreimage.CandidateManifest = nil
	if err := validateRuntimeCandidateAttemptSessionBindings([]runtimecontract.RuntimeCandidateAttemptRecord{attempt}, []runtimecontract.RuntimeCandidateRequestSession{missingPreimage}); err == nil || !strings.Contains(err.Error(), "no persisted manifest preimage") {
		t.Fatalf("wire diagnostic without manifest preimage survived fresh load: %v", err)
	}
	missingIdentity := attempt.Clone()
	missingIdentity.Failure.Diagnostic.WireRule = nil
	if err := validateRuntimeCandidateAttemptSessionBindings([]runtimecontract.RuntimeCandidateAttemptRecord{missingIdentity}, []runtimecontract.RuntimeCandidateRequestSession{session}); err == nil || !strings.Contains(err.Error(), "missing exact wire identity") {
		t.Fatalf("rule-selected schema diagnostic without wire identity survived fresh load: %v", err)
	}
}

func TestRuntimeAggregateEvidenceCommitLedgerRejectsAcceptedFactDriftAfterFreshLoad(t *testing.T) {
	baseline := validationAggregate(t)
	if err := baseline.Validate(); err != nil {
		t.Fatalf("valid accepted receipt aggregate rejected: %v", err)
	}
	if len(baseline.Facts.Evidence.Accepted) != 1 || len(baseline.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) == 0 {
		t.Fatalf("accepted receipt fixture lacks exact CWI credit: %+v", baseline.Facts.Evidence.Accepted)
	}
	factID := baseline.Facts.Evidence.Accepted[0].ID

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeAggregate)
		wantErr string
	}{
		{
			name: "deleted fact",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted = nil
			},
			wantErr: "must exist exactly once",
		},
		{
			name: "moved to rejected ledger",
			mutate: func(value *RuntimeAggregate) {
				fact := value.Facts.Evidence.Accepted[0]
				value.Facts.Evidence.Accepted = nil
				value.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{fact}
			},
			wantErr: "accepted/rejected ledger",
		},
		{
			name: "focus drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].FocusID = "focus-forged"
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "dispatch/focus/authority/operation",
		},
		{
			name: "receipt payload drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].Summary = "coordinated forged summary"
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "payload does not match",
		},
		{
			name: "binding drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].BindingRefs = []string{"binding/forged"}
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "binding refs do not match",
		},
		{
			name: "cleared CWI credit with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].CoverageWorkItemIDs = nil
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "coverage does not match",
		},
		{
			name: "reason drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].EvidenceReason = "coordinated forged reason"
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "reason does not match",
		},
		{
			name: "duplicate processed fence",
			mutate: func(value *RuntimeAggregate) {
				value.ProcessedEventIDs = append(value.ProcessedEventIDs, string(factID))
			},
			wantErr: "one processed-event fence",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			test.mutate(&value)
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
			if err := validateRuntimeAggregateEvidenceCommitLedger(value); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load accepted fact error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeAggregateEvidenceCommitLedgerRejectsOrphanOperationFacts(t *testing.T) {
	baseline := validationAggregate(t)
	original := baseline.Facts.Evidence.Accepted[0]
	for _, test := range []struct {
		name     string
		rejected bool
		mutate   func(*runtimecontract.EvidenceFact)
		wantErr  string
	}{
		{name: "accepted orphan", wantErr: "no exact evidence-committed execution"},
		{name: "rejected orphan", rejected: true, wantErr: "no exact evidence-committed execution"},
		{name: "accepted missing focus", mutate: func(fact *runtimecontract.EvidenceFact) { fact.FocusID = "" }, wantErr: "requires focus, dispatch and operation lineage"},
		{name: "accepted missing dispatch", mutate: func(fact *runtimecontract.EvidenceFact) { fact.DispatchID = "" }, wantErr: "requires focus, dispatch and operation lineage"},
		{name: "accepted missing operation", mutate: func(fact *runtimecontract.EvidenceFact) { fact.OperationID = "" }, wantErr: "requires focus, dispatch and operation lineage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			orphan := original
			orphan.ID = common.EvidenceRecordID("runtime-receipt|orphan-" + strings.ReplaceAll(test.name, " ", "-"))
			orphan.Revision = "runtime-evidence-99999999999999999999"
			if test.mutate != nil {
				test.mutate(&orphan)
			}
			if test.rejected {
				value.Facts.Evidence.Rejected = append(value.Facts.Evidence.Rejected, orphan)
			} else {
				value.Facts.Evidence.Accepted = append(value.Facts.Evidence.Accepted, orphan)
			}
			value.ProcessedEventIDs = append(value.ProcessedEventIDs, string(orphan.ID))
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
			if err := validateRuntimeAggregateEvidenceCommitLedger(value); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load orphan fact error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeAggregateEvidenceCommitLedgerRejectsAcceptedValidationFactDrift(t *testing.T) {
	baseline := runtimeTestAcceptedValidationEvidenceAggregate(t)
	if err := baseline.Validate(); err != nil {
		t.Fatalf("valid accepted validation aggregate rejected: %v", err)
	}
	if len(baseline.Facts.Evidence.Accepted) < 2 {
		t.Fatalf("accepted validation fixture has no validation fact: %+v", baseline.Facts.Evidence.Accepted)
	}
	var fact runtimecontract.EvidenceFact
	for _, candidate := range baseline.Facts.Evidence.Accepted {
		if candidate.Kind == runtimecontract.EvidenceKindValidation {
			fact = candidate
			break
		}
	}
	if fact.ID == "" {
		t.Fatalf("accepted validation fixture has no validation fact: %+v", baseline.Facts.Evidence.Accepted)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeAggregate)
		wantErr string
	}{
		{
			name: "deleted validation fact",
			mutate: func(value *RuntimeAggregate) {
				kept := value.Facts.Evidence.Accepted[:0]
				for _, candidate := range value.Facts.Evidence.Accepted {
					if candidate.ID != fact.ID {
						kept = append(kept, candidate)
					}
				}
				value.Facts.Evidence.Accepted = kept
			},
			wantErr: "must exist exactly once",
		},
		{
			name: "validation observation drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				for index := range value.Facts.Evidence.Accepted {
					if value.Facts.Evidence.Accepted[index].ID == fact.ID {
						value.Facts.Evidence.Accepted[index].Validation.RecipeID = "recipe/forged"
					}
				}
				runtimeTestRebindEvidenceFactHash(t, value, fact.ID)
			},
			wantErr: "payload does not match",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			test.mutate(&value)
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
			if err := validateRuntimeAggregateEvidenceCommitLedger(value); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load accepted validation fact error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func runtimeTestAcceptedValidationEvidenceAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	repository := &durableRuntimeStateRepository{aggregate: validationAggregate(t)}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := ExecutionEvidence{
		EventID: "accepted-validation-evidence-invariant", WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindValidation,
		Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("commit accepted validation fixture: %v", err)
	}
	if committed.Disposition != CommitDispositionCommitted {
		t.Fatalf("accepted validation fixture disposition = %q, aggregate=%+v", committed.Disposition, committed.Aggregate)
	}
	validationPersisted := false
	for _, fact := range committed.Aggregate.Facts.Evidence.Accepted {
		if fact.Kind == runtimecontract.EvidenceKindValidation {
			validationPersisted = true
			break
		}
	}
	if len(committed.Aggregate.Facts.Evidence.Accepted) < 2 || !validationPersisted {
		t.Fatalf("accepted validation fixture did not persist validation fact: %+v", committed.Aggregate.Facts.Evidence.Accepted)
	}
	return committed.Aggregate
}

func runtimeTestRebindEvidenceFactHash(t *testing.T, aggregate *RuntimeAggregate, factID common.EvidenceRecordID) {
	t.Helper()
	var fact *runtimecontract.EvidenceFact
	for index := range aggregate.Facts.Evidence.Accepted {
		if aggregate.Facts.Evidence.Accepted[index].ID == factID {
			fact = &aggregate.Facts.Evidence.Accepted[index]
		}
	}
	for index := range aggregate.Facts.Evidence.Rejected {
		if aggregate.Facts.Evidence.Rejected[index].ID == factID {
			fact = &aggregate.Facts.Evidence.Rejected[index]
		}
	}
	if fact == nil {
		t.Fatalf("fact %q is missing", factID)
	}
	factHash := runtimecontract.StableRuntimeEvidenceFactHash(*fact)
	for dispatchIndex := range aggregate.CompletedDispatches {
		for executionIndex := range aggregate.CompletedDispatches[dispatchIndex].OperationExecutions {
			execution := &aggregate.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex]
			if execution.EvidenceFactID == factID {
				execution.EvidenceFactHash = factHash
			}
		}
	}
	if aggregate.ActiveDispatch != nil {
		for executionIndex := range aggregate.ActiveDispatch.OperationExecutions {
			execution := &aggregate.ActiveDispatch.OperationExecutions[executionIndex]
			if execution.EvidenceFactID == factID {
				execution.EvidenceFactHash = factHash
			}
		}
	}
}

func runtimeTestEvidenceAggregateRoundTrip(t *testing.T, aggregate RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	payload, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeAggregate
	if err := json.Unmarshal(payload, &loaded); err != nil {
		t.Fatal(err)
	}
	return loaded
}

func runtimeTestOpaqueProofRequirementBinding(
	t testing.TB,
	requirement runtimecontract.RuntimeValidationProofRequirement,
	recipeID string,
) runtimecontract.RuntimeValidationProofRequirementBinding {
	t.Helper()
	attestations := runtimeTestValidationCheckAttestations(requirement, recipeID)
	binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipeID, attestations...)
	if err != nil {
		t.Fatalf("bind exact validation proof requirement: %v", err)
	}
	return binding
}

func TestRuntimeAggregateShellValidationProofApplicabilitySurvivesSerializationAndFailsClosed(t *testing.T) {
	items := []runtimecontract.RuntimeCoverageItem{
		{ID: "cwi-proof-a", SliceID: "slice-proof", SourceScope: "dodItem", SourceID: "dod-a", TargetText: "first exact requirement"},
		{ID: "cwi-proof-b", SliceID: "slice-proof", SourceScope: "dodItem", SourceID: "dod-b", TargetText: "second exact requirement"},
	}
	focuses := []runtimecontract.RuntimeDefinitionFocus{
		{
			ID: "focus-proof-a", SliceID: "slice-proof", AssessmentScope: "dodItem", AssessmentID: "dod-a", AssessmentText: "first exact requirement",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-proof-a"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-a"), ValidationRecipeID: "shared-proof-recipe", RequiredCheckRefs: []string{"profile-check-a"},
		},
		{
			ID: "focus-proof-b", SliceID: "slice-proof", AssessmentScope: "dodItem", AssessmentID: "dod-b", AssessmentText: "second exact requirement",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-proof-b"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-b"), ValidationRecipeID: "shared-proof-recipe", RequiredCheckRefs: []string{"profile-check-b"},
		},
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition-proof", items, focuses)
	if err != nil || len(requirements) != 2 {
		t.Fatalf("compile exact proof requirements: requirements=%+v err=%v", requirements, err)
	}
	bindings := make([]runtimecontract.RuntimeValidationProofRequirementBinding, 0, len(requirements))
	checks := make([]string, 0)
	for _, requirement := range requirements {
		bindings = append(bindings, runtimeTestOpaqueProofRequirementBinding(t, requirement, requirement.ValidationRecipeID))
		checks = append(checks, requirement.RequiredCheckRefs...)
	}
	shellCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "proof-shell-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds:       []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationRecipeRefs: []string{"shared-proof-recipe"},
		ValidationRecipes:    []runtimecontract.RuntimeValidationRecipe{{ID: "shared-proof-recipe", Command: "true"}},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: "shared-proof-recipe", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:     runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "proof-input-schema", InputBindingRef: "proof-input-binding",
			CheckRefs: checks, ProofApplicability: runtimecontract.RuntimeValidationProofDirect, ProofRequirementBindings: bindings,
			EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}},
		ObservedRevision: "proof-shell-revision",
	}
	shellCatalog = shellCatalog.Normalized()
	shellCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	facts := runtimecontract.RuntimeFacts{
		DefinitionRevision: "definition-proof",
		DefinitionSeed:     runtimecontract.RuntimeDefinitionSeed{CoverageItems: items},
		DefinitionFocuses:  focuses,
		Resources:          runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{shellCatalog}},
	}
	payload, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	var loaded runtimecontract.RuntimeFacts
	if err := json.Unmarshal(payload, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimeValidationProofApplicability(loaded, nil); err != nil {
		t.Fatalf("serialized exact Shell applicability was rejected: %v", err)
	}
	if loaded.DefinitionFocuses[0].EvidenceDeclarationRef == "" || loaded.DefinitionFocuses[0].ValidationRecipeID == "" ||
		len(loaded.Resources.OperationAdmissibilities[0].ValidationSurfaces[0].ProofRequirementBindings) != 2 {
		t.Fatalf("serialization lost immutable proof lineage: %+v", loaded)
	}
	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFacts)
	}{
		{
			name: "legacy synthetic surface without exact bindings",
			mutate: func(value *runtimecontract.RuntimeFacts) {
				value.Resources.OperationAdmissibilities[0].ValidationSurfaces[0].ProofRequirementBindings = nil
			},
		},
		{
			name: "another CWI marker cannot close the requirement",
			mutate: func(value *runtimecontract.RuntimeFacts) {
				surface := &value.Resources.OperationAdmissibilities[0].ValidationSurfaces[0]
				surface.ProofRequirementBindings[0].RequiredCheckRefs = append([]string(nil), requirements[1].RequiredCheckRefs...)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var drifted runtimecontract.RuntimeFacts
			if err := json.Unmarshal(payload, &drifted); err != nil {
				t.Fatal(err)
			}
			test.mutate(&drifted)
			if err := validateRuntimeValidationProofApplicability(drifted, nil); err == nil {
				t.Fatalf("non-exact Shell proof applicability survived fresh-load validation: %+v", drifted.Resources.OperationAdmissibilities)
			}
		})
	}

	mcpFacts := runtimecontract.RuntimeFacts{
		DefinitionRevision: "definition-mcp-proof",
		DefinitionSeed: runtimecontract.RuntimeDefinitionSeed{CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: "cwi-mcp", SliceID: "slice-mcp", SourceScope: "dodItem", SourceID: "dod-mcp", TargetText: "MCP validation remains legal",
		}}},
		DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
			ID: "focus-mcp", SliceID: "slice-mcp", AssessmentScope: "dodItem", AssessmentID: "dod-mcp", AssessmentText: "MCP validation remains legal",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-mcp"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-mcp"),
		}},
		Resources: runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{{
			PrerequisiteID: "proof-mcp-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			OperationKinds:       []string{runtimecontract.StepKindMCPToolCall},
			ValidationRecipeRefs: []string{"mcp/tool/validate"},
			ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
				ID: "mcp/tool/validate", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall,
				Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "mcp-input-schema", InputBindingRef: "mcp-input-binding",
				CheckRefs: []string{"mcp-generic-check"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
				EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
			}},
			ObservedRevision: "proof-mcp-revision",
		}}},
	}
	mcpFacts.Resources.OperationAdmissibilities[0] = mcpFacts.Resources.OperationAdmissibilities[0].Normalized()
	mcpFacts.Resources.OperationAdmissibilities[0].SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(mcpFacts.Resources.OperationAdmissibilities[0])
	if err := validateRuntimeValidationProofApplicability(mcpFacts, nil); err != nil {
		t.Fatalf("non-Shell validation was incorrectly subjected to Shell proof binding: %v", err)
	}
}

func TestRuntimeValidationProofApplicabilityAllowsOwnerRecipeAlternativesAndRejectsSurfaceDrift(t *testing.T) {
	item := runtimecontract.RuntimeCoverageItem{
		ID: "cwi-neutral-proof", SliceID: "slice-neutral-proof", SourceScope: "dodItem", SourceID: "dod-neutral-proof", TargetText: "neutral proof requirement",
	}
	focus := runtimecontract.RuntimeDefinitionFocus{
		ID: "focus-neutral-proof", SliceID: item.SliceID,
		AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition-neutral-proof", []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeDefinitionFocus{focus})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile neutral requirement: requirements=%+v err=%v", requirements, err)
	}
	requirement := requirements[0]
	newSurface := func(recipeID string) runtimecontract.RuntimeValidationSurface {
		binding := runtimeTestOpaqueProofRequirementBinding(t, requirement, recipeID)
		return runtimecontract.RuntimeValidationSurface{
			ID: recipeID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "test/schema/" + recipeID, InputBindingRef: "test/input/" + recipeID,
			CheckRefs: append([]string(nil), requirement.RequiredCheckRefs...), ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{binding},
			EffectScope:              runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "neutral-proof-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds:       []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationRecipeRefs: []string{"recipe/a", "recipe/b"},
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{
			{ID: "recipe/a", Command: "true"},
			{ID: "recipe/b", Command: "true"},
		},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{newSurface("recipe/a"), newSurface("recipe/b")},
		ObservedRevision:   "neutral-proof-v1",
	}
	catalog = catalog.Normalized()
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, []runtimecontract.RuntimeOperationAdmissibility{catalog}); err != nil {
		t.Fatalf("two ready compatible recipe alternatives were rejected: %v", err)
	}

	duplicate := runtimecontract.CloneRuntimeOperationAdmissibilities([]runtimecontract.RuntimeOperationAdmissibility{catalog})[0]
	duplicate.ValidationSurfaces[0].ProofRequirementBindings = append(
		duplicate.ValidationSurfaces[0].ProofRequirementBindings,
		duplicate.ValidationSurfaces[0].ProofRequirementBindings[0],
	)
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, []runtimecontract.RuntimeOperationAdmissibility{duplicate}); err == nil {
		t.Fatal("duplicate exact bindings on one ready surface were accepted")
	}

	notDeclared := runtimecontract.CloneRuntimeOperationAdmissibilities([]runtimecontract.RuntimeOperationAdmissibility{catalog})[0]
	notDeclared.ValidationSurfaces[0].ProofApplicability = runtimecontract.RuntimeValidationProofNotDeclared
	notDeclared.ValidationSurfaces[0].ProofRequirementBindings = nil
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, []runtimecontract.RuntimeOperationAdmissibility{notDeclared}); err == nil {
		t.Fatal("ready surface removed its exact binding by changing proof applicability")
	}

	constrained := requirement.Clone()
	constrained.ValidationRecipeID = "recipe/a"
	constrained.Hash = runtimecontract.StableRuntimeValidationProofRequirementHash(constrained)
	unavailable := runtimecontract.CloneRuntimeOperationAdmissibilities([]runtimecontract.RuntimeOperationAdmissibility{catalog})[0]
	unavailable.ValidationSurfaces = []runtimecontract.RuntimeValidationSurface{{
		ID: "recipe/a", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Readiness: runtimecontract.RuntimeValidationSurfaceUnavailable, ReadinessReason: "test unavailable", InputSchemaHash: "test/schema/recipe/a",
		ProofApplicability: runtimecontract.RuntimeValidationProofNotDeclared,
		EffectScope:        runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	unavailable.ValidationRecipeRefs = nil
	unavailable.ValidationRecipes = nil
	unavailable = unavailable.Normalized()
	unavailable.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(unavailable)
	if err := runtimecontract.ValidateRuntimeValidationProofRequirementCatalogApplicability([]runtimecontract.RuntimeValidationProofRequirement{constrained}, []runtimecontract.RuntimeOperationAdmissibility{unavailable}); err != nil {
		t.Fatalf("constrained unavailable surface was incorrectly required to bind: %v", err)
	}
}

func TestRuntimeAggregateRejectsOwnerFactAcquisitionTerminalCredit(t *testing.T) {
	for _, retireManifest := range []bool{false, true} {
		name := "active_manifest"
		if retireManifest {
			name = "retired_manifest"
		}
		t.Run(name, func(t *testing.T) {
			_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t, retireManifest)
			base := *repository.aggregate
			if err := base.Validate(); err != nil {
				t.Fatalf("valid acquisition aggregate rejected: %v", err)
			}

			corrupt := base
			corrupt.Facts = base.Facts.Normalized()
			corrupt.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
			corrupt.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
			if err := corrupt.Validate(); err == nil || !strings.Contains(err.Error(), "acquisition CWI") {
				t.Fatalf("acquisition terminal credit validation err=%v", err)
			}
		})
	}
}

func TestRuntimeAggregateRejectsRetiredFailureTargetPrerequisite(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Prerequisites.Pending = append(aggregate.Facts.Prerequisites.Pending, runtimecontract.RuntimePrerequisite{
		ID: "failure_target:retired", Kind: runtimecontract.PrerequisiteKind("failure_target"),
	})
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("retired failure_target prerequisite validation err=%v", err)
	}
}

func TestRuntimeAggregateRejectsPendingCandidateFeasibilityBindingDriftAfterFreshLoad(t *testing.T) {
	baseline := runtimePendingCandidateFeasibilityAggregate(t)
	if err := baseline.Validate(); err != nil {
		t.Fatalf("valid pending candidate feasibility aggregate rejected: %v", err)
	}
	if len(baseline.CandidateAttempts) != 1 || len(baseline.CandidateAttempts[0].Feasibilities) != 1 {
		t.Fatalf("pending feasibility fixture is incomplete: %+v", baseline.CandidateAttempts)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*runtimecontract.RuntimeCandidateFeasibilityRecord)
		wantErr string
	}{
		{
			name: "operation absent from proposal",
			mutate: func(record *runtimecontract.RuntimeCandidateFeasibilityRecord) {
				record.OperationID = "forged-operation"
				record.ReplayProof.OperationID = "forged-operation"
			},
			wantErr: "absent from its proposal",
		},
		{
			name: "manifest drift",
			mutate: func(record *runtimecontract.RuntimeCandidateFeasibilityRecord) {
				record.CandidateManifestHash = "forged-manifest"
			},
			wantErr: "stale manifest identity",
		},
		{
			name: "catalog drift",
			mutate: func(record *runtimecontract.RuntimeCandidateFeasibilityRecord) {
				record.AdmissibilitySnapshotID = "forged-catalog"
				record.ReplayProof.AdmissibilitySnapshotID = "forged-catalog"
			},
			wantErr: "does not match current owner admissibility",
		},
		{
			name: "owner policy drift",
			mutate: func(record *runtimecontract.RuntimeCandidateFeasibilityRecord) {
				record.Observation.OwnerPolicy.OwnerModuleID = runtimecontract.RuntimeModuleMCP
			},
			wantErr: "owner policy does not match",
		},
		{
			name: "non-feasible fact cannot survive as pending half-state",
			mutate: func(record *runtimecontract.RuntimeCandidateFeasibilityRecord) {
				failure := runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassCandidateInfeasible, Code: "owner_infeasible",
					Signature: "owner-infeasible", Message: "owner rejected the operation",
				}
				feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(
					runtimecontract.RuntimeFeedbackScope{WorkspaceID: "ws-forged", RunID: "run-forged"},
					runtimecontract.RuntimeFeedbackDomainFeasibility,
					runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown),
					runtimecontract.RuntimeModuleMCP,
					failure,
				)
				if err != nil {
					t.Fatal(err)
				}
				record.Observation.Outcome = runtimecontract.RuntimeOwnerFeasibilityInfeasible
				record.Observation.Feedback = &feedback
				record.ReplayProof = nil
			},
			wantErr: "cannot retain an undecided owner feasibility fact",
		},
		{
			name: "replay proof target drift",
			mutate: func(record *runtimecontract.RuntimeCandidateFeasibilityRecord) {
				record.ReplayProof.TargetRefs = []string{"forged-target.txt"}
			},
			wantErr: "replay proof does not match sealed owner facts",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loaded := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			test.mutate(&loaded.CandidateAttempts[0].Feasibilities[0])
			loaded = runtimeTestEvidenceAggregateRoundTrip(t, loaded)
			if err := loaded.Validate(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load pending feasibility error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func runtimePendingCandidateFeasibilityAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID:     "operation-admissibility:file/fresh-load",
		OwnerModuleID:      runtimecontract.RuntimeModuleFile,
		OperationKinds:     []string{runtimecontract.StepKindFileEdit},
		TargetRefs:         []string{"target.txt"},
		AcquisitionBinding: "file-capability-v1",
		ObservedRevision:   "workspace-tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: inventory.OwnerModuleID, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("pending feasibility fixture decision = %+v", initialized.Decision)
	}
	inventory = runtimeTestOperationAdmissibilityForOwner(t, initialized.Facts, runtimecontract.RuntimeModuleFile)
	oldText, newText := "before", "after"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "edit-target", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), initialized.Facts.Focus.CoverageWorkItemIDs...),
		TargetRefs:          []string{"target.txt"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		},
	}}}}}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, facts.WorkspaceID, facts.RunID, 0, "fresh-load-feasibility"); err != nil {
		t.Fatal(err)
	}
	proposed, err := controller.recordCandidateProposal(ctx, facts.WorkspaceID, facts.RunID, 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	pending := proposed.CandidateAttempts[len(proposed.CandidateAttempts)-1]
	operations := candidateRuntimeOperations(*pending.Proposal)
	if len(operations) != 1 {
		t.Fatalf("pending feasibility fixture lost operation: %+v", pending.Proposal)
	}
	operation := operations[0].Normalized()
	ownerObservation := runtimecontract.RuntimeOwnerFeasibilityObservation{
		Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible,
		OwnerPolicy: runtimecontract.RuntimeOwnerPolicyIdentity{
			OwnerModuleID: runtimecontract.RuntimeModuleFile, PolicyRevision: "file-owner-feasibility-v1",
			CatalogSnapshotID: inventory.SnapshotID,
		},
	}
	replayProof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: operation.ID, TargetRefs: operation.TargetRefs,
		BeforeHash: "target-before-v1", AfterHash: "target-after-v1",
		AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
	}
	if _, err := controller.recordCandidateFeasibility(ctx, facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
		CandidateManifestHash: pending.ManifestHash, Observation: &ownerObservation, ReplayProof: &replayProof,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := controller.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestRuntimeAggregateSemanticPurposeInvariantFailsClosedWithoutExactLineage(t *testing.T) {
	_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t)
	base := *repository.aggregate

	withoutAssessment := base
	withoutAssessment.Facts = base.Facts.Normalized()
	withoutAssessment.Facts.Focus.RequireAssessment = false
	if err := validateRuntimeSemanticBindingPurposeInvariant(withoutAssessment); err == nil || !strings.Contains(err.Error(), "requires assessment") {
		t.Fatalf("acquisition without assessment validation err=%v", err)
	}

	missing := base
	missing.Facts = base.Facts.Normalized()
	ref := missing.Facts.Coverage[0].CurrentBindingRef.Clone()
	ref.SemanticManifestHash = "missing-semantic-manifest"
	missing.Facts.Coverage[0].CurrentBindingRef = &ref
	missing.Facts.Coverage[0].BindingLineage[len(missing.Facts.Coverage[0].BindingLineage)-1] = ref
	if err := validateRuntimeSemanticBindingPurposeInvariant(missing); err == nil || !strings.Contains(err.Error(), "no exact semantic purpose lineage") {
		t.Fatalf("missing semantic purpose validation err=%v", err)
	}
}

func TestRuntimeAggregateSemanticPurposeInvariantAllowsDefinitionTerminalCredit(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
	aggregate.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
	if err := validateRuntimeSemanticBindingPurposeInvariant(aggregate); err != nil {
		t.Fatalf("Definition terminal proof was rejected: %v", err)
	}
}

func TestRuntimeAggregateValidationPhasesRejectOwnedInvalidState(t *testing.T) {
	base := validationAggregate(t)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid aggregate rejected: %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(*RuntimeAggregate)
		validate func(RuntimeAggregate) error
	}{
		{
			name: "sm2 authority",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.WorkspaceID = ""
			},
			validate: func(a RuntimeAggregate) error {
				active, completed := referenceAggregateDispatchProjection(a)
				return validateRuntimeAggregateSM2Authority(a, active, completed)
			},
		},
		{
			name: "sm3 active dispatch",
			mutate: func(value *RuntimeAggregate) {
				value.ActiveDispatch = &SealedDispatch{}
			},
			validate: validateRuntimeAggregateSM3ActiveDispatch,
		},
		{
			name: "activation fence",
			mutate: func(value *RuntimeAggregate) {
				value.Termination = &runtimecontract.RuntimeTerminationRecord{}
			},
			validate: validateRuntimeAggregateActivationFence,
		},
		{
			name: "sm3 candidate transaction",
			mutate: func(value *RuntimeAggregate) {
				value.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{}}
			},
			validate: func(a RuntimeAggregate) error {
				active, completed := referenceAggregateDispatchProjection(a)
				return validateRuntimeAggregateSM3CandidateTransaction(a, active, completed)
			},
		},
		{
			name: "governance ledger",
			mutate: func(value *RuntimeAggregate) {
				value.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{}}
			},
			validate: func(a RuntimeAggregate) error {
				active, completed := referenceAggregateDispatchProjection(a)
				return validateRuntimeAggregateGovernanceLedgers(a, active, completed)
			},
		},
		{
			name: "sm4 delivery acknowledgement",
			mutate: func(value *RuntimeAggregate) {
				value.DeliveryAcknowledgements = []runtimecontract.RuntimeDeliveryAcknowledgement{{}}
			},
			validate: validateRuntimeAggregateSM4DeliveryAcknowledgements,
		},
		{
			name: "sm4 non-terminal delivery budget",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Recovery.TerminalDeliveryAttempts = 1
			},
			validate: validateRuntimeAggregateSM4DeliveryAcknowledgements,
		},
		{
			name: "sm2 repair ledger",
			mutate: func(value *RuntimeAggregate) {
				value.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{{}}
			},
			validate: validateRuntimeAggregateSM2RepairLedgers,
		},
		{
			name: "sm4 drive lease",
			mutate: func(value *RuntimeAggregate) {
				value.DriveLease = &runtimecontract.RuntimeDriveLease{}
			},
			validate: validateRuntimeAggregateSM4DriveLease,
		},
		{
			name: "cross-machine governance fence",
			mutate: func(value *RuntimeAggregate) {
				value.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{}}
			},
			validate: func(a RuntimeAggregate) error {
				active, completed := referenceAggregateDispatchProjection(a)
				return validateRuntimeAggregateCrossMachineFences(a, active, completed)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if err := test.validate(value); err == nil {
				t.Fatal("invalid state was accepted by its validation phase")
			}
		})
	}
}
