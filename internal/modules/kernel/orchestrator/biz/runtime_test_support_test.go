package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var runtimeCandidateRequestContentTestStores sync.Map

func runtimeWorkspaceForPersistedRunTest(
	t *testing.T,
	ctx context.Context,
	runs repository.RunRepository,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	deliverableDirectory string,
	internalDirectory string,
) *workspacemodel.Workspace {
	t.Helper()
	item, err := runs.GetByWorkspaceID(ctx, workspaceID, runID)
	if err != nil {
		t.Fatalf("load current Run projection fixture: %v", err)
	}
	fact := runevents.RunStateChangedDataFromModel(item)
	if err := fact.Validate(); err != nil {
		t.Fatalf("current Run projection fixture: %v", err)
	}
	return &workspacemodel.Workspace{
		ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID,
		DeliverableDirectory: deliverableDirectory, InternalDirectory: internalDirectory,
		RunProjectionCursor: workspacemodel.WorkspaceRunProjectionCursor{
			WorkspaceRunGeneration: fact.WorkspaceRunGeneration,
			RunID:                  fact.RunID, RunRevision: fact.RunRevision,
			RuntimeStateRevision: fact.RuntimeStateRevision, RuntimeStateRef: fact.RuntimeStateRef,
			CausationID: fact.CausationID,
		},
	}
}

func runtimeProviderFeedbackForTest(t *testing.T, command llmevents.RequestRuntimeCandidateCommand, failure runtimecontract.RuntimeCandidateFailure) *runtimecontract.RuntimeFeedback {
	t.Helper()
	category := runtimecontract.RuntimeFeedbackCategoryProviderProtocol
	switch failure.Class {
	case runtimecontract.RuntimeCandidateFailureClassProviderAuthentication:
		category = runtimecontract.RuntimeFeedbackCategoryProviderAuthentication
	case runtimecontract.RuntimeCandidateFailureClassProviderConfiguration:
		category = runtimecontract.RuntimeFeedbackCategoryProviderConfiguration
	case runtimecontract.RuntimeCandidateFailureClassProviderTransient:
		category = runtimecontract.RuntimeFeedbackCategoryProviderTransient
	}
	if failure.Code == "" {
		failure.Code = string(category)
	}
	if failure.Signature == "" {
		failure.Signature = "test-" + string(category)
	}
	scope := runtimecontract.RuntimeFeedbackScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		scope.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromCandidateFailure(scope, runtimecontract.RuntimeFeedbackDomainProviderRequest, category, runtimecontract.RuntimeModuleLLM, failure)
	if err != nil {
		t.Fatal(err)
	}
	return &feedback
}

// runtimeProviderResultAuditForTest builds the same exact, secret-free
// producer fact required from the LLM EventHub result contract. Scripted
// provider results represent a physical call unless they explicitly report a
// pre-dispatch stale fact; focused release tests construct no-submit audits
// directly instead of weakening this common fixture.
func runtimeProviderResultAuditForTest(command llmevents.RequestRuntimeCandidateCommand, result llmevents.RequestRuntimeCandidateResult) *runtimecontract.ProviderRequestAuditView {
	submitted := result.Stale == nil
	status := "completed"
	failureClass := ""
	if result.Feedback != nil {
		status = "failed"
		if result.Feedback.OriginalDiagnostic != nil {
			failureClass = result.Feedback.OriginalDiagnostic.OriginalClass
		}
	}
	if result.Stale != nil {
		status = "failed"
		failureClass = string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration)
	}
	target := command.CandidateContext.Activation.ProviderSelection.Target.Normalized()
	kind := "runtime_candidate"
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		target = command.SemanticContext.Activation.ProviderSelection.Target.Normalized()
		kind = "runtime_semantic_plan"
	}
	if target.ProviderID == "" || target.ModelID == "" {
		target = runtimecontract.ProviderModelTarget{ProviderID: "test-provider", ModelID: "test-model"}
	}
	actualTokens := 0
	if result.Usage != nil {
		actualTokens = result.Usage.TotalTokens
	}
	audit := &runtimecontract.ProviderRequestAuditView{
		RequestID: command.RequestIdentity.PhysicalCallID, LogicalRequestID: command.RequestIdentity.LogicalRequestID,
		PhysicalCallID: command.RequestIdentity.PhysicalCallID, WorkspaceID: string(command.WorkspaceID), RunID: string(command.RunID),
		Target: target, Operation: "responses", StablePrefixHash: command.StablePrefixHash, Kind: kind,
		PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
		ContextBudget: llmaudit.ContextBudget{
			ProviderID: target.ProviderID, ModelID: target.ModelID, ProviderSubmitted: submitted,
			ActualTokens: actualTokens, StablePrefixHash: command.StablePrefixHash,
		},
		ProviderSubmitted: submitted, Status: status, FailureClass: failureClass,
	}
	if submitted {
		audit.RequestBytes = 1
	}
	return audit
}

func runtimeProviderResultWithAuditForTest(command llmevents.RequestRuntimeCandidateCommand, result llmevents.RequestRuntimeCandidateResult) llmevents.RequestRuntimeCandidateResult {
	result.WorkspaceID = command.WorkspaceID
	result.RunID = command.RunID
	result.Transaction = command.Transaction
	result.DocumentKind = command.DocumentKind
	result.RequestIdentity = command.RequestIdentity
	result.ExpectedDocumentHash = runtimeExpectedDocumentHash(command.ExpectedDocument)
	result.Audit = runtimeProviderResultAuditForTest(command, result)
	return result
}

func runtimePreflightFeedbackForTest(t *testing.T, command llmevents.PreflightRuntimeCandidateCommand, reason string) runtimecontract.RuntimeFeedback {
	t.Helper()
	feedback := runtimecontract.RuntimeFeedback{
		Domain: runtimecontract.RuntimeFeedbackDomainProviderPreflight, StableCategory: runtimecontract.RuntimeFeedbackCategoryAccepted,
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		feedback.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
	}
	if reason != "" {
		feedback.StableCategory = runtimecontract.RuntimeFeedbackCategoryContextIntegrity
		feedback.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			OriginalClass: "context_integrity", OriginalCode: "candidate_context_integrity", OriginalMessage: reason, Signature: "test-context-integrity",
		}
	}
	if err := feedback.Validate(); err != nil {
		t.Fatal(err)
	}
	return feedback
}

func runtimeTestAggregateWithSemanticPlanning(t *testing.T, aggregate runtime.RuntimeAggregate) runtime.RuntimeAggregate {
	t.Helper()
	if aggregate.Facts.Activation.ExecutionContract.OperationContractRevision == "" {
		aggregate.Facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	}
	if aggregate.Facts.Activation.ExecutionContract.StateChangePolicy == "" {
		aggregate.Facts.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	}
	if aggregate.Facts.ConvergencePolicy.MaxRepairAttemptsPerFocus == 0 {
		aggregate.Facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	}
	if aggregate.Facts.RecoveryPolicy.MaxProviderTransportAttempts == 0 {
		aggregate.Facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	}
	if aggregate.Facts.ExecutionBudgetPolicy.PolicyRevision == "" {
		aggregate.Facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(aggregate.Facts.ConvergencePolicy, aggregate.Facts.RecoveryPolicy)
	}
	if aggregate.ExecutionBudget.Policy.PolicyRevision == "" {
		policy := aggregate.Facts.ExecutionBudgetPolicy
		ledger, err := runtimecontract.NewRuntimeExecutionBudgetLedger(policy)
		if err != nil {
			t.Fatalf("initialize test Runtime execution budget: %v", err)
		}
		aggregate.ExecutionBudget = ledger
	}
	if aggregate.Facts.Resources.ProviderCeiling == nil {
		aggregate.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
			Selection:                aggregate.Facts.Activation.ProviderSelection,
			ContextWindowTokens:      128000,
			ProviderMaxOutputTokens:  16384,
			EffectiveMaxOutputTokens: 8192,
			MaxInputTokens:           119808,
			MaxBytes:                 128 * 1024,
			CapabilityRevision:       "capability-1",
			ConfigurationRevision:    "config-1",
		}
	}
	if aggregate.Facts.DefinitionSeed.SchemaVersion == "" {
		items := make([]runtimecontract.RuntimeCoverageItem, 0, len(aggregate.Facts.Coverage))
		slicesByID := map[string][]common.CoverageWorkItemID{}
		for _, fact := range aggregate.Facts.Coverage {
			sliceID := fact.SliceID
			if sliceID == "" {
				sliceID = "slice-test-" + string(fact.ID)
			}
			sourceScope, sourceID := fact.SourceScope, fact.SourceID
			if sourceScope == "" {
				sourceScope = "test"
			}
			if sourceID == "" {
				sourceID = string(fact.ID)
			}
			items = append(items, runtimecontract.RuntimeCoverageItem{ID: fact.ID, SliceID: sliceID, SourceScope: sourceScope, SourceID: sourceID, TargetText: fact.TargetText, PrerequisiteCoverageWorkItemIDs: fact.PrerequisiteCoverageWorkItemIDs, ReopenDependencies: fact.ReopenDependencies})
			slicesByID[sliceID] = append(slicesByID[sliceID], fact.ID)
		}
		if len(items) == 0 {
			sliceID := aggregate.Facts.Focus.SliceID
			if sliceID == "" {
				sliceID = "slice-test"
			}
			for _, coverageID := range aggregate.Facts.Focus.CoverageWorkItemIDs {
				items = append(items, runtimecontract.RuntimeCoverageItem{ID: coverageID, SliceID: sliceID, SourceScope: "test", SourceID: string(coverageID)})
				slicesByID[sliceID] = append(slicesByID[sliceID], coverageID)
			}
		}
		slices := make([]runtimecontract.RuntimeAcceptanceSlice, 0, len(slicesByID))
		for sliceID, coverageIDs := range slicesByID {
			slices = append(slices, runtimecontract.RuntimeAcceptanceSlice{ID: sliceID, CoverageWorkItemIDs: coverageIDs})
		}
		aggregate.Facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
			SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: aggregate.Facts.DefinitionRevision,
			PolicyHash: "policy-test", CoverageItems: items, Slices: slices,
		}
		aggregate.Facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(aggregate.Facts.DefinitionSeed)
	} else {
		// Test fixtures often append a queued focus after constructing the base
		// DefinitionSeed. Keep the immutable seed in lock-step before building
		// semantic order lineage; production DefinitionSeed remains immutable.
		seed := aggregate.Facts.DefinitionSeed.Clone()
		known := make(map[common.CoverageWorkItemID]struct{}, len(seed.CoverageItems))
		for _, item := range seed.CoverageItems {
			known[item.ID] = struct{}{}
		}
		sliceIndex := make(map[string]int, len(seed.Slices))
		for idx, slice := range seed.Slices {
			sliceIndex[slice.ID] = idx
		}
		changed := false
		for _, fact := range aggregate.Facts.Coverage {
			if _, exists := known[fact.ID]; exists {
				continue
			}
			sliceID := fact.SliceID
			if sliceID == "" {
				sliceID = "slice-test-" + string(fact.ID)
			}
			sourceScope, sourceID := fact.SourceScope, fact.SourceID
			if sourceScope == "" {
				sourceScope = "test"
			}
			if sourceID == "" {
				sourceID = string(fact.ID)
			}
			seed.CoverageItems = append(seed.CoverageItems, runtimecontract.RuntimeCoverageItem{
				ID: fact.ID, SliceID: sliceID, SourceScope: sourceScope, SourceID: sourceID,
				TargetText: fact.TargetText, PrerequisiteCoverageWorkItemIDs: fact.PrerequisiteCoverageWorkItemIDs,
				ReopenDependencies: fact.ReopenDependencies,
			})
			known[fact.ID] = struct{}{}
			if idx, exists := sliceIndex[sliceID]; exists {
				seed.Slices[idx].CoverageWorkItemIDs = append(seed.Slices[idx].CoverageWorkItemIDs, fact.ID)
			} else {
				seed.Slices = append(seed.Slices, runtimecontract.RuntimeAcceptanceSlice{ID: sliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{fact.ID}})
				sliceIndex[sliceID] = len(seed.Slices) - 1
			}
			changed = true
		}
		if changed {
			seed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(seed)
			aggregate.Facts.DefinitionSeed = seed
		}
	}
	// Direct fixtures that do not start from RuntimeBootstrap still need an
	// immutable declaration. Preserve every declaration produced by
	// RuntimeBootstrap.InitialFacts: the pre-semantic queue is only a cursor and
	// deliberately does not carry the declaration's target-grounding policy.
	// Production creates this inventory exactly once during activation.
	queue := append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...)
	queue = append(queue, aggregate.Facts.CompletedFocuses...)
	items := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(aggregate.Facts.DefinitionSeed.CoverageItems))
	for _, item := range aggregate.Facts.DefinitionSeed.CoverageItems {
		items[item.ID] = item
	}
	for index := range queue {
		if len(queue[index].CoverageWorkItemIDs) == 1 {
			item := items[queue[index].CoverageWorkItemIDs[0]]
			if queue[index].SliceID == "" {
				queue[index].SliceID = item.SliceID
			}
			// RuntimeFocusSeed is deliberately non-authoritative and therefore
			// omits the immutable admitted-CWI declaration. Aggregate-only tests
			// specialize that generation cursor before sealing a synthetic SM-3
			// document, so restore the declaration exclusively from DefinitionSeed
			// instead of copying a generation field or inventing a second owner.
			queue[index].AssessmentScope = item.SourceScope
			queue[index].AssessmentID = item.SourceID
			queue[index].AssessmentText = item.TargetText
			queue[index].EvidenceDeclarationRef = runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
		}
		if queue[index].TargetSelectorRef != "" && queue[index].TargetSelectorOwnerRef != "" && queue[index].TargetSelectorSubjectRef == "" && len(queue[index].CoverageWorkItemIDs) == 1 {
			queue[index].TargetSelectorSubjectRef = string(queue[index].CoverageWorkItemIDs[0])
		}
	}
	declarations := runtimecontract.CloneRuntimeDefinitionFocuses(aggregate.Facts.DefinitionFocuses)
	// Aggregate-only reducer fixtures intentionally have no presentation-slice
	// projection. They often specialize the pre-semantic focus immediately
	// before calling this helper; mirror that specialization into their test
	// Definition declaration. Run-backed fixtures always carry Facts.Slices and
	// must preserve the immutable activation inventory exactly.
	if len(aggregate.Facts.Slices) == 0 {
		declarations = runtimecontract.CompileRuntimeDefinitionFocuses(queue)
	}
	declaredCoverage := make(map[common.CoverageWorkItemID]struct{}, len(declarations))
	for _, declaration := range declarations {
		for _, coverageID := range declaration.CoverageWorkItemIDs {
			declaredCoverage[coverageID] = struct{}{}
		}
	}
	missingDeclarations := make([]runtimecontract.CoverageFocus, 0, len(queue))
	for _, source := range queue {
		missing := false
		for _, coverageID := range source.CoverageWorkItemIDs {
			if _, found := declaredCoverage[coverageID]; !found {
				missing = true
				declaredCoverage[coverageID] = struct{}{}
			}
		}
		if missing {
			missingDeclarations = append(missingDeclarations, source)
		}
	}
	declarations = append(declarations, runtimecontract.CompileRuntimeDefinitionFocuses(missingDeclarations)...)
	aggregate.Facts.DefinitionFocuses = runtimecontract.NormalizeRuntimeDefinitionFocuses(declarations)
	if len(aggregate.Facts.Slices) == 0 {
		// Aggregate-only fixtures do not have a persisted Run bootstrap from
		// which the Definition capability contract can be reloaded. Keep that
		// immutable test input aligned with the exact operations declared by the
		// specialized focus instead of retaining runtimeDefinition's File-only
		// placeholder while constructing a Shell or MCP semantic manifest.
		operationKinds := make([]string, 0)
		for _, declaration := range aggregate.Facts.DefinitionFocuses {
			operationKinds = append(operationKinds, declaration.AcceptedStepKinds...)
		}
		if len(operationKinds) == 0 {
			operationKinds = append(operationKinds, aggregate.Facts.Capabilities.StateChangeKinds...)
			operationKinds = append(operationKinds, aggregate.Facts.Capabilities.ValidationKinds...)
			operationKinds = append(operationKinds, aggregate.Facts.Capabilities.GovernanceKinds...)
		}
		if operationKinds = runtimeTestNormalizeStrings(operationKinds); len(operationKinds) != 0 {
			aggregate.Definition.CapabilityContract = operationKinds
		}
	}
	var proofErr error
	aggregate.Facts, proofErr = runtimeTestBindExactManagedValidationProofFacts(aggregate.Facts)
	if proofErr != nil {
		t.Fatalf("bind exact managed-validation test facts: %v", proofErr)
	}
	if aggregate.Facts.CWILedgerSchemaVersion == "" {
		// Test-only post-semantic fixtures begin from the same unbound activation
		// shape as production. Do not let an older fixture's hand-written binding
		// turn into an implicit activation authority.
		aggregate.Facts = runtimeTestFactsWithoutSemanticBindings(aggregate.Facts)
		initialized, err := runtimecontract.InitializeRuntimeCWILedgerForActivation(aggregate.Facts)
		if err != nil {
			t.Fatalf("initialize test canonical CWI ledger: %v", err)
		}
		aggregate.Facts = initialized
	}
	if aggregate.SemanticPlanning.SchemaVersion == "" {
		ledger, err := runtimecontract.NewRuntimeSemanticPlanningLedger(aggregate.Facts.DefinitionSeed)
		if err != nil {
			t.Fatal(err)
		}
		aggregate.SemanticPlanning = ledger
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		aggregate.SemanticPlanning = runtimeTestSealedSemanticPlanning(t, aggregate, aggregate.SemanticPlanning)
		aggregate.Facts = runtimeTestProjectSealedSemanticFacts(t, aggregate.Facts, aggregate.SemanticPlanning)
		facts, decision, err := runtime.ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
		if err != nil {
			t.Fatalf("reduce test canonical semantic facts: %v", err)
		}
		aggregate.Facts = facts
		aggregate.Decision = decision
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("test canonical semantic aggregate: %v", err)
	}
	return aggregate
}

