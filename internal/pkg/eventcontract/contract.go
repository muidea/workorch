package eventcontract

import (
	"fmt"
	"strings"
)

// Layer is the architectural layer occupied by a contract. EventHub topics
// always belong to the receiving owner's wire-contract layer; private reducer
// facts are classified inside their owner and owner-neutral values are guarded
// by their package boundary. Neither becomes a shared production registry or
// an EventHub topic.
type Layer string

const (
	LayerOwnerEventHub           Layer = "owner_eventhub_contract"
	LayerOrchestratorPrivateFact Layer = "orchestrator_private_transition_fact"
	LayerOwnerNeutralPureValue   Layer = "owner_neutral_pure_value"
)

type StateMachine string

const (
	StateMachineNone StateMachine = "none"
	StateMachineSM1  StateMachine = "sm_1"
	StateMachineSM2  StateMachine = "sm_2"
	StateMachineSM3  StateMachine = "sm_3"
	StateMachineSM4  StateMachine = "sm_4"
)

// StateMachineRole distinguishes participation in a coordinated Runtime
// machine from authority to reduce it. EventHub envelopes are never reducer
// calls: an owner ingress must first translate the envelope into an
// owner-private internal event.
type StateMachineRole string

const (
	StateMachineRoleNone                 StateMachineRole = "none"
	StateMachineRoleParticipant          StateMachineRole = "participant"
	StateMachineRoleOwnerIngress         StateMachineRole = "owner_ingress"
	StateMachineRoleOwnerObservation     StateMachineRole = "owner_observation"
	StateMachineRoleOwnerInternalReducer StateMachineRole = "owner_internal_reducer"
	StateMachineRoleOpaquePersistence    StateMachineRole = "opaque_persistence"
)

// EffectClass describes what the receiver may do while handling the wire
// contract. It is metadata for validation and audit only; it never selects a
// Runtime transition or retry action.
type EffectClass string

const (
	EffectClassReadOnly           EffectClass = "read_only"
	EffectClassOwnerState         EffectClass = "owner_state"
	EffectClassExternalSideEffect EffectClass = "external_side_effect"
	EffectClassPublishedFact      EffectClass = "published_fact"
	EffectClassReceiverDefined    EffectClass = "receiver_defined"
	EffectClassOpaquePersistence  EffectClass = "opaque_persistence"
)

type SourcePolicy string

const (
	SourcePolicyCallerContext SourcePolicy = "caller_context"
	SourcePolicyOwnerFact     SourcePolicy = "owner_fact"
)

type ScopePolicy string

const (
	ScopePolicyProcess          ScopePolicy = "process"
	ScopePolicyDeclared         ScopePolicy = "declared_scope"
	ScopePolicyWorkspace        ScopePolicy = "workspace"
	ScopePolicyWorkspaceRun     ScopePolicy = "workspace_run"
	ScopePolicyWorkspaceRunStep ScopePolicy = "workspace_run_step"
)

type IdentityPolicy string

const (
	IdentityPolicyTopicOwnerReceiver IdentityPolicy = "topic_owner_receiver"
	IdentityPolicyProcessLifecycle   IdentityPolicy = "process_lifecycle"
	IdentityPolicyRuntimeAuthority   IdentityPolicy = "runtime_authority"
	IdentityPolicyRuntimeDispatch    IdentityPolicy = "runtime_dispatch"
	IdentityPolicyRuntimeOperation   IdentityPolicy = "runtime_operation"
)

type TransitionTarget string

const (
	TransitionTargetNone                 TransitionTarget = "none"
	TransitionTargetReceiverOwnerPrivate TransitionTarget = "receiver_owner_private_logic"
	TransitionTargetOpaquePersistence    TransitionTarget = "opaque_persistence_only"
)

type Metadata struct {
	Layer             Layer
	StateMachine      StateMachine
	ReducerOwner      string
	MachineRole       StateMachineRole
	EffectClass       EffectClass
	SourcePolicy      SourcePolicy
	ScopePolicy       ScopePolicy
	IdentityPolicy    IdentityPolicy
	CanonicalFeedback bool
	TransitionTarget  TransitionTarget
	LaneIdentity      string
}

type CommandClass string

const (
	CommandClassRead    CommandClass = "read"
	CommandClassWrite   CommandClass = "write"
	CommandClassExecute CommandClass = "execute"
	CommandClassNotify  CommandClass = "notify"
	CommandClassFacade  CommandClass = "facade"
)

type LanePolicy string

