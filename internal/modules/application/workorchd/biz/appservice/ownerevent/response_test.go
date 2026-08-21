package ownerevent

import (
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	mcpregistryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestOwnerResponseDecodersFailClosedForNilWrongTypeAndOwnerError(t *testing.T) {
	topic := "/test/owner/response"
	tests := []struct {
		name    string
		correct any
		decode  func(magicevent.Result) error
	}{
		{name: "workspace", correct: workspaceevents.WorkspaceCommandResult{}, decode: func(response magicevent.Result) error { _, err := Workspace(response, topic); return err }},
		{name: "workspace focused", correct: workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, decode: func(response magicevent.Result) error {
			_, err := WorkspaceOwner[workspaceevents.SubmitDefinitionAdmissionCandidateResult](response, topic)
			return err
		}},
		{name: "workspace lifecycle", correct: workspaceevents.WorkspaceLifecycleCommandResult{}, decode: func(response magicevent.Result) error { _, err := WorkspaceLifecycle(response, topic); return err }},
		{name: "run", correct: runevents.GetRunResult{}, decode: func(response magicevent.Result) error {
			_, err := Run[runevents.GetRunResult](response, topic)
			return err
		}},
		{name: "run lifecycle", correct: runevents.RunLifecycleCommandResult{}, decode: func(response magicevent.Result) error { _, err := RunLifecycle(response, topic); return err }},
		{name: "orchestrator", correct: orchestratorevents.OrchestratorCommandResult{}, decode: func(response magicevent.Result) error { _, err := Orchestrator(response, topic); return err }},
		{name: "decision", correct: decisionevents.GetTicketResult{}, decode: func(response magicevent.Result) error {
			_, err := Decision[decisionevents.GetTicketResult](response, topic)
			return err
		}},
		{name: "approval", correct: approvalevents.GetApprovalResult{}, decode: func(response magicevent.Result) error {
			_, err := Approval[approvalevents.GetApprovalResult](response, topic)
			return err
		}},
		{name: "memory", correct: memoryevents.GetMemoryResult{}, decode: func(response magicevent.Result) error {
			_, err := Memory[memoryevents.GetMemoryResult](response, topic)
			return err
		}},
		{name: "result", correct: resultevents.GetResultResult{}, decode: func(response magicevent.Result) error {
			_, err := Result[resultevents.GetResultResult](response, topic)
			return err
		}},
		{name: "skill", correct: skillevents.GetSkillResult{}, decode: func(response magicevent.Result) error {
			_, err := Skill[skillevents.GetSkillResult](response, topic)
			return err
		}},
		{name: "mcp registry", correct: mcpregistryevents.GetServerResult{}, decode: func(response magicevent.Result) error {
			_, err := MCP[mcpregistryevents.GetServerResult](response, topic)
			return err
		}},
		{name: "eventstream", correct: eventstreamevents.ListEventsResult{}, decode: func(response magicevent.Result) error {
			_, err := EventStream[eventstreamevents.ListEventsResult](response, topic)
			return err
		}},
		{name: "governance", correct: governanceevents.GovernanceCommandResult{}, decode: func(response magicevent.Result) error { _, err := Governance(response, topic); return err }},
	}
	ownerErr := errors.New("owner unavailable")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.decode(nil); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("nil result error = %v, want ErrInvalidArgument", err)
			}

			wrong := magicevent.NewResult(topic, "test-owner", "test")
			wrong.Set(struct{ Wrong bool }{Wrong: true}, nil)
			if err := tt.decode(wrong); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("wrong result type error = %v, want ErrInvalidArgument", err)
			}

			failed := magicevent.NewResult(topic, "test-owner", "test")
			failed.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "owner unavailable", Cause: ownerErr})
			if err := tt.decode(failed); !errors.Is(err, ownerErr) {
				t.Fatalf("owner error = %v, want preserved cause", err)
			}

			correct := magicevent.NewResult(topic, "test-owner", "test")
			correct.Set(tt.correct, nil)
			if err := tt.decode(correct); err != nil {
				t.Fatalf("correct result error = %v", err)
			}
		})
	}
}

func TestMemoryResponseRequiresConcreteResultType(t *testing.T) {
	if _, err := Memory[memoryevents.GetMemoryResult](nil, memoryevents.TopicMemoryCommandGet); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil response error = %v", err)
	}

	response := magicevent.NewResult(memoryevents.TopicMemoryCommandGet, memoryevents.CommandDestination, "test")
	response.Set(memoryevents.ListMemoryByWorkspaceResult{}, nil)
	if _, err := Memory[memoryevents.GetMemoryResult](response, memoryevents.TopicMemoryCommandGet); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("wrong concrete response error = %v", err)
	}

	response = magicevent.NewResult(memoryevents.TopicMemoryCommandGet, memoryevents.CommandDestination, "test")
	want := memoryevents.GetMemoryResult{}
	response.Set(want, nil)
	got, err := Memory[memoryevents.GetMemoryResult](response, memoryevents.TopicMemoryCommandGet)
	if err != nil {
		t.Fatalf("decode concrete memory result: %v", err)
	}
	if got.Item != nil {
		t.Fatalf("decoded concrete memory result = %+v", got)
	}
}
