package activationports

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeBootstrapStartupSealRejectsCoordinatedSourceOmission(t *testing.T) {
	template := softwareDevProfileTemplateForTest(t)
	workspace := &workspacemodel.Workspace{ID: "ws-source-seal", TemplateID: template.ID, DeliverableDirectory: t.TempDir(), EvidencePolicies: workorcheventpayload.CloneSlice(template.EvidencePolicies)}
	goal := &workspacemodel.Goal{ID: "goal-source", WorkspaceID: workspace.ID, Text: "完整目标", DefinitionRevision: "goal-r1"}
	dod := &workspacemodel.DoD{ID: "dod-source", WorkspaceID: workspace.ID, Items: append([]string{"共同判据"}, template.DoDItems...), DefinitionRevision: "dod-r1"}
	candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
	if err != nil {
		t.Fatal(err)
	}
	spec := workspacemodel.GoalSpecification{Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: candidate.GoalInput.CandidateItemRef, GoalTextHash: workspacemodel.StableDefinitionTextHash(goal.Text), Dimensions: []workspacemodel.GoalSpecificationDimension{{ID: "all", Required: true, Activated: true}}}
	for n, item := range candidate.DoDInputs {
		id := fmt.Sprintf("req-%03d", n+1)
		spec.Requirements = append(spec.Requirements, workspacemodel.GoalSpecificationRequirement{ID: id, DimensionID: "all", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		spec.Obligations = append(spec.Obligations, workspacemodel.GoalSpecificationObligation{ID: "obl-" + id, RequirementID: id, DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		spec.Verifications = append(spec.Verifications, workspacemodel.GoalSpecificationVerification{ID: "ver-" + id, ObligationID: "obl-" + id, Declaration: "验证该源原判据", Status: workspacemodel.GoalVerificationDeclared, ProfileID: "software-dev", EvidenceRequirementID: "*", Scope: "dodItem"})
	}
	second := spec.Obligations[0]
	second.ID, second.Text = "shared-B", " B 独立原判据\r\n "
	spec.Obligations = append(spec.Obligations, second)
	verification := spec.Verifications[0]
	verification.ID, verification.ObligationID = "shared-B-verification", second.ID
	spec.Verifications = append(spec.Verifications, verification)
	spec.EvidencePurposeMappings = requiredPurposeMappingsFixture(t, candidate)
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		ids, err := spec.RequiredReviewRequirementIDs(kind)
		if err != nil {
			t.Fatal(err)
		}
		spec.Reviews = append(spec.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewPass, SpecificationHash: spec.ContentHash(), Reason: "独立夹具审查", EvidenceRefs: []string{candidate.GoalInput.CandidateItemRef}, RequirementIDs: ids})
	}
	candidate.Specification = &spec
	candidate = candidate.WithHash()
	startup := startupInputForActivationPortTest(candidate)
	startup.SpecificationHash = spec.AdmissionHash()
	startup = startup.WithHash()
	admission := workspacemodel.WorkspaceActivationAdmission{WorkspaceID: workspace.ID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash, DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible}
	workspace.StartupInput, workspace.ActivationAdmission = &startup, &admission
	inventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil {
		t.Fatal(err)
	}
	execution := runtimeExecutionContract(template.ContextHints.Map())
	execution.ProfileContracts = candidate.RuntimeProfileContracts
	policies, err := compileRuntimeEvidencePolicies("Workspace "+string(workspace.ID), workspace.EvidencePolicies)
	if err != nil {
		t.Fatal(err)
	}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}}, ExecutionContract: execution, EvidencePolicies: policies}
	compile := func(value *runtimecontract.RuntimeSourceObligationInventory) *runtimecontract.RuntimeBootstrap {
		t.Helper()
		bootstrap, err := activationcore.RuntimeBootstrapWithSourceObligations(goal, dod, startup.DefinitionRevision, startup.StartupInputHash, nil, activation, value)
		if err != nil {
			t.Fatalf("反例必须先通过完整 bootstrap 校验: %v", err)
		}
		return bootstrap
	}
	original := compile(inventory)
	if err := validateRuntimeBootstrapStartupSeal(workspace, template, candidate, original); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"proof method", "deliverable description", "multiple Profiles"} {
		t.Run(change, func(t *testing.T) {
			current := activation.Clone()
			switch change {
			case "proof method":
				current.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].Description += " current implementation"
			case "deliverable description":
				for n := range current.ExecutionContract.ProfileContracts[0].DeliverableExpectations {
					current.ExecutionContract.ProfileContracts[0].DeliverableExpectations[n].Description += " current description"
				}
			case "multiple Profiles":
				current.ExecutionContract.ProfileContracts = append(current.ExecutionContract.ProfileContracts, runtimecontract.RuntimeProfileContract{ID: "independent"})
			}
			bootstrap, err := activationcore.RuntimeBootstrapWithSourceObligations(goal, dod, startup.DefinitionRevision, startup.StartupInputHash, nil, current, inventory)
			if err != nil {
				t.Fatalf("full bootstrap with original required source and current Profile: %v", err)
			}
			if !reflect.DeepEqual(inventory, bootstrap.Definition.SourceObligations) || !reflect.DeepEqual(inventory, bootstrap.DefinitionSeed.SourceObligations) {
				t.Fatal("bootstrap changed original inventory")
			}
			for _, mapping := range inventory.EvidencePurposeMappings {
				if mapping.ContractRef != runtimecontract.RuntimeProfileDeliverableEvidencePurpose {
					continue
				}
				found := false
				for _, item := range bootstrap.CoverageItems {
					if item.SourceScope != "deliverable" || item.SourceID != mapping.HistoricalDeliverableID {
						continue
					}
					found = true
					if item.TargetText != strings.TrimSpace(mapping.HistoricalText) || item.HistoricalAcceptance == nil {
						t.Fatal("current description replaced original coverage endpoint")
					}
					endpoints, err := runtimecontract.ResolveRuntimeEvidencePurposeEndpoints(bootstrap.Definition, bootstrap.Activation.ExecutionContract, bootstrap.DefinitionSeed, item.ID)
					if err != nil || endpoints.Mapping != mapping {
						t.Fatalf("semantic endpoint lost original source: %v", err)
					}
				}
				if !found {
					t.Fatal("required source missing from coverage")
				}
			}
			raw, err := json.Marshal(bootstrap)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded runtimecontract.RuntimeBootstrap
			if err := json.Unmarshal(raw, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err != nil {
				t.Fatal(err)
			}
			facts, err := reloaded.InitialFacts(workspace.ID, "current-run")
			if err != nil {
				t.Fatal(err)
			}
			raw, err = json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			var fresh runtimecontract.RuntimeFacts
			if err := json.Unmarshal(raw, &fresh); err != nil {
				t.Fatal(err)
			}
			if err := fresh.Validate(); err != nil {
				t.Fatalf("fresh facts rejected unchanged original source: %v", err)
			}
			if !reflect.DeepEqual(original.DefinitionSeed.SourceObligations, reloaded.DefinitionSeed.SourceObligations) {
				t.Fatal("reload changed original history")
			}
		})
	}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coordinated delete B", "remove inventory", "coordinated historical text", "coordinated historical hash"} {
		t.Run(name, func(t *testing.T) {
			if name == "remove inventory" {
				if _, err := activationcore.RuntimeBootstrapWithSourceObligations(goal, dod, startup.DefinitionRevision, startup.StartupInputHash, nil, activation, nil); err == nil {
					t.Fatal("required Profile source missing inventory accepted at bootstrap")
				}
				return
			}
			var forged *runtimecontract.RuntimeSourceObligationInventory
			if name == "coordinated delete B" {
				value := inventory.Clone()
				for n, obligation := range value.Obligations {
					if obligation.ID == second.ID {
						value.Obligations = append(value.Obligations[:n], value.Obligations[n+1:]...)
						break
					}
				}
				for n, check := range value.Verifications {
					if check.ID == verification.ID {
						value.Verifications = append(value.Verifications[:n], value.Verifications[n+1:]...)
						break
					}
				}
				value = value.WithHash()
				forged = &value
			}
			if name == "coordinated historical text" || name == "coordinated historical hash" {
				value := inventory.Clone()
				if name == "coordinated historical text" {
					value.EvidencePurposeMappings[0].HistoricalText = "forged original obligation"
				} else {
					value.EvidencePurposeMappings[0].HistoricalProfileHash = "runtime-profile-contracts:candidate-prefix-01234567890123456789"
				}
				value = value.WithHash()
				forged = &value
			}
			bootstrap := compile(forged)
			encoded, err := json.Marshal(bootstrap)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded runtimecontract.RuntimeBootstrap
			if err := json.Unmarshal(encoded, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err != nil {
				t.Fatalf("反例必须保持 reload bootstrap 自洽: %v", err)
			}
			err = validateRuntimeBootstrapStartupSeal(workspace, template, candidate, &reloaded)
			if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), "source obligations drifted") {
				t.Fatalf("最终 candidate 前像未拒绝协调遗漏: %v", err)
			}
		})
	}
	after, err := json.Marshal(original)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("来源反证改写原始 bootstrap")
	}
}
