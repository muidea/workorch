package skill

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
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
