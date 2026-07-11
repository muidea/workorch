package events

import (
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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
	for _, topic := range []string{
		runevents.TopicRunStarted,
		runevents.TopicRunBlockedOnDecision,
		runevents.TopicRunCompleted,
		runevents.TopicStepCreated,
		runevents.TopicStepSucceeded,
		decisionevents.TopicDecisionTicketCreated,
		decisionevents.TopicDecisionTicketSuperseded,
		approvalevents.TopicApprovalApproved,
		approvalevents.TopicApprovalSuperseded,
		resultevents.TopicResultCreated,
		orchestratorevents.TopicOrchestratorContractViolation,
	} {
		if !ValidateWorkorchEventVersion(topic, WorkorchEventVersion) {
			t.Fatalf("ValidateWorkorchEventVersion should include audit topic %q", topic)
		}
	}
}

func TestValidateWorkorchEventVersionRejectsUnknownTopic(t *testing.T) {
	if ValidateWorkorchEventVersion("/unknown/topic", WorkorchEventVersion) {
		t.Fatalf("ValidateWorkorchEventVersion should reject unknown topic")
	}
}
