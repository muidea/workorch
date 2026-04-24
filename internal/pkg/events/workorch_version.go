package events

const (
	WorkorchEventVersion    = "workorch.event.v1"
	WorkorchSnapshotVersion = "workorch-demo.v1"
)

type WorkorchContractSpec struct {
	Topic          string
	CurrentVersion string
}

var (
	workorchEventContracts = map[string]WorkorchContractSpec{
		TopicWorkspaceCreated:      newWorkorchEventContract(TopicWorkspaceCreated),
		TopicWorkspaceStateChanged: newWorkorchEventContract(TopicWorkspaceStateChanged),
		TopicRunStateChanged:       newWorkorchEventContract(TopicRunStateChanged),
		TopicStepDispatched:        newWorkorchEventContract(TopicStepDispatched),
		TopicStepStateChanged:      newWorkorchEventContract(TopicStepStateChanged),
		TopicTicketCreated:         newWorkorchEventContract(TopicTicketCreated),
		TopicTicketResolved:        newWorkorchEventContract(TopicTicketResolved),
		TopicApprovalCreated:       newWorkorchEventContract(TopicApprovalCreated),
		TopicApprovalResolved:      newWorkorchEventContract(TopicApprovalResolved),
	}
	workorchSnapshotContracts = map[string]WorkorchContractSpec{
		"/workspace/snapshot":       newWorkorchSnapshotContract("/workspace/snapshot"),
		"/run/snapshot":             newWorkorchSnapshotContract("/run/snapshot"),
		"/step/snapshot":            newWorkorchSnapshotContract("/step/snapshot"),
		"/decision/ticket/snapshot": newWorkorchSnapshotContract("/decision/ticket/snapshot"),
		"/approval/snapshot":        newWorkorchSnapshotContract("/approval/snapshot"),
	}
)

func WorkorchEventContract(topic string) (WorkorchContractSpec, bool) {
	spec, ok := workorchEventContracts[topic]
	return spec, ok
}

func WorkorchSnapshotContract(topic string) (WorkorchContractSpec, bool) {
	spec, ok := workorchSnapshotContracts[topic]
	return spec, ok
}

func ValidateWorkorchEventVersion(topic, version string) bool {
	spec, ok := WorkorchEventContract(topic)
	return ok && version == spec.CurrentVersion
}

func ValidateWorkorchSnapshotVersion(topic, version string) bool {
	spec, ok := WorkorchSnapshotContract(topic)
	return ok && version == spec.CurrentVersion
}

func newWorkorchEventContract(topic string) WorkorchContractSpec {
	return WorkorchContractSpec{
		Topic:          topic,
		CurrentVersion: WorkorchEventVersion,
	}
}

func newWorkorchSnapshotContract(topic string) WorkorchContractSpec {
	return WorkorchContractSpec{
		Topic:          topic,
		CurrentVersion: WorkorchSnapshotVersion,
	}
}
