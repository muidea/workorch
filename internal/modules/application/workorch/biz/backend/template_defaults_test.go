package backend_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDefaultDoDItemsForWorkspaceUsesMatchingTemplate(t *testing.T) {
	be := newFakeBackend()
	be.workspaces = append(be.workspaces, backend.WorkspaceInfo{
		ID:            "ws-template",
		SoulBindings:  map[string]common.SoulID{"execution": "soul-execution"},
		DefaultSoulID: "soul-default",
	})
	be.templates = append(be.templates, backend.TemplateInfo{
		ID:            "software-dev",
		DefaultSoulID: "soul-default",
		SoulBindings:  map[string]string{"execution": "soul-execution"},
		DoDItems:      []string{"tests pass", "diff is delivered"},
	})

	got := backend.DefaultDoDItemsForWorkspace(context.Background(), be, "ws-template")

	want := []string{"tests pass", "diff is delivered"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DoD items = %+v, want %+v", got, want)
	}
}

func TestDefaultDoDItemsForWorkspaceFallsBackWhenTemplateDoesNotMatch(t *testing.T) {
	be := newFakeBackend()
	be.workspaces = append(be.workspaces, backend.WorkspaceInfo{ID: "ws-generic"})

	got := backend.DefaultDoDItemsForWorkspace(context.Background(), be, "ws-generic")

	want := []string{"目标已完成，输出包含明确的执行结果"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DoD items = %+v, want %+v", got, want)
	}
}