// runtimeTestBindExactManagedValidationProofFacts migrates hand-written
// post-semantic fixtures onto the exact per-CWI owner fact now required by the
// Shell contract. It is deliberately test-only: real owner integration tests
// must acquire native attestations from Shell through EventHub and never call
// this helper.
func runtimeTestBindExactManagedValidationProofFacts(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeFacts, error) {
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		facts.DefinitionRevision,
		facts.DefinitionSeed.CoverageItems,
		facts.DefinitionFocuses,
	)
	if err != nil || len(requirements) == 0 {
		return facts, err
	}
	requirements = runtimecontract.NormalizeRuntimeValidationProofRequirements(requirements)
	for catalogIndex := range facts.Resources.OperationAdmissibilities {
		catalog := facts.Resources.OperationAdmissibilities[catalogIndex].Normalized()
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			continue
		}
		oldSnapshotID := catalog.SnapshotID
		assigned := make(map[string]struct{}, len(requirements))
		for surfaceIndex := range catalog.ValidationSurfaces {
			surface := catalog.ValidationSurfaces[surfaceIndex].Normalized()
			if surface.OperationKind != runtimecontract.RuntimeBlockKindManagedValidationSession ||
				surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady ||
				surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect {
				continue
			}
			bindings := runtimecontract.CloneRuntimeValidationProofRequirementBindings(surface.ProofRequirementBindings)
			checks := make([]string, 0)
			for _, binding := range bindings {
				assigned[binding.RequirementHash] = struct{}{}
				checks = append(checks, binding.RequiredCheckRefs...)
			}
			for _, requirement := range requirements {
				if _, exists := assigned[requirement.Hash]; exists ||
					(requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != surface.ID) {
					continue
				}
				attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(requirement.RequiredCheckRefs))
				for index, checkRef := range requirement.RequiredCheckRefs {
					attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
						RequiredCheckRef: checkRef, ObservationProtocolRef: "test/opaque-validation-observation/v1",
						NativeCheckRef: fmt.Sprintf("test/native/%s/%s/%d", requirement.CoverageWorkItemID, surface.ID, index),
					})
				}
				binding, bindErr := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, surface.ID, attestations...)
				if bindErr != nil {
					return facts, bindErr
				}
				bindings = append(bindings, binding)
				checks = append(checks, requirement.RequiredCheckRefs...)
				assigned[requirement.Hash] = struct{}{}
			}
			if len(bindings) != 0 {
				surface.ProofRequirementBindings = runtimecontract.NormalizeRuntimeValidationProofRequirementBindings(bindings)
				surface.CheckRefs = runtimeTestNormalizeStrings(checks)
				catalog.ValidationSurfaces[surfaceIndex] = surface
			}
		}
		catalog.ValidationSurfaces = runtimecontract.NormalizeRuntimeValidationSurfaces(catalog.ValidationSurfaces)
		catalog.ValidationRecipeRefs = runtimecontract.RuntimeSelectableValidationRecipeRefs(catalog.ValidationSurfaces)
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		if err := catalog.Validate(); err != nil {
			return facts, err
		}
		facts.Resources.OperationAdmissibilities[catalogIndex] = catalog
		for observationIndex := range facts.Resources.PrerequisiteObservations {
			observation := facts.Resources.PrerequisiteObservations[observationIndex].Normalized()
			if observation.PrerequisiteID != catalog.PrerequisiteID || observation.OwnerModuleID != catalog.OwnerModuleID ||
				(observation.ContentHash != oldSnapshotID && observation.ContentHash != "") {
				continue
			}
			observation.ContentHash = catalog.SnapshotID
			observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
			facts.Resources.PrerequisiteObservations[observationIndex] = observation
		}
	}
	return facts.Normalized(), nil
}

func runtimeTestFactsWithoutSemanticBindings(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	facts.Focus.CoverageEvidenceBindings = nil
	for index := range facts.RemainingFocuses {
		facts.RemainingFocuses[index].CoverageEvidenceBindings = nil
	}
	for index := range facts.CompletedFocuses {
		facts.CompletedFocuses[index].CoverageEvidenceBindings = nil
	}
	return facts.Normalized()
}

// runtimeTestProjectSealedSemanticFacts is test scaffolding for operation
// fixtures. It projects an already compiled semantic document through the
// same canonical fields that production's private seal transition writes;
// it is never a production activation or recovery path.
func runtimeTestProjectSealedSemanticFacts(t *testing.T, facts runtimecontract.RuntimeFacts, ledger runtimecontract.RuntimeSemanticPlanningLedger) runtimecontract.RuntimeFacts {
	t.Helper()
	if facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent || ledger.Manifest == nil || ledger.Document == nil {
		t.Fatalf("test semantic projection requires initialized ledger and complete document")
	}
	var acceptedBatch *runtimecontract.RuntimeSemanticPlanBatch
	for index := range ledger.Batches {
		if ledger.Batches[index].BatchID == ledger.Document.BatchID {
			value := ledger.Batches[index].Clone()
			acceptedBatch = &value
			break
		}
	}
	if acceptedBatch == nil || acceptedBatch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted || acceptedBatch.DocumentHash != ledger.DocumentHash {
		t.Fatalf("test semantic projection requires the exact accepted document batch")
	}
	focuses, contract, err := runtimecontract.CompileRuntimeSemanticBindings(*ledger.Manifest, acceptedBatch.RequestLogicalID, *ledger.Document, ledger.Feasibilities)
	if err != nil {
		t.Fatalf("compile test semantic bindings for canonical projection: %v", err)
	}
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(facts)
	if len(focuses) == 0 || len(focuses) != len(ledger.Document.BatchCoverageWorkItemIDs) || len(focuses) > len(eligible) {
		t.Fatalf("test semantic projection must bind the accepted batch exactly once: focuses=%d batch=%d eligible=%d", len(focuses), len(ledger.Document.BatchCoverageWorkItemIDs), len(eligible))
	}
	eligibleByID := make(map[common.CoverageWorkItemID]struct{}, len(eligible))
	for _, coverageID := range eligible {
		eligibleByID[coverageID] = struct{}{}
	}
	sealedByID := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(focuses))
	coverageIndex := make(map[common.CoverageWorkItemID]int, len(facts.Coverage))
	for index, item := range facts.Coverage {
		coverageIndex[item.ID] = index
	}
	sourceByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(facts.Coverage))
	for _, source := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		source = source.Normalized()
		for _, coverageID := range source.CoverageWorkItemIDs {
			sourceByCoverage[coverageID] = source
		}
	}
	// The pre-seal queue is intentionally only a generation cursor. Restore the
	// immutable declaration view before copying target-grounding context onto
	// the newly sealed semantic focus; otherwise this test projection would
	// erase the very Definition authority production keeps separately.
	for _, declaration := range facts.DefinitionFocuses {
		declaration = declaration.Normalized()
		for _, coverageID := range declaration.CoverageWorkItemIDs {
			sourceByCoverage[coverageID] = declaration.CoverageFocusDeclaration()
		}
	}
	for focusIndex := range focuses {
		focus := focuses[focusIndex].Normalized()
		if len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 1 {
			t.Fatalf("test semantic focus is not one-CWI canonical projection: %+v", focus)
		}
		coverageID := focus.CoverageWorkItemIDs[0]
		if _, eligible := eligibleByID[coverageID]; !eligible {
			t.Fatalf("test semantic projection binds ineligible coverage %q", coverageID)
		}
		if _, duplicate := sealedByID[coverageID]; duplicate {
			t.Fatalf("test semantic projection duplicates coverage %q", coverageID)
		}
		// CompileRuntimeSemanticBindings already projects the immutable
		// Definition obligation onto the semantic focus. Do not overlay the
		// pre-seal cursor here: that would create a focus which has no exact
		// compiler/ledger preimage and could therefore pass tests in a shape that
		// production recovery must reject.
		index, found := coverageIndex[coverageID]
		if !found {
			t.Fatalf("test semantic focus references unknown coverage %q", coverageID)
		}
		binding := focus.CoverageEvidenceBindings[0]
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, ledger.Manifest.Hash)
		item := &facts.Coverage[index]
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateBound
		item.CurrentBindingRef = &ref
		item.BindingLineage = append(item.BindingLineage, ref)
		item.FocusID = focus.ID
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateOpen
		}
		// The projected queue below uses focuses, not sealedByID. Keep the
		// source-declared grounding facts on that exact focus instance so the
		// normal prerequisite dispatcher can acquire File's selection.
		focuses[focusIndex] = focus
		sealedByID[coverageID] = focus
	}
	pendingByID := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(eligible)-len(sealedByID))
	for _, coverageID := range eligible {
		if _, sealed := sealedByID[coverageID]; sealed {
			continue
		}
		source, found := sourceByCoverage[coverageID]
		if !found {
			t.Fatalf("test semantic projection has no source focus for pending coverage %q", coverageID)
		}
		source.ID = "semantic-pending/" + string(coverageID)
		source.CoverageWorkItemIDs = []common.CoverageWorkItemID{coverageID}
		source.CoverageEvidenceBindings = nil
		pendingByID[coverageID] = source.Normalized()
		index := coverageIndex[coverageID]
		item := &facts.Coverage[index]
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatalf("test semantic projection cannot return settled coverage %q to pending", coverageID)
		}
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
		item.CurrentBindingRef = nil
	}
	queue := append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	retained := make([]runtimecontract.CoverageFocus, 0, len(queue))
	currentReplaced := false
	for queueIndex, source := range queue {
		source = source.Normalized()
		remainingIDs := make([]common.CoverageWorkItemID, 0, len(source.CoverageWorkItemIDs))
		for _, coverageID := range source.CoverageWorkItemIDs {
			if _, replaced := sealedByID[coverageID]; replaced {
				if queueIndex == 0 {
					currentReplaced = true
				}
				continue
			}
			if _, replaced := pendingByID[coverageID]; replaced {
				continue
			}
			remainingIDs = append(remainingIDs, coverageID)
		}
		if len(remainingIDs) == 0 {
			continue
		}
		source.CoverageWorkItemIDs = remainingIDs
		bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(source.CoverageEvidenceBindings))
		for _, binding := range source.CoverageEvidenceBindings {
			if _, sealed := sealedByID[binding.CoverageWorkItemID]; sealed {
				continue
			}
			if _, pending := pendingByID[binding.CoverageWorkItemID]; !pending {
				bindings = append(bindings, binding)
			}
		}
		source.CoverageEvidenceBindings = bindings
		retained = append(retained, source.Normalized())
	}
	if !currentReplaced {
		t.Fatalf("test semantic projection does not bind the current eligible focus")
	}
	pending := make([]runtimecontract.CoverageFocus, 0, len(pendingByID))
	for _, coverageID := range eligible {
		if source, found := pendingByID[coverageID]; found {
			pending = append(pending, source)
		}
	}
	projected := append(append(append([]runtimecontract.CoverageFocus(nil), focuses...), pending...), retained...)
	if len(projected) == 0 {
		t.Fatal("test semantic projection has no current focus")
	}
	facts.Focus = projected[0]
	facts.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), projected[1:]...)

	focusByCoverage := make(map[common.CoverageWorkItemID]string, len(facts.Coverage))
	boundFocuses := make([]runtimecontract.CoverageFocus, 0, len(projected)+len(facts.CompletedFocuses))
	for _, source := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		source = source.Normalized()
		for _, coverageID := range source.CoverageWorkItemIDs {
			if previous, duplicate := focusByCoverage[coverageID]; duplicate {
				t.Fatalf("test semantic projection duplicates coverage %q in focus %q and %q", coverageID, previous, source.ID)
			}
			focusByCoverage[coverageID] = source.ID
		}
		if len(source.CoverageEvidenceBindings) != 0 {
			boundFocuses = append(boundFocuses, source)
		}
	}
	for index := range facts.Coverage {
		focusID, found := focusByCoverage[facts.Coverage[index].ID]
		if !found {
			t.Fatalf("test semantic projection leaves coverage %q without a focus", facts.Coverage[index].ID)
		}
		facts.Coverage[index].FocusID = focusID
	}
	if len(boundFocuses) == len(focuses) && len(facts.CompletedFocuses) == 0 {
		facts.ReconciliationContract = contract
	} else {
		currentContract, compileErr := runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, boundFocuses)
		if compileErr != nil {
			t.Fatalf("compile test canonical reconciliation contract: %v", compileErr)
		}
		facts.ReconciliationContract = currentContract
	}
	// Mirror the production semantic-seal boundary: continuations that were
	// derived for the pre-seal focus cannot remain authority after SM-2 replaces
	// that focus with its CWI-scoped semantic projection. The subsequent reducer
	// reconstructs any still-applicable repair directive for the new focus.
	facts.RepairDirective = nil
	facts.Feasibility = nil
	facts.Failure = nil
	return facts.Normalized()
}

