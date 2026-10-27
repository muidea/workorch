package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type preparationCatalogRepository struct {
	repository.SkillRepository
	items  []*skillmodel.Skill
	err    error
	calls  int
	cancel context.CancelFunc
}

func (r *preparationCatalogRepository) List(context.Context) ([]*skillmodel.Skill, error) {
	r.calls++
	if r.cancel != nil {
		r.cancel()
	}
	return r.items, r.err
}

func TestPreparationRegistryCatalogOwnerRejectsIncompleteInvalidAndCancelledReads(t *testing.T) {
	fixture := skillFromCommand(skillevents.CreateSkillCommand{SkillID: "skill", Name: "name", Version: "v1", Instruction: "原始指令\n第二行"})
	for _, scenario := range []string{"empty", "valid", "duplicate", "nil_row", "bad_hash", "noncanonical", "too_many", "too_large", "encoded_too_large", "repository_error", "cancelled_before", "cancelled_during", "scope"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &preparationCatalogRepository{items: []*skillmodel.Skill{skillmodel.CloneSkill(fixture)}}
			switch scenario {
			case "empty":
				repo.items = nil
			case "duplicate":
				repo.items = append(repo.items, skillmodel.CloneSkill(fixture))
			case "nil_row":
				repo.items[0] = nil
			case "bad_hash":
				repo.items[0].Provenance.ContentHash = "wrong"
			case "noncanonical":
				repo.items[0].Instruction += " "
			case "too_many":
				repo.items = make([]*skillmodel.Skill, skillevents.RegistryCatalogMaxItems+1)
			case "too_large", "encoded_too_large":
				repo.items = nil
				count, char := 130, "x"
				if scenario == "encoded_too_large" {
					count, char = 70, `\`
				}
				for i := 0; i < count; i++ {
					repo.items = append(repo.items, skillFromCommand(skillevents.CreateSkillCommand{SkillID: common.SkillID(fmt.Sprintf("skill-%03d", i)), Name: "name", Version: "v1", Instruction: strings.Repeat(char, 65536)}))
				}
			case "repository_error":
				repo.err = errors.New("unaltered repository diagnostic")
			case "cancelled_before":
				cancel()
			case "cancelled_during":
				repo.cancel = cancel
			case "scope":
				ctx, _ = common.BindRuntimeWorkspaceContext(ctx, "another")
			}
			hub := magicevent.NewHub(8)
			defer hub.Terminate(context.Background())
			owner, err := New(hub, nil, Dependencies{Skills: repo})
			if err != nil {
				t.Fatal(err)
			}
			value, err := owner.observeRegistryCatalog(ctx, skillevents.ObserveRegistryCatalogCommand{WorkspaceID: "ws"})
			if scenario == "valid" || scenario == "empty" {
				if err != nil || value.ValidateFor(skillevents.ObserveRegistryCatalogCommand{WorkspaceID: "ws"}) != nil {
					t.Fatalf("valid catalog: %v", err)
				}
				if scenario == "valid" {
					value.Skills[0].Instruction = "caller"
					if repo.items[0].Instruction != fixture.Instruction {
						t.Fatal("caller changed owner record")
					}
				}
			} else if err == nil || value.Version != "" || value.Skills != nil {
				t.Fatal("invalid or partial read returned as complete")
			}
			if scenario == "repository_error" && !errors.Is(err, repo.err) {
				t.Fatal("original diagnostic lost")
			}
			want := 1
			if scenario == "cancelled_before" || scenario == "scope" {
				want = 0
			}
			if repo.calls != want {
				t.Fatalf("list calls %d want %d", repo.calls, want)
			}
		})
	}
}

func TestPreparationRegistryCatalogTypedIngressDrainAndEpoch(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	repo := &preparationCatalogRepository{SkillRepository: memory.NewSkillRepository()}
	owner, err := New(hub, nil, Dependencies{Skills: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	query := func(source string, data any) magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(skillevents.TopicObserveRegistryCatalog, source, skillevents.CommandDestination, magicevent.NewHeader(), ctx, data))
	}
	command := skillevents.ObserveRegistryCatalogCommand{WorkspaceID: "ws"}
	for _, source := range []string{applicationOwnerReadSource, applicationContextRegistrySource, "untrusted", " " + applicationPreparationSource + " "} {
		if result := query(source, command); result == nil || !errors.Is(result.Error(), common.ErrInvalidArgument) {
			t.Fatal("unauthorized source accepted")
		}
	}
	if result := query(applicationPreparationSource, skillevents.ListSkillsCommand{}); result == nil || !errors.Is(result.Error(), common.ErrInvalidArgument) {
		t.Fatal("wrong typed command accepted")
	}
	owner.handleSkillCommand(magicevent.NewEventWithContext(skillevents.TopicObserveRegistryCatalog, applicationPreparationSource, skillevents.CommandDestination, nil, ctx, command), nil)
	if repo.calls != 0 {
		t.Fatal("invalid command or Post performed I/O")
	}
	owner.BeginShutdown(ctx)
	result := query(applicationPreparationSource, command)
	if result == nil || result.Error() != nil {
		t.Fatalf("drain observation failed: %v", result)
	}
	data, getErr := result.Get()
	value, ok := data.(skillevents.ObserveRegistryCatalogResult)
	if getErr != nil || !ok || value.ValidateFor(command) != nil || repo.calls != 1 {
		t.Fatalf("invalid typed drain result: value=%+v err=%v typed=%v calls=%d validation=%v", value, getErr, ok, repo.calls, value.ValidateFor(command))
	}
	other, err := New(hub, nil, Dependencies{Skills: repo})
	if err != nil || other.preparationEpoch == owner.preparationEpoch {
		t.Fatal("epoch reused across owner reconstruction")
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	result = query(applicationPreparationSource, command)
	if result != nil && result.Error() == nil {
		t.Fatal("released owner returned success")
	}
	if repo.calls != 1 {
		t.Fatal("released owner read repository")
	}
}
