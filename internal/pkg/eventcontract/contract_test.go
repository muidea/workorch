package eventcontract

import "testing"

func TestBuildAssignsCompleteOwnerEventHubMetadata(t *testing.T) {
	classes := []CommandClass{CommandClassRead, CommandClassWrite, CommandClassExecute, CommandClassNotify, CommandClassFacade}
	for _, class := range classes {
		contracts := Build("owner", class, LanePolicyState, "/topic/"+string(class))
		if len(contracts) != 1 {
			t.Fatalf("class %s contract count = %d, want 1", class, len(contracts))
		}
		if err := Validate(contracts[0]); err != nil {
			t.Fatalf("class %s default contract is invalid: %v", class, err)
		}
	}
}

func TestValidateRejectsCrossLayerAndReceiverShape(t *testing.T) {
	command := Build("owner", CommandClassWrite, LanePolicyState, "/command")[0]
	command.ReceiverOwners = nil
	if err := Validate(command); err == nil {
		t.Fatal("command without receiver owner was accepted")
	}

	notification := Build("owner", CommandClassNotify, LanePolicyState, "/notification")[0]
	notification.ReceiverOwners = []string{"owner"}
	if err := Validate(notification); err == nil {
		t.Fatal("notification with receiver owner was accepted")
	}

	crossLayer := Build("owner", CommandClassRead, LanePolicyQuery, "/cross-layer")[0]
	crossLayer.Metadata.Layer = LayerOwnerNeutralPureValue
	if err := Validate(crossLayer); err == nil {
		t.Fatal("owner-neutral value registered as EventHub contract was accepted")
	}
}

func TestRuntimeMetadataSeparatesOwnerIngressFromParticipant(t *testing.T) {
	participant := Build("llm", CommandClassExecute, LanePolicyExecute, "/llm/runtime-candidate/request")
	participant = Annotate(participant, RuntimeMetadata(CommandClassExecute, StateMachineSM3, ScopePolicyWorkspaceRun, IdentityPolicyRuntimeAuthority, true), "/llm/runtime-candidate/request")
	if got := participant[0].Metadata; got.ReducerOwner != "orchestrator" || got.MachineRole != StateMachineRoleParticipant {
		t.Fatalf("LLM SM-3 metadata = %+v, want orchestrator participant", got)
	}
	if err := Validate(participant[0]); err != nil {
		t.Fatalf("participant contract: %v", err)
	}

	ingress := Build("run", CommandClassWrite, LanePolicyState, "/run/workspace-execution-intent")
	ingress = Annotate(ingress, RuntimeMetadata(CommandClassWrite, StateMachineSM1, ScopePolicyWorkspaceRun, IdentityPolicyRuntimeAuthority, true), "/run/workspace-execution-intent")
	if got := ingress[0].Metadata; got.ReducerOwner != "run" || got.MachineRole != StateMachineRoleOwnerIngress {
		t.Fatalf("Run SM-1 metadata = %+v, want Run owner ingress", got)
	}
	if err := Validate(ingress[0]); err != nil {
		t.Fatalf("owner ingress contract: %v", err)
	}
}

func TestValidateRejectsRuntimeReducerOwnerDrift(t *testing.T) {
	contracts := Build("llm", CommandClassExecute, LanePolicyExecute, "/llm/runtime-candidate/request")
	contracts = Annotate(contracts, RuntimeMetadata(CommandClassExecute, StateMachineSM3, ScopePolicyWorkspaceRun, IdentityPolicyRuntimeAuthority, true), "/llm/runtime-candidate/request")
	contract := contracts[0]
	contract.Metadata.ReducerOwner = "llm"
	contract.Metadata.MachineRole = StateMachineRoleOwnerIngress
	if err := Validate(contract); err == nil {
		t.Fatal("LLM was accepted as the SM-3 reducer owner")
	}
}
