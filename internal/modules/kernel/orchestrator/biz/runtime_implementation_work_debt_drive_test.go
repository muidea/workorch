package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 这是 pre-activation gate 后的 owner-private aggregate fixture，不是公开 activation 成功。
// 后续 scope/acquisition/manifest 均实际调用 production SM-4 driver 与 File owner。
func implementationWorkPurposeDebtDriverFixture(t *testing.T) (*Biz, *runtimeDriveRepository, runtime.RuntimeAggregate) {
	t.Helper()
	return implementationWorkPurposeDebtDriverFixtureWithEmptyDirectory(t, false)
}

func implementationWorkPurposeDebtDriverFixtureWithEmptyDirectory(t *testing.T, empty bool) (*Biz, *runtimeDriveRepository, runtime.RuntimeAggregate) {
	t.Helper()
	ctx := context.Background()
	directory := t.TempDir()
	if !empty {
		if err := os.WriteFile(filepath.Join(directory, "resource-a"), []byte("original implementation"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	file := filemodule.New()
	if err := file.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Stop(ctx) })
	shell := shellmodule.New()
	if err := shell.Configure(shellbiz.Dependencies{EventHub: hub, RuntimeAdapter: shellbiz.NewRuntimeAdapter()}); err != nil {
		t.Fatal(err)
	}
	if err := shell.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shell.Stop(ctx) })
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: directory, InternalDirectory: filepath.Join(directory, ".internal")}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	a := runtimeAggregate(t, 1)
	// Aggregate-only base fixture 不含 provider progress 要求的 immutable source projection。
	for index, item := range a.Facts.DefinitionSeed.CoverageItems {
		a.Facts.Coverage[index].SourceScope = item.SourceScope
		a.Facts.Coverage[index].SourceID = item.SourceID
		a.Facts.Coverage[index].TargetText = item.TargetText
	}
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations:     []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		TargetSelectorRef:     "file/workspace_deliverable", TargetSelectorOwnerRef: "profile/work-debt", TargetSelectorSubjectRef: "implementation",
		TargetSelectorCandidateRefs: []string{"resource-a"}, MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1,
		RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
	if empty {
		policy.AllowedOperations = append(policy.AllowedOperations, runtimecontract.RuntimeOperationVariantRef{Kind: runtimecontract.StepKindFileWrite})
		policy.AllowedTargetPolicies = append(policy.AllowedTargetPolicies, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget)
		policy.ProspectiveTargetAllOfConstraintRefs = []string{"workspace-relative-path", "new-target-requires-owner-selector"}
		a.Facts.Capabilities.StateChangeKinds = append(a.Facts.Capabilities.StateChangeKinds, runtimecontract.StepKindFileWrite)
		a.Definition.CapabilityContract = append(a.Definition.CapabilityContract, runtimecontract.StepKindFileWrite)
	}
	a.Facts.DefinitionFocuses[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	a.Facts.DefinitionFocuses[0].AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	a.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	a.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	a.Definition.CapabilityContract = append(a.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindManagedValidationSession)
	a.Facts.DefinitionFocuses[0].ImplementationWorkPolicy = &policy
	// 声明 recipe 是既有 owner 输入合同；是否 ready 仍由真实 Shell catalog 决定，不在工作 gate 强求。
	a.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{runtimeFileBackedManagedValidationEvidencePolicy()}
	a.Facts.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "work-debt", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: a.Facts.DefinitionSeed.CoverageItems[0].SourceScope, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy}}}}
	fileKinds := []string{runtimecontract.StepKindFileEdit}
	if empty {
		fileKinds = append(fileKinds, runtimecontract.StepKindFileWrite)
	}
	a.Facts = runtime.PreflightRuntimeFacts(a.Facts, []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: fileKinds},
		{Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator},
	})
	var err error
	a.Facts, a.Decision, err = runtime.ReduceRuntimeStateWithDirective(a.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	repo := &runtimeDriveRepository{aggregate: a}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repo); err != nil {
		t.Fatal(err)
	}
	return newTestBiz(owner, hub, nil), repo, a
}

