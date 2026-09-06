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
	// The unused generic Run Create command was removed with the Orchestrator
	// lifecycle mutation facade. Run creation now has one production path: the
	// Run owner's typed workspace-execution intent reducer. Decision's separate
	// Ticket/Resolution delete topics were also replaced by one workspace-scoped
	// aggregate purge topic, reducing the inventory by one without weakening an
	// owner interaction. Approval's Runtime closure adds four adjudicated topics:
	// capability acquisition, request creation, Orchestrator SM-2 commit and the
	// durable Eventstream resolution record handshake. MCP Registry adds two
	// bounded concrete reads so activation and execution consume one
	// revision-consistent selection/full aggregate instead of joining lists.
	// Definition admission has one Workspace-owner intent. Dedicated
	// clarification has five Decision-owner write commands (static open/
	// reconcile/admission-resolution and Runtime open/replacement-resolution)
	// plus one Decision-owned persisted-fact query used by Workspace before its
	// private reducer/CAS. These contracts replace implicit setup/governance
	// mutations without accepting caller-authored provenance.
	// Run's retired Runtime-activation compatibility notification is absent:
	// current-format Runtime state is loaded directly or rejected by its owner.
	// Shell adds one bounded, pure prospective-satisfaction query so SM-2 can
	// prove an unexecuted File manifest delta would unlock the required Shell
	// validation surface without crossing owner state or performing an effect.
	// Memory removes the unused single RecallTrace get command; all production
	// consumers use the bounded workspace/run-scoped list contract instead.
	// The receiver-only Run delete/purge-runtime-resources, Orchestrator
	// purge-run, Result/Artifact delete and RecallTrace delete mutations are
	// absent. Workspace deletion now has one durable coordinator and invokes
	// only each owner's workspace-scoped seal/purge contract. Two-phase Runtime
	// retirement adds Orchestrator Begin/Confirm quiescence and Run Finalize;
	// each carries immutable facts while the respective owner-private reducer
	// remains the sole transition authority.
	// Orchestrator's generic Runtime governance-resolution command is absent:
	// the concrete Decision/Approval result adapters now reload their exact
	// persisted binding before invoking the owner-private SM-2 transition.
	// File adds one SM-3 read/query topic for manifest-declared operation
	// candidate conformance. It returns an owner fact only; the Orchestrator
	// private reducer remains the sole candidate/retry transition authority.
	// Soul and Workspace each add an independent builtin preview query and
	// guarded upgrade command. Only their private owner transactions may write;
	// no cross-owner upgrade state machine or implicit activation is introduced.
	const expectedProductionTopicCount = 287
	if len(contracts) != expectedProductionTopicCount {
		t.Fatalf("production EventHub topic inventory = %d, want %d; adjudicate additions/removals in the owning component contracts and README", len(contracts), expectedProductionTopicCount)
	}
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
		if isWorkspaceRetirementResourceTopic(topic) {
			if contract.Metadata.StateMachine != eventcontract.StateMachineNone || contract.Metadata.MachineRole != eventcontract.StateMachineRoleNone ||
				contract.Metadata.EffectClass != eventcontract.EffectClassOwnerState || contract.Metadata.TransitionTarget != eventcontract.TransitionTargetReceiverOwnerPrivate {
				t.Fatalf("Workspace retirement resource topic %s is mislabeled as a Runtime state machine: %+v", topic, contract.Metadata)
			}
			continue
		}
		if strings.Contains(strings.ToLower(topic), "runtime") {
			opaquePersistence := contract.Metadata.StateMachine == eventcontract.StateMachineNone && contract.Metadata.EffectClass == eventcontract.EffectClassOpaquePersistence && contract.Metadata.TransitionTarget == eventcontract.TransitionTargetOpaquePersistence
			if (!opaquePersistence && contract.Metadata.StateMachine == eventcontract.StateMachineNone) || contract.Metadata.ScopePolicy == eventcontract.ScopePolicyDeclared || contract.Metadata.IdentityPolicy == eventcontract.IdentityPolicyTopicOwnerReceiver {
				t.Fatalf("Runtime-sensitive topic %s lacks explicit SM/scope/identity metadata: %+v", topic, contract.Metadata)
			}
			if opaquePersistence {
				if contract.Metadata.ReducerOwner != "orchestrator" || contract.Metadata.MachineRole != eventcontract.StateMachineRoleOpaquePersistence {
					t.Fatalf("Runtime opaque persistence topic %s has ambiguous reducer ownership: %+v", topic, contract.Metadata)
				}
			} else if contract.Metadata.ReducerOwner != eventcontract.ReducerOwnerForStateMachine(contract.Metadata.StateMachine) || contract.Metadata.MachineRole == eventcontract.StateMachineRoleNone {
				t.Fatalf("Runtime-sensitive topic %s has ambiguous reducer ownership: %+v", topic, contract.Metadata)
			}
		}
	}
}

func isWorkspaceRetirementResourceTopic(topic string) bool {
	switch topic {
	case orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
		orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence,
		runevents.TopicRunCommandPrepareWorkspaceRetirement,
		runevents.TopicRunCommandFinalizeWorkspaceRetirement,
		runevents.TopicRunCommandPurgeWorkspace:
		return true
	default:
		return false
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

func TestContractOwnerIdentityMapsToOneDefinitionRoot(t *testing.T) {
	_, messages := testContractCatalog(t)
	ownerRoots := map[string]string{}
	for topic, message := range messages {
		owner := strings.TrimSpace(message.Owner)
		root := strings.TrimSuffix(strings.TrimSpace(message.DefinitionPackage), "/pkg/events")
		if owner == "" || root == "" {
			t.Fatalf("topic %s has incomplete owner identity: %+v", topic, message)
		}
		if previous, exists := ownerRoots[owner]; exists && previous != root {
			t.Fatalf("contract owner %q maps to multiple component roots: %s and %s (topic %s)", owner, previous, root, topic)
		}
		ownerRoots[owner] = root
	}
	if root := ownerRoots["mcp"]; root != "github.com/muidea/workorch/internal/modules/blocks/mcp" {
		t.Fatalf("MCP capability owner root = %q", root)
	}
	if root := ownerRoots["mcp-registry"]; root != "github.com/muidea/workorch/internal/modules/blocks/mcpregistry" {
		t.Fatalf("MCP Registry owner root = %q", root)
	}
}

func TestMCPRegistryAndCapabilityLaneIdentitiesStayDistinct(t *testing.T) {
	contracts, _ := testContractCatalog(t)
	registry := contracts[mcpregistryevents.TopicMCPCommandCreateServer]
	if registry.Owner != "mcp-registry" || !slices.Equal(registry.ReceiverOwners, []string{"mcp-registry"}) || registry.Metadata.LaneIdentity != "mcp-registry" {
		t.Fatalf("MCP Registry contract identity = %+v", registry)
	}
	capability := contracts[mcpexecuteevents.TopicExecuteOperation]
	if capability.Owner != "mcp" || !slices.Equal(capability.ReceiverOwners, []string{"mcp"}) || capability.Metadata.LaneIdentity != "mcp" {
		t.Fatalf("MCP capability execute identity changed: %+v", capability)
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
