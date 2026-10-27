package skillhubpreparation

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func installedFixture() (Installed, Status) {
	raw := []byte("---\r\nname: 原文\r\nrequired_tools: [shell]\r\n---\r\nbody\x00\r\n")
	f := Instruction{SkillID: "s", Path: "/fixture/.agents/skills/s/SKILL.md", Raw: raw, RawSHA256: fileobservation.Digest(raw), ObservedSize: int64(len(raw)), ReadBytes: uint64(len(raw)), Complete: true}
	now := time.Now().UTC()
	return Installed{Version: InstalledVersion, Root: "/fixture/.agents/skills", First: []Instruction{f}, Second: []Instruction{f}, StartedAt: now, CompletedAt: now}, Status{Scope: "project", ProjectPath: "/fixture", Items: []StatusItem{{SkillID: "s", LocalPath: f.Path}}}
}

func TestInstalledFactsRequireBothActualReadsAndStrictMetadata(t *testing.T) {
	f, status := installedFixture()
	if !f.CompleteFor(status, "/fixture", false) {
		t.Fatal("full exact materials rejected")
	}
	for name, mutate := range map[string]func(*Installed){
		"missing_second": func(v *Installed) { v.Second = nil },
		"partial_second": func(v *Installed) { v.Second[0].Complete = false },
		"different_bytes": func(v *Installed) {
			v.Second[0].Raw[0] = 'X'
			v.Second[0].RawSHA256 = fileobservation.Digest(v.Second[0].Raw)
		},
		"hash":    func(v *Installed) { v.First[0].RawSHA256 = strings.Repeat("0", 64) },
		"path":    func(v *Installed) { v.First[0].Path = "/outside/SKILL.md" },
		"failure": func(v *Installed) { v.FailureCode = "root_drift" },
		"metadata": func(v *Installed) {
			for _, files := range [][]Instruction{v.First, v.Second} {
				files[0].Raw = []byte("---\nrequired_tools: true\n---\nbody")
				files[0].RawSHA256 = fileobservation.Digest(files[0].Raw)
				files[0].ReadBytes = uint64(len(files[0].Raw))
				files[0].ObservedSize = int64(len(files[0].Raw))
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := f.Clone()
			mutate(&v)
			if v.CompleteFor(status, "/fixture", false) {
				t.Fatal("incomplete or drifting material credited")
			}
		})
	}
	if !f.CompleteFor(status, "/fixture", false) {
		t.Fatal("clone aliased original")
	}
	partial := f.Clone()
	partial.Second = nil
	partial.FailureCode = "read_failed"
	if partial.Validate(status, "/fixture", false) != nil {
		t.Fatal("historical partial facts rejected")
	}
	partial.ErrorText = []byte("原始错误\xff")
	partial.ErrorBytes = uint64(len(partial.ErrorText))
	partial.ErrorSHA256 = fileobservation.Digest(partial.ErrorText)
	if partial.Validate(status, "/fixture", false) != nil || partial.CompleteFor(status, "/fixture", false) {
		t.Fatal("diagnostic lost or falsely complete")
	}
	clone := partial.Clone()
	clone.ErrorText[0] = 'X'
	if bytes.Equal(clone.ErrorText, partial.ErrorText) {
		t.Fatal("diagnostic alias")
	}
}

func TestInstalledEmptyInventoryRequiresBothReadRounds(t *testing.T) {
	now := time.Now().UTC()
	f := Installed{Version: InstalledVersion, Root: "/fixture/.agents/skills", StartedAt: now, CompletedAt: now, First: []Instruction{}, Second: []Instruction{}}
	s := Status{Scope: "project", ProjectPath: "/fixture", Items: []StatusItem{}}
	if !f.Clone().CompleteFor(s, "/fixture", false) {
		t.Fatal("empty observed rounds lost")
	}
	f.Second = nil
	if f.Clone().CompleteFor(s, "/fixture", false) {
		t.Fatal("unobserved empty inventory credited")
	}
}

func TestStatusRawAmbiguityAndScopeRejected(t *testing.T) {
	valid := []byte(`{"scope":"project","project_path":"/fixture","items":[],"skill_count":0,"unknown":{"原文":"retained"}}`)
	original := bytes.Clone(valid)
	if _, err := ParseStatus(valid, "/fixture", false); err != nil || !bytes.Equal(valid, original) {
		t.Fatal("raw changed or valid inventory rejected", err)
	}
	for _, raw := range []string{
		`{"scope":"project","Scope":"project","project_path":"/fixture","items":[]}`,
		`{"scope":"project","project_path":"/fixture","items":[],"x":{"a":1,"a":2}}`,
		`{"scope":"project","project_path":"/fixture","items":[],"x":"\ud800"}`,
		`{"scope":"project","project_path":"/fixture","items":null}`,
		`{"scope":"project","project_path":"/fixture","items":[],"skill_count":1}`,
		`{"scope":"global","items":[]}`,
		`{"scope":"project","project_path":"/other","items":[]}`,
		`{"scope":"project","project_path":"/fixture","items":[{"skill_id":"s"},{"skill_id":"s"}]}`,
		string(valid) + ` {}`, string(valid) + "\xff", strings.Repeat(" ", 1<<20) + string(valid),
	} {
		if _, err := ParseStatus([]byte(raw), "/fixture", false); err == nil {
			t.Fatalf("invalid inventory accepted: %.120s", raw)
		}
	}
}
