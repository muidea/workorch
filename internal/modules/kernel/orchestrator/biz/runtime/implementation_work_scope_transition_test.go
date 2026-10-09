package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 此 fixture 只隔离已持久 scope 的 exact replay 顺序；不证明工作准入、
// owner effect 或多轮成功。完整正例仍必须由 production owner 流程生成。
func TestImplementationWorkScopeExactReplayDoesNotReopenAfterPhaseChange(t *testing.T) {
	request := runtimecontract.RuntimeImplementationWorkScopeRequest{WorkspaceID: "ws-1", RunID: "run-1", RequestID: "persisted-request", CoverageWorkItemID: "original-cwi"}
	observation := runtimecontract.RuntimeImplementationWorkScopeObservation{Request: request, Selection: runtimecontract.RuntimeTargetSelection{SnapshotID: "persisted-scope"}}
	for _, phase := range []runtimecontract.RuntimeSemanticPlanningPhase{runtimecontract.RuntimeSemanticPlanningRequired, runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningFeasibilityRequired, runtimecontract.RuntimeSemanticPlanningSealed} {
		for _, closed := range []bool{false, true} {
			t.Run(string(phase)+"/closed="+map[bool]string{false: "false", true: "true"}[closed], func(t *testing.T) {
				round := runtimecontract.RuntimeImplementationWorkRound{ScopeRequest: request, ScopeObservation: &observation}
				if closed {
					round.DispatchID, round.CompletedReceiptSetHash = "persisted-dispatch", "persisted-collective-receipts"
				}
				aggregate := RuntimeAggregate{StateRevision: 19, Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-1", RunID: "run-1"}, SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{Phase: phase}, ImplementationWork: runtimecontract.RuntimeImplementationWorkLedger{Rounds: []runtimecontract.RuntimeImplementationWorkRound{round}}}
				if phase == runtimecontract.RuntimeSemanticPlanningSealed && !closed {
					aggregate.ActiveDispatch = &SealedDispatch{ID: "persisted-dispatch"}
				}
				repository := &controllerRepository{aggregate: &aggregate}
				controller, err := NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				for range 2 {
					for _, event := range []RuntimeInternalEvent{
						{Kind: RuntimeInternalEventImplementationWorkScopeRequired, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-1", RunID: "run-1"}, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: &request}},
						{Kind: RuntimeInternalEventImplementationWorkScopeObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-1", RunID: "run-1"}, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeObservation: &observation}},
					} {
						result, err := controller.ApplyInternalEvent(context.Background(), event)
						if err != nil || !reflect.DeepEqual(result.Aggregate, aggregate) || repository.saves != 0 {
							t.Fatalf("exact replay changed/reopened persisted round: err=%v saves=%d", err, repository.saves)
						}
					}
				}
				changedRequest := request.Clone()
				changedRequest.CoverageWorkItemID = "other-cwi"
				if _, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventImplementationWorkScopeRequired, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-1", RunID: "run-1"}, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: &changedRequest}}); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("changed same-request replay accepted: %v", err)
				}
				changedObservation := observation.Clone()
				changedObservation.Selection.SnapshotID = "changed-scope"
				if _, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventImplementationWorkScopeObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-1", RunID: "run-1"}, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeObservation: &changedObservation}}); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("changed owner observation replay accepted: %v", err)
				}
				if repository.saves != 0 || !reflect.DeepEqual(*repository.aggregate, aggregate) {
					t.Fatal("rejected replay mutated aggregate")
				}
			})
		}
	}
}

type implementationWorkDetachedRepository struct {
	controllerRepository
	failBefore bool
	failAfter  bool
}

func (r *implementationWorkDetachedRepository) Save(ctx context.Context, aggregate RuntimeAggregate, revision uint64) error {
	if r.failBefore {
		r.failBefore = false
		return errors.New("injected pre-CAS failure")
	}
	if err := r.controllerRepository.Save(ctx, aggregate, revision); err != nil {
		return err
	}
	if r.failAfter {
		r.failAfter = false
		return errors.New("injected committed response loss")
	}
	return nil
}

func (r *implementationWorkDetachedRepository) Load(ctx context.Context, workspace common.WorkspaceID, run common.RunID) (RuntimeAggregate, error) {
	value, err := r.controllerRepository.Load(ctx, workspace, run)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	var detached RuntimeAggregate
	if err := json.Unmarshal(data, &detached); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := detached.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	return detached, nil
}

// 完整 private Controller/CAS fixture；不是公开 Workspace admission 成功正例。
func implementationWorkPurposeController(t *testing.T, targetOptions ...[]string) (*RuntimeController, *implementationWorkDetachedRepository, RuntimeAggregate) {
	t.Helper()
	targets := []string{"target.txt"}
	if len(targetOptions) > 0 {
		targets = targetOptions[0]
	}
	return implementationWorkPurposeControllerWithLineage(t, targets, nil)
}

func implementationWorkPurposeControllerWithLineage(t *testing.T, targets []string, rules []runtimecontract.RuntimeLineageAssertionRule) (*RuntimeController, *implementationWorkDetachedRepository, RuntimeAggregate) {
	t.Helper()
	facts := controllerFacts()
	policy := runtimecontract.RuntimeImplementationWorkPolicy{ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile, AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileWrite}}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}, TargetSelectorRef: "work-scope", TargetSelectorOwnerRef: "profile/work-test", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: targets, MaxRoundsPerCWI: 2, MaxTargetsPerRound: len(targets), RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true}
	facts.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "work-test", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy}}}}
	facts.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.DefinitionFocuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.DefinitionFocuses[0].ImplementationWorkPolicy = &policy
	facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].LineageAssertions = append([]runtimecontract.RuntimeLineageAssertionRule(nil), rules...)
	facts.DefinitionFocuses[0].LineageAssertions = append([]runtimecontract.RuntimeLineageAssertionRule(nil), rules...)
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	catalog.SelectorRefs = []string{"work-scope"}
	catalog.TargetRefs = targets
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation.ContentHash = catalog.SnapshotID
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	manifest := runtimecontract.WorkspaceManifest{Revision: catalog.ObservedRevision, TargetRefs: targets}
	manifestObservation := observation
	manifestObservation.Kind, manifestObservation.PrerequisiteID, manifestObservation.ContentHash = runtimecontract.PrerequisiteKindManifest, "work-manifest", manifest.Revision
	manifestObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(manifestObservation)
	facts.Resources.Manifest = &manifest
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation, manifestObservation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite, {ID: manifestObservation.PrerequisiteID, Kind: manifestObservation.Kind, OwnerModuleID: manifestObservation.OwnerModuleID}}
	runtimeTestSealCurrentPolicies(&facts)
	repository := &implementationWorkDetachedRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	return controller, repository, aggregate
}