const (
	LanePolicyTopic   LanePolicy = "topic"
	LanePolicyQuery   LanePolicy = "query"
	LanePolicyState   LanePolicy = "state"
	LanePolicyExecute LanePolicy = "execute"
	LanePolicyGlobal  LanePolicy = "global"
)

type Contract struct {
	Topic string
	Owner string
	// ReceiverOwners identifies the component owner(s) that consume a command.
	// It is empty for broadcast notification data. Owner remains the authority
	// that defines the contract; callers must not infer the receiver from a
	// topic prefix or definition package.
	ReceiverOwners []string
	CommandClass   CommandClass
	LanePolicy     LanePolicy
	Metadata       Metadata
}

// LaneScope identifies the authority object that a command lane serializes.
// It lives with the lane policy contract so topic packages can bind their own
// events without importing the repository-wide event catalog.
type LaneScope struct {
	SubjectType string
	SubjectID   string
	WorkspaceID string
	RunID       string
	StepID      string
}

func TopicLaneKey(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "topic/unknown"
	}
	return "topic/" + topic
}

func QueryLaneKey(topic string, scope LaneScope) string {
	topicPart := strings.Trim(strings.TrimSpace(topic), "/")
	if topicPart == "" {
		topicPart = "unknown"
	}
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if runID := strings.TrimSpace(scope.RunID); runID != "" {
		if workspaceID != "" {
			return "query/" + topicPart + "/workspace/" + workspaceID + "/run/" + runID
		}
		return "query/" + topicPart + "/run/" + runID
	}
	if stepID := strings.TrimSpace(scope.StepID); stepID != "" {
		if workspaceID != "" {
			return "query/" + topicPart + "/workspace/" + workspaceID + "/step/" + stepID
		}
		return "query/" + topicPart + "/step/" + stepID
	}
	if workspaceID != "" {
		return "query/" + topicPart + "/workspace/" + workspaceID
	}
	if subjectID := strings.TrimSpace(scope.SubjectID); subjectID != "" {
		return "query/" + topicPart + "/subject/" + subjectID
	}
	return "query/" + topicPart
}

func StateLaneKey(owner string, scope LaneScope) string {
	owner = lanePart(owner, "owner")
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	switch owner {
	case "run":
		if runID := strings.TrimSpace(scope.RunID); runID != "" {
			if workspaceID != "" {
				return "state/run/" + workspaceID + "/" + runID
			}
			return "state/run/" + runID
		}
	case "orchestrator":
		if runID := strings.TrimSpace(scope.RunID); runID != "" {
			if workspaceID != "" {
				return "state/orchestrator/workspace/" + workspaceID + "/run/" + runID
			}
			return "state/orchestrator/run/" + runID
		}
	case "workspace":
		if workspaceID != "" {
			return "state/workspace/" + workspaceID
		}
	}
	if subjectID := strings.TrimSpace(scope.SubjectID); subjectID != "" {
		if workspaceID != "" {
			return "state/" + owner + "/workspace/" + workspaceID + "/subject/" + subjectID
		}
		return "state/" + owner + "/" + subjectID
	}
	if runID := strings.TrimSpace(scope.RunID); runID != "" {
		if workspaceID != "" {
			return "state/" + owner + "/workspace/" + workspaceID + "/run/" + runID
		}
		return "state/" + owner + "/run/" + runID
	}
	if workspaceID != "" {
		return "state/" + owner + "/workspace/" + workspaceID
	}
	return "state/" + owner
}

func lanePart(value string, fallback string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return fallback
	}
	return strings.ReplaceAll(value, "/", "-")
}

type MessageContract struct {
	Topic             string
	Owner             string
	ReceiverOwners    []string
	DefinitionPackage string
	CommandClass      CommandClass
	LanePolicy        LanePolicy
}

func Build(owner string, class CommandClass, lane LanePolicy, topics ...string) []Contract {
	receivers := []string(nil)
	if class != CommandClassNotify {
		receivers = []string{owner}
	}
	return BuildForReceivers(owner, receivers, class, lane, topics...)
}

// BuildWithMetadata is used when a contract participates in one of the four
// Runtime state machines and therefore cannot use the generic owner-contract
// metadata. Metadata remains declarative: EventHub dispatch does not branch on
// it.
func BuildWithMetadata(owner string, class CommandClass, lane LanePolicy, metadata Metadata, topics ...string) []Contract {
	receivers := []string(nil)
	if class != CommandClassNotify {
		receivers = []string{owner}
	}
	return BuildForReceiversWithMetadata(owner, receivers, class, lane, metadata, topics...)
}