// runtimeTestPersistSealedSemanticAuthority upgrades an already initialized
// post-semantic operation fixture through the same RuntimeStateRepository CAS
// boundary used by production. It is intentionally test-only: semantic-drive
// tests must execute the real proposal/feasibility/seal sequence instead.
func runtimeTestPersistSealedSemanticAuthority(
	t *testing.T,
	ctx context.Context,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
) runtime.RuntimeAggregate {
	return runtimeTestPersistSealedSemanticAuthorityWithProviderCeiling(t, ctx, repository, aggregate, true)
}

func runtimeTestPersistSealedSemanticAuthorityWithoutProviderCeiling(
	t *testing.T,
	ctx context.Context,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
) runtime.RuntimeAggregate {
	return runtimeTestPersistSealedSemanticAuthorityWithProviderCeiling(t, ctx, repository, aggregate, false)
}

func runtimeTestPersistSealedSemanticAuthorityWithProviderCeiling(
	t *testing.T,
	ctx context.Context,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
	retainProviderCeiling bool,
) runtime.RuntimeAggregate {
	t.Helper()
	if repository == nil {
		t.Fatal("runtime test semantic authority repository is nil")
	}
	next := runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	if !retainProviderCeiling {
		next.Facts.Resources.ProviderCeiling = nil
	}
	if next.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		t.Fatalf("runtime test semantic authority phase = %q", next.SemanticPlanning.Phase)
	}
	next.StateRevision = aggregate.StateRevision + 1
	if err := repository.Save(ctx, next, aggregate.StateRevision); err != nil {
		t.Fatalf("persist runtime test semantic authority: %v", err)
	}
	return next
}

func runtimeTestInitializeOperationFixture(
	t *testing.T,
	ctx context.Context,
	owner *Biz,
	repository runtime.RuntimeStateRepository,
	facts runtimecontract.RuntimeFacts,
	definition runtimecontract.RuntimeDefinitionContext,
	requirements []runtimecontract.RuntimeRequirement,
) runtime.RuntimeAggregate {
	t.Helper()
	initialized, err := owner.InitializeRuntime(ctx, facts, definition, requirements)
	if err != nil {
		t.Fatalf("initialize runtime operation fixture: %v", err)
	}
	return runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
}

// runtimeCandidateSealFileEditFacts retains the File owner's selector-scoped
// admissibility preimage used by the synthetic semantic fixture. Candidate
// admission and final seal still compile the manifest and per-operation target
// snapshot through the production path.
func runtimeCandidateSealFileEditFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	facts := runtimeAggregate(t, 1).Facts
	if len(facts.Focus.CoverageWorkItemIDs) != 1 {
		t.Fatalf("candidate seal fixture requires one coverage item: %+v", facts.Focus)
	}
	selectorRef := "test/fixture-owner-selector/" + string(facts.Focus.CoverageWorkItemIDs[0])
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID:   "test-semantic-" + runtimecontract.RuntimeModuleFile,
		OwnerModuleID:    runtimecontract.RuntimeModuleFile,
		OperationKinds:   []string{runtimecontract.StepKindFileEdit},
		SelectorRefs:     []string{selectorRef},
		ObservedRevision: "test-semantic-revision-" + runtimecontract.RuntimeModuleFile,
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
			SelectorRefs:          []string{selectorRef},
		}},
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("candidate seal fixture File owner catalog: %v", err)
	}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimeOperationAdmissibilityPrerequisite(catalog))
	facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, runtimeFileAdmissibilityObservation(catalog))
	return facts
}

// runtimeCandidateSealExactFileEditFacts supplies one finite File-owned target
// together with the catalog snapshot and freshness observation that authorized
// it. Restart fixtures use this when the durable proposal/document itself names
// a concrete target; selector-only authority would make that persisted input
// invalid before the restart boundary under test is reached.
func runtimeCandidateSealExactFileEditFacts(t *testing.T, targetRef string) runtimecontract.RuntimeFacts {
	t.Helper()
	if targetRef == "" {
		t.Fatal("candidate seal exact File edit fixture requires a target")
	}
	return runtimeCandidateSealExactFileEditTargetsFacts(t, []string{targetRef})
}

func runtimeCandidateSealExactFileEditTargetsFacts(t *testing.T, targetRefs []string) runtimecontract.RuntimeFacts {
	t.Helper()
	if len(targetRefs) == 0 {
		t.Fatal("candidate seal exact File edit fixture requires targets")
	}
	facts := runtimeCandidateSealFileEditFacts(t)
	facts.Focus.TargetRefs = append([]string(nil), targetRefs...)
	catalog := facts.Resources.OperationAdmissibilities[0]
	catalog.TargetRefs = append([]string(nil), targetRefs...)
	catalog.SelectorRefs = nil
	for index := range catalog.OperationVariants {
		catalog.OperationVariants[index].AllowedTargetPolicies = []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget}
		catalog.OperationVariants[index].SelectorRefs = nil
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("candidate seal exact File owner catalog: %v", err)
	}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeOperationAdmissibilityPrerequisite(catalog)}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeFileAdmissibilityObservation(catalog)}
	return facts
}

// startRuntimeCandidateFileEditAdmissionOwners supplies the two typed owner
// facts needed after the File edit catalog is compiled into an
// existing_owner_scope candidate manifest. The Workspace owner resolves that
// scope, while the File owner returns the operation-scoped feasibility fact;
// neither observer manufactures or bypasses candidate authority.
func startRuntimeCandidateFileEditAdmissionOwners(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	deliverableDirectory, internalDirectory := t.TempDir(), t.TempDir()
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: deliverableDirectory, InternalDirectory: internalDirectory,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		response, responseErr := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, "", nil, nil, nil, nil)
		if responseErr != nil {
			t.Errorf("build File owner feasibility result: %v", responseErr)
			result.Set(nil, runtimeOperationMagicFailure(responseErr))
			return
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility) })
}

// runtimeTestPersistBootstrappedPreSemanticFixture persists only the immutable
// startup aggregate. It is for prerequisite/semantic-drive tests whose subject
// would be bypassed by a pre-sealed synthetic semantic ledger.
func runtimeTestPersistBootstrappedPreSemanticFixture(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	bootstrap runtimecontract.RuntimeBootstrap,
) runtime.RuntimeAggregate {
	t.Helper()
	if hub == nil {
		t.Fatal("runtime test pre-semantic fixture hub is nil")
	}
	facts, err := bootstrap.InitialFacts(workspaceID, runID)
	if err != nil {
		t.Fatalf("build runtime pre-semantic fixture facts: %v", err)
	}
	repository := runtimeStateRepository{hub: hub}
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatalf("configure runtime pre-semantic fixture repository: %v", err)
	}
	initialized, err := owner.InitializeRuntime(ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatalf("initialize runtime pre-semantic fixture: %v", err)
	}
	return initialized
}

// runtimeTestPersistBootstrappedOperationFixture prepares a Run-backed test
// that starts from a RuntimeBootstrap but only exercises the post-semantic
// operation/recovery mainline. Fresh semantic-drive tests must not use it.
func runtimeTestPersistBootstrappedOperationFixture(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	bootstrap runtimecontract.RuntimeBootstrap,
) runtime.RuntimeAggregate {
	t.Helper()
	if hub == nil {
		t.Fatal("runtime test operation fixture hub is nil")
	}
	facts, err := bootstrap.InitialFacts(workspaceID, runID)
	if err != nil {
		t.Fatalf("build runtime operation fixture facts: %v; definition focuses=%+v", err, bootstrap.DefinitionFocuses)
	}
	repository := runtimeStateRepository{hub: hub}
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatalf("configure runtime operation fixture repository: %v", err)
	}
	initialized, err := owner.InitializeRuntime(ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatalf("initialize bootstrapped runtime operation fixture: %v", err)
	}
	// Production seals semantic authority only after capability owners have
	// committed their pre-semantic facts. In particular, capability-state and
	// operation-admissibility snapshots must be the exact inputs later copied
	// into the semantic manifest; a synthetic catalog cannot be replaced after
	// seal without invalidating restart authority.
	prepared, err := owner.prepareRuntimePrerequisites(ctx, initialized)
	if err != nil {
		t.Fatalf("prepare bootstrapped runtime operation fixture prerequisites: %v", err)
	}
	return runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, prepared)
}

// Most orchestrator drive fixtures exercise the post-semantic operations
// mainline. Seal a synthetic, internally valid semantic ledger so those tests
// do not accidentally become semantic-provider fixtures. Dedicated semantic
// planning tests initialize the controller directly and retain the required
// phase.
type runtimeTestSemanticFixtureSpec struct {
	proposal         runtimecontract.RuntimeSemanticProofProposal
	feasibilityRefs  []string
	validationChecks []string
	selectorRefs     []string
	constraintRefs   []string
	targetRefs       []string
}

// runtimeTestAdvanceSemanticPlanBatchState seals fixture state without
// exposing the production SM-3 reducer outside biz/runtime.
func runtimeTestAdvanceSemanticPlanBatchState(batch runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (runtimecontract.RuntimeSemanticPlanBatch, error) {
	batch = batch.Normalized()
	batch.State = next
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	return batch, nil
}

// runtimeTestSealedSemanticPlanning builds a minimal valid semantic fact set
// from the fixture's declared CWI focus. It intentionally does not invent a
// File-only target or operation: post-semantic runtime tests need their
// original owner, kind, target policy and validation facts to remain intact.
func runtimeTestSealedSemanticPlanning(t *testing.T, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) runtimecontract.RuntimeSemanticPlanningLedger {
	t.Helper()
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	if len(eligible) == 0 {
		t.Fatal("test semantic fixture has no CWI eligible for planning")
	}
	validationProofRequirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		aggregate.Facts.DefinitionRevision,
		aggregate.Facts.DefinitionSeed.CoverageItems,
		aggregate.Facts.DefinitionFocuses,
	)
	if err != nil {
		t.Fatalf("compile test validation proof requirements: %v", err)
	}
	fixtureSpecs := runtimeTestSemanticFixtureSpecs(t, aggregate, eligible)
	catalogs := runtimeTestSemanticFixtureCatalogs(t, aggregate.Facts.Resources.OperationAdmissibilities, fixtureSpecs, validationProofRequirements)
	coverageItems := runtimeTestSemanticFixtureCoverageItems(t, aggregate.Facts.DefinitionSeed, eligible)
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(coverageItems))
	for _, item := range coverageItems {
		coverageIDs = append(coverageIDs, item.ID)
	}
	compiledManifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.DefinitionFocuses,
		aggregate.Facts.Activation.ExecutionContract,
		catalogs,
		coverageIDs,
	)
	if err != nil {
		t.Fatalf("compile test semantic proof obligations: %v", err)
	}
	manifest := compiledManifest.Clone()
	coverageOrder, err := runtimeTestSemanticCoverageOrderForItems(coverageItems)
	if err != nil {
		t.Fatalf("build test semantic coverage order: %v", err)
	}
	orderDocument := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: coverageOrder,
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("validate test semantic coverage order: %v", err)
	}
	activeCoverageIDs := runtimeTestSemanticActiveCoverageIDs(aggregate.Facts, eligible)
	if len(activeCoverageIDs) == 0 {
		if len(coverageOrder) == 0 {
			t.Fatal("test semantic fixture has no coverage order")
		}
		activeCoverageIDs = []common.CoverageWorkItemID{coverageOrder[0]}
	}
	batch, err := semanticplan.CompileFocusBatch(
		manifest, orderDocument, 0, "test-semantic-request", activeCoverageIDs, coverageOrder, len(coverageOrder),
	)
	if err != nil {
		t.Fatalf("compile test semantic focus batch: %v", err)
	}
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchRequesting,
		runtimecontract.RuntimeSemanticPlanBatchDocumentComplete,
	} {
		batch, err = runtimeTestAdvanceSemanticPlanBatchState(batch, next)
		if err != nil {
			t.Fatalf("advance test semantic focus batch: %v", err)
		}
	}
	specsByCoverage := make(map[common.CoverageWorkItemID][]runtimeTestSemanticFixtureSpec, len(fixtureSpecs))
	obligationsByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, obligation := range manifest.ProofObligations {
		obligationsByCoverage[obligation.CoverageWorkItemID] = obligation.Clone()
	}
	for _, spec := range fixtureSpecs {
		coverageID := spec.proposal.CoverageWorkItemID
		obligation, found := obligationsByCoverage[coverageID]
		if !found {
			t.Fatalf("test semantic fixture has no sealed Definition obligation for coverage %q", coverageID)
		}
		spec.proposal.DeclarationRef = obligation.DeclarationRef
		if obligation.ValidationProofRequirement != nil &&
			runtimeTestStringContains(spec.proposal.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) {
			requirement := obligation.ValidationProofRequirement.Clone()
			// A recipe-neutral exact requirement is selected by the owner catalog;
			// retain the concrete fixture proposal recipe in that case. Only an
			// immutable Profile pin may replace it here.
			if requirement.ValidationRecipeID != "" {
				spec.proposal.ValidationRecipeID = requirement.ValidationRecipeID
			}
			spec.validationChecks = append([]string(nil), requirement.RequiredCheckRefs...)
		}
		spec.proposal = spec.proposal.Normalized()
		specsByCoverage[coverageID] = append(specsByCoverage[coverageID], spec)
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
	}
	feasibilities := make([]runtimecontract.RuntimeSemanticProofFeasibility, 0, len(fixtureSpecs))
	for _, coverageID := range batch.CoverageWorkItemIDs {
		specs, found := specsByCoverage[coverageID]
		if !found {
			t.Fatalf("test semantic batch references unknown fixture coverage %q", coverageID)
		}
		for _, spec := range specs {
			proposal := spec.proposal
			document.Proposals = append(document.Proposals, proposal)
			catalog := runtimeTestSemanticFixtureCatalogForOwner(t, catalogs, proposal.OwnerModuleID)
			feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
				SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
				ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
				CoverageWorkItemID: coverageID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
				OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
				ObservedRevision: catalog.ObservedRevision, TargetRefs: append([]string(nil), spec.feasibilityRefs...),
				ValidationRecipeID: proposal.ValidationRecipeID, RequiredCheckRefs: append([]string(nil), spec.validationChecks...),
			}
			runtimeTestAttachProspectiveOwnerFactSatisfaction(t, manifest, proposal, catalog, &feasibility)
			feasibilities = append(feasibilities, feasibility)
		}
	}
	if len(document.Proposals) < len(batch.CoverageWorkItemIDs) {
		t.Fatal("test semantic focus proposals do not cover every batch CWI")
	}
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("validate test semantic proof document: %v policies=%v proposals=%+v", err, manifest.AllowedTargetPolicies, document.Proposals)
	}
	batch.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete,
		runtimecontract.RuntimeSemanticPlanBatchAccepted,
	} {
		batch, err = runtimeTestAdvanceSemanticPlanBatchState(batch, next)
		if err != nil {
			t.Fatalf("accept test semantic focus batch: %v", err)
		}
	}
	merged := document.Clone()
	_, reconciliation, err := runtimecontract.CompileRuntimeSemanticBindings(manifest, "test-semantic-request", document, feasibilities)
	if err != nil {
		t.Fatalf("compile test semantic bindings: %v proposals=%+v", err, document.Proposals)
	}
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.Manifest = &manifest
	ledger.CoverageOrderDocument = &orderDocument
	ledger.Batches = []runtimecontract.RuntimeSemanticPlanBatch{batch}
	ledger.BatchDocuments = []runtimecontract.RuntimeSemanticPlanDocument{document}
	batchFeasibility := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: batch.BatchID, BatchHash: batch.Hash,
		DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(document),
		Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), feasibilities...),
	}
	batchFeasibility.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchFeasibility)
	ledger.BatchFeasibilityResults = []runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{batchFeasibility}
	ledger.CompletedBatchIDs = []string{batch.BatchID}
	ledger.NextBatchSequence = 1
	ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), batch.PendingCoverageWorkItemIDs...)
	ledger.StablePrefixHash = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	ledger.LogicalRequestID = "test-semantic-request"
	ledger.Document = &document
	ledger.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(manifest, document)
	ledger.Feasibilities = feasibilities
	ledger.FeasibilitySetHash = runtimecontract.StableRuntimeSemanticFeasibilitySetHash(feasibilities)
	ledger.MergedDocument = &merged
	ledger.MergedDocumentHash = ledger.DocumentHash
	ledger.SealedBindingSetHash = reconciliation.BindingSetHash
	ledger.SealedLineageRefs = []string{manifest.Hash, runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(orderDocument), ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, ledger.SealedBindingSetHash}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("build sealed semantic planning fixture: %v", err)
	}
	return ledger.Normalized()
}

