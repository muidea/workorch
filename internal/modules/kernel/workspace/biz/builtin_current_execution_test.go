package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func updatedBuiltinTemplate(t *testing.T, owners *fileDefinitionAdmissionOwners, w *workspace.Workspace, description string) *workspace.WorkspaceTemplate {
	t.Helper()
	current, err := owners.workspace.resolvedTemplate(context.Background(), w.TemplateID)
	if err != nil {
		t.Fatal(err)
	}
	hints := current.ContextHints.Map()
	hints["profileContracts"].(map[string]any)["generic"].(map[string]any)["evidenceRequirements"].([]any)[0].(map[string]any)["description"] = description
	// A new default budget must not alter an already confirmed authorization.
	hints["executionLimits"] = map[string]any{"protocolRef": rc.RuntimeExecutionLimitsV1, "maxProviderCalls": 300, "maxCapabilityOperations": 300, "maxProofBatchesPerCWI": 7, "maxDurableDispatchesPerCWI": 9}
	current.ContextHints, err = payload.FromMap(hints)
	if err != nil {
		t.Fatal(err)
	}
	current.Description = description
	current.DefaultSoulID = "current-soul"
	current.SoulBindings = map[string]string{"execution": "current-soul"}
	current.DoDItems = append(current.DoDItems, "new template criterion must not enter old business acceptance")
	current.EffectiveSources = nil
	if err := owners.repositories.WorkspaceTemplates.Update(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	resolved, err := owners.workspace.resolvedTemplate(context.Background(), w.TemplateID)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestConfirmedBusinessUsesCurrentBuiltinsWithoutReseal(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-current-builtins")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	confirmation := w.DefinitionAuthoringConfirmation.Clone()
	// This is the same private event used after the first activation. A later
	// restart must not compare its derived default with the confirmation hash.
	w, err := owners.workspace.submitWorkspaceExecutionInternalEvent(ctx, workspaceExecutionInternalEvent{
		Kind:        workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID: w.ID, StartupInputHash: w.StartupInput.StartupInputHash,
		ProviderSelection: rc.ProviderInferenceSelection{Target: rc.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "current-model"}},
	})
	if err != nil || workspace.WorkspaceConfigurationHash(w) == confirmation.ConfigurationHash {
		t.Fatal("fixture did not materialize a distinct execution default", err)
	}
	original, _ := json.Marshal(w)
	originalCandidate, present, err := w.ValidateActivationAdmissionPersistence()
	if err != nil || !present {
		t.Fatal(err)
	}
	current := updatedBuiltinTemplate(t, owners, w, "current execution declaration")
	goal, dod, err := owners.workspace.currentExecutionDefinitions(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, owners.hub, w, goal, dod)
	if err != nil {
		t.Fatalf("builtin update blocked confirmed business: %v", err)
	}
	if admitted.Candidate.CandidateHash != originalCandidate.CandidateHash {
		t.Fatal("business candidate replaced")
	}
	compiled, err := definitionconfiguration.Compile(w, current)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.ProfileContractsHash == w.StartupInput.RuntimeProfileContractsHash || compiled.ExecutionLimits != nil {
		t.Fatal("current profile or original budget absence lost")
	}
	execution := rc.RuntimeExecutionContract{ProfileContracts: compiled.ProfileContracts}
	if err := definitionconfiguration.ValidateExecutionPolicy(w, current, execution); err != nil {
		t.Fatalf("owner rejected current policy: %v", err)
	}
	execution.ProfileContracts = originalCandidate.RuntimeProfileContracts
	if definitionconfiguration.ValidateExecutionPolicy(w, current, execution) == nil {
		t.Fatal("obsolete or forged execution policy accepted")
	}
	execution.ProfileContracts = compiled.ProfileContracts
	if err := definitionconfiguration.ValidateExecutionActivation(w, current, rc.RuntimeActivationContext{ExecutionContract: execution}); err != nil {
		t.Fatal(err)
	}
	if definitionconfiguration.ValidateExecutionActivation(w, current, rc.RuntimeActivationContext{ExecutionContract: execution, EvidencePolicies: []rc.RuntimeEvidencePolicy{{ID: "injected", AppliesTo: "file/read"}}}) == nil {
		t.Fatal("Application injected evidence policy outside current declaration")
	}
	inventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(originalCandidate, *w.StartupInput, *w.ActivationAdmission)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := rc.RuntimeBootstrap{Activation: rc.RuntimeActivationContext{ExecutionContract: execution}, Definition: rc.RuntimeDefinitionContext{Revision: w.StartupInput.DefinitionRevision, StartupInputHash: w.StartupInput.StartupInputHash, SourceObligations: inventory}, DefinitionSeed: rc.RuntimeDefinitionSeed{SourceObligations: inventory}}
	if err := definitionconfiguration.ValidateExecutionBootstrap(w, current, bootstrap); err != nil {
		t.Fatalf("original source with current proof: %v", err)
	}
	bootstrap = bootstrap.Clone()
	bootstrap.Definition.SourceObligations.Sources[0].Text += " forged history"
	if definitionconfiguration.ValidateExecutionBootstrap(w, current, bootstrap) == nil {
		t.Fatal("current declaration authenticated invented historical source")
	}
	execution.ExecutionLimits = &rc.RuntimeExecutionLimits{ProtocolRef: rc.RuntimeExecutionLimitsV1, MaxProviderCalls: 300, MaxCapabilityOperations: 300}
	if definitionconfiguration.ValidateExecutionPolicy(w, current, execution) == nil {
		t.Fatal("new default increased original budget authority")
	}
	after, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	persisted, _ := json.Marshal(after)
	if !bytes.Equal(original, persisted) {
		t.Fatal("current compilation modified confirmed owner input or history")
	}
	// Business changes still require exact confirmation. A configuration hash
	// is not a substitute for checking the actual business after-images.
	tamperedGoal := *goal
	tamperedGoal.Text += " unconfirmed requirement"
	if _, err := activationports.EnsureWorkspaceDefinitionAdmission(ctx, owners.hub, w, &tamperedGoal, dod); err == nil {
		t.Fatal("changed business input bypassed confirmation")
	}
}

func TestPreparationKeepsSelectedCurrentDeclarationAcrossLaterUpdate(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-current-check")
	w, err := owners.workspace.submitWorkspaceExecutionInternalEvent(ctx, workspaceExecutionInternalEvent{
		Kind:        workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID: w.ID, StartupInputHash: w.StartupInput.StartupInputHash,
		ProviderSelection: rc.ProviderInferenceSelection{Target: rc.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "current-model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	selected := updatedBuiltinTemplate(t, owners, w, "selected for original check")
	candidateSelection := workspace.NewPreparationTemplate(*selected)
	if err := candidateSelection.Validate(); err != nil {
		t.Fatalf("selected declaration: %v", err)
	}
	if _, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision}); err != nil {
		t.Fatalf("original business: %v", err)
	}
	command := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "selected-check", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, command, false, true); err != nil {
		t.Fatal(err)
	}
	accepted, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := accepted.StartupPreparation.Attempts[0]
	if a.ExecutionTemplate == nil || a.ExecutionTemplate.SnapshotHash != workspace.WorkspaceTemplateSnapshotHash(selected) {
		t.Fatal("acceptance did not seal current source")
	}
	effective, err := workspace.PreparationWorkspace(accepted, a)
	if err != nil || effective.DefaultSoulID != "current-soul" {
		t.Fatalf("selected route unavailable: %v", err)
	}
	updatedBuiltinTemplate(t, owners, w, "later deployment declaration")
	seal := owners.workspace.observeOriginalPreparationSeal(ctx, command)
	if seal == nil {
		t.Fatal("exact original seal unavailable")
	}
	if seal.Before.AuthoringConfigurationHash != workspace.WorkspaceConfigurationHash(w) || seal.Before.AuthoringConfigurationHash == w.DefinitionAuthoringConfirmation.ConfigurationHash {
		t.Fatal("preparation pinned configuration to the historical confirmation")
	}
	p := workspace.PreparationProcessPolicy{MaxRuntimeRepairAttempts: 2, MaxDocumentRecoveryAttempts: 1}
	now := time.Now().UTC()
	f := workspacePreparationBlocked{Command: command, Reason: "check_pipeline_incomplete", SealEvidence: seal, ProcessPolicyFacts: &workspace.PreparationProcessPolicyFacts{Version: workspace.PreparationExecutionInputsVersion, WorkspaceID: w.ID, WorkspaceRevision: accepted.Revision, StartedAt: now, CompletedAt: now, Before: p, After: p}}
	if _, err := owners.workspace.finishBlockedPreparation(ctx, f); err != nil {
		t.Fatal(err)
	}
	saved, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	inputs := saved.PreparationSealEvidence[0].ExecutionInputs
	if inputs == nil || !reflect.DeepEqual(inputs.Template, selected.Clone()) {
		t.Fatal("settlement silently changed original operation declaration")
	}
	policy, err := preparationcompiler.CompileOriginalPreparedExecutionPolicy(saved, saved.StartupPreparation.Attempts[0])
	if err != nil || len(policy.ExecutionContract.ProfileContracts) == 0 {
		t.Fatalf("original archive cannot replay: %v", err)
	}
	if !reflect.DeepEqual(saved.StartupInput, w.StartupInput) || !reflect.DeepEqual(saved.DefinitionAuthoringConfirmation, w.DefinitionAuthoringConfirmation) {
		t.Fatal("preparation rewrote business seal")
	}
	// The new configuration identity is authenticated by the complete original
	// execution input, including on a fresh repository read. Removing the old
	// journal equality must not allow an injected preparation configuration.
	for _, mutate := range []func(*workspace.Workspace){
		func(w *workspace.Workspace) {
			w.PreparationSealEvidence[0].ExecutionInputs.Configuration.InternalDirectory += "-foreign"
		},
		func(w *workspace.Workspace) {
			w.PreparationSealEvidence[0].ExecutionInputs.Configuration.DefaultLLMSelection.Target.ModelID = "foreign-model"
		},
	} {
		bad, err := owners.repositories.Workspaces.Get(ctx, w.ID)
		if err != nil {
			t.Fatal(err)
		}
		mutate(bad)
		if workspace.ValidatePreparationEvidencePersistence(bad) == nil {
			t.Fatal("changed preparation preimage accepted")
		}
	}
}

func TestMaterializedConfirmationReplayAfterExecutionDefaultsAndReopen(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	w := seedPreparationEligible(t, owners, "ws-confirmed-default-replay")
	command := authoringConfirmationCommandFromPersistedLedger(t, w)
	journal := w.DefinitionAuthoringConfirmation.Clone()
	selected := workspaceExecutionInternalEvent{Kind: workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID: w.ID, StartupInputHash: w.StartupInput.StartupInputHash,
		ProviderSelection: rc.ProviderInferenceSelection{Target: rc.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "current-model"}}}
	w, err := owners.workspace.submitWorkspaceExecutionInternalEvent(ctx, selected)
	if err != nil {
		t.Fatal(err)
	}
	updatedBuiltinTemplate(t, owners, w, "new deployment")
	before, _ := json.Marshal(w)
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	for range 2 {
		if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: command}); err != nil {
			t.Fatalf("materialized confirmation replay blocked by execution defaults: %v", err)
		}
		if _, err := owners.workspace.submitWorkspaceExecutionInternalEvent(ctx, selected); err != nil {
			t.Fatal(err)
		}
		after, err := owners.repositories.Workspaces.Get(ctx, w.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(after)
		if !bytes.Equal(before, raw) || !reflect.DeepEqual(journal, *after.DefinitionAuthoringConfirmation) {
			t.Fatal("replay changed business seal, revision or execution metadata")
		}
	}
	stale := command
	stale.SpecificationHash += "-foreign"
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: stale}); err == nil {
		t.Fatal("foreign confirmation identity accepted")
	}
}

