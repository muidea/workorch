package events

import (
	"slices"
	"sort"
	"strings"
	"testing"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	mcpexecuteevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpregistryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestOwnerEventContractsAreCompleteAndUnique(t *testing.T) {
	contracts, messages := testContractCatalog(t)
	if len(contracts) != len(messages) {
		t.Fatalf("delivery contract count %d differs from message contract count %d", len(contracts), len(messages))
	}
	topics := make([]string, 0, len(contracts))
	for topic := range contracts {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	for _, topic := range topics {
		contract := contracts[topic]
		message, ok := messages[topic]
		if !ok {
			t.Fatalf("topic %s has delivery contract but no message contract", topic)
		}
		if err := eventcontract.Validate(contract); err != nil {
			t.Fatalf("topic %s contract is invalid: %v", topic, err)
		}
		if strings.TrimSpace(message.DefinitionPackage) == "" {
			t.Fatalf("topic %s message contract has empty definition package", topic)
		}
		if message.Owner != contract.Owner || message.CommandClass != contract.CommandClass || message.LanePolicy != contract.LanePolicy || !slices.Equal(message.ReceiverOwners, contract.ReceiverOwners) {
			t.Fatalf("topic %s message/delivery metadata mismatch: message=%+v delivery=%+v", topic, message, contract)
		}
		if strings.Contains(strings.ToLower(topic), "runtime") {
			if contract.Metadata.StateMachine == eventcontract.StateMachineNone || contract.Metadata.ScopePolicy == eventcontract.ScopePolicyDeclared || contract.Metadata.IdentityPolicy == eventcontract.IdentityPolicyTopicOwnerReceiver {
				t.Fatalf("Runtime-sensitive topic %s lacks explicit SM/scope/identity metadata: %+v", topic, contract.Metadata)
			}
		}
	}
}

func TestRuntimeCapabilityExecutionContractsDeclareActualReceivers(t *testing.T) {
	contracts, _ := testContractCatalog(t)
	expected := map[string]string{
		fileevents.TopicExecuteFileListOperation: "file", fileevents.TopicExecuteFileReadOperation: "file",
		fileevents.TopicExecuteFileEditOperation: "file", fileevents.TopicExecuteFileWriteOperation: "file",
		fileevents.TopicExecutePatchApplyOperation: "file", fileevents.TopicExecutePatchPreviewOperation: "file",
		shellevents.TopicExecuteOperation: "shell", gitevents.TopicExecuteOperation: "git",
		mcpexecuteevents.TopicExecuteOperation: "mcp", sshevents.TopicExecuteOperation: "ssh", webevents.TopicExecuteOperation: "web",
	}
	for topic, receiver := range expected {
		contract, ok := contracts[topic]
		if !ok || !slices.Equal(contract.ReceiverOwners, []string{receiver}) {
			t.Fatalf("runtime execution topic %s contract = %+v, want receiver %s", topic, contract, receiver)
		}
	}
}

func testContractCatalog(t *testing.T) (map[string]eventcontract.Contract, map[string]eventcontract.MessageContract) {
	t.Helper()
	contractSources := [][]eventcontract.Contract{
		workorchdevents.Contracts(), approvalevents.Contracts(), decisionevents.Contracts(), eventstreamevents.Contracts(),
		fileevents.Contracts(), gitevents.Contracts(), llmevents.Contracts(), mcpexecuteevents.Contracts(), mcpregistryevents.Contracts(),
		memoryevents.Contracts(), resultevents.Contracts(), runevents.Contracts(), shellevents.Contracts(), skillevents.Contracts(),
		soulevents.Contracts(), sshevents.Contracts(), webevents.Contracts(), workspaceevents.Contracts(), governanceevents.Contracts(), orchestratorevents.Contracts(),
	}
	messageSources := [][]eventcontract.MessageContract{
		workorchdevents.MessageContracts(), approvalevents.MessageContracts(), decisionevents.MessageContracts(), eventstreamevents.MessageContracts(),
		fileevents.MessageContracts(), gitevents.MessageContracts(), llmevents.MessageContracts(), mcpexecuteevents.MessageContracts(), mcpregistryevents.MessageContracts(),
		memoryevents.MessageContracts(), resultevents.MessageContracts(), runevents.MessageContracts(), shellevents.MessageContracts(), skillevents.MessageContracts(),
		soulevents.MessageContracts(), sshevents.MessageContracts(), webevents.MessageContracts(), workspaceevents.MessageContracts(), governanceevents.MessageContracts(), orchestratorevents.MessageContracts(),
	}
	contracts := map[string]eventcontract.Contract{}
	for _, source := range contractSources {
		for _, contract := range source {
			if _, exists := contracts[contract.Topic]; exists {
				t.Fatalf("duplicate delivery contract topic %s", contract.Topic)
			}
			contracts[contract.Topic] = contract
		}
	}
	messages := map[string]eventcontract.MessageContract{}
	for _, source := range messageSources {
		for _, message := range source {
			if _, exists := messages[message.Topic]; exists {
				t.Fatalf("duplicate message contract topic %s", message.Topic)
			}
			messages[message.Topic] = message
		}
	}
	return contracts, messages
}
