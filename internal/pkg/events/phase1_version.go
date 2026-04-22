package events

const (
	Phase1EventVersion    = "phase1.event.v1"
	Phase1SnapshotVersion = "phase1-demo.v1"
)

type Phase1ContractSpec struct {
	Topic          string
	CurrentVersion string
}

var (
	phase1EventContracts = map[string]Phase1ContractSpec{
		TopicWorkspaceCreated:      newPhase1EventContract(TopicWorkspaceCreated),
		TopicWorkspaceStateChanged: newPhase1EventContract(TopicWorkspaceStateChanged),
		TopicRunStateChanged:       newPhase1EventContract(TopicRunStateChanged),
		TopicStepDispatched:        newPhase1EventContract(TopicStepDispatched),
		TopicStepStateChanged:      newPhase1EventContract(TopicStepStateChanged),
		TopicTicketCreated:         newPhase1EventContract(TopicTicketCreated),
		TopicTicketResolved:        newPhase1EventContract(TopicTicketResolved),
		TopicApprovalCreated:       newPhase1EventContract(TopicApprovalCreated),
		TopicApprovalResolved:      newPhase1EventContract(TopicApprovalResolved),
	}
	phase1SnapshotContracts = map[string]Phase1ContractSpec{
		"/workspace/snapshot":       newPhase1SnapshotContract("/workspace/snapshot"),
		"/run/snapshot":             newPhase1SnapshotContract("/run/snapshot"),
		"/step/snapshot":            newPhase1SnapshotContract("/step/snapshot"),
		"/decision/ticket/snapshot": newPhase1SnapshotContract("/decision/ticket/snapshot"),
		"/approval/snapshot":        newPhase1SnapshotContract("/approval/snapshot"),
	}
)

func Phase1EventContract(topic string) (Phase1ContractSpec, bool) {
	spec, ok := phase1EventContracts[topic]
	return spec, ok
}

func Phase1SnapshotContract(topic string) (Phase1ContractSpec, bool) {
	spec, ok := phase1SnapshotContracts[topic]
	return spec, ok
}

func ValidatePhase1EventVersion(topic, version string) bool {
	spec, ok := Phase1EventContract(topic)
	return ok && version == spec.CurrentVersion
}

func ValidatePhase1SnapshotVersion(topic, version string) bool {
	spec, ok := Phase1SnapshotContract(topic)
	return ok && version == spec.CurrentVersion
}

func newPhase1EventContract(topic string) Phase1ContractSpec {
	return Phase1ContractSpec{
		Topic:          topic,
		CurrentVersion: Phase1EventVersion,
	}
}

func newPhase1SnapshotContract(topic string) Phase1ContractSpec {
	return Phase1ContractSpec{
		Topic:          topic,
		CurrentVersion: Phase1SnapshotVersion,
	}
}
