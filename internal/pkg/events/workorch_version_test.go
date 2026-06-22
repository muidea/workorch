package events

import (
	"testing"

	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
)

func TestValidateWorkorchEventVersionUsesTopicMatrix(t *testing.T) {
	if !ValidateWorkorchEventVersion(runevents.TopicRunStateChanged, WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should accept exact current version")
	}
	if !ValidateWorkorchEventVersion(governanceevents.TopicGovernanceAutoEvaluated, WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should include governance auto topics")
	}
	if !ValidateWorkorchEventVersion(runevents.TopicRunLLMStreamProgress, WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should include llm stream progress topic")
	}
	if !ValidateWorkorchEventVersion(runevents.TopicRunLLMUsageRecorded, WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should include llm usage topic")
	}
}

func TestWorkorchTopicsAreVersionedByTopicMatrix(t *testing.T) {
	topics := WorkorchTopics()
	if len(topics) == 0 {
		t.Fatal("WorkorchTopics returned no topics")
	}
	seen := map[string]bool{}
	for _, topic := range topics {
		if topic == "" {
			t.Fatal("WorkorchTopics returned an empty topic")
		}
		if seen[topic] {
			t.Fatalf("WorkorchTopics returned duplicate topic %q", topic)
		}
		seen[topic] = true
		if !ValidateWorkorchEventVersion(topic, WorkorchEventVersion) {
			t.Fatalf("WorkorchTopics topic %q is missing from version matrix", topic)
		}
	}
	if len(seen) != len(workorchEventContracts) {
		t.Fatalf("version matrix topic count = %d, WorkorchTopics count = %d", len(workorchEventContracts), len(seen))
	}
	for topic := range workorchEventContracts {
		if !seen[topic] {
			t.Fatalf("version matrix topic %q is missing from WorkorchTopics", topic)
		}
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
