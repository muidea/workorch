package events

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLLMOwnerCommandContractsRemainTypedAndLocal(t *testing.T) {
	want := map[string]struct {
		class eventcontract.CommandClass
		lane  eventcontract.LanePolicy
	}{
		TopicLLMCommandGetActiveConfiguration: {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicAcquireProviderCeiling:           {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicPreflightRuntimeCandidate:        {class: eventcontract.CommandClassRead, lane: eventcontract.LanePolicyQuery},
		TopicRequestRuntimeCandidate:          {class: eventcontract.CommandClassExecute, lane: eventcontract.LanePolicyExecute},
	}

	contracts := make(map[string]eventcontract.Contract, len(Contracts()))
	for _, contract := range Contracts() {
		contracts[contract.Topic] = contract
	}
	messages := make(map[string]eventcontract.MessageContract, len(MessageContracts()))
	for _, contract := range MessageContracts() {
		messages[contract.Topic] = contract
	}

	for topic, expected := range want {
		contract, ok := contracts[topic]
		if !ok {
			t.Fatalf("LLM owner contract %q is not registered", topic)
		}
		if contract.Owner != "llm" || !slices.Equal(contract.ReceiverOwners, []string{"llm"}) || contract.CommandClass != expected.class || contract.LanePolicy != expected.lane {
			t.Fatalf("LLM owner contract %q = %+v", topic, contract)
		}
		message, ok := messages[topic]
		if !ok {
			t.Fatalf("LLM owner message contract %q is not registered", topic)
		}
		if message.Owner != contract.Owner || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) || message.CommandClass != contract.CommandClass || message.LanePolicy != contract.LanePolicy || message.DefinitionPackage != definitionPackage {
			t.Fatalf("LLM owner message contract %q = %+v, event contract = %+v", topic, message, contract)
		}
	}
}

func TestValidateRuntimeProviderRequestFeedbackOwnsClosedCategoryClassMapping(t *testing.T) {
	tests := []struct {
		name     string
		category runtimecontract.RuntimeFeedbackCategory
		class    runtimecontract.RuntimeCandidateFailureClass
	}{
		{name: "authentication", category: runtimecontract.RuntimeFeedbackCategoryProviderAuthentication, class: runtimecontract.RuntimeCandidateFailureClassProviderAuthentication},
		{name: "configuration", category: runtimecontract.RuntimeFeedbackCategoryProviderConfiguration, class: runtimecontract.RuntimeCandidateFailureClassProviderConfiguration},
		{name: "transient", category: runtimecontract.RuntimeFeedbackCategoryProviderTransient, class: runtimecontract.RuntimeCandidateFailureClassProviderTransient},
		{name: "explicit protocol", category: runtimecontract.RuntimeFeedbackCategoryProviderProtocol, class: runtimecontract.RuntimeCandidateFailureClassProviderProtocol},
		{name: "generic provider protocol", category: runtimecontract.RuntimeFeedbackCategoryProviderProtocol, class: runtimecontract.RuntimeCandidateFailureClassProvider},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			feedback, err := runtimecontract.RuntimeFeedbackFromCandidateFailure(
				runtimecontract.RuntimeFeedbackScope{WorkspaceID: "workspace-1", RunID: "run-1"},
				runtimecontract.RuntimeFeedbackDomainProviderRequest,
				tc.category,
				runtimecontract.RuntimeModuleLLM,
				runtimecontract.RuntimeCandidateFailure{Class: tc.class, Code: "provider-failure", Signature: "provider-failure-1", Reason: "safe provider diagnostic"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateRuntimeProviderRequestFeedback(feedback); err != nil {
				t.Fatalf("valid owner feedback rejected: %v", err)
			}
			mismatch := feedback.CloneOriginal()
			mismatch.StableCategory = runtimecontract.RuntimeFeedbackCategoryProviderAuthentication
			if mismatch.StableCategory == tc.category {
				mismatch.StableCategory = runtimecontract.RuntimeFeedbackCategoryProviderTransient
			}
			if err := ValidateRuntimeProviderRequestFeedback(mismatch); err == nil {
				t.Fatal("category/class mismatch was accepted")
			}
		})
	}
}
