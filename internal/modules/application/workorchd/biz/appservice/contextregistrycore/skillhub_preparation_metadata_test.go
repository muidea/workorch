package contextregistrycore

import (
	"bytes"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"reflect"
	"strings"
	"testing"
)

func TestPreparationSkillMetadataDecodesYAMLWithoutRewritingRaw(t *testing.T) {
	raw := []byte("---\r\nname: '原始: 名称'\r\nversion: '1.0'\r\ndescription: >-\r\n  first line\r\n  second line\r\ntrigger_rules:\r\n  - 'quoted, comma'\r\n  - go\r\nrequired_tools: [shell, file]\r\nrequired_mcp_server_refs: server-a, server-b\r\nrequired_capability_refs: [cap-a]\r\nreference_refs: ['references/a.md']\r\nconstraints: []\r\nmetadata:\r\n  custom: {number: 1, enabled: true}\r\n---\r\n正文\x00\r\n")
	original := bytes.Clone(raw)
	value, err := skillhubpreparation.ParsePreparationSkillHubMetadata(raw)
	if err != nil || value.Name != "原始: 名称" || value.Version != "1.0" || value.Description != "first line second line" ||
		!reflect.DeepEqual(value.TriggerRules, []string{"quoted, comma", "go"}) || !reflect.DeepEqual(value.RequiredToolKinds, []string{"shell", "file"}) ||
		!reflect.DeepEqual(value.RequiredMCPServerRefs, []string{"server-a", "server-b"}) || !reflect.DeepEqual(value.RequiredCapabilityRefs, []string{"cap-a"}) ||
		!reflect.DeepEqual(value.ReferenceRefs, []string{"references/a.md"}) || len(value.Constraints) != 0 || !bytes.Equal(raw, original) {
		t.Fatalf("metadata or original bytes lost: %+v %v", value, err)
	}
	if value, err := skillhubpreparation.ParsePreparationSkillHubMetadata([]byte("headerless 原始 instruction\n")); err != nil || !reflect.DeepEqual(value, skillhubpreparation.SkillHubPreparationMetadata{}) {
		t.Fatal("headerless instruction convention changed")
	}
	if value, err := skillhubpreparation.ParsePreparationSkillHubMetadata([]byte("---\nversion: 1.0\n---\nbody")); err != nil || value.Version != "1.0" {
		t.Fatal("unquoted version scalar spelling changed")
	}
}

func TestPreparationSkillMetadataRejectsAmbiguityAndUnsupportedRequirements(t *testing.T) {
	for name, header := range map[string]string{
		"duplicate": "name: first\nname: second", "duplicate_nested": "metadata: {a: 1, a: 2}",
		"alias_key": "triggers: [go]\ntriggerRules: [python]", "case_alias": "RequiredToolKinds: [shell]",
		"merge": "<<: {name: inherited}", "anchor": "metadata: &a [go]", "alias": "triggers: *a",
		"custom_tag": "name: !custom abc", "binary": "name: !!binary YQ==", "numeric_name": "name: 123",
		"bool_version": "version: true", "bool_requirement": "required_tools: true", "null_requirement": "required_tools: null",
		"nested_requirement": "required_tools: [[shell]]", "object_requirement": "required_tools: {a: shell}",
		"numeric_requirement": "required_tools: [1]", "empty_requirement": "required_tools: ['']",
		"nonstring_key": "1: string", "sequence_root": "- name: sequence", "invalid_yaml": "name: [unterminated",
		"two_documents": "name: first\n...\nname: second", "deep": "metadata: " + strings.Repeat("[", 18) + "a" + strings.Repeat("]", 18),
		"nodes": "metadata: [" + strings.Repeat("a,", 4096) + "a]", "big_name": "name: " + strings.Repeat("a", 257),
		"big_description": "description: " + strings.Repeat("a", 4097), "many_rules": "triggers: [" + strings.Repeat("a,", 64) + "a]",
		"long_rule": "triggers: ['" + strings.Repeat("a", 2049) + "']",
	} {
		t.Run(name, func(t *testing.T) {
			raw := []byte("---\n" + header + "\n---\nbody")
			original := bytes.Clone(raw)
			if _, err := skillhubpreparation.ParsePreparationSkillHubMetadata(raw); err == nil || !bytes.Equal(raw, original) {
				t.Fatal("ambiguous metadata accepted or raw rewritten")
			}
		})
	}
	for _, raw := range [][]byte{nil, []byte("---\nname: unfinished"), []byte(" ---\nname: invalid delimiter\n---"), []byte("--- # unsupported header\nrequired_tools: [shell]\n---"), []byte("\xef\xbb\xbf---\nname: bom\n---"), []byte("bad\xff"), []byte(strings.Repeat("x", 65537))} {
		if _, err := skillhubpreparation.ParsePreparationSkillHubMetadata(raw); err == nil {
			t.Fatal("incomplete or unrepresentable metadata accepted")
		}
	}
}

func TestPreparationSkillMetadataInstalledVersionNeverUsesRepositoryFallback(t *testing.T) {
	item := SkillHubStatusItem{SkillID: "s", Version: "remote-v2", LocalVersion: "local-v1", RepoVersion: "repo-v3"}
	raw := []byte("---\nversion: installed-v0\n---\nraw")
	metadata, err := skillhubpreparation.ParsePreparationSkillHubMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	value := MapPreparationSkillHubItem(item, SkillScopeGlobal, "", SkillHubStatus{}, raw, metadata)
	if value.Version != "installed-v0" || value.Instruction != string(raw) || item.Version != "remote-v2" || item.LocalVersion != "local-v1" || item.RepoVersion != "repo-v3" {
		t.Fatal("status overrode installed metadata or diagnostic fields changed")
	}
	item.Version, item.LocalVersion = "", ""
	value = MapPreparationSkillHubItem(item, SkillScopeGlobal, "", SkillHubStatus{}, []byte("headerless"), skillhubpreparation.SkillHubPreparationMetadata{})
	if value.Version != "" {
		t.Fatal("repository version replaced missing installed version")
	}
}

func FuzzPreparationSkillMetadata(f *testing.F) {
	for _, raw := range []string{"headerless", "---\nname: sample\n---\nbody", "---\nrequired_tools: [shell]\n---\n", "---\na: &a [*a]\n---\n"} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		original := bytes.Clone(raw)
		_, _ = skillhubpreparation.ParsePreparationSkillHubMetadata(raw)
		if !bytes.Equal(raw, original) {
			t.Fatal("metadata parser changed original bytes")
		}
	})
}