func runtimeTestAttachProspectiveOwnerFactSatisfaction(t *testing.T, manifest runtimecontract.RuntimeSemanticPlanManifest, proposal runtimecontract.RuntimeSemanticProofProposal, catalog runtimecontract.RuntimeOperationAdmissibility, feasibility *runtimecontract.RuntimeSemanticProofFeasibility) {
	t.Helper()
	if feasibility == nil {
		t.Fatal("prospective owner-fact fixture requires feasibility")
	}
	var acquisition *runtimecontract.RuntimeSemanticOwnerFactAcquisition
	for _, obligation := range manifest.ProofObligations {
		if obligation.CoverageWorkItemID == proposal.CoverageWorkItemID && obligation.OwnerFactAcquisition != nil {
			value := obligation.OwnerFactAcquisition.Clone()
			acquisition = &value
			break
		}
	}
	if acquisition == nil || acquisition.AcquisitionTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return
	}
	feasibility.CatalogObservationID = "test-prospective-file-observation"
	delta, err := runtimecontract.NewRuntimeProspectiveWorkspaceManifestDelta(
		manifest, proposal, acquisition.ProspectiveSatisfactionContractRef, catalog,
		runtimecontract.WorkspaceManifest{Revision: catalog.ObservedRevision, TargetRefs: append([]string(nil), catalog.TargetRefs...)},
	)
	if err != nil {
		t.Fatalf("build test prospective workspace delta: %v proposal=%+v catalog=%+v", err, proposal, catalog)
	}
	var dependencyCatalog runtimecontract.RuntimeOperationAdmissibility
	for _, candidate := range manifest.OwnerFactDependencyCatalogs {
		if candidate.OwnerModuleID == acquisition.Requirement.OwnerModuleID {
			dependencyCatalog = candidate
			break
		}
	}
	if dependencyCatalog.SnapshotID == "" {
		t.Fatalf("build test prospective satisfaction: missing dependency catalog %q", acquisition.Requirement.OwnerModuleID)
	}
	satisfaction := runtimecontract.RuntimeOwnerFactProspectiveSatisfaction{
		SchemaVersion: runtimecontract.RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent,
		WorkspaceID:   manifest.WorkspaceID, RunID: manifest.RunID, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), Requirement: acquisition.Requirement,
		ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
		OwnerModuleID:                      acquisition.Requirement.OwnerModuleID, CatalogSnapshotID: dependencyCatalog.SnapshotID,
		CatalogObservationID: "test-prospective-required-owner-observation", ObservedRevision: dependencyCatalog.ObservedRevision,
		ProspectiveWorkspaceManifestDeltaHash: runtimecontract.StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta),
		Outcome:                               runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy, ProspectiveValidationRecipeRefs: []string{"test-prospective-validation-recipe"},
	}
	if err := satisfaction.Validate(); err != nil {
		t.Fatalf("build test prospective owner-fact satisfaction: %v", err)
	}
	feasibility.ProspectiveWorkspaceManifestDelta = &delta
	observation := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{
		FeasibilityBatchID: "test-feasibility-batch", RequestHash: "test-prospective-request", Satisfaction: &satisfaction,
	}
	feasibility.OwnerFactProspectiveSatisfactionObservation = &observation
}

func runtimeTestStringContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func runtimeTestSemanticFixtureSpecs(t *testing.T, aggregate runtime.RuntimeAggregate, eligible []common.CoverageWorkItemID) []runtimeTestSemanticFixtureSpec {
	t.Helper()
	sources := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(aggregate.Facts.Coverage))
	for _, source := range append(append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...), aggregate.Facts.CompletedFocuses...) {
		source = source.Normalized()
		for _, coverageID := range source.CoverageWorkItemIDs {
			if previous, duplicate := sources[coverageID]; duplicate {
				t.Fatalf("test semantic fixture duplicates coverage %q in focus %q and %q", coverageID, previous.ID, source.ID)
			}
			sources[coverageID] = source
		}
	}
	// RuntimeDefinitionFocus is the activation declaration SoT. Overlay its
	// immutable fields onto any hand-written generation fixture while retaining
	// generation-only target/operation data that a direct post-semantic test
	// explicitly supplied. A production semantic proposal receives the same
	// declaration through RuntimeSemanticProofObligation.
	for _, declaration := range aggregate.Facts.DefinitionFocuses {
		declaration = declaration.Normalized()
		for _, coverageID := range declaration.CoverageWorkItemIDs {
			source := sources[coverageID]
			generationID := source.ID
			generationTargetRefs := append([]string(nil), source.TargetRefs...)
			generationProspectiveTargetRefs := append([]string(nil), source.ProspectiveTargetRefs...)
			generationAcceptedKinds := append([]string(nil), source.AcceptedStepKinds...)
			generationAcceptedVariants := append([]runtimecontract.RuntimeOperationVariantRef(nil), source.AcceptedOperationVariants...)
			generationValidationRecipeID := source.ValidationRecipeID
			generationRequiredCheckRefs := append([]string(nil), source.RequiredCheckRefs...)
			source = declaration.CoverageFocusDeclaration()
			if generationID != "" {
				source.ID = generationID
			}
			source.TargetRefs = generationTargetRefs
			source.ProspectiveTargetRefs = generationProspectiveTargetRefs
			source.AcceptedStepKinds = generationAcceptedKinds
			source.AcceptedOperationVariants = generationAcceptedVariants
			source.ValidationRecipeID = generationValidationRecipeID
			source.RequiredCheckRefs = generationRequiredCheckRefs
			sources[coverageID] = source.Normalized()
		}
	}
	specs := make([]runtimeTestSemanticFixtureSpec, 0, len(eligible)*2)
	for _, coverageID := range eligible {
		source, found := sources[coverageID]
		if !found {
			t.Fatalf("test semantic fixture has no source focus for coverage %q", coverageID)
		}
		// A missing immutable owner fact specializes the current proof to the
		// one pre-seal acquisition declared by Definition. Resolve that rule
		// before projecting terminal evidence kinds: otherwise a synthetic Shell
		// validation proposal can be paired later with a File acquisition
		// obligation, a shape production admission never permits.
		if len(runtimecontract.RuntimeMissingOwnerFactRequirements(source.RequiredOwnerFacts, aggregate.Facts.Resources.OperationAdmissibilities)) != 0 {
			convergence, required := runtimecontract.ResolveRuntimeOwnerFactAcquisition(
				source.RequiredOwnerFacts,
				source.OwnerFactConvergences,
				aggregate.Facts.Resources.OperationAdmissibilities,
			)
			if !required {
				t.Fatalf("test semantic fixture coverage %q has a missing owner fact without one ready acquisition", coverageID)
			}
			specs = append(specs, runtimeTestSemanticFixtureAcquisitionSpec(t, aggregate, source, coverageID, convergence))
			continue
		}
		source = runtimeTestSemanticFixtureProfileOverlay(aggregate, source, coverageID)
		evidenceKinds, required := runtimeTestSemanticFixtureEvidenceKinds(source)
		for _, evidenceKind := range evidenceKinds {
			kind, ok := runtimeTestSemanticFixtureOperation(aggregate, source, evidenceKind)
			if !ok {
				if required {
					t.Fatalf("test semantic fixture coverage %q has no declared operation for required evidence %q", coverageID, evidenceKind)
				}
				continue
			}
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !found {
				t.Fatalf("test semantic fixture operation %q has no descriptor", kind)
			}
			variantID := runtimeTestSemanticFixtureVariant(t, aggregate, source, kind)
			target, feasibilityRefs, selectorRefs, constraintRefs, catalogTargets := runtimeTestSemanticFixtureTarget(t, aggregate.Facts, source, coverageID, evidenceKind, kind, variantID)
			proposal := runtimecontract.RuntimeSemanticProofProposal{
				CoverageWorkItemID: coverageID,
				OwnerModuleID:      descriptor.OwnerModuleID,
				OperationKind:      kind,
				OperationVariantID: variantID,
				Target:             target,
				RequiredEvidenceKinds: []string{
					evidenceKind,
				},
				DeclarationRef: "test/semantic-fixture/" + string(coverageID),
			}
			checks := []string(nil)
			switch evidenceKind {
			case runtimecontract.EvidenceKindStateChange:
				proposal.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
			case runtimecontract.EvidenceKindValidation:
				proposal.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
				if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
					proposal.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession}
				}
				proposal.ValidationRecipeID = source.ValidationRecipeID
				if proposal.ValidationRecipeID == "" {
					recipes := runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies)
					if len(recipes) == 1 {
						proposal.ValidationRecipeID = recipes[0].ID
					}
				}
				if proposal.ValidationRecipeID == "" {
					proposal.ValidationRecipeID = "test-runtime-validation-recipe"
				}
				checks = append([]string(nil), source.RequiredCheckRefs...)
				if len(checks) == 0 {
					checks = runtimeTestSemanticFixtureCatalogChecks(
						aggregate.Facts.Resources.OperationAdmissibilities,
						kind,
						variantID,
						proposal.ValidationRecipeID,
					)
				}
				if len(checks) == 0 && len(aggregate.Facts.Resources.OperationAdmissibilities) == 0 {
					checks = []string{proposal.ValidationRecipeID}
				}
			default:
				t.Fatalf("test semantic fixture selected unsupported evidence kind %q", evidenceKind)
			}
			specs = append(specs, runtimeTestSemanticFixtureSpec{
				proposal: proposal.Normalized(), feasibilityRefs: feasibilityRefs,
				validationChecks: checks, selectorRefs: selectorRefs, constraintRefs: constraintRefs,
				targetRefs: catalogTargets,
			})
		}
	}
	if len(specs) == 0 {
		t.Fatal("test semantic fixture has no proof proposals")
	}
	return specs
}

func runtimeTestSemanticFixtureCatalogChecks(
	catalogs []runtimecontract.RuntimeOperationAdmissibility,
	operationKind, variantID, recipeID string,
) []string {
	catalog, found := runtimeTestSemanticFixtureCurrentCatalog(catalogs, operationKind, variantID)
	if !found {
		return nil
	}
	for _, surface := range catalog.Normalized().ValidationSurfaces {
		surface = surface.Normalized()
		if surface.ID == recipeID && surface.OperationKind == runtimecontract.CanonicalRuntimeBlockKind(operationKind) {
			return append([]string(nil), surface.CheckRefs...)
		}
	}
	return nil
}

