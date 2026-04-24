package events

import "testing"

func TestValidateWorkorchEventVersionUsesTopicMatrix(t *testing.T) {
	if !ValidateWorkorchEventVersion(TopicRunStateChanged, WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should accept exact current version")
	}
}

func TestValidateWorkorchEventVersionRejectsUnknownTopic(t *testing.T) {
	if ValidateWorkorchEventVersion("/unknown/topic", WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should reject unknown topic")
	}
}

func TestValidateWorkorchSnapshotVersionUsesTopicMatrix(t *testing.T) {
	if !ValidateWorkorchSnapshotVersion("/run/snapshot", WorkorchSnapshotVersion) {
		t.Fatalf("ValidateWorkorchSnapshotVersion should accept exact current version")
	}
}

func TestValidateWorkorchSnapshotVersionRejectsUnknownTopic(t *testing.T) {
	if ValidateWorkorchSnapshotVersion("/unknown/snapshot", WorkorchSnapshotVersion) {
		t.Fatalf("ValidateWorkorchSnapshotVersion should reject unknown topic")
	}
}