// BuildForReceivers records the real command consumer owner set. It is used
// when a protocol authority defines one concrete command consumed by another
// owner or by a closed set of capability owners.
func BuildForReceivers(owner string, receiverOwners []string, class CommandClass, lane LanePolicy, topics ...string) []Contract {
	return BuildForReceiversWithMetadata(owner, receiverOwners, class, lane, DefaultMetadata(class), topics...)
}

func BuildForReceiversWithMetadata(owner string, receiverOwners []string, class CommandClass, lane LanePolicy, metadata Metadata, topics ...string) []Contract {
	receiverOwners = normalizedOwners(receiverOwners)
	if strings.TrimSpace(metadata.LaneIdentity) == "" {
		metadata.LaneIdentity = lanePart(owner, "owner")
	}
	ret := make([]Contract, 0, len(topics))
	for _, topic := range topics {
		if topic == "" {
			continue
		}
		ret = append(ret, Contract{
			Topic:          topic,
			Owner:          owner,
			ReceiverOwners: append([]string(nil), receiverOwners...),
			CommandClass:   class,
			LanePolicy:     lane,
			Metadata:       metadata,
		})
	}
	return ret
}

func DefaultMetadata(class CommandClass) Metadata {
	metadata := Metadata{
		Layer:            LayerOwnerEventHub,
		StateMachine:     StateMachineNone,
		MachineRole:      StateMachineRoleNone,
		SourcePolicy:     SourcePolicyCallerContext,
		ScopePolicy:      ScopePolicyDeclared,
		IdentityPolicy:   IdentityPolicyTopicOwnerReceiver,
		TransitionTarget: TransitionTargetReceiverOwnerPrivate,
	}
	switch class {
	case CommandClassRead:
		metadata.EffectClass = EffectClassReadOnly
	case CommandClassWrite:
		metadata.EffectClass = EffectClassOwnerState
	case CommandClassExecute:
		metadata.EffectClass = EffectClassExternalSideEffect
	case CommandClassNotify:
		metadata.EffectClass = EffectClassPublishedFact
		metadata.SourcePolicy = SourcePolicyOwnerFact
		metadata.TransitionTarget = TransitionTargetNone
	case CommandClassFacade:
		metadata.EffectClass = EffectClassReceiverDefined
	}
	return metadata
}

func RuntimeMetadata(class CommandClass, stateMachine StateMachine, scope ScopePolicy, identity IdentityPolicy, canonicalFeedback bool) Metadata {
	metadata := DefaultMetadata(class)
	metadata.StateMachine = stateMachine
	metadata.ReducerOwner = ReducerOwnerForStateMachine(stateMachine)
	metadata.MachineRole = StateMachineRoleParticipant
	metadata.ScopePolicy = scope
	metadata.IdentityPolicy = identity
	metadata.CanonicalFeedback = canonicalFeedback
	return metadata
}

// OpaquePersistenceMetadata classifies physical storage/CAS of a payload whose
// state-machine semantics remain owned by another component.
func OpaquePersistenceMetadata(scope ScopePolicy, identity IdentityPolicy) Metadata {
	metadata := RuntimeMetadata(CommandClassWrite, StateMachineNone, scope, identity, true)
	metadata.ReducerOwner = ReducerOwnerForStateMachine(StateMachineSM4)
	metadata.MachineRole = StateMachineRoleOpaquePersistence
	metadata.EffectClass = EffectClassOpaquePersistence
	metadata.TransitionTarget = TransitionTargetOpaquePersistence
	return metadata
}

// Annotate applies explicit metadata to an already grouped contract list. It
// lets owner packages keep their command-class grouping while declaring the
// smaller Runtime-sensitive subset without a second topic registry.
func Annotate(contracts []Contract, metadata Metadata, topics ...string) []Contract {
	selected := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if topic = strings.TrimSpace(topic); topic != "" {
			selected[topic] = struct{}{}
		}
	}
	for idx := range contracts {
		if _, ok := selected[contracts[idx].Topic]; ok {
			annotated := metadata
			annotated = classifyRuntimeEventHubRole(contracts[idx], annotated)
			if strings.TrimSpace(annotated.LaneIdentity) == "" {
				annotated.LaneIdentity = contracts[idx].Metadata.LaneIdentity
			}
			contracts[idx].Metadata = annotated
			delete(selected, contracts[idx].Topic)
		}
	}
	if len(selected) != 0 {
		for topic := range selected {
			panic(fmt.Sprintf("Runtime contract metadata references unregistered topic %s", topic))
		}
	}
	return contracts
}

