package events

import (
	"strings"
	"testing"
	"time"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func registryCatalogFixture() ObserveRegistryCatalogResult {
	item := &skillmodel.Skill{ID: "skill-a", Name: "name", Version: "v1", Instruction: "原始指令\n第二行", TriggerRules: []string{"go"}}
	item.Provenance = workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: workorchskillcontract.StableContentHash(item.Content())}
	value := ObserveRegistryCatalogResult{Version: RegistryCatalogVersion, WorkspaceID: "ws", OwnerEpoch: "skill-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: time.Now().UTC(), Skills: []*skillmodel.Skill{item}}
	value.CatalogHash, _ = RegistryCatalogHash(value.Skills)
	return value
}

func TestRegistryCatalogValidatesFullRecordScopeOrderAndBounds(t *testing.T) {
	command := ObserveRegistryCatalogCommand{WorkspaceID: "ws"}
	base := registryCatalogFixture()
	if err := base.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"version", "scope", "epoch", "time", "missing_rows", "nil_row", "duplicate", "hash", "content", "provenance", "noncanonical_provenance", "noncanonical_content", "oversize_instruction", "invalid_utf8", "too_many"} {
		t.Run(scenario, func(t *testing.T) {
			value := base.Clone()
			switch scenario {
			case "version":
				value.Version = "other"
			case "scope":
				value.WorkspaceID = "other"
			case "epoch":
				value.OwnerEpoch = "skill-preparation-epoch:"
			case "time":
				value.ObservedAt = time.Time{}
			case "missing_rows":
				value.Skills = nil
			case "nil_row":
				value.Skills[0] = nil
			case "duplicate":
				value.Skills = append(value.Skills, skillmodel.CloneSkill(value.Skills[0]))
			case "hash":
				value.CatalogHash = strings.Repeat("a", 64)
			case "content":
				value.Skills[0].Instruction = "tampered"
			case "provenance":
				value.Skills[0].Provenance.Source = workorchskillcontract.SourceSkillHub
			case "noncanonical_provenance":
				value.Skills[0].Provenance.ContentHash = strings.ToUpper(value.Skills[0].Provenance.ContentHash)
			case "noncanonical_content":
				value.Skills[0].Instruction += " "
			case "oversize_instruction":
				value.Skills[0].Instruction = strings.Repeat("x", 65537)
			case "invalid_utf8":
				value.Skills[0].Description = "\xff"
			case "too_many":
				count := RegistryCatalogMaxItems + 1
				value.Skills = make([]*skillmodel.Skill, count)
			}
			// Recomputing a hash cannot make an invalid record valid.
			if scenario != "hash" {
				value.CatalogHash, _ = RegistryCatalogHash(value.Skills)
			}
			if value.ValidateFor(command) == nil {
				t.Fatal("malformed catalog accepted")
			}
		})
	}
	for _, id := range []string{"", " ws", "ws\nother", "\xff", strings.Repeat("x", 257)} {
		if (ObserveRegistryCatalogCommand{WorkspaceID: common.WorkspaceID(id)}).Validate() == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	clone := base.Clone()
	clone.Skills[0].TriggerRules[0] = "caller"
	if base.Skills[0].TriggerRules[0] != "go" {
		t.Fatal("result aliases source")
	}
	snapshot := base.Snapshot()
	if snapshot.ValidateFor(command.WorkspaceID) != nil || snapshot.CatalogHash != base.CatalogHash {
		t.Fatal("explicit model snapshot lost contract")
	}
	snapshot.Skills[0].Instruction = "caller"
	if base.Skills[0].Instruction != registryCatalogFixture().Skills[0].Instruction {
		t.Fatal("pure snapshot aliases event result")
	}
	empty := base.Clone()
	empty.Skills = []*skillmodel.Skill{}
	empty.CatalogHash, _ = RegistryCatalogHash(empty.Skills)
	if empty.ValidateFor(command) != nil {
		t.Fatal("observed empty registry rejected")
	}
}