func TestCurrentBuiltinsRetainConfirmedBudgetAndExplicitEvidencePolicy(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	limits := map[string]any{"protocolRef": rc.RuntimeExecutionLimitsV1, "maxProviderCalls": 120, "maxCapabilityOperations": 80}
	w := seedPreparationEligibleWithConfiguration(t, owners, "ws-current-budget", "delivery", func(w *workspace.Workspace) {
		w.EvidencePolicies = []payload.Payload{payload.New(payload.StringField("id", "project-evidence"), payload.StringField("appliesTo", "file/read"))}
	}, func(tpl *workspace.WorkspaceTemplate) {
		hints := tpl.ContextHints.Map()
		hints["executionLimits"] = limits
		var err error
		tpl.ContextHints, err = payload.FromMap(hints)
		if err != nil {
			t.Fatal(err)
		}
	})
	current := updatedBuiltinTemplate(t, owners, w, "new declaration with larger default budget")
	current.EvidencePolicies = []payload.Payload{payload.New(payload.StringField("id", "new-system-evidence"), payload.StringField("appliesTo", "file/read"))}
	compiled, err := definitionconfiguration.Compile(w, current)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.ExecutionLimits == nil || compiled.ExecutionLimits.MaxProviderCalls != 120 || compiled.ExecutionLimits.MaxCapabilityOperations != 80 || compiled.ExecutionLimits.MaxProofBatchesPerCWI != 0 || compiled.ExecutionLimits.MaxDurableDispatchesPerCWI != 0 {
		t.Fatalf("confirmed limits changed: %+v", compiled.ExecutionLimits)
	}
	activation := rc.RuntimeActivationContext{ExecutionContract: rc.RuntimeExecutionContract{ProfileContracts: compiled.ProfileContracts, ExecutionLimits: compiled.ExecutionLimits}, EvidencePolicies: []rc.RuntimeEvidencePolicy{{ID: "project-evidence", AppliesTo: "file/read"}}}
	if err := definitionconfiguration.ValidateExecutionActivation(w, current, activation); err != nil {
		t.Fatalf("original business boundaries: %v", err)
	}
	activation.ExecutionContract.ExecutionLimits = &rc.RuntimeExecutionLimits{ProtocolRef: rc.RuntimeExecutionLimitsV1, MaxProviderCalls: 300, MaxCapabilityOperations: 300}
	if definitionconfiguration.ValidateExecutionActivation(w, current, activation) == nil {
		t.Fatal("budget expansion accepted")
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(stored.StartupInput, w.StartupInput) {
		t.Fatalf("business source changed: %v", err)
	}
}