func runtimeTestSemanticFixtureAcquisitionSpec(
	t *testing.T,
	aggregate runtime.RuntimeAggregate,
	source runtimecontract.CoverageFocus,
	coverageID common.CoverageWorkItemID,
	convergence runtimecontract.RuntimeOwnerFactConvergence,
) runtimeTestSemanticFixtureSpec {
	t.Helper()
	source = source.Normalized()
	convergence = convergence.Normalized()
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(convergence.AcquisitionOperationKind)
	if !found || descriptor.OwnerModuleID != convergence.AcquisitionOwnerModuleID {
		t.Fatalf("test semantic acquisition %q has invalid owner operation %q", coverageID, convergence.AcquisitionOperationKind)
	}
	catalog := runtimeTestSemanticFixtureCatalogForOwner(t, aggregate.Facts.Resources.OperationAdmissibilities, convergence.AcquisitionOwnerModuleID).Normalized()
	variantIDs := []string(nil)
	if convergence.AcquisitionOperationVariantID != "" {
		variantIDs = []string{convergence.AcquisitionOperationVariantID}
	}
	allowed, selectors, constraints, variantFound, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
		catalog,
		convergence.AcquisitionOperationKind,
		variantIDs,
		convergence.AcquisitionTargetPolicy,
	)
	if err != nil || !variantFound || !allowed || !runtimeTestStringContains(selectors, convergence.AcquisitionTargetSelectorRef) {
		t.Fatalf("test semantic acquisition %q is outside owner catalog: found=%t allowed=%t selectors=%v err=%v", coverageID, variantFound, allowed, selectors, err)
	}

	target := runtimecontract.RuntimeOperationTargetAuthorization{Policy: convergence.AcquisitionTargetPolicy}
	feasibilityRefs := append([]string(nil), catalog.TargetRefs...)
	selectorRefs := []string{convergence.AcquisitionTargetSelectorRef}
	constraintRefs := []string(nil)
	switch convergence.AcquisitionTargetPolicy {
	case runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope:
		target.OwnerSelectorRefs = append([]string(nil), selectorRefs...)
	case runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		prospectiveTargets := append([]string(nil), source.ProspectiveTargetRefs...)
		if len(prospectiveTargets) == 0 && len(source.TargetRefs) != 0 {
			// TargetRefs here are the generation cursor retained before Definition
			// overlay; they are not copied from the startup declaration.
			prospectiveTargets = append([]string(nil), source.TargetRefs...)
		}
		if len(prospectiveTargets) == 0 && len(source.TargetSelectorCandidateRefs) != 0 {
			prospectiveTargets = []string{source.TargetSelectorCandidateRefs[0]}
		}
		if len(prospectiveTargets) == 0 {
			t.Fatalf("test semantic acquisition %q has neither generation target nor bounded selector candidate", coverageID)
		}
		for _, prospectiveTarget := range prospectiveTargets {
			if runtimeTestStringContains(catalog.TargetRefs, prospectiveTarget) {
				t.Fatalf("test semantic acquisition %q prospective target %q already exists in owner catalog", coverageID, prospectiveTarget)
			}
		}
		target.OwnerSelectorRefs = append([]string(nil), selectorRefs...)
		target.ProspectiveConstraintRefs = append([]string(nil), constraints...)
		target.ProspectiveTargetRefs = append([]string(nil), prospectiveTargets...)
		feasibilityRefs = append([]string(nil), prospectiveTargets...)
		constraintRefs = append([]string(nil), constraints...)
	default:
		t.Fatalf("test semantic acquisition %q has unsupported target policy %q", coverageID, convergence.AcquisitionTargetPolicy)
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: coverageID,
		OwnerModuleID:      convergence.AcquisitionOwnerModuleID,
		OperationKind:      convergence.AcquisitionOperationKind,
		OperationVariantID: convergence.AcquisitionOperationVariantID,
		Target:             target,
		RequiredEvidenceKinds: []string{
			runtimecontract.EvidenceKindStateChange,
		},
		RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:       "test/semantic-fixture/acquisition/" + string(coverageID),
	}.Normalized()
	return runtimeTestSemanticFixtureSpec{
		proposal: proposal, feasibilityRefs: feasibilityRefs,
		selectorRefs: selectorRefs, constraintRefs: constraintRefs,
		targetRefs: append([]string(nil), catalog.TargetRefs...),
	}
}

// runtimeTestSemanticFixtureProfileOverlay supplies generation-only values
// from the activation-sealed profile declaration. RuntimeDefinitionFocus is
// intentionally unable to carry these values; production SM-2 receives them
// through the semantic manifest, while this scripted provider needs the same
// declarative source to construct a valid proposal.
func runtimeTestSemanticFixtureProfileOverlay(aggregate runtime.RuntimeAggregate, source runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) runtimecontract.CoverageFocus {
	var matches []runtimecontract.RuntimeProfileEvidenceRequirement
	var coverage runtimecontract.RuntimeCoverageItemFact
	coverageFound := false
	for _, item := range aggregate.Facts.Coverage {
		if item.ID == coverageID {
			coverage, coverageFound = item, true
			break
		}
	}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(aggregate.Facts.Activation.ExecutionContract.ProfileContracts) {
		for _, requirement := range profile.EvidenceRequirements {
			if coverageFound && requirement.Scope == coverage.SourceScope && requirement.ID == coverage.SourceID {
				matches = append(matches, requirement)
			}
		}
	}
	if len(matches) != 1 {
		return source.Normalized()
	}
	requirement := matches[0]
	if len(source.TargetRefs) == 0 {
		source.TargetRefs = append([]string(nil), requirement.TargetRefs...)
	}
	if len(source.AcceptedStepKinds) == 0 {
		source.AcceptedStepKinds = append([]string(nil), requirement.AcceptedStepKinds...)
	}
	if len(source.AcceptedOperationVariants) == 0 {
		source.AcceptedOperationVariants = append([]runtimecontract.RuntimeOperationVariantRef(nil), requirement.AcceptedOperationVariants...)
	}
	if source.ValidationRecipeID == "" {
		source.ValidationRecipeID = requirement.ValidationRecipeID
	}
	if len(source.RequiredCheckRefs) == 0 {
		source.RequiredCheckRefs = append([]string(nil), requirement.RequiredCheckRefs...)
	}
	return source.Normalized()
}

func runtimeTestSemanticFixtureEvidenceKinds(source runtimecontract.CoverageFocus) ([]string, bool) {
	source = source.Normalized()
	ret := make([]string, 0, 2)
	for _, evidenceKind := range []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation} {
		if runtimeTestStringIncluded(source.RequiredEvidenceKinds, evidenceKind) {
			ret = append(ret, evidenceKind)
		}
	}
	if len(ret) != 0 {
		return ret, true
	}
	// A source focus without an independent proof declaration still needs one
	// bounded fixture proof. Prefer a state change and fall back to validation
	// only when that is the sole declared capability.
	return []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, false
}

func runtimeTestSemanticFixtureOperation(aggregate runtime.RuntimeAggregate, source runtimecontract.CoverageFocus, evidenceKind string) (string, bool) {
	source = source.Normalized()
	candidates := append([]string(nil), source.AcceptedStepKinds...)
	if len(candidates) == 0 {
		candidates = append(candidates, aggregate.Facts.Capabilities.StateChangeKinds...)
		candidates = append(candidates, aggregate.Facts.Capabilities.ValidationKinds...)
		candidates = append(candidates, aggregate.Definition.CapabilityContract...)
	}
	candidates = runtimeTestNormalizeStrings(candidates)
	for _, candidate := range candidates {
		kind := runtimecontract.CanonicalRuntimeBlockKind(candidate)
		if kind == "" || !runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
			continue
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID == "" {
			continue
		}
		return kind, true
	}
	return "", false
}

func runtimeTestSemanticFixtureVariant(t *testing.T, aggregate runtime.RuntimeAggregate, source runtimecontract.CoverageFocus, kind string) string {
	t.Helper()
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, ref := range source.Normalized().AcceptedOperationVariants {
		if ref.Kind == kind {
			return ref.VariantID
		}
	}
	profileVariants := make([]string, 0, 1)
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(aggregate.Facts.Activation.ExecutionContract.ProfileContracts) {
		for _, requirement := range profile.EvidenceRequirements {
			for _, ref := range requirement.AcceptedOperationVariants {
				if runtimecontract.CanonicalRuntimeBlockKind(ref.Kind) == kind {
					profileVariants = append(profileVariants, ref.VariantID)
				}
			}
		}
	}
	profileVariants = runtimeTestNormalizeStrings(profileVariants)
	if len(profileVariants) == 1 {
		return profileVariants[0]
	}
	if len(profileVariants) > 1 {
		t.Fatalf("test semantic fixture operation %q has ambiguous profile variants %v", kind, profileVariants)
	}
	contract, found := runtimecontract.RuntimeOperationContractForKind(kind)
	if !found || len(contract.Variants) == 0 {
		return ""
	}
	// RuntimeDefinitionFocus cannot carry generation-specific operation
	// variants. Dedicated variant-admission tests declare one on their generated
	// focus; generic post-semantic fixtures deterministically choose the first
	// owner-declared variant while constructing their synthetic SM-3 binding.
	return contract.Variants[0].ID
}

func runtimeTestSemanticFixtureTarget(
	t *testing.T,
	facts runtimecontract.RuntimeFacts,
	source runtimecontract.CoverageFocus,
	coverageID common.CoverageWorkItemID,
	evidenceKind string,
	operationKind string,
	variantID string,
) (runtimecontract.RuntimeOperationTargetAuthorization, []string, []string, []string, []string) {
	t.Helper()
	source = source.Normalized()
	targets := append([]string(nil), source.TargetRefs...)
	if len(targets) == 0 && facts.Resources.TargetSelection != nil {
		targets = append(targets, facts.Resources.TargetSelection.TargetRefs...)
	}
	if catalog, found := runtimeTestSemanticFixtureCurrentCatalog(facts.Resources.OperationAdmissibilities, operationKind, variantID); found {
		// A bounded multi-proof CWI must share one target surface across owners.
		// The pre-seal File catalog is the owner-grounded workspace inventory and
		// Shell receives that same bounded set when it publishes its validation
		// surface. Use the exact current catalog targets instead of two unrelated
		// owner selectors; no path is invented by the fixture.
		if len(targets) == 0 && len(source.ProspectiveTargetRefs) == 0 && len(source.TargetSelectorCandidateRefs) == 0 &&
			len(source.RequiredEvidenceKinds) > 1 && len(catalog.TargetRefs) != 0 {
			targets = append([]string(nil), catalog.TargetRefs...)
		}
		return runtimeTestSemanticFixtureTargetFromCatalog(t, source, coverageID, evidenceKind, operationKind, variantID, targets, catalog)
	}
	selector := source.TargetSelectorRef
	if selector == "" {
		selector = "test/fixture-owner-selector/" + string(coverageID)
	}
	prospectiveTargetRefs := append([]string(nil), source.ProspectiveTargetRefs...)
	if len(prospectiveTargetRefs) == 0 && len(targets) == 0 && len(source.TargetSelectorCandidateRefs) != 0 {
		prospectiveTargetRefs = []string{source.TargetSelectorCandidateRefs[0]}
	}
	if len(prospectiveTargetRefs) != 0 && evidenceKind == runtimecontract.EvidenceKindStateChange {
		constraints := []string{"test/fixture-prospective-target/" + string(coverageID)}
		if descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operationKind); found && descriptor.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			// File is the owner of workspace-path creation semantics. Use its
			// concrete selector/constraints so the post-dispatch reconciliation
			// observes the same owner surface instead of an invented test selector.
			selector = "workspace-relative-new-file-target"
			constraints = []string{"new-target-requires-owner-selector", "workspace-relative-path"}
		}
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:                    runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{selector},
			ProspectiveConstraintRefs: constraints,
			ProspectiveTargetRefs:     prospectiveTargetRefs,
		}, prospectiveTargetRefs, []string{selector}, constraints, nil
	}
	if len(prospectiveTargetRefs) != 0 && evidenceKind == runtimecontract.EvidenceKindValidation {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: prospectiveTargetRefs,
		}, prospectiveTargetRefs, nil, nil, prospectiveTargetRefs
	}
	if len(targets) == 0 && source.TargetSelectorRef != "" {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{selector},
		}, nil, []string{selector}, nil, nil
	}
	if len(targets) != 0 {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: targets,
		}, targets, nil, nil, targets
	}
	// A single-proof direct fixture can retain the historical closed synthetic
	// owner scope: no second owner exists with which its selector ownership could
	// conflict, and forcing an invented exact file would add a target-snapshot
	// prerequisite unrelated to the test. Multi-proof fixtures need one bounded
	// exact surface so independent state-change and validation owners converge.
	if len(source.RequiredEvidenceKinds) == 1 {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{selector},
		}, nil, []string{selector}, nil, []string{"target.txt"}
	}
	targets = []string{"target.txt"}
	return runtimecontract.RuntimeOperationTargetAuthorization{
		Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: targets,
	}, targets, nil, nil, targets
}

// runtimeTestSemanticFixtureCurrentCatalog returns the already persisted owner
// catalog for a proposal. An operation-ready fixture must never replace this
// snapshot with a synthetic selector after acquisition; that would sever the
// semantic binding from the exact owner observation used by recovery.
func runtimeTestSemanticFixtureCurrentCatalog(values []runtimecontract.RuntimeOperationAdmissibility, operationKind, variantID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operationKind)
	if !found {
		return runtimecontract.RuntimeOperationAdmissibility{}, false
	}
	var match runtimecontract.RuntimeOperationAdmissibility
	matched := false
	for _, value := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(values) {
		value = value.Normalized()
		if value.OwnerModuleID != descriptor.OwnerModuleID {
			continue
		}
		if _, factFound, err := runtimecontract.RuntimeOperationAdmissibilityVariantForProposal(value, operationKind, variantID); err != nil || !factFound {
			// Non-discriminated kinds legitimately use the catalog-level fact.
			if contract, ok := runtimecontract.RuntimeOperationContractForKind(operationKind); ok && len(contract.Variants) != 0 {
				continue
			}
			kindFound := false
			for _, kind := range value.OperationKinds {
				if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.CanonicalRuntimeBlockKind(operationKind) {
					kindFound = true
					break
				}
			}
			if !kindFound {
				continue
			}
		}
		// Mirror CompileRuntimeSemanticPlanManifest: when distinct prerequisite
		// identities publish the same owner, the normalized last owner snapshot is
		// the exact manifest input. Do not merge those snapshots in test code.
		match, matched = value, true
	}
	return match, matched
}

