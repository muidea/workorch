package orchestrator

import (
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// orchestratorbiz keeps moved package-local tests readable without importing
// their own package through its old root path.
var orchestratorbiz orchestratorBizTestShim

type orchestratorBizTestShim struct{}

// Module keeps moved legacy test names readable. Production lifecycle wiring
// uses Biz exclusively.
type Module = Biz

var runtimeV2CandidateRequestContentTestStores sync.Map

// runtimeV2CandidateRequestContentTestStore gives all Biz instances wired to
// one test EventHub the same opaque content repository. The production
// equivalent is process wiring to the durable repository; this helper keeps
// restart fixtures from replacing that repository while leaving raw provider
// documents subject to the normal C4/C5 persist-and-reload path.
func runtimeV2CandidateRequestContentTestStore(hub magicevent.Hub) repository.ArtifactContinuationRepository {
	if hub == nil {
		return memory.NewArtifactContinuationRepository()
	}
	store, _ := runtimeV2CandidateRequestContentTestStores.LoadOrStore(hub, memory.NewArtifactContinuationRepository())
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
	observer.Subscribe(orchestratorevents.TopicOrchestratorPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.PreflightRuntimeCandidateCommand)
		if !ok {
			return
		}
		result.Set(orchestratorevents.PreflightRuntimeCandidateResult{Transaction: command.Transaction, Ready: true}, nil)
	})
}

func newTestBiz(owner *RuntimeOrchestrator, hub magicevent.Hub, background magictask.BackgroundRoutine) *Biz {
	ensureRuntimeCandidatePreflightTestResponder(hub)
	// C4/C5 fixtures need the same opaque content carrier supplied by
	// production wiring. This is only test setup: a complete scripted provider
	// document still must persist and be decoded via the request-session path.
	if owner != nil && owner.evidence.ArtifactContinuations == nil {
		owner.evidence.ArtifactContinuations = runtimeV2CandidateRequestContentTestStore(hub)
	}
	return &Biz{Base: basebiz.New(ModuleID, hub, background), bizPtr: owner}
}

func (orchestratorBizTestShim) NewRuntimeOrchestrator() *RuntimeOrchestrator {
	return NewRuntimeOrchestrator()
}

// runtimeV2CandidateResultForTest is the only scripted-provider success
// builder for the orchestrator package. It deliberately encodes the provider
// wire schema, rather than marshaling RuntimeCandidate: RuntimeCandidate is
// an internal DTO and its field layout is not the provider response contract.
// C5 must consume DocumentContent just like production does; Candidate remains
// only as an optional assertion aid for legacy test bodies.
func runtimeV2CandidateResultForTest(t *testing.T, command orchestratorevents.RequestRuntimeCandidateCommand, candidate runtimecontract.RuntimeCandidate) orchestratorevents.RequestRuntimeCandidateResult {
	t.Helper()
	candidate = bindRuntimeV2TestCandidateToManifest(command.CandidateManifest, candidate)
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(command.CandidateManifest, candidate)
	if err != nil {
		// Never Fatal inside an EventHub observer: that leaves hub.Send blocked.
		// Surface the encode/membership failure as a typed complete rejection.
		return orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID,
			RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
			Failure: &runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid",
				Signature: "candidate-contract-invalid", Reason: err.Error(),
			},
		}
	}
	return orchestratorevents.RequestRuntimeCandidateResult{
		Transaction: command.Transaction, LogicalRequestID: command.LogicalRequestID,
		RequestSequence: command.RequestSequence, PhysicalCallID: command.PhysicalCallID,
		DocumentContent: document,
	}
}