func TestImplementationWorkPublicActivationRemainsClosed(t *testing.T) {
	_, _, aggregate := implementationWorkPurposeController(t)
	repository := &implementationWorkDetachedRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventActivationObserved, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Activation: &RuntimeInternalActivationPayload{ActivationFacts: &aggregate.Facts, ActivationDefinition: &aggregate.Definition}})
	if !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkUnavailable) || repository.saves != 0 {
		t.Fatalf("public activation availability bypass: %v saves=%d", err, repository.saves)
	}
}

func TestImplementationWorkPurposeFirstPrivateCASAndTerminalEntry(t *testing.T) {
	for _, purpose := range []runtimecontract.RuntimeImplementationWorkPurpose{runtimecontract.RuntimeImplementationWorkPurposeEffect, runtimecontract.RuntimeImplementationWorkPurposeTerminal} {
		for _, targets := range [][]string{{"target.txt"}, {"other.txt", "target.txt"}} {
			t.Run(string(purpose)+fmt.Sprint(len(targets)), func(t *testing.T) {
				implementationWorkPrivatePipeline(t, purpose, targets, nil)
			})
		}
	}
}

type implementationWorkRealFileFixture struct {
	hub        magicevent.Hub
	directory  string
	controller *RuntimeController
	repository *implementationWorkDetachedRepository
	aggregate  *RuntimeAggregate
	lineageRules []runtimecontract.RuntimeLineageAssertionRule
}

func (f *implementationWorkRealFileFixture) send(t *testing.T, topic string, command any) any {
	t.Helper()
	destination := runtimecontract.RuntimeModuleFile
	if topic == fileevents.TopicAcquireRuntimeTargetSelection {
		destination = fileevents.PrerequisiteCommandDestination
	}
	result := f.hub.Send(magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, destination, magicevent.NewHeader(), context.Background(), command))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// 初始 private fixture 不经过公开 activation；此后所有 authority 变化均由内部 event/CAS 生成。
func (f *implementationWorkRealFileFixture) current(t *testing.T, initial RuntimeAggregate) (*RuntimeController, *implementationWorkDetachedRepository, RuntimeAggregate) {
	t.Helper()
	if f.aggregate != nil {
		fresh, err := f.controller.Load(context.Background(), f.aggregate.Facts.WorkspaceID, f.aggregate.Facts.RunID)
		if err != nil {
			t.Fatal(err)
		}
		return f.controller, f.repository, fresh
	}
	policy := initial.Facts.DefinitionFocuses[0].ImplementationWorkPolicy
	policy.TargetSelectorRef = "file/workspace_deliverable"
	policy.AllowedOperations = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}}
	initial.Definition.CapabilityContract = append(initial.Definition.CapabilityContract, runtimecontract.StepKindFileEdit)
	initial.Facts.Capabilities.StateChangeKinds = append(initial.Facts.Capabilities.StateChangeKinds, runtimecontract.StepKindFileEdit)
	initial.Facts.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].ImplementationWorkPolicy = policy
	manifest := sendObservedManifest(t, context.Background(), f.hub, initial, runtimecontract.RuntimePrerequisite{ID: "work-manifest", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile}, f.directory)
	initial.Facts.Resources.Manifest = &manifest.Manifest
	catalogPrerequisite := initial.Facts.Prerequisites.Satisfied[0]
	catalogPrerequisite.RequiredCapabilities = []string{runtimecontract.StepKindFileEdit}
	initial.Facts.Prerequisites.Satisfied[0] = catalogPrerequisite
	catalog := f.send(t, fileevents.TopicAcquireRuntimeOperationAdmissibility, fileevents.AcquireRuntimeOperationAdmissibilityCommand{WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID, AuthoritySnapshotID: initial.Decision.AuthoritySnapshot.ID, PrerequisiteID: catalogPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, WorkspaceDeliverableDirectory: f.directory, RequiredKinds: []string{runtimecontract.StepKindFileEdit}, WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: manifest.Manifest, Observation: manifest.Observation}, MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount}).(fileevents.AcquireRuntimeOperationAdmissibilityResult)
	initial.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog.Acquisition.Admissibility}
	initial.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{catalog.Acquisition.Observation, manifest.Observation}
	initial.Facts.Prerequisites.Satisfied[1] = runtimecontract.RuntimePrerequisite{ID: manifest.Observation.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile}
	runtimeTestSealCurrentPolicies(&initial.Facts)
	var err error
	initial.Facts, initial.Decision, err = reduceRuntimeAggregateWithDirective(initial, initial.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Validate(); err != nil {
		t.Fatal(err)
	}
	f.repository = &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &initial}}
	f.controller, err = NewRuntimeController(f.repository)
	if err != nil {
		t.Fatal(err)
	}
	registerRuntimeOperationReceiptOwner(t, f.hub, f.controller)
	return f.controller, f.repository, initial
}

func (f *implementationWorkRealFileFixture) scope(t *testing.T, request runtimecontract.RuntimeImplementationWorkScopeRequest) runtimecontract.RuntimeImplementationWorkScopeObservation {
	t.Helper()
	p := request.Policy
	response := f.send(t, fileevents.TopicAcquireRuntimeTargetSelection, fileevents.AcquireRuntimeTargetSelectionCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID, PrerequisiteID: request.RequestID, WorkspaceDeliverableDirectory: f.directory, TargetRole: "implementation_work", TargetSelectorRef: p.TargetSelectorRef, TargetSelectorOwnerRef: p.TargetSelectorOwnerRef, TargetSelectorSubjectRef: p.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: p.TargetSelectorCandidateRefs, ProspectiveTargetAllOfConstraintRefs: p.ProspectiveTargetAllOfConstraintRefs, MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount}).(fileevents.AcquireRuntimeTargetSelectionResult)
	return runtimecontract.RuntimeImplementationWorkScopeObservation{Request: request, Selection: response.Selection, Observation: response.Observation}
}

func (f *implementationWorkRealFileFixture) semanticFeasibility(t *testing.T, a RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest, proposal runtimecontract.RuntimeSemanticProofProposal) runtimecontract.RuntimeSemanticProofFeasibility {
	t.Helper()
	catalog := manifest.OwnerCatalogs[0]
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(a.Facts.Resources.PrerequisiteObservations, catalog.PrerequisiteID)
	if !found {
		t.Fatal("real File catalog has no observation")
	}
	response := f.send(t, fileevents.TopicProbeRuntimeSemanticProofFeasibility, fileevents.ProbeRuntimeSemanticProofFeasibilityCommand{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, FeasibilityBatchID: a.SemanticPlanning.FeasibilityBatchID, WorkspaceDeliverableDirectory: f.directory, Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog, CatalogObservation: &observation}).(fileevents.ProbeRuntimeSemanticProofFeasibilityResult)
	return response.Observation
}