func AnnotateLaneIdentity(contracts []Contract, laneIdentity string, topics ...string) []Contract {
	selected := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if topic = strings.TrimSpace(topic); topic != "" {
			selected[topic] = struct{}{}
		}
	}
	for idx := range contracts {
		if _, ok := selected[contracts[idx].Topic]; ok {
			contracts[idx].Metadata.LaneIdentity = lanePart(laneIdentity, "owner")
			delete(selected, contracts[idx].Topic)
		}
	}
	for topic := range selected {
		panic(fmt.Sprintf("lane identity references unregistered topic %s", topic))
	}
	return contracts
}

func Validate(contract Contract) error {
	if strings.TrimSpace(contract.Topic) == "" || strings.TrimSpace(contract.Owner) == "" {
		return fmt.Errorf("event contract requires topic and owner")
	}
	if contract.CommandClass != CommandClassNotify && len(normalizedOwners(contract.ReceiverOwners)) == 0 {
		return fmt.Errorf("event command contract %s requires a receiver owner", contract.Topic)
	}
	if contract.CommandClass == CommandClassNotify && len(normalizedOwners(contract.ReceiverOwners)) != 0 {
		return fmt.Errorf("event notification contract %s must not declare receiver owners", contract.Topic)
	}
	metadata := contract.Metadata
	if metadata.Layer != LayerOwnerEventHub {
		return fmt.Errorf("EventHub contract %s has invalid layer %q", contract.Topic, metadata.Layer)
	}
	if metadata.StateMachine == "" || metadata.MachineRole == "" || metadata.EffectClass == "" || metadata.SourcePolicy == "" || metadata.ScopePolicy == "" || metadata.IdentityPolicy == "" || metadata.TransitionTarget == "" || strings.TrimSpace(metadata.LaneIdentity) == "" {
		return fmt.Errorf("EventHub contract %s has incomplete metadata", contract.Topic)
	}
	if err := validateStateMachineOwnership(contract); err != nil {
		return err
	}
	return nil
}

// ReducerOwnerForStateMachine is the canonical owner identity used by EventHub
// contracts. SM-1 belongs to Run; SM-2 through SM-4 belong to Orchestrator.
func ReducerOwnerForStateMachine(stateMachine StateMachine) string {
	switch stateMachine {
	case StateMachineSM1:
		return "run"
	case StateMachineSM2, StateMachineSM3, StateMachineSM4:
		return "orchestrator"
	default:
		return ""
	}
}

func classifyRuntimeEventHubRole(contract Contract, metadata Metadata) Metadata {
	if metadata.Layer != LayerOwnerEventHub || metadata.StateMachine == StateMachineNone {
		return metadata
	}
	metadata.ReducerOwner = ReducerOwnerForStateMachine(metadata.StateMachine)
	metadata.MachineRole = StateMachineRoleParticipant
	if contract.CommandClass == CommandClassNotify && contract.Owner == metadata.ReducerOwner {
		metadata.MachineRole = StateMachineRoleOwnerObservation
		return metadata
	}
	if contract.CommandClass != CommandClassWrite && contract.CommandClass != CommandClassExecute {
		return metadata
	}
	for _, receiver := range normalizedOwners(contract.ReceiverOwners) {
		if receiver == metadata.ReducerOwner {
			metadata.MachineRole = StateMachineRoleOwnerIngress
			break
		}
	}
	return metadata
}

func validateStateMachineOwnership(contract Contract) error {
	metadata := contract.Metadata
	if metadata.StateMachine == StateMachineNone {
		if metadata.EffectClass == EffectClassOpaquePersistence {
			if metadata.ReducerOwner != ReducerOwnerForStateMachine(StateMachineSM4) || metadata.MachineRole != StateMachineRoleOpaquePersistence || metadata.TransitionTarget != TransitionTargetOpaquePersistence {
				return fmt.Errorf("opaque Runtime persistence contract %s has invalid reducer ownership", contract.Topic)
			}
			return nil
		}
		if strings.TrimSpace(metadata.ReducerOwner) != "" || metadata.MachineRole != StateMachineRoleNone {
			return fmt.Errorf("non-Runtime contract %s declares reducer ownership", contract.Topic)
		}
		return nil
	}
	wantOwner := ReducerOwnerForStateMachine(metadata.StateMachine)
	if wantOwner == "" || metadata.ReducerOwner != wantOwner {
		return fmt.Errorf("Runtime contract %s declares reducer owner %q, want %q", contract.Topic, metadata.ReducerOwner, wantOwner)
	}
	switch metadata.MachineRole {
	case StateMachineRoleParticipant:
		return nil
	case StateMachineRoleOwnerIngress:
		if contract.CommandClass != CommandClassWrite && contract.CommandClass != CommandClassExecute {
			return fmt.Errorf("Runtime owner ingress %s must be write or execute", contract.Topic)
		}
		for _, receiver := range normalizedOwners(contract.ReceiverOwners) {
			if receiver == wantOwner {
				return nil
			}
		}
		return fmt.Errorf("Runtime owner ingress %s does not target reducer owner %q", contract.Topic, wantOwner)
	case StateMachineRoleOwnerObservation:
		if contract.CommandClass != CommandClassNotify || contract.Owner != wantOwner {
			return fmt.Errorf("Runtime owner observation %s is not published by reducer owner %q", contract.Topic, wantOwner)
		}
		return nil
	default:
		return fmt.Errorf("EventHub Runtime contract %s has invalid machine role %q", contract.Topic, metadata.MachineRole)
	}
}

