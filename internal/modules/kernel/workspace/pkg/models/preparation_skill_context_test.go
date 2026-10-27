package workspace

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func preparationContextSkill(id, instruction string) PreparationSelectedSkill {
	c := workorchskillcontract.Content{ID: id, Name: "go", Instruction: instruction, TriggerRules: []string{"go"}, ReferenceRefs: []string{"refs/a.md"}, RequiredToolKinds: []string{"shell"}, RequiredMCPServerRefs: []string{"mcp-a"}, RequiredCapabilityRefs: []string{"cap-a"}, Constraints: []string{"original"}}
	return PreparationSelectedSkill{Content: c, Provenance: workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: workorchskillcontract.StableContentHash(c)}}
}

func TestPreparationSkillContextUsesExistingBudgetWithoutReplacingOriginals(t *testing.T) {
	selected := []PreparationSelectedSkill{}
	for i := range 5 {
		selected = append(selected, preparationContextSkill(fmt.Sprintf("s%d", i), strings.Repeat("界", 9000)+"\x00\r\n"))
	}
	original := (PreparationSkillSelectionCompilation{Selected: selected}).Clone().Selected
	value := CompilePreparationSkillContext(selected)
	if value.Version != PreparationSkillContextVersion || value.Summary() != "bounded_checks_pending" || value.Budget == nil || value.Budget.TruncatedSkills != 5 || value.Budget.IncludedInstructionBytes > 24*1024 || len(value.Projection) != 5 || !reflect.DeepEqual(selected, original) {
		t.Fatal("budget policy drifted or source material changed")
	}
	if runtimecontract.ValidateRuntimeSkillContexts(value.Projection, value.Budget) != nil {
		t.Fatal("projection does not satisfy existing contract")
	}
	for _, p := range value.Projection {
		if len(p.Instruction) > 8*1024 || !utf8.ValidString(p.Instruction) || !p.InstructionTruncated || !strings.HasPrefix(original[0].Content.Instruction, p.Instruction) {
			t.Fatal("invalid bounded projection")
		}
	}
	clone := value.Clone()
	clone.Budget.TruncatedSkills = 0
	clone.Projection[0].TriggerRules[0] = "caller"
	clone.Projection[0].ReferenceRefs[0] = "caller"
	clone.Projection[0].RequiredToolKinds[0] = "caller"
	clone.Projection[0].RequiredMCPServerRefs[0] = "caller"
	clone.Projection[0].RequiredCapabilityRefs[0] = "caller"
	clone.Projection[0].Constraints[0] = "caller"
	if !reflect.DeepEqual(value, CompilePreparationSkillContext(selected)) {
		t.Fatal("context clone aliases original")
	}
	small := CompilePreparationSkillContext([]PreparationSelectedSkill{preparationContextSkill("s", "original\x00\r\n")})
	if small.Summary() != "compiled_checks_pending" || small.Budget == nil || small.Budget.TruncatedSkills != 0 || small.Projection[0].Instruction != "original\x00" {
		t.Fatal("existing explicit context normalization changed")
	}
	empty := CompilePreparationSkillContext(nil)
	if empty.Summary() != "compiled_checks_pending" || empty.Budget != nil || empty.Projection != nil {
		t.Fatal("existing empty-context contract changed")
	}
}

func TestPreparationSkillContextRejectsInvalidOrExcessMaterialAndRetainsDiagnostic(t *testing.T) {
	for name, selected := range map[string][]PreparationSelectedSkill{
		"empty_instruction":  {preparationContextSkill("s", " \r\n")},
		"invalid_utf8":       {preparationContextSkill("s", "\xff")},
		"duplicate":          {preparationContextSkill("s", "a"), preparationContextSkill("s", "b")},
		"too_many":           {preparationContextSkill("1", "a"), preparationContextSkill("2", "a"), preparationContextSkill("3", "a"), preparationContextSkill("4", "a"), preparationContextSkill("5", "a"), preparationContextSkill("6", "a")},
		"missing_provenance": {{Content: workorchskillcontract.Content{ID: "s", Instruction: "a"}}},
	} {
		t.Run(name, func(t *testing.T) {
			value := CompilePreparationSkillContext(selected)
			if value.Summary() != "rejected" || len(value.Projection) != 0 || value.Budget != nil || len(value.Diagnostic) == 0 || value.DiagnosticBytes != uint64(len(value.Diagnostic)) || value.DiagnosticSHA256 != fileobservation.Digest(value.Diagnostic) {
				t.Fatal("bad material credited or diagnostic lost")
			}
			copy := value.Clone()
			copy.Diagnostic[0] = '!'
			if bytes.Equal(copy.Diagnostic, value.Diagnostic) {
				t.Fatal("diagnostic aliases clone")
			}
		})
	}
	large := preparationContextSkill("s", "a")
	large.Content.Description = strings.Repeat("d", 16*1024+1)
	large.Provenance.ContentHash = workorchskillcontract.StableContentHash(large.Content)
	if CompilePreparationSkillContext([]PreparationSelectedSkill{large}).Summary() != "rejected" {
		t.Fatal("metadata ceiling widened")
	}
	largeID := preparationContextSkill(strings.Repeat("i", PreparationSkillHubErrorLimit+1), "")
	value := CompilePreparationSkillContext([]PreparationSelectedSkill{largeID})
	if value.Summary() != "rejected" || len(value.Diagnostic) != 0 || value.DiagnosticBytes <= PreparationSkillHubErrorLimit || value.DiagnosticNotSavedReason != "error_exceeds_storage_limit" || len(value.DiagnosticSHA256) != 64 {
		t.Fatal("oversized diagnostic silently truncated or marked saved")
	}
}
