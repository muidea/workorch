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
