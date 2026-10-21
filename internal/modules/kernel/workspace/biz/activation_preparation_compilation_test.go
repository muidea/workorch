package workspace

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationManifestPreservesEveryRequiredCategoryAndExactBinding(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-manifest")
	read, err := owners.workspace.queryExactConfirmedStartupSnapshot(context.Background(), workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := startuppreparation.CompilePreparationManifest(read.Snapshot)
	if err != nil || manifest.ValidateFor(read.Snapshot) != nil {
		t.Fatalf("compile exact manifest: %v", err)
	}
	expectedOwners := map[startuppreparation.CheckID]string{
		startuppreparation.CheckExactSeal: "workspace", startuppreparation.CheckDirectory: "file",
		startuppreparation.CheckManagedConfigs: "file", startuppreparation.CheckSoulSelection: "soul",
		startuppreparation.CheckSkillSelection: "skill", startuppreparation.CheckMCPSelection: "mcpregistry",
		startuppreparation.CheckMemorySelection: "memory", startuppreparation.CheckProviderCache: "llm",
		startuppreparation.CheckPersistence: "workspace", startuppreparation.CheckNoRun: "run",
	}
	for _, check := range manifest.Checks {
		owner, expected := expectedOwners[check.ID]
		if expected {
			if owner != check.Owner || !check.Required || check.RequirementSourceRef == "" || check.EvidenceRule == "" || check.FreshnessRule == "" || check.NotApplicableReason != "" {
				t.Fatalf("required declaration incomplete: %+v", check)
			}
			delete(expectedOwners, check.ID)
		}
	}
	if len(expectedOwners) != 0 {
		t.Fatalf("missing required categories: %+v", expectedOwners)
	}
	for name, mutate := range map[string]func(*startuppreparation.Manifest){
		"omit_required":    func(m *startuppreparation.Manifest) { m.Checks = m.Checks[1:] },
		"optionalize":      func(m *startuppreparation.Manifest) { m.Checks[0].Required = false },
		"invent_na":        func(m *startuppreparation.Manifest) { m.Checks[0].NotApplicableReason = "not implemented" },
		"substitute_owner": func(m *startuppreparation.Manifest) { m.Checks[0].Owner = "application" },
		"replace_subject":  func(m *startuppreparation.Manifest) { m.Checks[0].SubjectRef = "other" },
		"policy":           func(m *startuppreparation.Manifest) { m.PolicyVersion = "other" },
		"snapshot":         func(m *startuppreparation.Manifest) { m.SnapshotHash = "other" },
		"hash":             func(m *startuppreparation.Manifest) { m.ManifestHash = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := manifest.Clone()
			mutate(&changed)
			if changed.ValidateFor(read.Snapshot) == nil {
				t.Fatal("accepted changed required declaration")
			}
		})
	}
	second, err := startuppreparation.CompilePreparationManifest(read.Snapshot)
	if err != nil || !reflect.DeepEqual(manifest, second) {
		t.Fatal("pure manifest compilation is not stable")
	}
}

func TestPreparedBootstrapPureCompilationIsNotReadiness(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-preparation-compiler")
	read, err := owners.workspace.queryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: w.ID, ExpectedWorkspaceRevision: w.Revision})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := read.Snapshot
	manifest, err := startuppreparation.CompilePreparationManifest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "fixture"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired,
			ProfileContracts:  runtimecontract.CloneRuntimeProfileContracts(snapshot.Candidate.RuntimeProfileContracts),
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
				MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
				CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll,
				ResultPolicy:     runtimecontract.RuntimeAcceptanceSliceResultAllowed,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
					ID: "default", Title: "Default", Scope: "default", Priority: 100, MaxWorkItems: 1,
					Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
				}},
			},
		},
	}
	before, _ := json.Marshal(struct {
		Snapshot   startuppreparation.ExactConfirmedStartupSnapshot
		Manifest   startuppreparation.Manifest
		Activation runtimecontract.RuntimeActivationContext
	}{snapshot, manifest, activation})
	compiled, err := activationcore.CompilePreparedBootstrap(snapshot, manifest, activation)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.CompilerVersion != activationcore.PreparedBootstrapCompilerVersion || compiled.BootstrapHash == "" || compiled.SnapshotHash != snapshot.SnapshotHash || compiled.ManifestHash != manifest.ManifestHash || compiled.Bootstrap.Validate() != nil {
		t.Fatal("pure compilation lost exact binding")
	}
	if compiled.Bootstrap.Definition.SourceObligations == nil || compiled.Bootstrap.Definition.SourceObligations.SpecificationHash != snapshot.AuthoringRef.SpecificationHash || compiled.Bootstrap.Definition.SourceObligations.CandidateHash != snapshot.Candidate.CandidateHash {
		t.Fatal("compiled bootstrap lost original source obligations")
	}
	again, err := activationcore.CompilePreparedBootstrap(snapshot, manifest, activation)
	if err != nil || !reflect.DeepEqual(compiled, again) {
		t.Fatal("pure bootstrap compilation is not stable")
	}
	compiled.Bootstrap.Definition.SourceObligations.Sources[0].Text += "caller mutation"
	if reflect.DeepEqual(compiled, again) {
		t.Fatal("compilation results alias")
	}
	after, _ := json.Marshal(struct {
		Snapshot   startuppreparation.ExactConfirmedStartupSnapshot
		Manifest   startuppreparation.Manifest
		Activation runtimecontract.RuntimeActivationContext
	}{snapshot, manifest, activation})
	if string(before) != string(after) {
		t.Fatal("pure compilation changed input")
	}
	changed := activation.Clone()
	changed.ExecutionContract.ProfileContracts = nil
	if _, err := activationcore.CompilePreparedBootstrap(snapshot, manifest, changed); err == nil {
		t.Fatal("accepted substituted profile")
	}
	if _, err := activationcore.CompilePreparedBootstrap(snapshot, manifest, runtimecontract.RuntimeActivationContext{}); err == nil {
		t.Fatal("accepted missing activation input")
	}
	changedManifest := manifest.Clone()
	changedManifest.Checks = nil
	if _, err := activationcore.CompilePreparedBootstrap(snapshot, changedManifest, activation); err == nil {
		t.Fatal("compiled with missing required checks")
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || !reflect.DeepEqual(stored, w) || stored.ActiveRunID != "" {
		t.Fatal("pure compilation wrote owner state")
	}
}
