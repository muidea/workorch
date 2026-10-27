package workorchmanagedconfig

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func preparationHints(fields ...payload.Field) payload.Payload {
	return payload.New(payload.Field{Name: "managedWorkspaceConfigs", Value: payload.Value{Kind: payload.ValueArray, Array: []payload.Value{{Kind: payload.ValueObject, Object: fields}}}})
}

func TestPreparationPoliciesPreserveOriginalSemanticsAndPreciseNumbers(t *testing.T) {
	defaults := payload.New(payload.IntField("n", 9007199254740993), payload.ObjectField("nested", payload.New(payload.IntField("x", 1))))
	for _, tc := range []struct{ policy, raw, want string }{
		{PolicySystemExact, `{"nested":{"x":1.0},"n":9007199254740993e0}`, "managed_configs_observed_remaining_checks"},
		{PolicySystemExact, `{"nested":{"x":1},"n":9007199254740992}`, "managed_config_content_conflict"},
		{PolicySystemExact, `{"nested":{"x":1},"n":9007199254740993,"extra":true}`, "managed_config_content_conflict"},
		{PolicyWorkspacePreserve, `{"override":true}`, "managed_configs_observed_remaining_checks"},
		{PolicyWorkspaceMergeMissing, `{"n":null,"nested":{"x":false,"extra":1}}`, "managed_configs_observed_remaining_checks"},
		{PolicyWorkspaceMergeMissing, `{"n":0,"nested":null}`, "managed_configs_observed_remaining_checks"},
		{PolicyWorkspaceMergeMissing, `{"n":0,"nested":[]}`, "managed_configs_observed_remaining_checks"},
		{PolicyWorkspaceMergeMissing, `{"n":0,"nested":{}}`, "managed_config_content_conflict"},
		{PolicyWorkspaceMergeMissing, `{"nested":{"x":1}}`, "managed_config_content_conflict"},
		{PolicyWorkspacePreserve, `{"x":1,"x":2}`, "managed_config_content_invalid"},
		{PolicyWorkspacePreserve, `null`, "managed_config_content_invalid"},
		{PolicyWorkspacePreserve, `[]`, "managed_config_content_invalid"},
		{PolicyWorkspacePreserve, `{} {}`, "managed_config_content_invalid"},
		{PolicyWorkspacePreserve, `{"x":"\ud800"}`, "managed_config_content_invalid"},
		{PolicyWorkspacePreserve, `{"x":"\udc00"}`, "managed_config_content_invalid"},
		{PolicyWorkspacePreserve, `{"x":"\ud83d\ude00"}`, "managed_configs_observed_remaining_checks"},
		{PolicyWorkspacePreserve, `{"x":"\\ud800"}`, "managed_configs_observed_remaining_checks"},
		{PolicyWorkspacePreserve, `{"x":1e99999999999999}`, "managed_config_content_invalid"},
	} {
		scope := ScopeWorkspace
		if tc.policy == PolicySystemExact {
			scope = ScopeSystem
		}
		d := PreparationDeclaration{Scope: scope, Path: "a.json", Policy: tc.policy, Defaults: defaults}
		raw := []byte(tc.raw)
		original := bytes.Clone(raw)
		if got := PreparationContentReason(d, raw); got != tc.want {
			t.Errorf("%s %s: got %s want %s", tc.policy, tc.raw, got, tc.want)
		}
		if !bytes.Equal(raw, original) {
			t.Fatal("modified original content")
		}
	}
	for _, raw := range [][]byte{{0xff}, bytes.Repeat([]byte(" "), fileobservation.ResourceMaxBytes+1), []byte(`{"a":` + strings.Repeat(`[`, 33) + `0` + strings.Repeat(`]`, 33) + `}`)} {
		if _, err := parsePreparationObject(raw); err == nil {
			t.Fatal("unbounded/invalid JSON accepted")
		}
	}
	for _, pair := range [][2]string{{"-0", "0.00"}, {"1e3", "1000.0"}, {"0.0010", "1e-3"}} {
		a, _ := preparationJSONNumber(pair[0])
		b, _ := preparationJSONNumber(pair[1])
		if a != b {
			t.Fatal("numeric equivalent differs", pair)
		}
	}
}

func TestCompilePreparationDeclarationsStrictSourcesAndBounds(t *testing.T) {
	fields := []payload.Field{payload.StringField("path", "mcp/config.json"), payload.ObjectField("content", payload.New(payload.StringField("value", "original")))}
	system, workspace := preparationHints(fields...), preparationHints(append(fields, payload.BoolField("mergeMissing", true))...)
	compiled, err := CompilePreparationDeclarations(system, workspace)
	if err != nil || len(compiled) != 2 || compiled[0].Policy != PolicySystemExact || compiled[1].Policy != PolicyWorkspaceMergeMissing {
		t.Fatal("sources mixed", err)
	}
	compiled[0].Defaults.Fields[0].Value.String = "changed"
	again, _ := CompilePreparationDeclarations(system, workspace)
	if value, _ := again[0].Defaults.String("value"); value != "original" {
		t.Fatal("declaration aliases source")
	}
	if empty, err := CompilePreparationDeclarations(payload.Payload{}, payload.Payload{}); err != nil || len(empty) != 0 {
		t.Fatal("empty declaration rejected")
	}
	for _, bad := range []payload.Payload{
		payload.New(payload.StringField("managedWorkspaceConfigs", "wrong")),
		preparationHints(payload.StringField("path", "../escape"), fields[1]),
		preparationHints(payload.StringField("path", "x/../a"), fields[1]),
		preparationHints(payload.StringField("path", " a"), fields[1]),
		preparationHints(append(fields, payload.StringField("format", "yaml"))...),
		preparationHints(append(fields, payload.StringField("mergeMissing", "yes"))...),
		preparationHints(append(fields, payload.StringField("unknown", "bad"))...),
		preparationHints(fields[0]),
		preparationHints(append(fields, fields[0])...),
		preparationHints(fields[0], payload.ObjectField("content", payload.New(payload.StringField("v", string([]byte{0xff}))))),
		preparationHints(fields[0], payload.ObjectField("content", payload.New(payload.StringField("v", strings.Repeat("x", fileobservation.ResourceMaxBytes))))),
	} {
		if _, err := CompilePreparationDeclarations(bad, payload.Payload{}); err == nil {
			t.Fatal("invalid declaration accepted")
		}
	}
	duplicate := system.Clone()
	duplicate.Fields[0].Value.Array = append(duplicate.Fields[0].Value.Array, duplicate.Fields[0].Value.Array[0])
	if _, err := CompilePreparationDeclarations(duplicate, payload.Payload{}); err == nil {
		t.Fatal("duplicate scope/path accepted")
	}
}