func (f *implementationWorkRealFileFixture) candidateFeasibilities(t *testing.T, a RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) []runtimecontract.RuntimeCandidateFeasibilityRecord {
	t.Helper()
	catalog := a.Facts.Resources.OperationAdmissibilities[0]
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(a.Facts.Resources.PrerequisiteObservations, catalog.PrerequisiteID)
	if !found {
		t.Fatal("real File catalog has no current observation")
	}
	var values []runtimecontract.RuntimeCandidateFeasibilityRecord
	for _, operation := range candidateRuntimeOperations(candidate) {
		var authorization runtimecontract.RuntimeCandidateOperationManifest
		for _, value := range manifest.Operations {
			if value.Kind == operation.Kind {
				authorization = value
			}
		}
		response := f.send(t, fileevents.TopicProbeRuntimeOperationFeasibility, fileevents.ProbeRuntimeOperationFeasibilityCommand{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, Transaction: a.Decision.AuthorityTransaction(), WorkspaceDeliverableDirectory: f.directory, Operation: operation, Admissibility: catalog, OperationAuthorization: authorization, CandidateManifestHash: manifest.Hash, Observation: &observation}).(fileevents.ProbeRuntimeOperationFeasibilityResult)
		fact, err := runtimecontract.RuntimeOwnerFeasibilityObservationFromOperation(runtimecontract.RuntimeOperationFeasibilityObservation(response))
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: response.OperationID, AdmissibilitySnapshotID: response.AdmissibilitySnapshotID, AcquisitionBinding: catalog.AcquisitionBinding, CandidateManifestHash: response.CandidateManifestHash, Observation: &fact, ReplayProof: response.ReplayProof})
	}
	return values
}

func (f *implementationWorkRealFileFixture) execute(t *testing.T, controller *RuntimeController, a RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	call, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: RuntimeInternalEventScope{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{}})
	if err != nil || call.OperationCall == nil {
		t.Fatalf("real File claim: %v", err)
	}
	request := call.OperationCall.Request
	sealed, err := fileevents.SealRuntimeOperation(request.Operation)
	if err != nil {
		t.Fatal(err)
	}
	execution := request.Execution
	command := fileevents.ExecuteFileEditOperationCommand{SealedOperation: sealed, RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction, OperationID: request.Operation.ID, TargetRefs: request.Operation.TargetRefs, EvidenceKind: request.EvidenceKind, EvidencePolicies: request.EvidencePolicies, WorkspaceDeliverableDirectory: f.directory, WorkspaceInternalDirectory: t.TempDir(), Execution: runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationHash: execution.OperationHash, ExpectedEffectScope: execution.ExpectedEffectScope, IdempotencyClass: execution.IdempotencyClass, AuthorityHash: execution.AuthorityHash, AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID, AcquisitionBinding: execution.AcquisitionBinding, ReplayProof: execution.ReplayProof}}}
	before := f.repository.saves
	response := f.send(t, fileevents.TopicExecuteFileEditOperation, command).(fileevents.ExecuteOperationResult)
	if f.repository.saves != before+1 {
		t.Fatal("real owner receipt did not use one aggregate CAS")
	}
	a, err = controller.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !response.Receipt.Changed || response.Receipt.BeforeHash == response.Receipt.AfterHash || len(response.Receipt.ResourceChanges) == 0 || a.ImplementationWork.Rounds[len(a.ImplementationWork.Rounds)-1].CompletedReceiptSetHash == "" {
		t.Fatalf("real File durable collective not completed: %#v", response.Receipt)
	}
	if response.Receipt.Evidence.Quality != "" {
		t.Fatal("real File original undeclared quality was filled")
	}
	persistedReceipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, response.Receipt.ExecutionKey, response.Receipt.Fence)
	if !found || !reflect.DeepEqual(persistedReceipt, response.Receipt) {
		t.Fatal("real File original receipt drifted in persistence")
	}
	if a.Facts.Coverage[0].CurrentBindingRef != nil {
		t.Fatal("real working receipt gained terminal binding")
	}
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.Quality != "" {
			t.Fatal("real working effect projection rewrote original undeclared quality")
		}
		if len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil {
			t.Fatal("real working receipt gained terminal credit")
		}
	}
	f.aggregate = &a
	return a
}

// 同进程 detached reopen 与真实 File effect；不是公开 activation/full-flow/fresh-process 正例。
func TestImplementationWorkSameCWIRealFileTwoRounds(t *testing.T) {
	testImplementationWorkRealFileTwoRounds(t, nil, nil)
}

func TestRuntimeLineageCompletedRoundsUsesRealFileReceiptsAndOriginalCutoff(t *testing.T) {
	rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "two-work-rounds", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 2}
	testImplementationWorkRealFileTwoRounds(t, []runtimecontract.RuntimeLineageAssertionRule{rule}, func(second RuntimeAggregate, firstCutoff string) {
		coverageID := second.ImplementationWork.Rounds[0].CoverageWorkItemID
		var item runtimecontract.RuntimeCoverageItem
		for _, candidate := range second.Facts.DefinitionSeed.CoverageItems { if candidate.ID == coverageID { item = candidate } }
		declaration := runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
		assertions, err := runtimecontract.CompileRuntimeLineageAssertions([]runtimecontract.RuntimeLineageAssertionRule{rule}, declaration, item)
		if err != nil { t.Fatal(err) }
		// 此只读投影只检验 guard；普通 terminal proof 必须由独立 assessment 另行证明。
		binding := runtimecontract.CoverageEvidenceBinding{ID: "read-only-round-guard", CoverageWorkItemID: coverageID, FocusID: "round-guard", DefinitionRevision: second.Facts.DefinitionRevision, DeclarationRef: declaration, TargetRefs: []string{"target.txt"}, LineageAssertions: assertions,
			Proofs: []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{runtimecontract.StepKindFileEdit}}}}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		before := second.CloneRaw()
		sources := runtimeLineageAssertionSources(second)
		if !runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, sources, second.Facts.EvidenceRevision) { t.Fatal("两轮真实 changed receipt 未满足附加 round guard") }
		if !runtimecontract.EvidenceRevisionAfter(second.Facts.EvidenceRevision, firstCutoff) { t.Fatal("第二轮没有推进原始 evidence cutoff") }
		if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, sources, firstCutoff) { t.Fatal("未来第二轮回执补进了原第一轮 cutoff") }
		for _, mutate := range []func(*RuntimeAggregate){
			func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[1].CompletedReceiptSetHash = "" },
			func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[1].Ordinal = a.ImplementationWork.Rounds[0].Ordinal },
			func(a *RuntimeAggregate) { a.OperationReceipts[0].Changed = false },
			func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[1].ScopeRequest.Policy.RequireFreshObservation = false },
		} {
			invalid := second.CloneRaw()
			mutate(&invalid)
			if runtimecontract.RuntimeExecutionLineageAssertionsSatisfied(binding, runtimeLineageAssertionSources(invalid), second.Facts.EvidenceRevision) { t.Fatal("不完整、重复、no-change 或非 fresh round 满足了 guard") }
		}
		if !reflect.DeepEqual(before, second) { t.Fatal("只读 guard 修改了原 aggregate") }
	})
}

