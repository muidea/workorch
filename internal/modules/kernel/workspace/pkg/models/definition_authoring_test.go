package workspace

import (
	"strings"
	"testing"
)

func TestDefinitionAuthoringAllowsGoalWithoutDoD(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "用户目标", TemplateID: "template"}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	if input.InputHash() == "" {
		t.Fatal("missing input hash")
	}
	// 创作输入不是正式候选，不能放宽原准入合同。
	if err := (DefinitionAdmissionCandidate{SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "ws", CandidateID: "candidate", CandidateRevision: 1, GoalInput: DefinitionCandidateItem{CandidateItemRef: "goal", Text: input.GoalText, SourceIdentity: "source"}}).Validate(); err == nil {
		t.Fatal("admission accepted absent DoD")
	}
}

func TestDefinitionAuthoringHashBindsScopeRevisionAndEveryInput(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "template", AcceptanceRequirements: []string{"甲", "乙"}, ResourceRefs: []string{"a", "b"}}
	changes := []func(*DefinitionAuthoringInput){
		func(i *DefinitionAuthoringInput) { i.WorkspaceID = "other" },
		func(i *DefinitionAuthoringInput) { i.Revision++ },
		func(i *DefinitionAuthoringInput) { i.GoalText += "修订" },
		func(i *DefinitionAuthoringInput) { i.TemplateID = "other" },
		func(i *DefinitionAuthoringInput) { i.AcceptanceRequirements[0] = "新约束" },
		func(i *DefinitionAuthoringInput) { i.AcceptanceRequirements = []string{"乙", "甲"} },
		func(i *DefinitionAuthoringInput) { i.ResourceRefs[0] = "c" },
	}
	original := input.InputHash()
	for n, change := range changes {
		next := input.Clone()
		change(&next)
		if next.InputHash() == original {
			t.Fatalf("change %d was not bound", n)
		}
		if input.InputHash() != original {
			t.Fatal("clone aliases input")
		}
	}
	padded := input.Clone()
	padded.GoalText = " 目标 "
	padded.ResourceRefs[0] = " a "
	if padded.InputHash() != original {
		t.Fatal("normalization changed semantic identity")
	}
}

func TestDefinitionAuthoringRejectsMalformedOrUnboundedInput(t *testing.T) {
	base := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "template"}
	changes := []func(*DefinitionAuthoringInput){
		func(i *DefinitionAuthoringInput) { i.Version = 2 },
		func(i *DefinitionAuthoringInput) { i.Revision = 0 },
		func(i *DefinitionAuthoringInput) { i.GoalText = " " },
		func(i *DefinitionAuthoringInput) { i.GoalText = string([]byte{0xff}) },
		func(i *DefinitionAuthoringInput) { i.GoalText = "a\x00b" },
		func(i *DefinitionAuthoringInput) { i.AcceptanceRequirements = []string{" "} },
		func(i *DefinitionAuthoringInput) { i.ResourceRefs = []string{"a", " a "} },
		func(i *DefinitionAuthoringInput) { i.ResourceRefs = make([]string, DefinitionAuthoringResourceLimit+1) },
		func(i *DefinitionAuthoringInput) {
			for range 128 {
				i.AcceptanceRequirements = append(i.AcceptanceRequirements, strings.Repeat("a", 4096))
			}
		},
	}
	for n, change := range changes {
		next := base.Clone()
		change(&next)
		if next.Validate() == nil {
			t.Fatalf("invalid input %d accepted", n)
		}
	}
}
