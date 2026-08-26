package events

import (
	"errors"
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestEventQueryNormalizedAppliesOwnerBounds(t *testing.T) {
	query, err := (EventQuery{Topic: " /run/state/changed ", ExcludeTopics: []string{" /a ", "/a", ""}}).Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if query.Topic != "/run/state/changed" || query.Limit != DefaultListLimit || len(query.ExcludeTopics) != 1 || query.ExcludeTopics[0] != "/a" {
		t.Fatalf("normalized query = %+v", query)
	}
	for _, limit := range []int{-1, MaxListLimit + 1} {
		if _, err := (EventQuery{Limit: limit}).Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("limit %d err = %v, want invalid argument", limit, err)
		}
	}
}

func TestApprovalResolutionRecordCommandIsEventstreamOwned(t *testing.T) {
	var contract eventcontract.Contract
	for _, candidate := range Contracts() {
		if candidate.Topic == TopicEventStreamCommandRecordApprovalResolution {
			contract = candidate
			break
		}
	}
	if contract.Owner != "eventstream" || !slices.Equal(contract.ReceiverOwners, []string{"eventstream"}) || contract.CommandClass != eventcontract.CommandClassWrite || contract.LanePolicy != eventcontract.LanePolicyState {
		t.Fatalf("approval resolution record contract = %+v", contract)
	}
	var message eventcontract.MessageContract
	for _, candidate := range MessageContracts() {
		if candidate.Topic == TopicEventStreamCommandRecordApprovalResolution {
			message = candidate
			break
		}
	}
	if message.Owner != contract.Owner || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) || message.DefinitionPackage != definitionPackage {
		t.Fatalf("approval resolution record message = %+v", message)
	}
}

func TestApprovalResolutionFactRequiresTerminalOwnerObservation(t *testing.T) {
	valid := ApprovalResolutionFact{
		NotificationID: "notification-1", ResolutionID: "resolution-1", ApprovalID: "approval-1",
		WorkspaceID: "workspace-1", RunID: "run-1", PrincipalKind: "human_operator",
		PrincipalSubjectID: "operator", State: common.ApprovalStateApproved,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid approval resolution fact rejected: %v", err)
	}
	valid.State = common.ApprovalStatePending
	if err := valid.Validate(); err == nil {
		t.Fatal("non-terminal approval resolution fact accepted")
	}
}