func testImplementationWorkRealFileTwoRounds(t *testing.T, rules []runtimecontract.RuntimeLineageAssertionRule, verify func(RuntimeAggregate, string)) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Teardown(ctx) })
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "target.txt"), []byte("initial implementation"), 0600); err != nil {
		t.Fatal(err)
	}
	owner := &implementationWorkRealFileFixture{hub: hub, directory: directory, lineageRules: rules}
	first := implementationWorkPrivatePipeline(t, runtimecontract.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, owner)
	for len(first.Facts.Prerequisites.Pending) > 0 {
		pending := first.Facts.Prerequisites.Pending[0]
		var observed PrerequisiteResult
		switch pending.Kind {
		case runtimecontract.PrerequisiteKindManifest:
			response := sendObservedManifest(t, ctx, hub, first, pending, directory)
			observed = PrerequisiteResult{PrerequisiteID: pending.ID, Manifest: &response.Manifest, Observation: &response.Observation}
		case runtimecontract.PrerequisiteKindOperationAdmissibility:
			response := sendObservedShellFileAdmissibility(t, ctx, hub, first, pending, directory)
			observed = PrerequisiteResult{PrerequisiteID: pending.ID, OperationAdmissibility: &response.Acquisition.Admissibility, Observation: &response.Acquisition.Observation}
		default:
			t.Fatalf("fresh work prerequisite unsupported by real File fixture: %#v", pending)
		}
		result, err := owner.controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventPrerequisiteObserved, Scope: RuntimeInternalEventScope{WorkspaceID: first.Facts.WorkspaceID, RunID: first.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{PrerequisiteResult: &observed}})
		if err != nil {
			t.Fatal(err)
		}
		first = result.Aggregate
	}
	owner.aggregate = &first
	second := implementationWorkPrivatePipeline(t, runtimecontract.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, owner)
	if len(second.ImplementationWork.Rounds) != 2 || second.ImplementationWork.Rounds[0].CoverageWorkItemID != second.ImplementationWork.Rounds[1].CoverageWorkItemID || second.ImplementationWork.Rounds[1].PrecedingReceiptSetHash != second.ImplementationWork.Rounds[0].CompletedReceiptSetHash || second.ImplementationWork.Rounds[0].ScopeObservation.Selection.ObservedRevision == second.ImplementationWork.Rounds[1].ScopeObservation.Selection.ObservedRevision {
		t.Fatal("same-CWI second round lost fresh original receipt lineage")
	}
	if data, err := os.ReadFile(filepath.Join(directory, "target.txt")); err != nil || string(data) != "实际工作内容-2" {
		t.Fatalf("second real owner content: %q %v", data, err)
	}
	if verify != nil { verify(second, first.Facts.EvidenceRevision) }
}

func implementationWorkFirstDifference(a, b reflect.Value, path string) string {
	if reflect.DeepEqual(a.Interface(), b.Interface()) {
		return ""
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() != b.IsNil() {
			return path + " nil"
		}
		return implementationWorkFirstDifference(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if a.Field(i).CanInterface() {
				if difference := implementationWorkFirstDifference(a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name); difference != "" {
					return difference
				}
			}
		}
	case reflect.Slice:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return fmt.Sprintf("%s length/nil (%d,%t)/(%d,%t)", path, a.Len(), a.IsNil(), b.Len(), b.IsNil())
		}
		for i := 0; i < a.Len(); i++ {
			if difference := implementationWorkFirstDifference(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); difference != "" {
				return difference
			}
		}
	}
	return path
}

