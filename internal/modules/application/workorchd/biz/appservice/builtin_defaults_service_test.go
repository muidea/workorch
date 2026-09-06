package appservice

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type builtinReplyHub struct {
	magicevent.Hub
	send func(magicevent.Event) magicevent.Result
}

func (h builtinReplyHub) Send(ev magicevent.Event) magicevent.Result { return h.send(ev) }

func builtinReply(ev magicevent.Event, value any) magicevent.Result {
	result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
	result.Set(value, nil)
	return result
}

func TestBuiltinDefaultsRealOwnerWiringAndReload(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	for attempt := 0; attempt < 2; attempt++ {
		session, err := NewRuntimeSessionWithBackend("file", dataRoot)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer session.Close(ctx)
			items, err := ListBuiltinDefaultsWithSession(ctx, session.Borrow())
			if err != nil || len(items) != 42 {
				t.Fatalf("builtin preview count=%d err=%v", len(items), err)
			}
			kinds := map[string]bool{}
			for _, item := range items {
				if !item.Present || item.UpgradeAvailable || item.CurrentContentHash != item.BuiltinContentHash {
					t.Fatalf("fresh default: %+v", item)
				}
				if kinds[item.Kind] {
					continue
				}
				kinds[item.Kind] = true
				result, err := UpgradeBuiltinDefaultWithSession(ctx, session.Borrow(), workorchmanagementcontract.BuiltinDefaultUpgrade{
					Kind: item.Kind, ID: item.ID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash,
				})
				if err != nil || result.Upgraded || result.Default.ID != item.ID {
					t.Fatalf("current builtin replay for %s/%s: %+v %v", item.Kind, item.ID, result, err)
				}
			}
		}()
	}
}

func TestBuiltinUpgradeRequiresExactOwnerReply(t *testing.T) {
	for _, kind := range []string{"soul", "workspace_template"} {
		for _, failure := range []string{"nil", "wrong type", "wrong id", "wrong hash", "not present", "still upgradeable", "valid"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				calls := 0
				hub := builtinReplyHub{send: func(ev magicevent.Event) magicevent.Result {
					calls++
					if failure == "nil" {
						return nil
					}
					if failure == "wrong type" {
						return builtinReply(ev, map[string]any{"upgraded": true})
					}
					id, hash, present, available := "builtin-1", "target", true, false
					switch failure {
					case "wrong id":
						id = "builtin-other"
					case "wrong hash":
						hash = "other-target"
					case "not present":
						present = false
					case "still upgradeable":
						available = true
					}
					if kind == "soul" {
						cmd, ok := ev.Data().(soulevents.UpgradeBuiltinSoulDefaultCommand)
						if !ok || ev.ID() != soulevents.TopicSoulCommandUpgradeBuiltinDefault || ev.Source() != "application/workorchd/context-registry" || cmd.ExpectedContentHash != "old" {
							t.Fatalf("wrong Soul command: %#v", ev.Data())
						}
						return builtinReply(ev, soulevents.UpgradeBuiltinSoulDefaultResult{Default: soulevents.BuiltinSoulDefault{SoulID: common.SoulID(id), Present: present, CurrentContentHash: hash, BuiltinContentHash: hash, UpgradeAvailable: available}, Upgraded: true})
					}
					cmd, ok := ev.Data().(workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultCommand)
					if !ok || ev.ID() != workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault || ev.Source() != "application/workorchd/appservice" || cmd.ExpectedContentHash != "old" {
						t.Fatalf("wrong Workspace command: %#v", ev.Data())
					}
					return builtinReply(ev, workspaceevents.UpgradeBuiltinWorkspaceTemplateDefaultResult{Default: workspaceevents.BuiltinWorkspaceTemplateDefault{TemplateID: id, Present: present, CurrentContentHash: hash, BuiltinContentHash: hash, UpgradeAvailable: available}, Upgraded: true})
				}}
				_, err := upgradeBuiltinDefault(context.Background(), hub, workorchmanagementcontract.BuiltinDefaultUpgrade{Kind: kind, ID: "builtin-1", ExpectedContentHash: "old", TargetContentHash: "target"})
				if (err == nil) != (failure == "valid") || calls != 1 {
					t.Fatalf("reply failure=%s err=%v calls=%d", failure, err, calls)
				}
			})
		}
	}
}

func TestBuiltinQueryRejectsMissingMalformedAndDuplicateOwnerResults(t *testing.T) {
	for _, failure := range []string{"nil", "wrong type", "empty", "duplicate", "oversized", "invalid observation", "valid"} {
		t.Run(failure, func(t *testing.T) {
			hub := builtinReplyHub{send: func(ev magicevent.Event) magicevent.Result {
				if ev.ID() == workspaceevents.TopicWorkspaceTemplateCommandQueryBuiltinDefaults {
					return builtinReply(ev, workspaceevents.QueryBuiltinWorkspaceTemplateDefaultsResult{Defaults: []workspaceevents.BuiltinWorkspaceTemplateDefault{{TemplateID: "template", BuiltinContentHash: "target"}}})
				}
				if failure == "nil" {
					return nil
				}
				if failure == "wrong type" {
					return builtinReply(ev, struct{}{})
				}
				items := []soulevents.BuiltinSoulDefault{{SoulID: "soul", BuiltinContentHash: "target"}}
				switch failure {
				case "empty":
					items = nil
				case "duplicate":
					items = append(items, items[0])
				case "oversized":
					items = make([]soulevents.BuiltinSoulDefault, 201)
				case "invalid observation":
					items[0].Present = true
				}
				return builtinReply(ev, soulevents.QueryBuiltinSoulDefaultsResult{Defaults: items})
			}}
			_, err := queryBuiltinDefaults(context.Background(), hub)
			if (err == nil) != (failure == "valid") {
				t.Fatalf("failure=%s err=%v", failure, err)
			}
		})
	}
}
