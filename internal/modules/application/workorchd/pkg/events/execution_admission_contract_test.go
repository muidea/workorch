package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
	hubevents "github.com/muidea/workorch/internal/pkg/events"
)

func TestExecutionAdmissionContractKeepsFocusedOwnerAndWorkspaceLane(t *testing.T) {
	const owner = "workorchd.execution_admission"
	var delivery *eventcontract.Contract
	for _, contract := range Contracts() {
		if contract.Topic == TopicWorkorchDCommandWorkspaceRunRequested {
			value := contract
			delivery = &value
			break
		}
	}
	if delivery == nil {
		t.Fatal("execution admission delivery contract is not registered")
	}
	if delivery.Owner != owner || !slices.Equal(delivery.ReceiverOwners, []string{owner}) ||
		delivery.CommandClass != eventcontract.CommandClassWrite || delivery.LanePolicy != eventcontract.LanePolicyState {
		t.Fatalf("execution admission delivery contract = %+v", delivery)
	}
	metadata := delivery.Metadata
	if metadata.StateMachine != eventcontract.StateMachineSM1 || metadata.ReducerOwner != "run" ||
		metadata.MachineRole != eventcontract.StateMachineRoleParticipant ||
		metadata.ScopePolicy != eventcontract.ScopePolicyWorkspaceRun ||
		metadata.IdentityPolicy != eventcontract.IdentityPolicyRuntimeAuthority || !metadata.CanonicalFeedback {
		t.Fatalf("execution admission Runtime metadata = %+v", metadata)
	}

	lane := hubevents.EventHubLaneKeyFor(Contracts(), TopicWorkorchDCommandWorkspaceRunRequested, eventcontract.LaneScope{WorkspaceID: "ws-contract"})
	if lane != "state/workorchd.execution_admission/workspace/ws-contract" {
		t.Fatalf("execution admission workspace lane = %q", lane)
	}
	otherLane := hubevents.EventHubLaneKeyFor(Contracts(), TopicWorkorchDCommandWorkspaceRunRequested, eventcontract.LaneScope{WorkspaceID: "ws-other"})
	if otherLane == lane {
		t.Fatalf("execution admission lanes collide across workspaces: %q", lane)
	}

	for _, message := range MessageContracts() {
		if message.Topic != TopicWorkorchDCommandWorkspaceRunRequested {
			continue
		}
		if message.Owner != owner || !slices.Equal(message.ReceiverOwners, []string{owner}) ||
			message.CommandClass != delivery.CommandClass || message.LanePolicy != delivery.LanePolicy ||
			message.DefinitionPackage != definitionPackage {
			t.Fatalf("execution admission message contract = %+v, delivery = %+v", message, delivery)
		}
		return
	}
	t.Fatal("execution admission message contract is not registered")
}