func implementationWorkPrivatePipeline(t *testing.T, purpose runtimecontract.RuntimeImplementationWorkPurpose, targets []string, owner *implementationWorkRealFileFixture) RuntimeAggregate {
	t.Helper()
	var rules []runtimecontract.RuntimeLineageAssertionRule
	if owner != nil { rules = owner.lineageRules }
	controller, repository, aggregate := implementationWorkPurposeControllerWithLineage(t, targets, rules)
	if owner != nil {
		controller, repository, aggregate = owner.current(t, aggregate)
	}
	priorRounds, priorChoices, priorHistory, priorEvidence := len(aggregate.ImplementationWork.Rounds), len(aggregate.ImplementationWork.PurposeChoices), len(aggregate.SemanticPlanning.History), len(aggregate.Facts.Evidence.Accepted)
	purposeRequestID := fmt.Sprintf("purpose-request-%d", priorChoices)
	workRequestID := fmt.Sprintf("work-request-%d", priorRounds)
	workProofRequestID := fmt.Sprintf("work-proof-request-%d", priorRounds)
	originalDefinition, originalFocus := aggregate.Definition, aggregate.Facts.DefinitionFocuses[0].Clone()
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.Scope = RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}
		beforeSaves := repository.saves
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("%s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
		if owner != nil && len(aggregate.CandidateAttempts) > 0 && aggregate.CandidateAttempts[len(aggregate.CandidateAttempts)-1].Failure != nil {
			t.Fatalf("real owner candidate rejected at %s: %+v", event.Kind, aggregate.CandidateAttempts[len(aggregate.CandidateAttempts)-1].Failure)
		}
		if repository.saves != beforeSaves+1 {
			t.Fatalf("%s did not use one CAS", event.Kind)
		}
		if err := aggregate.Validate(); err != nil {
			t.Fatal(err)
		}
		fresh, err := controller.Load(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if err != nil || !reflect.DeepEqual(fresh, aggregate) {
			t.Fatalf("fresh full aggregate reload after %s: %v; first mismatch %s", event.Kind, err, implementationWorkFirstDifference(reflect.ValueOf(fresh), reflect.ValueOf(aggregate), "aggregate"))
		}
	}
	request, err := NextRuntimeImplementationWorkScopeRequest(aggregate)
	if err != nil || request == nil {
		t.Fatalf("scope projection: %v", err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventImplementationWorkScopeRequired, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: request}})
	if len(aggregate.ImplementationWork.Rounds) != priorRounds || aggregate.ImplementationWork.ScopePreview == nil {
		t.Fatal("read-only preview granted an effect round")
	}
	selection := runtimecontract.RuntimeTargetSelection{PrerequisiteID: request.RequestID, OwnerModuleID: request.Policy.OwnerModuleID, TargetRole: "implementation_work", SelectorRef: request.Policy.TargetSelectorRef, SelectorOwnerRef: request.Policy.TargetSelectorOwnerRef, SelectorSubjectRef: request.Policy.TargetSelectorSubjectRef, SelectorCandidateRefs: request.Policy.TargetSelectorCandidateRefs, Disposition: runtimecontract.RuntimeTargetSelectionScope, CandidateCount: len(targets), CandidateTargetRefs: targets, TargetRefs: targets, ObservedRevision: request.ManifestBinding.Manifest.Revision}
	if len(targets) == 1 {
		selection.Disposition = runtimecontract.RuntimeTargetSelectionSelected
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := request.ManifestBinding.Observation
	observation.PrerequisiteID, observation.Kind, observation.ContentHash = request.RequestID, runtimecontract.PrerequisiteKindTargetSelection, selection.SnapshotID
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	scope := runtimecontract.RuntimeImplementationWorkScopeObservation{Request: *request, Selection: selection, Observation: observation}
	if owner != nil {
		scope = owner.scope(t, *request)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventImplementationWorkScopeObserved, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeObservation: &scope}})
	manifest, err := semanticplan.CompileManifest(semanticplan.ManifestIdentity{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed, DefinitionFocuses: aggregate.Facts.DefinitionFocuses, Execution: aggregate.Facts.Activation.ExecutionContract, Admissibilities: aggregate.Facts.Resources.OperationAdmissibilities, ImplementationPurposePreview: aggregate.ImplementationWork.ScopePreview, ImplementationWorkRounds: aggregate.ImplementationWork.Rounds})
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "purpose-prefix", LogicalRequestID: purposeRequestID}})
	document := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-v2"}, ImplementationPurpose: purpose}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &document, LogicalRequestID: purposeRequestID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventEligibleFocusProjected, Authority: &RuntimeInternalAuthorityPayload{SemanticOrderManifestHash: manifest.Hash, SemanticCoverageOrder: document.CoverageOrder}})
	if aggregate.ImplementationWork.ScopePreview != nil || len(aggregate.ImplementationWork.PurposeChoices) != priorChoices+1 || len(aggregate.SemanticPlanning.History) != priorHistory+1 || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
		t.Fatal("purpose did not settle exact source and retire generation atomically")
	}
	wantRounds := priorRounds
	if purpose == runtimecontract.RuntimeImplementationWorkPurposeEffect {
		wantRounds++
	}
	if len(aggregate.ImplementationWork.Rounds) != wantRounds {
		t.Fatal("wrong effect round authority")
	}
	if !reflect.DeepEqual(aggregate.Definition, originalDefinition) || !reflect.DeepEqual(aggregate.Facts.DefinitionFocuses[0], originalFocus) || len(aggregate.Facts.Evidence.Accepted) != priorEvidence || aggregate.Facts.Coverage[0].CurrentBindingRef != nil {
		t.Fatal("purpose rewrote startup or conferred terminal proof")
	}
	if purpose == runtimecontract.RuntimeImplementationWorkPurposeEffect {
		round := aggregate.ImplementationWork.Rounds[priorRounds]
		workManifest, err := semanticplan.CompileManifest(semanticplan.ManifestIdentity{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed, DefinitionFocuses: aggregate.Facts.DefinitionFocuses, Execution: aggregate.Facts.Activation.ExecutionContract, Admissibilities: aggregate.Facts.Resources.OperationAdmissibilities, ImplementationWork: &round})
		if err != nil {
			t.Fatal(err)
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &workManifest, StablePrefixHash: "purpose-prefix", LogicalRequestID: workRequestID}})
		order := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: workManifest.Hash, DefinitionSeedHash: workManifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: document.CoverageOrder}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: workRequestID}})
		batch, err := semanticplan.CompileFocusBatch(workManifest, order, 0, workProofRequestID, aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
		if err != nil {
			t.Fatal(err)
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchCompiled, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch}})
		proposal := runtimecontract.RuntimeSemanticProofProposal{CoverageWorkItemID: round.CoverageWorkItemID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite, Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: targets}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: originalFocus.EvidenceDeclarationRef}
		if owner != nil {
			proposal.OperationKind = runtimecontract.StepKindFileEdit
		}
		proof := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: workManifest.Hash, DefinitionSeedHash: workManifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: batch.Sequence, BatchCoverageWorkItemIDs: batch.CoverageWorkItemIDs, Proposals: []runtimecontract.RuntimeSemanticProofProposal{proposal}}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
		catalog := workManifest.OwnerCatalogs[0]
		catalogObservation := aggregate.Facts.Resources.PrerequisiteObservations[0]
		feasibility := runtimecontract.RuntimeSemanticProofFeasibility{SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: workManifest.Hash, DefinitionSeedHash: workManifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, CatalogObservationID: catalogObservation.ObservationID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision, TargetRefs: targets}
		if owner != nil {
			feasibility = owner.semanticFeasibility(t, aggregate, workManifest, proposal)
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
		merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(workManifest, aggregate.SemanticPlanning.Batches[0], proof)
		if err != nil {
			t.Fatal(err)
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
		if aggregate.ImplementationWork.Rounds[priorRounds].WorkingBinding == nil || aggregate.Decision.RequiredOutput.ImplementationWorkBindingHash == "" || aggregate.Facts.Coverage[0].CurrentBindingRef != nil || len(aggregate.Facts.Evidence.Accepted) != priorEvidence {
			t.Fatal("working seal did not preserve no-credit purpose")
		}
		candidateManifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
		if err != nil {
			t.Fatal(err)
		}
		if err := runtimecontract.ValidateRuntimeCandidateContractManifestRawCanonical(candidateManifest); err != nil {
			t.Fatalf("compiled work manifest canonical: %v", err)
		}
		expected := runtimeTestOperationsExpectedDocument(aggregate, candidateManifest.Hash, 0)
		sessionResult, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateRequestEnsured, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, StablePrefixHash: "work-operations-prefix", ManifestHash: candidateManifest.Hash, ExpectedDocument: &expected}})
		if err != nil {
			t.Fatal(err)
		}
		aggregate = sessionResult.Aggregate
		session := sessionResult.CandidateSession
		physicalID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateRequestSubmitted, Transaction: &RuntimeInternalTransactionPayload{LogicalRequestID: session.LogicalRequestID, Sequence: session.NextSequence, PhysicalCallID: physicalID}})
		settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physicalID, nil)
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateDocumentObserved, Transaction: &RuntimeInternalTransactionPayload{LogicalRequestID: session.LogicalRequestID, PhysicalCallID: physicalID, ProviderBudgetSettlement: settlement}})
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateAttemptBegun, Transaction: &RuntimeInternalTransactionPayload{StablePrefixHash: session.StablePrefixHash, LogicalRequestID: session.LogicalRequestID, Manifest: &candidateManifest}})
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{}}
		for i, requirement := range candidateManifest.RequiredOperationSequence {
			candidate.StateChange.Operations = append(candidate.StateChange.Operations, runtimecontract.StateChangingOperation{Operation: runtimecontract.RuntimeOperation{ID: fmt.Sprintf("work-op-%d", i), Kind: runtimecontract.StepKindFileWrite, RequiredOperationRef: requirement.RequirementID, TargetRefs: []string{requirement.TargetRef}, Parameters: runtimeFileWriteParameters(fmt.Sprintf("实际工作内容-%d", priorRounds+1))}})
		}
		if owner != nil {
			for i := range candidate.StateChange.Operations {
				operation := &candidate.StateChange.Operations[i].Operation
				data, readErr := os.ReadFile(filepath.Join(owner.directory, operation.TargetRefs[0]))
				if readErr != nil {
					t.Fatal(readErr)
				}
				oldText, newText := string(data), fmt.Sprintf("实际工作内容-%d", priorRounds+1)
				operation.Kind = runtimecontract.StepKindFileEdit
				sum := sha256.Sum256(data)
				baseHash := hex.EncodeToString(sum[:])
				operation.Parameters = []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}, {Name: "baseHash", String: &baseHash}}
			}
		}
		candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(candidateManifest, candidate)
		if err != nil {
			t.Fatal(err)
		}
		if err := runtimecontract.ValidateRuntimeCandidateAgainstManifest(candidateManifest, candidate); err != nil {
			t.Fatalf("real pipeline manifest candidate validation: %v", err)
		}
		if err := candidate.ValidateWithActivation(aggregate.Decision, runtimecontract.RuntimeActivationForFacts(aggregate.Facts)); err != nil {
			t.Fatalf("real pipeline candidate activation validation: %v", err)
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateProposalObserved, Transaction: &RuntimeInternalTransactionPayload{Manifest: &candidateManifest, LogicalRequestID: session.LogicalRequestID, Candidate: &candidate}})
		feasibilities := runtimeTestCurrentCandidateFeasibilities(t, aggregate, candidateManifest, candidate)
		if owner != nil {
			feasibilities = owner.candidateFeasibilities(t, aggregate, candidateManifest, candidate)
		}
		for _, fact := range feasibilities {
			apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{CandidateFeasibility: &fact}})
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventCandidateSealed, Transaction: &RuntimeInternalTransactionPayload{Manifest: &candidateManifest, LogicalRequestID: session.LogicalRequestID, Candidate: &candidate}})
		if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ImplementationWorkCommitment == nil {
			t.Fatal("working candidate did not seal exact private dispatch")
		}
		if owner == nil {
			implementationWorkReceiptMatrix(t, aggregate)
			implementationWorkReceiptCASLoss(t, aggregate)
			implementationWorkUnknownLateKnown(t, aggregate)
		} else {
			aggregate = owner.execute(t, controller, aggregate)
		}
	}
	before := repository.saves
	reopened, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reopened.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventEligibleFocusProjected, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticOrderManifestHash: manifest.Hash, SemanticCoverageOrder: document.CoverageOrder}})
	if err != nil || repository.saves != before || !reflect.DeepEqual(result.Aggregate, aggregate) {
		t.Fatalf("reopened exact purpose replay changed authority: %v", err)
	}
	return aggregate
}