func TestImplementationWorkProductionDriverDefersTerminalDebtWithoutFallback(t *testing.T) {
	ctx := context.Background()
	module, repo, initial := implementationWorkPurposeDebtDriverFixture(t)
	drive := func(a runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
		outcome, err := module.driveOneRuntimeTransition(ctx, nil, a, orchestratorevents.DriveRuntimeCommand{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID}, "")
		return outcome.aggregate, err
	}
	prepared, err := drive(initial)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || prepared.Facts.Resources.Manifest == nil || len(prepared.Facts.Resources.OperationAdmissibilities) != 2 || prepared.Facts.Resources.ValidationScratch != nil || len(prepared.Facts.Prerequisites.Pending) != 1 || prepared.Facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindValidationScratch {
		t.Fatalf("working prerequisite acquisition must retain terminal debt: phase=%s resources=%+v pending=%+v", prepared.SemanticPlanning.Phase, prepared.Facts.Resources, prepared.Facts.Prerequisites.Pending)
	}
	// 即使 receipt 前像显示 satisfied，也不能丢掉其 input-protection catalog。
	missing := prepared
	missing.Facts = prepared.Facts.Normalized()
	missing.Facts.Resources.OperationAdmissibilities = nil
	if request, err := runtime.NextRuntimeImplementationWorkScopeRequest(missing); err == nil || request != nil {
		t.Fatal("missing declared owner input-protection catalog admitted scope")
	}
	coordinated := prepared
	coordinated.Facts = prepared.Facts.Normalized()
	coordinated.Facts.Resources.OperationAdmissibilities = nil
	for _, catalog := range prepared.Facts.Resources.OperationAdmissibilities {
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			coordinated.Facts.Resources.OperationAdmissibilities = append(coordinated.Facts.Resources.OperationAdmissibilities, catalog)
		}
	}
	coordinated.Facts.Prerequisites.Satisfied = nil
	for _, value := range prepared.Facts.Prerequisites.Satisfied {
		if value.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			coordinated.Facts.Prerequisites.Satisfied = append(coordinated.Facts.Prerequisites.Satisfied, value)
		}
	}
	coordinated.Facts.Resources.PrerequisiteObservations = nil
	for _, value := range prepared.Facts.Resources.PrerequisiteObservations {
		if value.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			coordinated.Facts.Resources.PrerequisiteObservations = append(coordinated.Facts.Resources.PrerequisiteObservations, value)
		}
	}
	if err := coordinated.Facts.Validate(); err != nil {
		t.Fatalf("coordinated-removal negative must preserve old facts shape so sealed inventory gate rejects independently: %v", err)
	}
	if request, err := runtime.NextRuntimeImplementationWorkScopeRequest(coordinated); err == nil || request != nil {
		t.Fatal("coordinated removal of other owner's prerequisite/catalog/observation bypassed immutable input-protection inventory")
	}
	blocked := prepared
	blocked.Facts = prepared.Facts.Normalized()
	blocked.Facts.Prerequisites.Pending = append(blocked.Facts.Prerequisites.Pending, runtimecontract.RuntimePrerequisite{ID: "foreign-input-catalog", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: "different-input-owner"})
	if request, err := runtime.NextRuntimeImplementationWorkScopeRequest(blocked); err != nil || request != nil {
		t.Fatalf("work scope bypassed another owner's input-protection catalog debt: %v", err)
	}
	reserved, err := drive(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.ImplementationWork.ScopePreview == nil || reserved.SemanticPlanning.Manifest != nil || reserved.StateRevision != prepared.StateRevision+1 {
		t.Fatal("terminal-only debt silently bypassed implementation purpose scope")
	}
	observed, err := drive(reserved)
	if err != nil {
		t.Fatal(err)
	}
	if observed.ImplementationWork.ScopePreview == nil || observed.ImplementationWork.ScopePreview.Observation == nil || observed.Facts.Resources.TargetSelection != nil {
		t.Fatal("real File scope observation must be separate from terminal selection")
	}
	started, err := drive(observed)
	if err != nil {
		t.Fatal(err)
	}
	if started.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen || started.SemanticPlanning.Manifest == nil || started.SemanticPlanning.Manifest.ProofObligations[0].ImplementationWork == nil || !started.SemanticPlanning.Manifest.ProofObligations[0].ImplementationWork.PurposeChoiceOnly || started.Facts.Resources.ValidationScratch != nil || len(started.Facts.Prerequisites.Pending) != 1 || len(started.ImplementationWork.Rounds) != 0 {
		t.Fatalf("production driver lost purpose choice or consumed terminal debt/round before proposal: phase=%s failure=%+v manifest=%+v pending=%+v rounds=%d", started.SemanticPlanning.Phase, started.Facts.Failure, started.SemanticPlanning.Manifest, started.Facts.Prerequisites.Pending, len(started.ImplementationWork.Rounds))
	}
	fresh, err := module.runtimeController.Load(ctx, started.Facts.WorkspaceID, started.Facts.RunID)
	if err != nil || !reflect.DeepEqual(fresh.ImplementationWork, repo.aggregate.ImplementationWork) {
		t.Fatalf("persisted scope lineage: %v", err)
	}
	provider := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, module.EventHub())
	providerCalls := 0
	provider.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		manifest := command.SemanticManifest
		document := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}, Proposals: []runtimecontract.RuntimeSemanticProofProposal{}, ImplementationPurpose: runtimecontract.RuntimeImplementationWorkPurposeTerminal}
		payload, marshalErr := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
		if marshalErr != nil {
			t.Error(marshalErr)
			result.Set(nil, nil)
			return
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil)
	})
	t.Cleanup(func() { provider.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	accepted, err := drive(started)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted || providerCalls != 1 {
		t.Fatalf("purpose provider did not use original request transaction: phase=%s calls=%d failure=%+v correction=%+v", accepted.SemanticPlanning.Phase, providerCalls, accepted.Facts.Failure, accepted.SemanticPlanning.CorrectionDirective)
	}
	selected, err := drive(accepted)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.ImplementationWork.PurposeChoices) != 1 || selected.ImplementationWork.PurposeChoices[0].Purpose != runtimecontract.RuntimeImplementationWorkPurposeTerminal || selected.ImplementationWork.ScopePreview != nil || selected.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(selected.Facts.Prerequisites.Pending) == 0 {
		t.Fatal("terminal purpose selection bypassed original debt or failed to retire working generation")
	}
	terminalPrepared, err := drive(selected)
	if err != nil {
		t.Fatal(err)
	}
	if terminalPrepared.Facts.Resources.ValidationScratch == nil || len(terminalPrepared.Facts.Prerequisites.Pending) != 0 || providerCalls != 1 || terminalPrepared.SemanticPlanning.Manifest != nil {
		t.Fatal("terminal entry must acquire all original debt before new provider manifest")
	}
	// 原 unsupported production ingress 仍零写入，不把上面的 fixture 冒充公开 activation。
	closed := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(closed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventActivationObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID}, Activation: &runtime.RuntimeInternalActivationPayload{ActivationFacts: &initial.Facts, ActivationDefinition: &initial.Definition}})
	if !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkUnavailable) || closed.aggregate.StateRevision != 0 {
		t.Fatalf("unsupported public availability bypass: %v", err)
	}
}