type NoCommand struct{}
type NoData struct{}
type NoResult struct{}

type TypedTopic interface {
	EventContract() Contract
	MessageContract() MessageContract
}

type Topic[Command any, Data any, Result any] struct {
	Name              string
	Owner             string
	ReceiverOwners    []string
	DefinitionPackage string
	CommandClass      CommandClass
	LanePolicy        LanePolicy
}

func NewTopic[Command any, Data any, Result any](owner string, definitionPackage string, class CommandClass, lane LanePolicy, topic string) Topic[Command, Data, Result] {
	receivers := []string(nil)
	if class != CommandClassNotify {
		receivers = []string{owner}
	}
	return NewTopicForReceivers[Command, Data, Result](owner, receivers, definitionPackage, class, lane, topic)
}

func NewTopicForReceivers[Command any, Data any, Result any](owner string, receiverOwners []string, definitionPackage string, class CommandClass, lane LanePolicy, topic string) Topic[Command, Data, Result] {
	return Topic[Command, Data, Result]{
		Name:              topic,
		Owner:             owner,
		ReceiverOwners:    normalizedOwners(receiverOwners),
		DefinitionPackage: definitionPackage,
		CommandClass:      class,
		LanePolicy:        lane,
	}
}

func CommandTopic[Command any, Result any](owner string, definitionPackage string, class CommandClass, lane LanePolicy, topic string) Topic[Command, NoData, Result] {
	return NewTopic[Command, NoData, Result](owner, definitionPackage, class, lane, topic)
}

func CommandTopicForReceivers[Command any, Result any](owner string, receiverOwners []string, definitionPackage string, class CommandClass, lane LanePolicy, topic string) Topic[Command, NoData, Result] {
	return NewTopicForReceivers[Command, NoData, Result](owner, receiverOwners, definitionPackage, class, lane, topic)
}

func DataTopic[Data any](owner string, definitionPackage string, lane LanePolicy, topic string) Topic[NoCommand, Data, NoResult] {
	return NewTopic[NoCommand, Data, NoResult](owner, definitionPackage, CommandClassNotify, lane, topic)
}

func FacadeTopic[Command any, Result any](owner string, definitionPackage string, topic string) Topic[Command, NoData, Result] {
	return NewTopic[Command, NoData, Result](owner, definitionPackage, CommandClassFacade, LanePolicyTopic, topic)
}

func (t Topic[Command, Data, Result]) EventContract() Contract {
	return Contract{
		Topic:          t.Name,
		Owner:          t.Owner,
		ReceiverOwners: append([]string(nil), t.ReceiverOwners...),
		CommandClass:   t.CommandClass,
		LanePolicy:     t.LanePolicy,
	}
}

func (t Topic[Command, Data, Result]) MessageContract() MessageContract {
	return MessageContract{
		Topic:             t.Name,
		Owner:             t.Owner,
		ReceiverOwners:    append([]string(nil), t.ReceiverOwners...),
		DefinitionPackage: t.DefinitionPackage,
		CommandClass:      t.CommandClass,
		LanePolicy:        t.LanePolicy,
	}
}

func normalizedOwners(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func MessageContracts(topics ...TypedTopic) []MessageContract {
	ret := make([]MessageContract, 0, len(topics))
	for _, topic := range topics {
		if topic == nil {
			continue
		}
		contract := topic.MessageContract()
		if contract.Topic == "" {
			continue
		}
		ret = append(ret, contract)
	}
	return ret
}