// 此矩阵沿实际 private working seal/candidate admission 生成的 aggregate；
// owner receipt 为 typed 模拟事实，不能称真实 File owner 或独立 fresh-process 验收。
func implementationWorkReceiptMatrix(t *testing.T, sealed RuntimeAggregate) {
	t.Helper()
	for _, mode := range []string{"accepted", "noop", "weak", "unknown", "unknown_changed", "invalidation_missing", "governance"} {
		t.Run("receipt/"+mode, func(t *testing.T) {
			data, err := json.Marshal(sealed)
			if err != nil {
				t.Fatal(err)
			}
			var initial RuntimeAggregate
			if err := json.Unmarshal(data, &initial); err != nil {
				t.Fatal(err)
			}
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &initial}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			scope := RuntimeInternalEventScope{WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID}
			call, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{}})
			if err != nil || call.OperationCall == nil {
				t.Fatalf("claimed dispatch call: %v", err)
			}
			claimed := call.Aggregate
			if len(claimed.ActiveDispatch.OperationExecutions) > 1 {
				for _, other := range claimed.ActiveDispatch.OperationExecutions[1:] {
					claimed, err = controller.candidateDispatch.claimOperationExecution(context.Background(), scope.WorkspaceID, scope.RunID, other.OperationID)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			execution := claimed.ActiveDispatch.OperationExecutions[0]
			round := claimed.ImplementationWork.Rounds[0]
			dependency := round.WorkingBinding.CatalogObservation.Dependencies[0]
			receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: claimed.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.TargetRefs, BeforeHash: "before-content", AfterHash: "after-content", Evidence: runtimecontract.RuntimeOperationEvidence{ContentHash: "after-content", Summary: "原始 owner 证据", Quality: common.EvidenceQualityValid}, ResourceChanges: []runtimecontract.RuntimeResourceChange{{Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, BeforeRevision: dependency.Revision, AfterRevision: "fresh-revision"}}}
			switch mode {
			case "noop":
				receipt.Outcome, receipt.ObservedEffectScope, receipt.Changed = runtimecontract.RuntimeOperationOutcomeNoOp, runtimecontract.RuntimeEffectNone, false
				receipt.AfterHash, receipt.ResourceChanges = receipt.BeforeHash, nil
			case "weak":
				receipt.Evidence.Quality = common.EvidenceQualityInconclusive
			case "unknown", "unknown_changed":
				receipt.Outcome = runtimecontract.RuntimeOperationOutcomeUnknown
				if mode == "unknown" {
					receipt.ObservedEffectScope, receipt.Changed, receipt.ResourceChanges = runtimecontract.RuntimeEffectUnknown, false, nil
				}
			case "invalidation_missing":
				receipt.ResourceChanges = nil
			case "governance":
				receipt.Outcome, receipt.ObservedEffectScope, receipt.Changed, receipt.ResourceChanges = runtimecontract.RuntimeOperationOutcomeGovernanceRequired, runtimecontract.RuntimeEffectGovernanceOnly, false, nil
				receipt.Governance = &runtimecontract.RuntimeGovernanceRequirement{Kind: runtimecontract.GovernanceApproval, SubjectRef: "target.txt", RequestedAction: runtimecontract.StepKindFileWrite, Reason: "原始 owner 要求精确审批", AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "target.txt"))}
			}
			original, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := receipt.Validate(); err != nil {
				t.Fatal(err)
			}
			before := repository.saves
			event := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{DispatchID: claimed.ActiveDispatch.ID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}}
			result, err := controller.ApplyInternalEvent(context.Background(), event)
			if err != nil {
				t.Fatalf("owner receipt full gate: %v", err)
			}
			if repository.saves != before+1 {
				t.Fatal("receipt requires exactly one aggregate CAS")
			}
			fresh, err := controller.Load(context.Background(), scope.WorkspaceID, scope.RunID)
			if err != nil || !reflect.DeepEqual(fresh, result.Aggregate) {
				t.Fatalf("receipt fresh full aggregate: %v", err)
			}
			persisted, known := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(fresh.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
			data, err = json.Marshal(persisted)
			if err != nil || !known || !reflect.DeepEqual(data, original) {
				t.Fatal("receipt changed original owner fact")
			}
			if fresh.Facts.Coverage[0].CurrentBindingRef != nil || !reflect.DeepEqual(fresh.Definition, sealed.Definition) || !reflect.DeepEqual(fresh.Facts.DefinitionFocuses, sealed.Facts.DefinitionFocuses) {
				t.Fatal("work receipt rewrote startup or terminal binding")
			}
			for _, fact := range append(fresh.Facts.Evidence.Accepted, fresh.Facts.Evidence.Rejected...) {
				if len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil {
					t.Fatal("work receipt credited terminal proof")
				}
			}
			if mode == "accepted" && len(claimed.ActiveDispatch.OperationExecutions) == 1 && fresh.ImplementationWork.Rounds[0].CompletedReceiptSetHash == "" {
				t.Fatal("complete exact durable receipt set did not complete work")
			}
			if mode == "accepted" && len(claimed.ActiveDispatch.OperationExecutions) > 1 && (fresh.ActiveDispatch == nil || fresh.ImplementationWork.Rounds[0].CompletedReceiptSetHash != "") {
				t.Fatal("partial collective receipt falsely completed round")
			}
			if mode != "accepted" && fresh.ImplementationWork.Rounds[0].CompletedReceiptSetHash != "" {
				t.Fatal("non-success receipt completed work")
			}
			if mode == "unknown" || mode == "unknown_changed" {
				unknown := false
				for _, fact := range fresh.ExecutionBudget.Consumptions {
					if fact.UnknownAmount == 1 && fact.ConfirmedAmount == 0 && fact.OwnerResultHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
						unknown = true
					}
				}
				if !unknown {
					t.Fatal("unknown physical consumption was released or confirmed")
				}
			}
			reopened, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			before = repository.saves
			duplicate, err := reopened.ApplyInternalEvent(context.Background(), event)
			if err != nil || repository.saves != before || !reflect.DeepEqual(duplicate.Aggregate, fresh) {
				t.Fatalf("receipt duplicate reopened authority: %v", err)
			}
			if mode == "governance" && len(claimed.ActiveDispatch.OperationExecutions) == 1 {
				implementationWorkGovernanceContinuation(t, reopened, repository, fresh, receipt)
			}
			for i, execution := range claimed.ActiveDispatch.OperationExecutions[1:] {
				late := receipt.Clone()
				late.ExecutionKey, late.Fence, late.OperationID = execution.ExecutionKey, execution.Fence, execution.OperationID
				late.TargetRefs = claimed.ActiveDispatch.Candidate.StateChange.Operations[i+1].Operation.TargetRefs
				late.Outcome, late.ObservedEffectScope, late.Changed, late.Governance, late.Feedback, late.PrerequisiteStale = runtimecontract.RuntimeOperationOutcomeAccepted, runtimecontract.RuntimeEffectWorkspaceDurable, true, nil, nil, nil
				late.BeforeHash, late.AfterHash, late.Evidence.Quality = "late-before", "late-after", common.EvidenceQualityValid
				late.ResourceChanges = []runtimecontract.RuntimeResourceChange{{Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, BeforeRevision: dependency.Revision, AfterRevision: "late-fresh-revision"}}
				lateEvent := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{DispatchID: claimed.ActiveDispatch.ID, Fence: execution.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &late}}
				before = repository.saves
				settled, err := reopened.ApplyInternalEvent(context.Background(), lateEvent)
				if err != nil || repository.saves != before+1 {
					t.Fatalf("collective/archived lateknown single CAS: %v", err)
				}
				fresh, err = reopened.Load(context.Background(), scope.WorkspaceID, scope.RunID)
				if err != nil || !reflect.DeepEqual(fresh, settled.Aggregate) {
					t.Fatalf("lateknown fresh full load: %v", err)
				}
				if mode == "accepted" && fresh.ImplementationWork.Rounds[0].CompletedReceiptSetHash == "" {
					t.Fatal("complete collective original receipts did not complete round")
				}
				if mode != "accepted" && (fresh.ImplementationWork.Rounds[0].CompletedReceiptSetHash != "" || len(fresh.Facts.Evidence.Accepted) != 0) {
					t.Fatal("lateknown reopened closed work/credited effect")
				}
				before = repository.saves
				if _, err := reopened.ApplyInternalEvent(context.Background(), lateEvent); err != nil || repository.saves != before {
					t.Fatalf("lateknown duplicate mutated source: %v", err)
				}
			}
		})
	}
}

