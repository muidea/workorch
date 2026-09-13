package activationports

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func executionLimitsInput(calls, operations int) map[string]any {
	return map[string]any{"executionLimits": map[string]any{"protocolRef": runtimecontract.RuntimeExecutionLimitsV1, "maxProviderCalls": calls, "maxCapabilityOperations": operations}}
}

func executionLimitsPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	value, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestExecutionLimitsRejectInvalidSourcesBeforeMergeAndFinalActivation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"null", func(m map[string]any) { m["executionLimits"] = nil }},
		{"zero durable dispatches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = 0 }},
		{"negative durable dispatches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = -1 }},
		{"fraction durable dispatches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = 1.5 }},
		{"null durable dispatches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = nil }},
		{"string durable dispatches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = "2" }},
		{"overflow durable dispatches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = 8193 }},
		{"list", func(m map[string]any) { m["executionLimits"] = []any{} }},
		{"unknown", func(m map[string]any) { m["executionLimits"].(map[string]any)["nextAction"] = "execute" }},
		{"missing", func(m map[string]any) { delete(m["executionLimits"].(map[string]any), "maxProviderCalls") }},
		{"string", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProviderCalls"] = "3" }},
		{"fraction", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProviderCalls"] = 1.5 }},
		{"zero", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProviderCalls"] = 0 }},
		{"overflow", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxCapabilityOperations"] = 8193 }},
		{"protocol", func(m map[string]any) { m["executionLimits"].(map[string]any)["protocolRef"] = "unknown" }},
		{"zero proof batches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProofBatchesPerCWI"] = 0 }},
		{"fraction proof batches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProofBatchesPerCWI"] = 1.5 }},
		{"null proof batches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProofBatchesPerCWI"] = nil }},
		{"string proof batches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProofBatchesPerCWI"] = "2" }},
		{"overflow proof batches", func(m map[string]any) { m["executionLimits"].(map[string]any)["maxProofBatchesPerCWI"] = 18 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := executionLimitsInput(3, 2)
			tc.edit(input)
			if err := validateRuntimePresealTargetDeclarations(input, nil); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("invalid input accepted: %v", err)
			}
			if _, err := RuntimeActivationContext(runtimecontract.ProviderInferenceSelection{}, "", nil, nil, appmodelcore.SoulRouting{}, nil, nil, nil, nil, nil, nil, input, runtimecontract.RuntimeContinuationContext{}); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("final activation accepted: %v", err)
			}
			for _, templateInvalid := range []bool{false, true} {
				valid := executionLimitsInput(4, 4)
				th, wh := valid, input
				if templateInvalid {
					th, wh = input, valid
				}
				template := &workspacemodel.WorkspaceTemplate{ContextHints: executionLimitsPayload(t, th)}
				workspace := &workspacemodel.Workspace{ContextHints: executionLimitsPayload(t, wh)}
				beforeT, _ := json.Marshal(template.ContextHints)
				beforeW, _ := json.Marshal(workspace.ContextHints)
				if _, _, _, _, _, err := definitionAdmissionProfileBindings(workspace, template); !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("invalid source hidden by merge: %v", err)
				}
				afterT, _ := json.Marshal(template.ContextHints)
				afterW, _ := json.Marshal(workspace.ContextHints)
				if string(beforeT) != string(afterT) || string(beforeW) != string(afterW) {
					t.Fatal("validation changed sources")
				}
			}
		})
	}
}

