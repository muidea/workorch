package setupdraftports

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestBuildGenerationRequestNormalizesCommandWithoutTemplate(t *testing.T) {
	t.Parallel()

	got, err := BuildGenerationRequest(context.Background(), nil, setupdraftcore.GenerationCommand{
		Workspace: setupdraftcore.Workspace{
			Title:                "  Draft workspace  ",
			Description:          "  description  ",
			DeliverableDirectory: "  /tmp/ws  ",
		},
		Messages: []setupdraftcore.Message{
			{Role: " Assistant ", Content: "  previous answer  "},
			{Role: "unknown", Content: " next request "},
			{Role: "user", Content: "   "},
		},
		CurrentDraft:   setupdraftcore.Draft{GoalText: "  goal  ", DoDItems: []string{" done ", "done"}},
		DialogLanguage: " zh-CN ",
		Regenerate:     true,
	})
	if err != nil {
		t.Fatalf("BuildGenerationRequest: %v", err)
	}
	if got.Workspace.Title != "Draft workspace" || got.Workspace.Description != "description" || got.Workspace.DeliverableDirectory != "/tmp/ws" {
		t.Fatalf("Workspace = %+v, want trimmed workspace fields", got.Workspace)
	}
	wantMessages := []setupdraftcore.Message{
		{Role: "assistant", Content: "previous answer"},
		{Role: "user", Content: "next request"},
	}
	if !reflect.DeepEqual(got.Messages, wantMessages) {
		t.Fatalf("Messages = %+v, want %+v", got.Messages, wantMessages)
	}
	if got.CurrentDraft.GoalText != "goal" || !reflect.DeepEqual(got.CurrentDraft.DoDItems, []string{"done"}) {
		t.Fatalf("CurrentDraft = %+v, want normalized draft", got.CurrentDraft)
	}
	if got.DialogLanguage != "zh-CN" || !got.Regenerate {
		t.Fatalf("DialogLanguage=%q Regenerate=%v, want normalized options", got.DialogLanguage, got.Regenerate)
	}
}

func TestBuildGenerationRequestRejectsEmptyCommand(t *testing.T) {
	t.Parallel()

	_, err := BuildGenerationRequest(context.Background(), nil, setupdraftcore.GenerationCommand{})
	if err != common.ErrInvalidArgument {
		t.Fatalf("BuildGenerationRequest err = %v, want ErrInvalidArgument", err)
	}
}
