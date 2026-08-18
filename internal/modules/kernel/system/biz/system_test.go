package biz

import (
	"context"
	"errors"
	"reflect"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpregistryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ensureFailureMode string

const (
	ensureFailureMissingSubscriber ensureFailureMode = "missing-subscriber"
	ensureFailureNilResult         ensureFailureMode = "nil-result"
	ensureFailureWrongResult       ensureFailureMode = "wrong-result"
	ensureFailureOwnerError        ensureFailureMode = "owner-error"
	ensureFailureNotEnsured        ensureFailureMode = "not-ensured"
)

type scriptedEnsureHub struct {
	magicevent.Hub
	targetTopic     string
	failureMode     ensureFailureMode
	ownerError      error
	calls           []string
	invalidContract bool
}

func (s *scriptedEnsureHub) Send(ev magicevent.Event) magicevent.Result {
	s.calls = append(s.calls, ev.ID())
	if !ensureCommandMatchesContract(ev) {
		s.invalidContract = true
	}
	if ev.ID() != s.targetTopic {
		return successfulEnsureResult(ev)
	}

	switch s.failureMode {
	case ensureFailureMissingSubscriber:
		return s.Hub.Send(ev)
	case ensureFailureNilResult:
		return nil
	case ensureFailureWrongResult:
		result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		result.Set(struct{}{}, nil)
		return result
	case ensureFailureOwnerError:
		result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "owner ensure failed", Cause: s.ownerError})
		return result
	case ensureFailureNotEnsured:
		result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
		switch ev.ID() {
		case soulevents.TopicSoulCommandEnsureDefaults:
			result.Set(soulevents.EnsureDefaultsResult{}, nil)
		case workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults:
			result.Set(workspaceevents.EnsureWorkspaceTemplateDefaultsResult{}, nil)
		case mcpregistryevents.TopicMCPCommandEnsureDefaults:
			result.Set(mcpregistryevents.EnsureDefaultsResult{}, nil)
		}
		return result
	default:
		return successfulEnsureResult(ev)
	}
}

func successfulEnsureResult(ev magicevent.Event) magicevent.Result {
	result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
	switch ev.ID() {
	case soulevents.TopicSoulCommandEnsureDefaults:
		result.Set(soulevents.EnsureDefaultsResult{Ensured: true}, nil)
	case workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults:
		result.Set(workspaceevents.EnsureWorkspaceTemplateDefaultsResult{Ensured: true}, nil)
	case mcpregistryevents.TopicMCPCommandEnsureDefaults:
		result.Set(mcpregistryevents.EnsureDefaultsResult{Ensured: true}, nil)
	default:
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: "unexpected ensure topic"})
	}
	return result
}

func ensureCommandMatchesContract(ev magicevent.Event) bool {
	switch ev.ID() {
	case soulevents.TopicSoulCommandEnsureDefaults:
		_, ok := ev.Data().(soulevents.EnsureDefaultsCommand)
		return ok && ev.Destination() == soulevents.CommandDestination
	case workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults:
		_, ok := ev.Data().(workspaceevents.EnsureWorkspaceTemplateDefaultsCommand)
		return ok && ev.Destination() == workspaceevents.CommandDestination
	case mcpregistryevents.TopicMCPCommandEnsureDefaults:
		_, ok := ev.Data().(mcpregistryevents.EnsureDefaultsCommand)
		return ok && ev.Destination() == mcpregistryevents.CommandDestination
	default:
		return false
	}
}

func TestSystemStartFailsClosedForEveryEnsureContract(t *testing.T) {
	orderedTopics := []string{
		soulevents.TopicSoulCommandEnsureDefaults,
		workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults,
		mcpregistryevents.TopicMCPCommandEnsureDefaults,
	}
	failureModes := []ensureFailureMode{
		ensureFailureMissingSubscriber,
		ensureFailureNilResult,
		ensureFailureWrongResult,
		ensureFailureOwnerError,
		ensureFailureNotEnsured,
	}

	for targetIndex, targetTopic := range orderedTopics {
		for _, failureMode := range failureModes {
			t.Run(targetTopic+"/"+string(failureMode), func(t *testing.T) {
				baseHub := magicevent.NewHub(16)
				t.Cleanup(func() { baseHub.Terminate(context.Background()) })
				ownerErr := errors.New("injected owner error")
				hub := &scriptedEnsureHub{Hub: baseHub, targetTopic: targetTopic, failureMode: failureMode, ownerError: ownerErr}
				system, err := NewWithDependencies(Dependencies{EventHub: hub})
				if err != nil {
					t.Fatalf("new system: %v", err)
				}

				err = system.Start(context.Background())
				if err == nil {
					t.Fatal("Start error = nil, want fail-closed error")
				}
				if failureMode == ensureFailureOwnerError && !errors.Is(err, ownerErr) {
					t.Fatalf("Start error = %v, want owner cause %v", err, ownerErr)
				}
				if (failureMode == ensureFailureWrongResult || failureMode == ensureFailureNotEnsured) && !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("Start error = %v, want invalid argument", err)
				}
				wantCalls := orderedTopics[:targetIndex+1]
				if !reflect.DeepEqual(hub.calls, wantCalls) {
					t.Fatalf("ensure calls = %v, want fail-fast prefix %v", hub.calls, wantCalls)
				}
				if hub.invalidContract {
					t.Fatal("System emitted a topic, destination, or command type outside the explicit owner contracts")
				}
			})
		}
	}
}