// bindRuntimeV2TestCandidateToManifest adapts old in-process fixture literals
// to the provider wire contract. Production never takes this path: real
// provider JSON must name only sealed bindingRefs and raw CWI IDs remain a
// strict rejection. Keeping the adapter at the test-provider boundary lets
// broad restart fixtures exercise V6 evidence semantics without reintroducing
// a Runtime V2 compatibility branch.
func bindRuntimeV2TestCandidateToManifest(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	bind := func(operation *runtimecontract.RuntimeOperation) {
		if operation == nil || len(operation.CoverageWorkItemIDs) == 0 || len(operation.BindingRefs) != 0 {
			return
		}
		allowed := runtimeV2TestManifestBindingRefsForOperation(manifest, operation.Kind)
		if len(allowed) == 0 {
			return
		}
		refs := make([]string, 0, len(operation.CoverageWorkItemIDs))
		for _, coverageID := range operation.CoverageWorkItemIDs {
			for _, binding := range manifest.Bindings {
				if binding.CoverageWorkItemID != coverageID || !runtimeV2TestStringIncluded(allowed, binding.ID) {
					continue
				}
				refs = append(refs, binding.ID)
			}
		}
		refs = runtimeV2TestNormalizeStrings(refs)
		if len(refs) == 0 {
			return
		}
		operation.BindingRefs = refs
		operation.CoverageWorkItemIDs = nil
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

func runtimeV2TestManifestBindingRefsForOperation(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) []string {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Normalized().Operations {
		if operation.Kind == kind {
			return append([]string(nil), operation.AllowedBindingRefs...)
		}
	}
	return nil
}

func runtimeV2TestStringIncluded(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func runtimeV2TestNormalizeStrings(values []string) []string {
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

// runtimeV2TestBootstrapWithBindings upgrades an in-process fixture to the
// current schema's explicit proof shape. It is intentionally test-only: the
// production activation compiler must be the sole producer of bindings and
// fails closed when profile/template declarations are absent.
func runtimeV2TestBootstrapWithBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	definitionRevision := bootstrap.Definition.Revision
	for focusIndex := range bootstrap.Focuses {
		focus := &bootstrap.Focuses[focusIndex]
		if len(focus.CoverageWorkItemIDs) == 0 || len(focus.CoverageEvidenceBindings) != 0 {
			continue
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
					// Direct in-process fixtures must name an exact validation
					// recipe just as activationcore requires in production. This is
					// test data, not a production fallback or inferred recipe.
					focus.ValidationRecipeID = "test-runtime-v2-validation-recipe"
					break
				}
			}
		}
		proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
		for _, kind := range focus.RequiredEvidenceKinds {
			proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: kind}
			if kind == runtimecontract.EvidenceKindStateChange {
				proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
			} else if kind == runtimecontract.EvidenceKindValidation {
				proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
				proof.ValidationRecipeID = focus.ValidationRecipeID
				proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
			}
			for _, operationKind := range focus.AcceptedStepKinds {
				if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(operationKind, kind) {
					proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, operationKind)
				}
			}
			proofs = append(proofs, proof)
		}
		bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
		for _, coverageID := range focus.CoverageWorkItemIDs {
			binding := runtimecontract.CoverageEvidenceBinding{
				ID: "binding/test/" + focus.ID + "/" + string(coverageID), CoverageWorkItemID: coverageID,
				FocusID: focus.ID, DefinitionRevision: definitionRevision,
				Proofs:     append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
				TargetRefs: append([]string(nil), focus.TargetRefs...), TargetSelectorRef: focus.TargetSelectorRef,
				TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
				ProspectiveTargetRef: focus.ProspectiveTargetRef, DeclarationRef: "test/runtime-v2-fixture",
			}.Normalized()
			if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && binding.ProspectiveTargetRef == "" {
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
	}
	return bootstrap
}

// runtimeV2TestBootstrapWithReboundBindings is for fixtures that deliberately
// change a Focus proof declaration after starting from the shared bootstrap.
// A new proof declaration must get a new immutable binding; retaining the
// previous binding would model precisely the stale binding drift that V6
// rejects in production.
func runtimeV2TestBootstrapWithReboundBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	for index := range bootstrap.Focuses {
		bootstrap.Focuses[index].CoverageEvidenceBindings = nil
	}
	return runtimeV2TestBootstrapWithBindings(bootstrap)
}

// runtimeV2TestFactsWithReboundBindings applies the same test-only declaration
// compiler to a direct RuntimeFacts fixture after that fixture changes its
// focus contract. Production facts never take this path: activation compiles
// bindings once, before the Run owner persists the bootstrap.
func runtimeV2TestFactsWithReboundBindings(facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	bootstrap := runtimecontract.RuntimeBootstrap{
		Definition: definition,
		Focuses:    []runtimecontract.CoverageFocus{facts.Focus},
	}
	bootstrap = runtimeV2TestBootstrapWithReboundBindings(bootstrap)
	if len(bootstrap.Focuses) == 1 {
		facts.Focus = bootstrap.Focuses[0]
	}
	return facts
}

func runtimeV2CandidateDocumentForTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) []byte {
	t.Helper()
	candidate = bindRuntimeV2TestCandidateToManifest(manifest, candidate)
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(manifest, candidate)
	if err != nil {
		t.Fatalf("encode test provider document from sealed manifest: %v", err)
	}
	return document
}