func runtimeTestSemanticFixtureTargetFromCatalog(
	t *testing.T,
	source runtimecontract.CoverageFocus,
	coverageID common.CoverageWorkItemID,
	evidenceKind, operationKind, variantID string,
	targets []string,
	catalog runtimecontract.RuntimeOperationAdmissibility,
) (runtimecontract.RuntimeOperationTargetAuthorization, []string, []string, []string, []string) {
	t.Helper()
	catalog = catalog.Normalized()
	allows := func(policy runtimecontract.RuntimeOperationTargetPolicy) (bool, []string, []string) {
		allowed, selectors, constraints, _, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(
			catalog, operationKind, variantID, policy,
		)
		if err != nil {
			t.Fatalf("test semantic owner catalog %q policy %q: %v", catalog.OwnerModuleID, policy, err)
		}
		return allowed, selectors, constraints
	}
	containsAll := func(haystack, needles []string) bool {
		for _, needle := range needles {
			if !runtimeTestStringContains(haystack, needle) {
				return false
			}
		}
		return true
	}

	prospective := append([]string(nil), source.ProspectiveTargetRefs...)
	if len(prospective) == 0 && len(source.TargetSelectorCandidateRefs) != 0 {
		for _, candidate := range source.TargetSelectorCandidateRefs {
			if !runtimeTestStringContains(catalog.TargetRefs, candidate) {
				prospective = append(prospective, candidate)
			}
		}
	}
	if len(prospective) != 0 && evidenceKind == runtimecontract.EvidenceKindStateChange {
		allowed, selectors, constraints := allows(runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget)
		if !allowed || len(selectors) == 0 || len(constraints) == 0 {
			t.Fatalf("test semantic coverage %q prospective target is outside persisted owner catalog %q", coverageID, catalog.SnapshotID)
		}
		// Definition.TargetSelectorRef is the immutable discovery declaration;
		// the operation authorization selector is a distinct owner-published
		// vocabulary. The semantic proposal must use the latter verbatim.
		selectorRefs := append([]string(nil), selectors...)
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs: selectorRefs, ProspectiveConstraintRefs: constraints,
			ProspectiveTargetRefs: prospective,
		}, prospective, selectorRefs, constraints, append([]string(nil), catalog.TargetRefs...)
	}
	if len(targets) != 0 {
		allowed, _, _ := allows(runtimecontract.RuntimeOperationTargetPolicyExactTarget)
		if !allowed || !containsAll(catalog.TargetRefs, targets) {
			t.Fatalf("test semantic coverage %q exact targets %v are outside persisted owner catalog %q targets=%v", coverageID, targets, catalog.SnapshotID, catalog.TargetRefs)
		}
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: targets,
		}, targets, nil, nil, append([]string(nil), catalog.TargetRefs...)
	}
	allowed, selectors, _ := allows(runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)
	if !allowed {
		t.Fatalf("test semantic coverage %q has no declaration-compatible target policy in persisted owner catalog %q", coverageID, catalog.SnapshotID)
	}
	selectorRefs := append([]string(nil), selectors...)
	ownerScopeTargets := append([]string(nil), catalog.TargetRefs...)
	if len(ownerScopeTargets) == 0 && len(selectorRefs) == 0 {
		t.Fatalf("test semantic coverage %q persisted owner catalog %q has no bounded existing scope", coverageID, catalog.SnapshotID)
	}
	return runtimecontract.RuntimeOperationTargetAuthorization{
		Policy:               runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
		OwnerScopeTargetRefs: ownerScopeTargets, OwnerSelectorRefs: selectorRefs,
	}, ownerScopeTargets, selectorRefs, nil, ownerScopeTargets
}

func runtimeTestSemanticFixtureCatalogs(
	t *testing.T,
	current []runtimecontract.RuntimeOperationAdmissibility,
	specs []runtimeTestSemanticFixtureSpec,
	validationProofRequirements []runtimecontract.RuntimeValidationProofRequirement,
) []runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	currentByOwner := make(map[string]runtimecontract.RuntimeOperationAdmissibility, len(current))
	for _, catalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(current) {
		catalog = catalog.Normalized()
		currentByOwner[catalog.OwnerModuleID] = catalog
	}
	byOwner := make(map[string]*runtimecontract.RuntimeOperationAdmissibility, len(specs))
	order := make([]string, 0, len(specs))
	variantIndex := make(map[string]map[string]int, len(specs))
	validationSurfaceIndex := make(map[string]map[string]int, len(specs))
	for _, spec := range specs {
		proposal := spec.proposal.Normalized()
		if currentCatalog, found := currentByOwner[proposal.OwnerModuleID]; found {
			if proposal.ValidationRecipeID != "" {
				surface, surfaceFound := runtimecontract.RuntimeValidationSurfaceForID(currentCatalog.ValidationSurfaces, proposal.ValidationRecipeID)
				recipeFound := true
				if proposal.OwnerModuleID == runtimecontract.RuntimeModuleShell {
					_, recipeFound = runtimecontract.RuntimeValidationRecipeForID(currentCatalog.ValidationRecipes, proposal.ValidationRecipeID)
				}
				found = surfaceFound && recipeFound && surface.Readiness == runtimecontract.RuntimeValidationSurfaceReady &&
					surface.ProofApplicability == runtimecontract.RuntimeValidationProofDirect
			}
			if !found {
				// The current owner fact is intentionally unavailable in a few
				// feasibility/reconciliation tests. Their already-sealed semantic
				// generation still needs a valid historical catalog preimage; build
				// that test-only preimage below without mutating current Facts.
				goto syntheticCatalog
			}
			if _, alreadyAdded := byOwner[proposal.OwnerModuleID]; !alreadyAdded {
				catalog := currentCatalog.Normalized()
				byOwner[proposal.OwnerModuleID] = &catalog
				order = append(order, proposal.OwnerModuleID)
			}
			continue
		}
	syntheticCatalog:
		catalog := byOwner[proposal.OwnerModuleID]
		if catalog == nil {
			catalog = &runtimecontract.RuntimeOperationAdmissibility{
				PrerequisiteID:   "test-semantic-" + proposal.OwnerModuleID,
				OwnerModuleID:    proposal.OwnerModuleID,
				ObservedRevision: "test-semantic-revision-" + proposal.OwnerModuleID,
			}
			byOwner[proposal.OwnerModuleID] = catalog
			order = append(order, proposal.OwnerModuleID)
			variantIndex[proposal.OwnerModuleID] = make(map[string]int)
			validationSurfaceIndex[proposal.OwnerModuleID] = make(map[string]int)
		}
		if variantIndex[proposal.OwnerModuleID] == nil {
			variantIndex[proposal.OwnerModuleID] = make(map[string]int)
			for index, variant := range catalog.OperationVariants {
				variant = variant.Normalized()
				variantIndex[proposal.OwnerModuleID][variant.Kind+"|"+variant.VariantID] = index
			}
		}
		if validationSurfaceIndex[proposal.OwnerModuleID] == nil {
			validationSurfaceIndex[proposal.OwnerModuleID] = make(map[string]int)
			for index, surface := range catalog.ValidationSurfaces {
				validationSurfaceIndex[proposal.OwnerModuleID][surface.Normalized().ID] = index
			}
		}
		catalog.OperationKinds = append(catalog.OperationKinds, proposal.OperationKind)
		catalog.TargetRefs = append(catalog.TargetRefs, spec.targetRefs...)
		catalog.SelectorRefs = append(catalog.SelectorRefs, spec.selectorRefs...)
		catalog.ConstraintRefs = append(catalog.ConstraintRefs, spec.constraintRefs...)
		if proposal.ValidationRecipeID != "" {
			if proposal.OwnerModuleID == runtimecontract.RuntimeModuleShell {
				catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, proposal.ValidationRecipeID)
				// A generated post-semantic Shell fixture catalog needs an executable
				// recipe declaration in addition to the provider-visible surface
				// identity. Other capability owners publish typed proof surfaces but do
				// not own Shell recipe inventory.
				catalog.ValidationRecipes = append(catalog.ValidationRecipes, runtimecontract.RuntimeValidationRecipe{
					ID: proposal.ValidationRecipeID, Command: "true",
				})
			}
			effect := runtimecontract.RuntimeEffectNone
			if len(proposal.RequiredEffectScopes) != 0 {
				effect = proposal.RequiredEffectScopes[0]
			}
			surface := runtimecontract.RuntimeValidationSurface{
				ID: proposal.ValidationRecipeID, OwnerModuleID: proposal.OwnerModuleID, OperationKind: proposal.OperationKind,
				Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "test-semantic-input-schema",
				InputBindingRef: "test-semantic-input/" + proposal.ValidationRecipeID,
				CheckRefs:       append([]string(nil), spec.validationChecks...), ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
				EffectScope: effect, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
			}.Normalized()
			if index, duplicate := validationSurfaceIndex[proposal.OwnerModuleID][surface.ID]; duplicate {
				existing := catalog.ValidationSurfaces[index].Normalized()
				if existing.OwnerModuleID != surface.OwnerModuleID || existing.OperationKind != surface.OperationKind ||
					existing.Readiness != surface.Readiness || existing.InputSchemaHash != surface.InputSchemaHash ||
					existing.InputBindingRef != surface.InputBindingRef || existing.ProofApplicability != surface.ProofApplicability ||
					existing.EffectScope != surface.EffectScope || existing.IdempotencyClass != surface.IdempotencyClass {
					t.Fatalf("test semantic validation surface %q has conflicting declarations", surface.ID)
				}
				existing.CheckRefs = runtimeTestNormalizeStrings(append(existing.CheckRefs, surface.CheckRefs...))
				catalog.ValidationSurfaces[index] = existing
			} else {
				catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, surface)
				validationSurfaceIndex[proposal.OwnerModuleID][surface.ID] = len(catalog.ValidationSurfaces) - 1
				catalog.OperationPolicies = append(catalog.OperationPolicies, runtimecontract.RuntimeOperationPolicyFact{
					ID: "test-semantic-policy/" + proposal.ValidationRecipeID, OwnerModuleID: proposal.OwnerModuleID,
					OperationKind: proposal.OperationKind, EffectScope: effect,
					IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
				})
			}
		}
		// Publish one explicit policy fact per kind/variant. This matters when a
		// single owner catalog mixes a discriminated kind (for example patch/apply)
		// with a non-variant kind: once any variant facts exist, the broad catalog
		// union is vocabulary only and cannot authorize the other kind.
		key := proposal.OperationKind + "|" + proposal.OperationVariantID
		if index, found := variantIndex[proposal.OwnerModuleID][key]; found {
			catalog.OperationVariants[index].AllowedTargetPolicies = append(catalog.OperationVariants[index].AllowedTargetPolicies, proposal.Target.Policy)
			catalog.OperationVariants[index].SelectorRefs = append(catalog.OperationVariants[index].SelectorRefs, spec.selectorRefs...)
			catalog.OperationVariants[index].ConstraintRefs = append(catalog.OperationVariants[index].ConstraintRefs, spec.constraintRefs...)
			continue
		}
		catalog.OperationVariants = append(catalog.OperationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
			Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{proposal.Target.Policy},
			SelectorRefs:          append([]string(nil), spec.selectorRefs...),
			ConstraintRefs:        append([]string(nil), spec.constraintRefs...),
		})
		variantIndex[proposal.OwnerModuleID][key] = len(catalog.OperationVariants) - 1
	}
	// CompileRuntimeSemanticPlanManifestForCoverage seals every capability owner
	// from the immutable Definition, even when the selected CWI proposal uses
	// only one of them. Preserve valid current owner catalogs that are not part
	// of this proof set; the compiler itself filters catalogs outside the
	// Definition capability vocabulary.
	for _, currentCatalog := range runtimecontract.NormalizeRuntimeOperationAdmissibilities(current) {
		if _, alreadyAdded := byOwner[currentCatalog.OwnerModuleID]; alreadyAdded {
			continue
		}
		if err := currentCatalog.Validate(); err != nil {
			continue
		}
		catalog := currentCatalog.Normalized()
		byOwner[catalog.OwnerModuleID] = &catalog
		order = append(order, catalog.OwnerModuleID)
	}
	ret := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(order))
	for _, owner := range order {
		catalog := byOwner[owner]
		if catalog.OwnerModuleID == runtimecontract.RuntimeModuleShell {
			for surfaceIndex := range catalog.ValidationSurfaces {
				surface := catalog.ValidationSurfaces[surfaceIndex].Normalized()
				if surface.OperationKind != runtimecontract.RuntimeBlockKindManagedValidationSession ||
					surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady ||
					surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect {
					continue
				}
				bindings := runtimecontract.CloneRuntimeValidationProofRequirementBindings(surface.ProofRequirementBindings)
				checks := make([]string, 0)
				assigned := make(map[string]struct{}, len(bindings))
				for _, binding := range bindings {
					assigned[binding.RequirementHash] = struct{}{}
					checks = append(checks, binding.RequiredCheckRefs...)
				}
				for _, requirement := range runtimecontract.NormalizeRuntimeValidationProofRequirements(validationProofRequirements) {
					if _, exists := assigned[requirement.Hash]; exists ||
						(requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != surface.ID) {
						continue
					}
					attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(requirement.RequiredCheckRefs))
					for index, checkRef := range requirement.RequiredCheckRefs {
						attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
							RequiredCheckRef: checkRef, ObservationProtocolRef: "test/opaque-validation-observation/v1",
							NativeCheckRef: fmt.Sprintf("test/native/%s/%s/%d", requirement.CoverageWorkItemID, surface.ID, index),
						})
					}
					binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, surface.ID, attestations...)
					if err != nil {
						t.Fatalf("bind test semantic validation requirement %q: %v", requirement.Hash, err)
					}
					bindings = append(bindings, binding)
					checks = append(checks, requirement.RequiredCheckRefs...)
					assigned[requirement.Hash] = struct{}{}
				}
				if len(bindings) != 0 {
					surface.ProofRequirementBindings = runtimecontract.NormalizeRuntimeValidationProofRequirementBindings(bindings)
					surface.CheckRefs = runtimeTestNormalizeStrings(checks)
					catalog.ValidationSurfaces[surfaceIndex] = surface
				}
			}
			catalog.ValidationSurfaces = runtimecontract.NormalizeRuntimeValidationSurfaces(catalog.ValidationSurfaces)
			catalog.ValidationRecipeRefs = runtimecontract.RuntimeSelectableValidationRecipeRefs(catalog.ValidationSurfaces)
			catalog.ValidationRecipes = runtimecontract.NormalizeRuntimeValidationRecipes(catalog.ValidationRecipes)
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
		if err := catalog.Validate(); err != nil {
			t.Fatalf("build test semantic owner catalog %q: %v requirements=%+v surfaces=%+v", owner, err, validationProofRequirements, catalog.ValidationSurfaces)
		}
		ret = append(ret, catalog.Normalized())
	}
	return ret
}

func runtimeTestSemanticFixtureCatalogForOwner(t *testing.T, catalogs []runtimecontract.RuntimeOperationAdmissibility, owner string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	for _, catalog := range catalogs {
		if catalog.OwnerModuleID == owner {
			return catalog
		}
	}
	t.Fatalf("test semantic fixture has no catalog for owner %q", owner)
	return runtimecontract.RuntimeOperationAdmissibility{}
}

