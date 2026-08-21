package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestCreateCommandWithoutSynchronousResultDoesNotMutateRegistry(t *testing.T) {
	repository := memory.NewSkillRepository()
	hub := magicevent.NewHub(4)
	owner, err := New(hub, nil, Dependencies{Skills: repository, EventHub: hub})
	if err != nil {
		t.Fatalf("new owner: %v", err)
	}
	event := magicevent.NewEventWithContext(skillevents.TopicSkillCommandCreate, "test", skillevents.CommandDestination, magicevent.NewHeader(), context.Background(), skillevents.CreateSkillCommand{
		SkillID: "skill-post", Name: "post", Version: "v1", Instruction: "must not persist",
	})
	owner.handleSkillCommand(event, nil)
	if _, err := repository.Get(context.Background(), "skill-post"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Post-style create mutated registry: %v", err)
	}
}

func TestFileBackedOwnerRejectsLegacySkillWithoutProvenanceWithoutRewriting(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "skills.json")
	legacy := map[common.SkillID]skillmodel.Skill{
		"skill-legacy": {ID: "skill-legacy", Name: "legacy", Version: "v1", Instruction: "legacy instruction"},
	}
	encoded, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy Skill fixture: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write legacy Skill fixture: %v", err)
	}

	hub := magicevent.NewHub(8)
	owner, err := New(hub, nil, Dependencies{Skills: filerepo.NewSkillRepository(path), EventHub: hub})
	if err != nil {
		t.Fatalf("new file-backed Skill owner: %v", err)
	}
	if err := owner.Start(); err != nil {
		t.Fatalf("start file-backed Skill owner: %v", err)
	}
	t.Cleanup(func() {
		owner.Teardown(ctx)
		hub.Terminate(ctx)
	})

	for _, request := range []struct {
		name  string
		topic string
		data  any
	}{
		{name: "get", topic: skillevents.TopicSkillCommandGet, data: skillevents.GetSkillCommand{SkillID: "skill-legacy"}},
		{name: "list", topic: skillevents.TopicSkillCommandList, data: skillevents.ListSkillsCommand{}},
	} {
		t.Run(request.name, func(t *testing.T) {
			event := magicevent.NewEventWithContext(request.topic, "test", skillevents.CommandDestination, magicevent.NewHeader(), ctx, request.data)
			response := hub.Send(event)
			if response == nil {
				t.Fatalf("legacy %s response is nil", request.name)
			}
			if !errors.Is(response.Error(), common.ErrConflict) {
				t.Fatalf("legacy %s response error = %v, want ErrConflict", request.name, response.Error())
			}
		})
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy Skill file: %v", err)
	}
	if !bytes.Equal(after, encoded) {
		t.Fatalf("legacy Skill was implicitly upgraded or rewritten:\n%s", after)
	}
}