func TestExecutionLimitsAdmissionSealAndTemplatePrecedence(t *testing.T) {
	for _, templatePresent := range []bool{false, true} {
		workspace := &workspacemodel.Workspace{ID: "ws-limits", TemplateID: "template-limits", DeliverableDirectory: "/workspace", ContextHints: executionLimitsPayload(t, executionLimitsInput(5, 4))}
		template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "limits"}
		want := &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 5, MaxCapabilityOperations: 4}
		if templatePresent {
			template.ContextHints = executionLimitsPayload(t, executionLimitsInput(3, 2))
			want.MaxProviderCalls = 3
			want.MaxCapabilityOperations = 2
		}
		input := workspace.ContextHints.Map()
		input["executionLimits"].(map[string]any)["maxProofBatchesPerCWI"] = 4
		input["executionLimits"].(map[string]any)["maxDurableDispatchesPerCWI"] = 3
		workspace.ContextHints = executionLimitsPayload(t, input)
		if !templatePresent {
			want.MaxProofBatchesPerCWI = 4
			want.MaxDurableDispatchesPerCWI = 3
		}
		goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: workspace.ID, Text: "goal", DefinitionRevision: "goal-r1"}
		dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: workspace.ID, Items: []string{"done"}, DefinitionRevision: "dod-r1"}
		candidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
		if err != nil {
			t.Fatal(err)
		}
		effective := activationcore.MergeTemplateWorkspaceContextHintsForExecutionContract(template.ContextHints.Map(), workspace.ContextHints.Map())
		contract := runtimeExecutionContract(effective)
		if !runtimecontract.SameRuntimeExecutionLimits(contract.ExecutionLimits, want) {
			t.Fatal("template precedence/typed decode drift")
		}
		if err := validateRuntimeExecutionLimitsStartupSeal(candidate, want); err != nil {
			t.Fatal(err)
		}
		startup := startupInputForActivationPortTest(candidate)
		workspace.StartupInput = &startup
		workspace.ActivationAdmission = &workspacemodel.WorkspaceActivationAdmission{
			WorkspaceID: workspace.ID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
			DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
		}
		bootstrap := &runtimecontract.RuntimeBootstrap{
			Definition: runtimecontract.RuntimeDefinitionContext{Revision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash},
			Activation: runtimecontract.RuntimeActivationContext{ExecutionContract: contract},
		}
		if err := validateRuntimeBootstrapStartupSeal(workspace, template, candidate, bootstrap); err != nil {
			t.Fatalf("final bootstrap rejected admitted limits: %v", err)
		}
		drifted := bootstrap.Clone()
		drifted.Activation.ExecutionContract.ExecutionLimits.MaxProviderCalls++
		if validateRuntimeBootstrapStartupSeal(workspace, template, candidate, &drifted) == nil {
			t.Fatal("final bootstrap accepted limits absent from admission")
		}
		drifted = bootstrap.Clone()
		drifted.Activation.ExecutionContract.ExecutionLimits.MaxProofBatchesPerCWI++
		if validateRuntimeBootstrapStartupSeal(workspace, template, candidate, &drifted) == nil {
			t.Fatal("final bootstrap accepted proof budget drift")
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		var restored workspacemodel.DefinitionAdmissionCandidate
		if err := json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		if err := validateRuntimeExecutionLimitsStartupSeal(restored, want); err != nil {
			t.Fatal(err)
		}
		changed := *want
		changed.MaxProviderCalls++
		if validateRuntimeExecutionLimitsStartupSeal(restored, &changed) == nil || validateRuntimeExecutionLimitsStartupSeal(restored, nil) == nil {
			t.Fatal("changed/removed limits accepted")
		}
		restored.AdmissionPolicyBindings = append(restored.AdmissionPolicyBindings, runtimeExecutionLimitsAdmissionBinding(*want))
		if validateRuntimeExecutionLimitsStartupSeal(restored, want) == nil {
			t.Fatal("duplicate limit seal accepted")
		}
		if validateRuntimeExecutionLimitsStartupSeal(workspacemodel.DefinitionAdmissionCandidate{}, want) == nil {
			t.Fatal("unadmitted limits accepted")
		}
		if !templatePresent {
			workspace.ContextHints = executionLimitsPayload(t, executionLimitsInput(6, 4))
			newCandidate, err := definitionAdmissionCandidate(workspace, goal, dod, template)
			if err != nil || newCandidate.CandidateHash == candidate.CandidateHash {
				t.Fatal("limits absent from candidate identity")
			}
		}
	}
}