func runtimeTestSemanticFixtureCoverageItems(t *testing.T, seed runtimecontract.RuntimeDefinitionSeed, eligible []common.CoverageWorkItemID) []runtimecontract.RuntimeCoverageItem {
	t.Helper()
	wanted := make(map[common.CoverageWorkItemID]struct{}, len(eligible))
	for _, coverageID := range eligible {
		wanted[coverageID] = struct{}{}
	}
	items := make([]runtimecontract.RuntimeCoverageItem, 0, len(eligible))
	for _, item := range seed.CoverageItems {
		if _, found := wanted[item.ID]; !found {
			continue
		}
		items = append(items, item)
		delete(wanted, item.ID)
	}
	if len(wanted) != 0 {
		for coverageID := range wanted {
			t.Fatalf("test semantic fixture eligible coverage %q is absent from the definition seed", coverageID)
		}
	}
	return items
}

func runtimeTestSemanticActiveCoverageIDs(facts runtimecontract.RuntimeFacts, eligible []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	eligibleByID := make(map[common.CoverageWorkItemID]struct{}, len(eligible))
	for _, coverageID := range eligible {
		eligibleByID[coverageID] = struct{}{}
	}
	ret := make([]common.CoverageWorkItemID, 0, len(facts.Focus.CoverageWorkItemIDs))
	for _, coverageID := range facts.Focus.Normalized().CoverageWorkItemIDs {
		if _, found := eligibleByID[coverageID]; found {
			ret = append(ret, coverageID)
		}
	}
	return ret
}

func runtimeTestNormalizeTargetPolicies(values []runtimecontract.RuntimeOperationTargetPolicy) []runtimecontract.RuntimeOperationTargetPolicy {
	seen := make(map[runtimecontract.RuntimeOperationTargetPolicy]struct{}, len(values))
	ret := make([]runtimecontract.RuntimeOperationTargetPolicy, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

// runtimeTestSemanticCoverageOrderForSupport builds a deterministic,
// dependency-valid provider-order fixture. It is test scaffolding only: the
// production Orchestrator accepts the provider-authored order and never
// derives one from this helper.
func runtimeTestSemanticCoverageOrderForSupport(seed runtimecontract.RuntimeDefinitionSeed) ([]common.CoverageWorkItemID, error) {
	return runtimeTestSemanticCoverageOrderForItems(seed.CoverageItems)
}

func runtimeTestSemanticCoverageOrderForItems(values []runtimecontract.RuntimeCoverageItem) ([]common.CoverageWorkItemID, error) {
	items := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(values))
	for _, item := range values {
		items[item.ID] = item
	}
	state := make(map[common.CoverageWorkItemID]uint8, len(items))
	order := make([]common.CoverageWorkItemID, 0, len(items))
	var visit func(common.CoverageWorkItemID) error
	visit = func(id common.CoverageWorkItemID) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("test semantic coverage dependency cycle at %q", id)
		case 2:
			return nil
		}
		item, found := items[id]
		if !found {
			return fmt.Errorf("test semantic coverage dependency %q is not in seed", id)
		}
		state[id] = 1
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			if err := visit(prerequisiteID); err != nil {
				return err
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	for _, item := range values {
		if err := visit(item.ID); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// runtimeCandidateRequestContentTestStore gives all Biz instances wired to
// one test EventHub the same opaque content repository. The production
// equivalent is process wiring to the durable repository; this helper keeps
// restart fixtures from replacing that repository while leaving raw provider
// documents subject to the normal C4/C5 persist-and-reload path.
func runtimeCandidateRequestContentTestStore(hub magicevent.Hub) repository.ArtifactContinuationRepository {
	if hub == nil {
		return memory.NewArtifactContinuationRepository()
	}
	store, _ := runtimeCandidateRequestContentTestStores.LoadOrStore(hub, memory.NewArtifactContinuationRepository())
	return store.(repository.ArtifactContinuationRepository)
}

// ensureRuntimeCandidatePreflightTestResponder makes test fakes state their
// rendered-request contract explicitly. Production always uses the LLM Block
// handler; this responder never calls a provider and only exists on test hubs
// that otherwise mock the candidate command directly.
func ensureRuntimeCandidatePreflightTestResponder(hub magicevent.Hub) {
	if hub == nil {
		return
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
		if !ok {
			return
		}
		feedback := runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainProviderPreflight, StableCategory: runtimecontract.RuntimeFeedbackCategoryAccepted,
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		}
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
			feedback.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
		}
		result.Set(llmevents.PreflightRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, Feedback: feedback}, nil)
	})
}

func newRuntimeTestOwner() *Biz {
	return newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{})
}

func newRuntimeTestOwnerWithEvidence(evidence RuntimeEvidenceRepositories) *Biz {
	return &Biz{
		Base:     basebiz.New(ModuleID, runtimeTestHubWithRunRetirementFallback(nil), nil),
		evidence: evidence,
	}
}

func (m *Biz) ConfigureRuntimeRepository(repository runtime.RuntimeStateRepository) error {
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		return err
	}
	m.runtimeController = controller
	return nil
}

func (m *Biz) RuntimeRepositoryConfigured() bool {
	return m != nil && m.runtimeController != nil
}

func (m *Biz) loadRuntime(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return m.runtimeController.Load(ctx, workspaceID, runID)
}

func (m *Biz) QueryRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, ownerModuleID string, grant *runtimecontract.RuntimeOperationGovernanceGrant) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	if m == nil || m.runtimeController == nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	return m.runtimeController.QueryOperationReceipt(ctx, workspaceID, runID, dispatchID, operationID, executionKey, fence, ownerModuleID, grant)
}

func (m *Biz) EvidenceRepositories() RuntimeEvidenceRepositories {
	if m == nil {
		return RuntimeEvidenceRepositories{}
	}
	return m.evidence
}

func newTestBiz(owner *Biz, hub magicevent.Hub, background magictask.BackgroundRoutine) *Biz {
	ensureRuntimeCandidatePreflightTestResponder(hub)
	// C4/C5 fixtures need the same opaque content carrier supplied by
	// production wiring. This is only test setup: a complete scripted provider
	// document still must persist and be decoded via the request-session path.
	if owner == nil {
		owner = newRuntimeTestOwner()
	}
	if owner.evidence.ArtifactContinuations == nil {
		owner.evidence.ArtifactContinuations = runtimeCandidateRequestContentTestStore(hub)
	}
	if owner.workspaceDeletions == nil {
		evidenceOwner := memory.NewOrchestratorOwnerRepositorySet()
		owner.workspaceDeletions = evidenceOwner.WorkspaceDeletions()
		owner.workspacePurger = evidenceOwner.WorkspacePurger()
	}
	hub = runtimeTestHubWithRunRetirementFallback(hub)
	owner.Base = basebiz.New(ModuleID, hub, background)
	return owner
}

func runtimeTestHubWithRunRetirementFallback(hub magicevent.Hub) magicevent.Hub {
	if _, ok := hub.(runtimeTestRunRetirementFallbackHub); ok {
		return hub
	}
	return runtimeTestRunRetirementFallbackHub{Hub: hub}
}

// runtimeTestRunRetirementFallbackHub gives isolated Orchestrator fixtures the
// same typed "no retirement fence" fact that an empty Run owner would return.
// It only translates EventHub's missing-observer transport error; a real Run
// owner result, including every owner error, remains authoritative.
type runtimeTestRunRetirementFallbackHub struct {
	magicevent.Hub
}

func (h runtimeTestRunRetirementFallbackHub) Subscribe(eventID string, observer magicevent.Observer) {
	if h.Hub != nil {
		h.Hub.Subscribe(eventID, observer)
	}
}

func (h runtimeTestRunRetirementFallbackHub) Unsubscribe(eventID string, observer magicevent.Observer) {
	if h.Hub != nil {
		h.Hub.Unsubscribe(eventID, observer)
	}
}

func (h runtimeTestRunRetirementFallbackHub) Post(event magicevent.Event) {
	if h.Hub != nil {
		h.Hub.Post(event)
	}
}

func (h runtimeTestRunRetirementFallbackHub) Terminate(ctx context.Context) {
	if h.Hub != nil {
		h.Hub.Terminate(ctx)
	}
}

func (h runtimeTestRunRetirementFallbackHub) Send(event magicevent.Event) magicevent.Result {
	if event != nil && event.ID() == runevents.TopicRunCommandWorkspaceRetirementFact && h.Hub == nil {
		result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
		return result
	}
	if h.Hub == nil {
		return nil
	}
	result := h.Hub.Send(event)
	if event == nil || event.ID() != runevents.TopicRunCommandWorkspaceRetirementFact || result == nil || result.Error() == nil ||
		!strings.Contains(result.Error().Error(), "missing observer") {
		return result
	}
	result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
	return result
}

// runtimeCandidateResultForTest is the only scripted-provider success
// builder for the orchestrator package. It deliberately encodes the provider
// wire schema, rather than marshaling RuntimeCandidate: RuntimeCandidate is
// an internal DTO and its field layout is not the provider response contract.
// C5 must consume DocumentOutcome.Content just like production does; Candidate remains
// only as an optional assertion aid for test bodies.
func runtimeCandidateResultForTest(t *testing.T, command llmevents.RequestRuntimeCandidateCommand, candidate runtimecontract.RuntimeCandidate) llmevents.RequestRuntimeCandidateResult {
	t.Helper()
	candidate = bindRuntimeTestCandidateToManifest(command.CandidateManifest, candidate)
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(command.CandidateManifest, candidate)
	if err != nil {
		// Never Fatal inside an EventHub observer: that leaves hub.Send blocked.
		// Surface the encode/membership failure as a typed complete rejection.
		return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid",
				Signature: "candidate-contract-invalid", Reason: err.Error(),
			}),
		})
	}
	return runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
		Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
		ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
		DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, document),
	})
}

func runtimeCompleteDocumentOutcome(expected *runtimecontract.RuntimeExpectedDocumentContract, document []byte) *llmevents.RuntimeProviderDocumentOutcome {
	return &llmevents.RuntimeProviderDocumentOutcome{
		Framing: runtimecontract.RuntimeProviderDocumentFraming{
			Disposition:          runtimecontract.RuntimeProviderDocumentComplete,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(expected),
			ResponseHash:         runtimecontract.StableRuntimeProviderResponseHash(document), ResponseBytes: int64(len(document)),
		},
		Content: append([]byte(nil), document...),
	}
}

func runtimeProviderFinalOutputMissingOutcome(expected *runtimecontract.RuntimeExpectedDocumentContract) *llmevents.RuntimeProviderDocumentOutcome {
	return &llmevents.RuntimeProviderDocumentOutcome{
		Framing: runtimecontract.RuntimeProviderDocumentFraming{
			Disposition:          runtimecontract.RuntimeProviderDocumentEmpty,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(expected),
			Code:                 "provider_final_output_missing",
			Signature:            "provider-final-output-missing",
		},
	}
}

func runtimeProviderInvalidDocumentOutcome(expected *runtimecontract.RuntimeExpectedDocumentContract, body []byte) *llmevents.RuntimeProviderDocumentOutcome {
	return &llmevents.RuntimeProviderDocumentOutcome{
		Framing: runtimecontract.RuntimeProviderDocumentFraming{
			Disposition:          runtimecontract.RuntimeProviderDocumentInvalid,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(expected),
			ResponseHash:         runtimecontract.StableRuntimeProviderResponseHash(body), ResponseBytes: int64(len(body)),
			Code: runtimecontract.RuntimeProviderDocumentCodeFinalOutputInvalid, Signature: runtimecontract.RuntimeProviderDocumentSignatureOutputInvalid,
		},
	}
}

func runtimeExpectedDocumentHash(expected *runtimecontract.RuntimeExpectedDocumentContract) string {
	if expected == nil {
		return ""
	}
	return expected.Normalized().Hash
}

// bindRuntimeTestCandidateToManifest builds the complete typed operation used
// by scripted providers. EncodeRuntimeCandidatePayloadWithManifest then emits
// only current provider-selectable wire fields; raw CWI IDs never reach JSON.
func bindRuntimeTestCandidateToManifest(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	bind := func(operation *runtimecontract.RuntimeOperation) {
		if operation == nil || len(operation.CoverageWorkItemIDs) == 0 {
			return
		}
		// CoverageWorkItemIDs is only a local fixture hint used to select the
		// receiver-sealed binding. It is forbidden on the provider wire.
		coverageWorkItemIDs := append([]common.CoverageWorkItemID(nil), operation.CoverageWorkItemIDs...)
		operation.CoverageWorkItemIDs = nil
		if len(operation.BindingRefs) != 0 {
			return
		}
		operationManifest, found := runtimeTestManifestOperationForKind(manifest, operation.Kind)
		if !found {
			return
		}
		if operation.RequiredOperationRef == "" && len(operationManifest.RequiredOperationReferences) == 1 {
			operation.RequiredOperationRef = operationManifest.RequiredOperationReferences[0]
		}
		authorization, err := runtimecontract.RuntimeCandidateOperationBindingAuthorizationFor(operationManifest, *operation)
		if err != nil {
			return
		}
		allowed := authorization.AllowedBindingRefs
		if len(allowed) == 0 {
			return
		}
		refs := make([]string, 0, len(coverageWorkItemIDs))
		for _, coverageID := range coverageWorkItemIDs {
			for _, binding := range manifest.Bindings {
				if binding.CoverageWorkItemID != coverageID || !runtimeTestStringIncluded(allowed, binding.ID) {
					continue
				}
				refs = append(refs, binding.ID)
			}
		}
		refs = runtimeTestNormalizeStrings(refs)
		if len(refs) == 0 {
			return
		}
		operation.BindingRefs = refs
	}
	if candidate.StateChange != nil {
		for idx := range candidate.StateChange.Operations {
			bind(&candidate.StateChange.Operations[idx].Operation)
		}
	}
	if candidate.Validation != nil {
		for idx := range candidate.Validation.Operations {
			bind(&candidate.Validation.Operations[idx].Operation)
		}
	}
	return candidate
}

func runtimeTestManifestOperationForKind(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Normalized().Operations {
		if operation.Kind == kind {
			return operation, true
		}
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
}

func runtimeTestStringIncluded(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func runtimeTestNormalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func runtimeTestNativeGoValidationPolicy(id string) runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: id, AppliesTo: runtimecontract.StepKindShellCommand,
		CommandName: "go", RecipeDeclared: true, RecipeArgs: []string{"test", "./..."},
	}
}