func implementationWorkReceiptCASLoss(t *testing.T, sealed RuntimeAggregate) {
	t.Helper()
	for _, loss := range []string{"before", "after"} {
		t.Run("receipt-CAS-loss/"+loss, func(t *testing.T) {
			data, err := json.Marshal(sealed)
			if err != nil {
				t.Fatal(err)
			}
			var initial RuntimeAggregate
			if err := json.Unmarshal(data, &initial); err != nil {
				t.Fatal(err)
			}
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &initial}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			scope := RuntimeInternalEventScope{WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID}
			call, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{}})
			if err != nil || call.OperationCall == nil {
				t.Fatalf("initial call: %v", err)
			}
			claimed := call.Aggregate
			execution := call.OperationCall.Request.Execution
			dependency := claimed.ImplementationWork.Rounds[0].WorkingBinding.CatalogObservation.Dependencies[0]
			receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: call.OperationCall.Request.Operation.TargetRefs, BeforeHash: "before", AfterHash: "after", Evidence: runtimecontract.RuntimeOperationEvidence{ContentHash: "after", Quality: common.EvidenceQualityValid}, ResourceChanges: []runtimecontract.RuntimeResourceChange{{Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, BeforeRevision: dependency.Revision, AfterRevision: "cas-loss-fresh"}}}
			event := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{DispatchID: claimed.ActiveDispatch.ID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}}
			before := repository.saves
			repository.failBefore, repository.failAfter = loss == "before", loss == "after"
			if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil {
				t.Fatal("injected CAS loss not observed")
			}
			persisted, err := controller.Load(context.Background(), scope.WorkspaceID, scope.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if loss == "before" && (repository.saves != before || !reflect.DeepEqual(persisted, claimed)) {
				t.Fatal("pre-CAS failure leaked partial owner/evidence/resource/budget mutation")
			}
			if loss == "after" && (repository.saves != before+1 || len(persisted.OperationReceipts) != len(claimed.OperationReceipts)+1) {
				t.Fatal("response loss did not retain atomic committed owner receipt")
			}
			reopened, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			before = repository.saves
			result, err := reopened.ApplyInternalEvent(context.Background(), event)
			if err != nil {
				t.Fatal(err)
			}
			wantSaves := before
			if loss == "before" {
				wantSaves++
			}
			if repository.saves != wantSaves || len(result.Aggregate.OperationReceipts) != len(claimed.OperationReceipts)+1 {
				t.Fatal("reopened retry duplicated or lost exact owner fact")
			}
			if err := result.Aggregate.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func implementationWorkGovernanceContinuation(t *testing.T, controller *RuntimeController, repository *implementationWorkDetachedRepository, waiting RuntimeAggregate, blocked runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	ctx := context.Background()
	if waiting.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || waiting.ImplementationWork.Rounds[0].GovernanceReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(blocked) || waiting.ImplementationWork.Rounds[0].FailedReceiptHash != "" {
		t.Fatal("governance did not suspend exact outstanding work")
	}
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(waiting)
	if err != nil {
		t.Fatal(err)
	}
	dependency, known := GovernanceDependencyForAggregate(waiting)
	if !known {
		t.Fatal("work governance lost exact owner dependency")
	}
	candidate := runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency}}
	scope := RuntimeInternalEventScope{WorkspaceID: waiting.Facts.WorkspaceID, RunID: waiting.Facts.RunID}
	governed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventGovernanceDispatchSealed, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{Manifest: &manifest, Candidate: &candidate}})
	if err != nil {
		t.Fatal(err)
	}
	approvalID := common.ApprovalID("work-approval")
	request := runtimecontract.RuntimeGovernanceRequestRecord{ApprovalID: approvalID, AuthoritySnapshotID: governed.Aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: governed.Aggregate.ActiveDispatch.ID, DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef, RequestedAction: dependency.RequestedAction, RiskLevel: dependency.RiskLevel, AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext)}
	bound, err := controller.bindGovernanceRequest(ctx, scope.WorkspaceID, scope.RunID, request)
	if err != nil {
		t.Fatal(err)
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{ResolutionID: common.ResolutionID("work-resolution"), ApprovalID: approvalID, WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, AuthoritySnapshotID: bound.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: bound.ActiveDispatch.ID, DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef, Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "只授权原封存 operation", EvidenceRefs: []string{"approval://work"}}
	before := repository.saves
	continued, err := controller.commitGovernanceResolution(ctx, resolution)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != before+1 || continued.Aggregate.ActiveDispatch == nil || continued.Aggregate.ActiveDispatch.ImplementationWorkCommitment == nil || len(continued.Aggregate.ImplementationWork.Rounds) != 1 || continued.Aggregate.ImplementationWork.Rounds[0].GovernanceReceiptHash != "" || len(continued.Aggregate.ImplementationWork.Rounds[0].GovernanceDispatchIDs) != 1 {
		t.Fatal("governance continuation reopened round instead of original sealed work")
	}
	fresh, err := controller.Load(ctx, scope.WorkspaceID, scope.RunID)
	if err != nil || !reflect.DeepEqual(fresh, continued.Aggregate) {
		t.Fatalf("governance continuation fresh fullgate: %v", err)
	}
	call, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{}})
	if err != nil || call.OperationCall == nil {
		t.Fatalf("authorized original work call: %v", err)
	}
	execution := call.OperationCall.Request.Execution
	if execution.ExecutionKey != blocked.ExecutionKey || execution.Fence != blocked.Fence+1 || call.OperationCall.Request.GovernanceGrant == nil {
		t.Fatal("governance changed original operation identity or omitted grant")
	}
	dependencyRevision := call.Aggregate.ImplementationWork.Rounds[0].WorkingBinding.CatalogObservation.Dependencies[0]
	receipt := blocked.Clone()
	receipt.Fence, receipt.Outcome, receipt.ObservedEffectScope, receipt.Changed, receipt.Governance = execution.Fence, runtimecontract.RuntimeOperationOutcomeAccepted, runtimecontract.RuntimeEffectWorkspaceDurable, true, nil
	receipt.BeforeHash, receipt.AfterHash, receipt.Evidence.Quality = "before-content", "after-content", common.EvidenceQualityValid
	receipt.ResourceChanges = []runtimecontract.RuntimeResourceChange{{Domain: dependencyRevision.Domain, OwnerModuleID: dependencyRevision.OwnerModuleID, SubjectRef: dependencyRevision.SubjectRef, BeforeRevision: dependencyRevision.Revision, AfterRevision: "governance-fresh"}}
	before = repository.saves
	committed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{DispatchID: call.Aggregate.ActiveDispatch.ID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}})
	if err != nil || repository.saves != before+1 {
		t.Fatalf("authorized owner receipt single CAS: %v", err)
	}
	fresh, err = controller.Load(ctx, scope.WorkspaceID, scope.RunID)
	if err != nil || !reflect.DeepEqual(fresh, committed.Aggregate) || fresh.ImplementationWork.Rounds[0].CompletedReceiptSetHash == "" || fresh.Facts.Coverage[0].CurrentBindingRef != nil {
		t.Fatalf("governance work collective/no-credit source: %v", err)
	}
}

func TestImplementationWorkScopeEnvelopeRejectsAuthoritySmuggling(t *testing.T) {
	request := runtimecontract.RuntimeImplementationWorkScopeRequest{}
	for _, event := range []RuntimeInternalEvent{
		{Kind: RuntimeInternalEventReconcileAuthority, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: &request}},
		{Kind: RuntimeInternalEventImplementationWorkScopeRequired, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: &request, FailureFact: &runtimecontract.FailureFact{}}},
		{Kind: RuntimeInternalEventImplementationWorkScopeObserved, Authority: &RuntimeInternalAuthorityPayload{ImplementationWorkScopeRequest: &request}},
	} {
		event.Scope = RuntimeInternalEventScope{WorkspaceID: "ws-1", RunID: "run-1"}
		if err := event.validateEnvelope(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("scope payload smuggling accepted: %v", err)
		}
	}
}
