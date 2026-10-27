package workorchmanagedconfig

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const PreparationPolicyVersion = "workorch-managed-config-observation/v1"
const ScopeSystem = "system"
const ScopeWorkspace = "workspace"
const PolicySystemExact = "system_exact"
const PolicyWorkspacePreserve = "workspace_preserve"
const PolicyWorkspaceMergeMissing = "workspace_merge_missing"

// PreparationDeclaration is a detached, pure interpretation of an already
// confirmed declaration. It never writes/merges a file or chooses a fallback.
type PreparationDeclaration struct {
	Scope, Path, Policy string
	Defaults            payload.Payload
}

func (d PreparationDeclaration) Clone() PreparationDeclaration {
	d.Defaults = d.Defaults.Clone()
	return d
}

func ValidPreparationTarget(scope, path string) bool {
	clean, err := CleanPath(path)
	return (scope == ScopeSystem || scope == ScopeWorkspace) && err == nil && clean == path && filepath.ToSlash(path) == path && len(path) <= 1024 && utf8.ValidString(path) && !strings.ContainsAny(path, "\x00\\:")
}

func (d PreparationDeclaration) Validate() error {
	if !ValidPreparationTarget(d.Scope, d.Path) || d.Scope == ScopeSystem && d.Policy != PolicySystemExact || d.Scope == ScopeWorkspace && d.Policy != PolicyWorkspacePreserve && d.Policy != PolicyWorkspaceMergeMissing || !validPreparationValue(payload.Value{Kind: payload.ValueObject, Object: d.Defaults.Fields}, 0) {
		return common.ErrInvalidArgument
	}
	raw, err := json.Marshal(d.Defaults)
	if err != nil || len(raw) > fileobservation.ResourceMaxBytes {
		return common.ErrInvalidArgument
	}
	return nil
}

// Unlike the old materializer, malformed or ambiguous declarations are not
// silently skipped. Order is preserved independently in the two original roots.
func CompilePreparationDeclarations(system, workspace payload.Payload) ([]PreparationDeclaration, error) {
	var result []PreparationDeclaration
	total := 0
	for index, hints := range []payload.Payload{system, workspace} {
		declarations := 0
		for _, field := range hints.Fields {
			if field.Name == "managedWorkspaceConfigs" {
				declarations++
			}
		}
		if declarations > 1 {
			return nil, common.ErrInvalidArgument
		}
		value, found := hints.Field("managedWorkspaceConfigs")
		if !found {
			continue
		}
		if value.Kind != payload.ValueArray || len(value.Array) > fileobservation.ResourceLimit {
			return nil, common.ErrInvalidArgument
		}
		seen := make(map[string]bool)
		for _, item := range value.Array {
			if item.Kind != payload.ValueObject || !validPreparationValue(item, 0) {
				return nil, common.ErrInvalidArgument
			}
			p := payload.New(item.Object...)
			for _, field := range p.Fields {
				if field.Name != "path" && field.Name != "format" && field.Name != "mergeMissing" && field.Name != "content" {
					return nil, common.ErrInvalidArgument
				}
			}
			path, ok := p.String("path")
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			format, present := p.Field("format")
			if present && (format.Kind != payload.ValueString || strings.TrimSpace(format.String) != "" && !strings.EqualFold(strings.TrimSpace(format.String), "json")) {
				return nil, common.ErrInvalidArgument
			}
			merge, present := p.Field("mergeMissing")
			if present && merge.Kind != payload.ValueBool {
				return nil, common.ErrInvalidArgument
			}
			defaults, ok := p.Object("content")
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			d := PreparationDeclaration{Scope: ScopeSystem, Path: path, Policy: PolicySystemExact, Defaults: defaults}
			if index == 1 {
				d.Scope, d.Policy = ScopeWorkspace, PolicyWorkspacePreserve
				if merge.Bool {
					d.Policy = PolicyWorkspaceMergeMissing
				}
			}
			if d.Validate() != nil || seen[path] {
				return nil, common.ErrInvalidArgument
			}
			seen[path] = true
			raw, _ := json.Marshal(defaults)
			total += len(raw)
			result = append(result, d)
			if len(result) > fileobservation.ResourceLimit || total > fileobservation.ResourceTotalBytes {
				return nil, common.ErrInvalidArgument
			}
		}
	}
	return result, nil
}

func validPreparationValue(v payload.Value, depth int) bool {
	if depth > 32 {
		return false
	}
	switch v.Kind {
	case payload.ValueNull, payload.ValueBool, payload.ValueInt, payload.ValueUint:
		return true
	case payload.ValueFloat:
		return !math.IsNaN(v.Float) && !math.IsInf(v.Float, 0)
	case payload.ValueString:
		return utf8.ValidString(v.String)
	case payload.ValueObject:
		seen := make(map[string]bool)
		for _, field := range v.Object {
			if !utf8.ValidString(field.Name) || seen[field.Name] || !validPreparationValue(field.Value, depth+1) {
				return false
			}
			seen[field.Name] = true
		}
		return true
	case payload.ValueArray:
		for _, item := range v.Array {
			if !validPreparationValue(item, depth+1) {
				return false
			}
		}
		return true
	}
	return false
}

// PreparationContentReason only checks the read bytes. Raw content is never
// normalized or replaced. Valid preserved overrides need not equal defaults;
// mergeMissing checks whether the original merge would add any missing keys.
func PreparationContentReason(d PreparationDeclaration, raw []byte) string {
	if d.Validate() != nil {
		return "managed_config_declaration_invalid"
	}
	encoded, err := json.Marshal(d.Defaults)
	if err != nil {
		return "managed_config_declaration_invalid"
	}
	return PreparationJSONReason(d.Policy, encoded, raw)
}

// PreparationJSONReason is also used to validate private persisted evidence.
// Defaults are stored as exact bytes, not decoded through float64 on reopen.
func PreparationJSONReason(policy string, defaults, raw []byte) string {
	if policy != PolicySystemExact && policy != PolicyWorkspacePreserve && policy != PolicyWorkspaceMergeMissing {
		return "managed_config_declaration_invalid"
	}
	expected, err := parsePreparationObject(defaults)
	if err != nil {
		return "managed_config_declaration_invalid"
	}
	actual, err := parsePreparationObject(raw)
	if err != nil {
		return "managed_config_content_invalid"
	}
	if policy == PolicySystemExact && !equalPreparationJSON(actual, expected) || policy == PolicyWorkspaceMergeMissing && !hasPreparationDefaults(actual, expected) {
		return "managed_config_content_conflict"
	}
	return "managed_configs_observed_remaining_checks"
}
