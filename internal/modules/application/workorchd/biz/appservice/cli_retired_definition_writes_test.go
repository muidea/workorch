package appservice

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func TestCLIDispatcherRejectsRetiredDefinitionWritesWithoutOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	dispatcher := NewCLICommandDispatcher(&runtimecore.Session{}, hub, nil)
	if err := dispatcher.Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dispatcher.Close)
	for _, test := range []struct {
		topic string
		data  any
	}{
		{"/cli/workspace/create", workorchdevents.CLIWorkspaceCreateCommand{}},
		{"/cli/goal/define", workorchdevents.CLIGoalDefineCommand{WorkspaceID: "ws"}},
		{"/cli/goal/update", workorchdevents.CLIGoalUpdateCommand{WorkspaceID: "ws", GoalID: "goal"}},
		{"/cli/dod/define", workorchdevents.CLIDoDDefineCommand{WorkspaceID: "ws"}},
		{"/cli/dod/update", workorchdevents.CLIDoDUpdateCommand{WorkspaceID: "ws", DoDID: "dod"}},
	} {
		t.Run(test.topic, func(t *testing.T) {
			event := magicevent.NewEventWithContext(test.topic, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, test.data)
			for _, contract := range workorchdevents.Contracts() {
				if contract.Topic == test.topic {
					t.Fatal("退休命令仍声明为 production EventHub 合同")
				}
			}
			for _, topic := range workorchdevents.CLITopics() {
				if topic == test.topic {
					t.Fatal("退休命令仍注册到 dispatcher 生命周期")
				}
			}
			// 直接检查防御 handler；不为已退休命令保留可达订阅。
			result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
			if test.topic == "/cli/workspace/create" {
				dispatcher.handleTypedCLIWorkspace(event, result)
			} else {
				dispatcher.handleTypedCLIDefinition(event, result)
			}
			if result.Error() == nil || !strings.Contains(result.Error().Error(), "已退休") {
				t.Fatalf("退休命令未在 owner 调用前拒绝: %v", result.Error())
			}
		})
	}
}
