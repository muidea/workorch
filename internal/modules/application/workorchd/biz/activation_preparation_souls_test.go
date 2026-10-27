package biz

import (
	"context"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	soulcommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationSoulPlanMatchesExistingActivationPriorityWithoutRun(t *testing.T) {
	hub := magicevent.NewHub(32)
	defer hub.Terminate(context.Background())
	owner := magicevent.NewSimpleObserver(soulevents.CommandDestination, hub)
	if err := owner.Subscribe(soulevents.TopicSoulCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd := ev.Data().(soulevents.GetSoulCommand)
		result.Set(soulevents.GetSoulResult{Soul: &soulmodel.Soul{ID: cmd.SoulID, Name: "soul", SoulMD: "instruction\n"}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer owner.Unsubscribe(soulevents.TopicSoulCommandGet)
	for _, scenario := range []string{"unbound", "system", "workspace", "stage_partial", "stage_all", "no_system_stage"} {
		t.Run(scenario, func(t *testing.T) {
			w := &workspace.Workspace{}
			systemDefault := common.SoulID("system")
			switch scenario {
			case "unbound":
				systemDefault = ""
			case "workspace":
				w.DefaultSoulID = "workspace"
			case "stage_partial":
				w.DefaultSoulID = "workspace"
				w.SoulBindings = map[string]common.SoulID{"execution": "stage"}
			case "stage_all":
				w.SoulBindings = map[string]common.SoulID{}
				for _, stage := range appmodelcore.SoulBindingStages() {
					w.SoulBindings[stage] = "stage"
				}
			case "no_system_stage":
				systemDefault = ""
				w.SoulBindings = map[string]common.SoulID{"execution": "stage"}
			}
			plan, lookup, err := preparationSoulPlan(w, systemDefault)
			if err != nil {
				t.Fatal(err)
			}
			current, err := activationports.ResolveWorkspaceSoulRouting(context.Background(), activationports.WorkspaceSoulRoutingRequest{Hub: hub, Workspace: w, SystemDefaultSoulID: systemDefault})
			if err != nil {
				t.Fatal(err)
			}
			if (current.DefaultSoul == nil) != (lookup == "") {
				t.Fatal("default lookup behavior changed")
			}
			if current.DefaultSoul != nil && current.DefaultSoul.ID != lookup {
				t.Fatal("default identity changed")
			}
			for _, selected := range plan {
				binding, present := current.Routing.Bindings[selected.Stage]
				if !present {
					if selected.SoulID != "" || selected.Source != "unbound" {
						t.Fatal("unbound stage invented")
					}
					continue
				}
				if selected.SoulID != common.SoulID(binding.SoulID) || selected.Source != binding.Source {
					t.Fatal("routing priority changed")
				}
			}
		})
	}
	for _, w := range []*workspace.Workspace{nil, {DefaultSoulID: "../other"}, {SoulBindings: map[string]common.SoulID{"unknown": "soul"}}, {SoulBindings: map[string]common.SoulID{"execution": ""}}} {
		if _, _, err := preparationSoulPlan(w, ""); err == nil {
			t.Fatal("invalid bindings ignored")
		}
	}
}

func TestPreparationSoulSnapshotRejectsBadIdentityContentAndBounds(t *testing.T) {
	original := &soulmodel.Soul{ID: "soul", Revision: 2, Name: "Original name", SoulMD: "原始 instruction\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: []string{"a"}, AllowedToolKinds: []string{}}
	original.ContentHash = soulmodel.StableSoulContentHash(original)
	if !validPreparationSoul(original, "soul") {
		t.Fatal("valid fixture rejected")
	}
	cloned := clonePreparationSoul(original)
	if !reflect.DeepEqual(cloned, original) {
		t.Fatal("clone changed empty-list shape")
	}
	cloned.AllowedSkillRefs[0] = "changed"
	if original.AllowedSkillRefs[0] != "a" {
		t.Fatal("clone aliases source")
	}
	for _, mutate := range []func(*soulmodel.Soul){
		func(s *soulmodel.Soul) { s.ID = "other" }, func(s *soulmodel.Soul) { s.Revision = 0 }, func(s *soulmodel.Soul) { s.SourceProvenance = "unknown" },
		func(s *soulmodel.Soul) { s.ContentHash = "wrong" }, func(s *soulmodel.Soul) { s.SoulMD = "" }, func(s *soulmodel.Soul) { s.SoulMD = "\xff" },
		func(s *soulmodel.Soul) { s.Name = strings.Repeat("a", soulcommon.SoulNameMaxBytes+1) },
		func(s *soulmodel.Soul) { s.SoulMD = strings.Repeat("a", soulcommon.SoulInstructionMaxBytes+1) },
		func(s *soulmodel.Soul) { s.AllowedSkillRefs = make([]string, soulcommon.SoulListValueMaxCount+1) },
		func(s *soulmodel.Soul) { s.AllowedSkillRefs = []string{"\xff"} },
	} {
		item := clonePreparationSoul(original)
		mutate(item)
		if item.ContentHash != "wrong" {
			item.ContentHash = soulmodel.StableSoulContentHash(item)
		}
		if validPreparationSoul(item, "soul") {
			t.Fatal("invalid owner snapshot accepted")
		}
	}
	if validPreparationSoul(nil, "soul") {
		t.Fatal("nil owner snapshot accepted")
	}
}