// runtimeTestDeclareExactManagedValidationOwnerFact keeps Root fixtures on the
// same Definition contract as Application-compiled profiles. Acquisition=true
// authorizes one Runtime-owned SM-2/SM-3 File preparation followed by Shell
// re-observation; acquisition=false requires the harness to prepare the native
// proof through the real File and Shell owners before Runtime starts.
func runtimeTestDeclareExactManagedValidationOwnerFact(
	bootstrap *runtimecontract.RuntimeBootstrap,
	coverageID common.CoverageWorkItemID,
	recipeID string,
	acquisition bool,
) {
	if bootstrap == nil {
		return
	}
	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell,
		FactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	for index := range bootstrap.DefinitionFocuses {
		focus := &bootstrap.DefinitionFocuses[index]
		if len(focus.CoverageWorkItemIDs) != 1 || focus.CoverageWorkItemIDs[0] != coverageID {
			continue
		}
		focus.ValidationRecipeID = recipeID
		focus.RequiredCheckRefs = nil
		focus.AcceptedStepKinds = runtimeTestNormalizeStrings(append(
			focus.AcceptedStepKinds,
			runtimecontract.RuntimeBlockKindManagedValidationSession,
		))
		focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{requirement}
		if !acquisition {
			focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{{
				Requirement: requirement, RequirePreexisting: true,
			}}
			continue
		}
		if focus.TargetSelectorRef == "" {
			focus.TargetSelectorRef = "file/profile_declared_candidates"
		}
		if focus.TargetSelectorOwnerRef == "" {
			focus.TargetSelectorOwnerRef = "runtime-test-profile"
		}
		if focus.TargetSelectorSubjectRef == "" {
			focus.TargetSelectorSubjectRef = string(coverageID)
		}
		focus.AcceptedStepKinds = runtimeTestNormalizeStrings(append(focus.AcceptedStepKinds, runtimecontract.StepKindFileWrite))
		focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{{
			Requirement:                        requirement,
			ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
			MissingTrigger:                     runtimecontract.RuntimeOwnerFactMissingTrigger,
			AcquisitionOwnerModuleID:           runtimecontract.RuntimeModuleFile,
			AcquisitionOperationKind:           runtimecontract.StepKindFileWrite,
			AcquisitionTargetPolicy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			AcquisitionTargetSelectorRef:       "workspace-relative-new-file-target",
			TargetSelectorRef:                  focus.TargetSelectorRef,
			TargetSelectorOwnerRef:             focus.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:           focus.TargetSelectorSubjectRef,
			ReobserveOwnerModuleID:             runtimecontract.RuntimeModuleShell,
			ReobserveFactKind:                  runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
			MaxAcquisitionAttempts:             1,
		}}
	}
	for profileIndex := range bootstrap.Activation.ExecutionContract.ProfileContracts {
		profile := &bootstrap.Activation.ExecutionContract.ProfileContracts[profileIndex]
		for requirementIndex := range profile.EvidenceRequirements {
			declaration := &profile.EvidenceRequirements[requirementIndex]
			if common.CoverageWorkItemID(declaration.ID) != coverageID {
				continue
			}
			declaration.ValidationRecipeID = recipeID
			declaration.RequiredCheckRefs = nil
			declaration.RequiredEvidenceKinds = runtimeTestNormalizeStrings(append(
				declaration.RequiredEvidenceKinds,
				runtimecontract.EvidenceKindValidation,
			))
			declaration.AcceptedStepKinds = runtimeTestNormalizeStrings(append(
				declaration.AcceptedStepKinds,
				runtimecontract.RuntimeBlockKindManagedValidationSession,
			))
			declaration.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{requirement}
			if !acquisition {
				declaration.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{{
					Requirement: requirement, RequirePreexisting: true,
				}}
				continue
			}
			if declaration.TargetSelectorRef == "" {
				declaration.TargetSelectorRef = "file/profile_declared_candidates"
			}
			if declaration.TargetSelectorOwnerRef == "" {
				declaration.TargetSelectorOwnerRef = "runtime-test-profile"
			}
			if declaration.TargetSelectorSubjectRef == "" {
				declaration.TargetSelectorSubjectRef = string(coverageID)
			}
			declaration.AcceptedStepKinds = runtimeTestNormalizeStrings(append(declaration.AcceptedStepKinds, runtimecontract.StepKindFileWrite))
			declaration.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{{
				Requirement:                        requirement,
				ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
				MissingTrigger:                     runtimecontract.RuntimeOwnerFactMissingTrigger,
				AcquisitionOwnerModuleID:           runtimecontract.RuntimeModuleFile,
				AcquisitionOperationKind:           runtimecontract.StepKindFileWrite,
				AcquisitionTargetPolicy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				AcquisitionTargetSelectorRef:       "workspace-relative-new-file-target",
				TargetSelectorRef:                  declaration.TargetSelectorRef,
				TargetSelectorOwnerRef:             declaration.TargetSelectorOwnerRef,
				TargetSelectorSubjectRef:           declaration.TargetSelectorSubjectRef,
				ReobserveOwnerModuleID:             runtimecontract.RuntimeModuleShell,
				ReobserveFactKind:                  runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
				MaxAcquisitionAttempts:             1,
			}}
		}
	}
}

// runtimeTestBootstrapWithBindings prepares only the declarative input that a
// semantic plan may later bind. The historical name is retained for existing
// test callers, but a fresh bootstrap intentionally carries no proof binding.
func runtimeTestBootstrapWithBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	if bootstrap.Activation.ExecutionContract.StateChangePolicy == "" {
		bootstrap.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	}
	if bootstrap.ExecutionBudgetPolicy.PolicyRevision == "" {
		bootstrap.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	}
	for index := range bootstrap.CoverageItems {
		if bootstrap.CoverageItems[index].SourceScope == "" {
			bootstrap.CoverageItems[index].SourceScope = "test"
		}
		if bootstrap.CoverageItems[index].SourceID == "" {
			bootstrap.CoverageItems[index].SourceID = string(bootstrap.CoverageItems[index].ID)
		}
		if bootstrap.CoverageItems[index].TargetText == "" {
			bootstrap.CoverageItems[index].TargetText = "verify " + string(bootstrap.CoverageItems[index].ID)
		}
	}
	itemsByID := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(bootstrap.CoverageItems))
	for _, item := range bootstrap.CoverageItems {
		itemsByID[item.ID] = item
	}
	for index := range bootstrap.DefinitionFocuses {
		if len(bootstrap.DefinitionFocuses[index].CoverageWorkItemIDs) != 1 {
			continue
		}
		item, found := itemsByID[bootstrap.DefinitionFocuses[index].CoverageWorkItemIDs[0]]
		if !found {
			continue
		}
		bootstrap.DefinitionFocuses[index].SliceID = item.SliceID
		bootstrap.DefinitionFocuses[index].AssessmentScope = item.SourceScope
		bootstrap.DefinitionFocuses[index].AssessmentID = item.SourceID
		bootstrap.DefinitionFocuses[index].AssessmentText = item.TargetText
		bootstrap.DefinitionFocuses[index].EvidenceDeclarationRef = runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
		for convergenceIndex := range bootstrap.DefinitionFocuses[index].OwnerFactConvergences {
			convergence := &bootstrap.DefinitionFocuses[index].OwnerFactConvergences[convergenceIndex]
			if convergence.RequirePreexisting {
				continue
			}
			// Rebound Root fixtures may deliberately change the immutable target
			// selector before recompiling their Definition seed. Keep an already
			// declared acquisition tied to that exact declaration; this does not
			// create owner facts or operation authority.
			convergence.TargetSelectorRef = bootstrap.DefinitionFocuses[index].TargetSelectorRef
			convergence.TargetSelectorOwnerRef = bootstrap.DefinitionFocuses[index].TargetSelectorOwnerRef
			convergence.TargetSelectorSubjectRef = bootstrap.DefinitionFocuses[index].TargetSelectorSubjectRef
		}
	}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(bootstrap.Definition, bootstrap.Activation, bootstrap.CoverageItems, bootstrap.Slices)
	if err == nil {
		bootstrap.DefinitionSeed = seed
		requirements, requirementErr := runtimecontract.CompileRuntimeValidationProofRequirements(
			bootstrap.Definition.Revision,
			bootstrap.CoverageItems,
			bootstrap.DefinitionFocuses,
		)
		if requirementErr == nil && len(requirements) != 0 {
			bootstrap.DefinitionFocuses = runtimeTestBindDefinitionValidationOwnerFacts(
				bootstrap.DefinitionFocuses,
				requirements,
			)
		}
	}
	return bootstrap
}

// runtimeTestBindDefinitionValidationOwnerFacts mirrors the Application
// compiler's purely declarative hash binding for hand-written Root fixtures.
// It never creates a required owner fact or an owner catalog: tests that use
// managed validation must declare both explicitly, and Shell must still
// publish the matching native-proof binding through EventHub.
func runtimeTestBindDefinitionValidationOwnerFacts(
	focuses []runtimecontract.RuntimeDefinitionFocus,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
) []runtimecontract.RuntimeDefinitionFocus {
	ret := runtimecontract.CloneRuntimeDefinitionFocuses(focuses)
	for focusIndex := range ret {
		focus := &ret[focusIndex]
		if len(focus.CoverageWorkItemIDs) != 1 {
			continue
		}
		requirement, found := runtimecontract.RuntimeValidationProofRequirementForCoverage(requirements, focus.CoverageWorkItemIDs[0])
		if !found {
			continue
		}
		refRemap := make(map[string]string)
		for index := range focus.RequiredOwnerFacts {
			fact := focus.RequiredOwnerFacts[index].Normalized()
			if fact.OwnerModuleID != runtimecontract.RuntimeModuleShell ||
				fact.FactKind != runtimecontract.RuntimeOwnerFactSelectableValidationSurface ||
				fact.OperationKind != runtimecontract.RuntimeBlockKindManagedValidationSession {
				continue
			}
			oldRef := runtimecontract.RuntimeOwnerFactRequirementRef(fact)
			fact.ValidationProofRequirementHash = requirement.Hash
			focus.RequiredOwnerFacts[index] = fact
			refRemap[oldRef] = runtimecontract.RuntimeOwnerFactRequirementRef(fact)
		}
		for index := range focus.OwnerFactConvergences {
			convergence := focus.OwnerFactConvergences[index].Normalized()
			fact := convergence.Requirement
			if fact.OwnerModuleID == runtimecontract.RuntimeModuleShell &&
				fact.FactKind == runtimecontract.RuntimeOwnerFactSelectableValidationSurface &&
				fact.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession {
				oldRef := runtimecontract.RuntimeOwnerFactRequirementRef(fact)
				fact.ValidationProofRequirementHash = requirement.Hash
				convergence.Requirement = fact
				refRemap[oldRef] = runtimecontract.RuntimeOwnerFactRequirementRef(fact)
			}
			for dependencyIndex, dependency := range convergence.DependsOnRequirementRefs {
				if replacement, ok := refRemap[dependency]; ok {
					convergence.DependsOnRequirementRefs[dependencyIndex] = replacement
				}
			}
			focus.OwnerFactConvergences[index] = convergence
		}
	}
	return runtimecontract.NormalizeRuntimeDefinitionFocuses(ret)
}

// runtimeTestBootstrapWithReboundBindings clears any obsolete test-only proof
// data. The later semantic fixture seal creates the replacement binding.
func runtimeTestBootstrapWithReboundBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	return runtimeTestBootstrapWithBindings(bootstrap)
}

// runtimeTestFactsWithReboundBindings keeps direct post-semantic fixtures
// explicit. Production never calls this helper: its binding comes only from
// the Orchestrator semantic seal.
func runtimeTestFactsWithReboundBindings(facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	if facts.Activation.ExecutionContract.StateChangePolicy == "" {
		facts.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	}
	focus := facts.Focus.Normalized()
	definitionRevision := definition.Normalized().Revision
	if definitionRevision == "" {
		definitionRevision = facts.DefinitionRevision
	}
	if definitionRevision == "" {
		definitionRevision = "test-definition"
	}
	if len(focus.AcceptedStepKinds) == 0 {
		for _, kind := range focus.RequiredEvidenceKinds {
			switch kind {
			case runtimecontract.EvidenceKindStateChange:
				focus.AcceptedStepKinds = append(focus.AcceptedStepKinds, runtimecontract.StepKindFileEdit)
			case runtimecontract.EvidenceKindValidation:
				focus.AcceptedStepKinds = append(focus.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession)
			}
		}
	}
	if focus.ValidationRecipeID == "" {
		for _, kind := range focus.RequiredEvidenceKinds {
			if kind == runtimecontract.EvidenceKindValidation {
				focus.ValidationRecipeID = "test-runtime-validation-recipe"
				break
			}
		}
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, kind := range focus.RequiredEvidenceKinds {
		proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: kind}
		switch kind {
		case runtimecontract.EvidenceKindStateChange:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
		case runtimecontract.EvidenceKindValidation:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
			for _, operationKind := range focus.AcceptedStepKinds {
				if runtimecontract.CanonicalRuntimeBlockKind(operationKind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
					proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession}
					break
				}
			}
			proof.ValidationRecipeID = focus.ValidationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
		}
		for _, operationKind := range focus.AcceptedStepKinds {
			if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(operationKind, kind) {
				proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, operationKind)
			}
		}
		proofs = append(proofs, proof.Normalized())
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID: "binding/test/" + focus.ID + "/" + string(coverageID), CoverageWorkItemID: coverageID,
			FocusID: focus.ID, DefinitionRevision: definitionRevision,
			Proofs:     append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
			TargetRefs: append([]string(nil), focus.TargetRefs...), TargetSelectorRef: focus.TargetSelectorRef,
			TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs:          append([]string(nil), focus.TargetSelectorCandidateRefs...),
			ProspectiveTargetAllOfConstraintRefs: append([]string(nil), focus.ProspectiveTargetAllOfConstraintRefs...),
			ProspectiveTargetRefs:                append([]string(nil), focus.ProspectiveTargetRefs...), DeclarationRef: "test/runtime-fixture",
		}.Normalized()
		if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && len(binding.ProspectiveTargetRefs) == 0 {
			binding.TargetSelectorRef = "test/fixture-owner-selector"
			binding.TargetSelectorOwnerRef = "test-fixture"
			binding.TargetSelectorSubjectRef = focus.ID
		}
		if binding.TargetSelectorRef != "" && binding.TargetSelectorOwnerRef == "" {
			binding.TargetSelectorOwnerRef = "test-fixture"
		}
		if binding.TargetSelectorRef != "" && binding.TargetSelectorSubjectRef == "" {
			binding.TargetSelectorSubjectRef = focus.ID
		}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		bindings = append(bindings, binding)
	}
	focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	facts.Focus = focus
	return facts
}

func runtimeCandidateDocumentForTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) []byte {
	t.Helper()
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(manifest, candidate)
	if err != nil {
		t.Fatalf("encode test provider document from sealed manifest: %v", err)
	}
	return document
}