func TestImplementationWorkPrerequisitePurposeKeepsAllOwnerCatalogsAndTerminalGate(t *testing.T) {
	module, _, a := implementationWorkPurposeDebtDriverFixture(t)
	_ = module
	for _, kind := range []runtimecontract.PrerequisiteKind{runtimecontract.PrerequisiteKindManifest, runtimecontract.PrerequisiteKindProviderCeiling, runtimecontract.PrerequisiteKindCapability, runtimecontract.PrerequisiteKindCapabilityState, runtimecontract.PrerequisiteKindOperationAdmissibility, "unknown"} {
		value := runtimecontract.RuntimePrerequisite{ID: "required", Kind: kind, OwnerModuleID: "different-input-owner"}
		if !runtime.RuntimeSemanticPrerequisiteRequiredForAggregate(a, value) {
			t.Fatalf("work skipped necessary owner fact: %s", kind)
		}
	}
	for _, kind := range []runtimecontract.PrerequisiteKind{runtimecontract.PrerequisiteKindTargetSelection, runtimecontract.PrerequisiteKindTargetSnapshot} {
		value := runtimecontract.RuntimePrerequisite{ID: "terminal-target", Kind: kind, Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: a.Facts.Focus.ID, SubjectRef: "terminal-target"}
		if runtime.RuntimeSemanticPrerequisiteRequiredForAggregate(a, value) {
			t.Fatalf("working purpose required terminal target: %s", kind)
		}
		if kind == runtimecontract.PrerequisiteKindTargetSnapshot && !runtimecontract.RuntimeImplementationWorkPrerequisiteRequired(value, a.Facts.Focus.ID, []string{value.SubjectRef}) {
			t.Fatal("working exact target snapshot bypassed")
		}
	}
	for _, value := range []runtimecontract.RuntimePrerequisite{
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "other-focus"},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, Scope: "unknown", FocusID: a.Facts.Focus.ID},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch, Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, FocusID: "foreign-focus"},
	} {
		if !runtime.RuntimeSemanticPrerequisiteRequiredForAggregate(a, value) {
			t.Fatalf("work skipped foreign or unknown scratch scope: %+v", value)
		}
	}
	// 此断言只测试分类；terminal choice 的来源/CAS 由既有 private pipeline 回归反证。
	a.ImplementationWork.PurposeChoices = []runtimecontract.RuntimeImplementationWorkPurposeChoice{{CoverageWorkItemID: a.Facts.Focus.CoverageWorkItemIDs[0], Purpose: runtimecontract.RuntimeImplementationWorkPurposeTerminal}}
	for _, value := range a.Facts.Prerequisites.Pending {
		if !runtime.RuntimeSemanticPrerequisiteRequiredForAggregate(a, value) {
			t.Fatalf("terminal entry weakened original prerequisite gate: %+v", value)
		}
	}
}
