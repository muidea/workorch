package events

import "testing"

func TestValidatePhase1EventVersionUsesTopicMatrix(t *testing.T) {
	if !ValidatePhase1EventVersion(TopicRunStateChanged, Phase1EventVersion) {
		t.Fatalf("ValidatePhase1EventVersion should accept exact current version")
	}
}

func TestValidatePhase1EventVersionRejectsUnknownTopic(t *testing.T) {
	if ValidatePhase1EventVersion("/unknown/topic", Phase1EventVersion) {
		t.Fatalf("ValidatePhase1EventVersion should reject unknown topic")
	}
}

func TestValidatePhase1SnapshotVersionUsesTopicMatrix(t *testing.T) {
	if !ValidatePhase1SnapshotVersion("/run/snapshot", Phase1SnapshotVersion) {
		t.Fatalf("ValidatePhase1SnapshotVersion should accept exact current version")
	}
}

func TestValidatePhase1SnapshotVersionRejectsUnknownTopic(t *testing.T) {
	if ValidatePhase1SnapshotVersion("/unknown/snapshot", Phase1SnapshotVersion) {
		t.Fatalf("ValidatePhase1SnapshotVersion should reject unknown topic")
	}
}
