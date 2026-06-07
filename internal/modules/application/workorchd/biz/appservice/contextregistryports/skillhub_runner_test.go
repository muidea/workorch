package contextregistryports

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
)

type fakeSkillHubRunner struct {
	responses map[string][]byte
}

func (f fakeSkillHubRunner) RunSkillHub(_ context.Context, dir string, args ...string) ([]byte, error) {
	key := strings.TrimSpace(dir) + "|" + strings.Join(args, " ")
	if output, ok := f.responses[key]; ok {
		return output, nil
	}
	return nil, fmt.Errorf("unexpected skill-hub command %s", key)
}

func TestListSkillHubGlobalSkillsUsesCodexGlobalStatus(t *testing.T) {
	ctx := context.Background()
	runner := fakeSkillHubRunner{
		responses: map[string][]byte{
			"|status --global --json --agent codex": []byte(`{
				"items":[
					{"skill_id":"go-service","status":"ok","version":"1.0.0"},
					{"skill_id":"go-service","status":"duplicate"},
					{"skill_id":""}
				]
			}`),
		},
	}

	items, err := ListSkillHubGlobalSkills(ctx, runner)
	if err != nil {
		t.Fatalf("ListSkillHubGlobalSkills: %v", err)
	}
	if got := skillHubRunnerSkillIDs(items); !reflect.DeepEqual(got, []string{"skillhub:global:go-service"}) {
		t.Fatalf("ids = %+v, want deduplicated global skill", got)
	}
	if items[0].Scope != "global" || items[0].SourceSkillID != "go-service" || items[0].Version != "1.0.0" {
		t.Fatalf("item = %+v, want mapped global skill metadata", items[0])
	}
}

func TestListSkillHubWorkspaceSkillsSkipsEmptyDeliverableDirectory(t *testing.T) {
	items, err := ListSkillHubWorkspaceSkills(context.Background(), fakeSkillHubRunner{}, "ws-001", " ")
	if err != nil {
		t.Fatalf("ListSkillHubWorkspaceSkills: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
}

func TestListSkillHubSkillsSkipsNilRunner(t *testing.T) {
	items, err := ListSkillHubSkills(context.Background(), nil, "", "global", "", "status")
	if err != nil {
		t.Fatalf("ListSkillHubSkills: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
}

func skillHubRunnerSkillIDs(items []contextregistrycore.Skill) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return ret
}
