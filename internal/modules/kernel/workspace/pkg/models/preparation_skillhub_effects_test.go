package workspace

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func skillHubStatusFactsFixture() PreparationSkillHubStatusFacts {
	now := time.Now().UTC()
	raw := []byte("原始 stdout\xff\n")
	return PreparationSkillHubStatusFacts{Version: "workorch-skillhub-preparation-observation/v2", EffectContract: "skillhub-status-effects/v1", Scope: "global", StartedAt: now, CompletedAt: now, RawOutput: raw, TotalOutputBytes: uint64(len(raw)), RawSHA256: fileobservation.Digest(raw), StderrSHA256: fileobservation.Digest(nil), OutputComplete: true, StderrComplete: true, CommandStarted: true, ExitCodeKnown: true}
}

func TestPreparationSkillHubRawCaptureAndEffectStates(t *testing.T) {
	f := skillHubStatusFactsFixture()
	if f.ValidateFor("global", "") != nil || f.EffectsUnresolved() {
		t.Fatal("valid full capture rejected")
	}
	clone := f.Clone()
	clone.RawOutput[0] = 'X'
	if bytes.Equal(f.RawOutput, clone.RawOutput) {
		t.Fatal("clone aliases bytes")
	}
	for name, mutate := range map[string]func(*PreparationSkillHubStatusFacts){
		"hash":               func(f *PreparationSkillHubStatusFacts) { f.RawSHA256 = "wrong" },
		"count":              func(f *PreparationSkillHubStatusFacts) { f.TotalOutputBytes++ },
		"scope":              func(f *PreparationSkillHubStatusFacts) { f.Scope = "project" },
		"exit_without_start": func(f *PreparationSkillHubStatusFacts) { f.CommandStarted = false },
		"unknown_exit_value": func(f *PreparationSkillHubStatusFacts) { f.ExitCodeKnown = false; f.ExitCode = 2 },
		"negative_exit":      func(f *PreparationSkillHubStatusFacts) { f.ExitCode = -1 },
		"time":               func(f *PreparationSkillHubStatusFacts) { f.CompletedAt = f.StartedAt.Add(-time.Second) },
		"oversized": func(f *PreparationSkillHubStatusFacts) {
			f.RawOutput = make([]byte, PreparationSkillHubOutputLimit+1)
			f.TotalOutputBytes = uint64(len(f.RawOutput))
			f.RawSHA256 = fileobservation.Digest(f.RawOutput)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := f.Clone()
			mutate(&bad)
			if bad.ValidateFor("global", "") == nil {
				t.Fatal("invalid facts accepted")
			}
		})
	}
	for _, failure := range []string{"command_failed", "invalid_status", "command_incomplete"} {
		bad := f.Clone()
		bad.FailureCode = failure
		bad.ExitCode = 7
		if bad.ValidateFor("global", "") != nil || !bad.EffectsUnresolved() || bad.ExitCode != 7 || !bytes.Equal(bad.RawOutput, f.RawOutput) {
			t.Fatal("known failure lost or marked resolved")
		}
	}
	partial := f.Clone()
	partial.TotalOutputBytes++
	partial.OutputComplete = false
	if partial.ValidateFor("global", "") != nil || !partial.EffectsUnresolved() {
		t.Fatal("incomplete output falsely complete")
	}
	missing := PreparationSkillHubStatusFacts{Version: f.Version, EffectContract: f.EffectContract, Scope: "global", StartedAt: f.StartedAt, CompletedAt: f.CompletedAt, FailureCode: "not_started"}
	if missing.ValidateFor("global", "") != nil || missing.EffectsUnresolved() {
		t.Fatal("known not started changed")
	}
	plan := WorkspacePreparationSkillHubEffects{Version: PreparationSkillHubEffectsVersion, Ref: PreparationSkillHubRefPrefix + strings.Repeat("a", 32), WorkspaceID: "ws", RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, Binding: preparationRecordFixture().Attempts[0].Binding, ProjectDirectory: "delivery", ReservedAt: f.StartedAt}
	plan.ContentHash = plan.Hash()
	if plan.Validate() != nil || plan.Summary() != PreparationSkillHubReserved {
		t.Fatal("valid reservation rejected")
	}
	plan.CompletedAt = f.CompletedAt
	plan.Global = &f
	project := missing.Clone()
	project.Scope = "workspace"
	project.Directory = "delivery"
	plan.Project = &project
	plan.ContentHash = plan.Hash()
	if plan.Validate() != nil || plan.Summary() != PreparationSkillHubRecorded {
		t.Fatal("historical recorded state rejected")
	}
	plan.Project = nil
	plan.ContentHash = plan.Hash()
	if plan.Validate() != nil || plan.Summary() != PreparationSkillHubUnresolved {
		t.Fatal("missing receipt resolved")
	}
}
