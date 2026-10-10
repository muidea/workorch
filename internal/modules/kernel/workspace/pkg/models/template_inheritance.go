package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	maxTemplateDepth   = 8
	maxTemplateNodes   = 32
	maxTemplateBytes   = 256 << 10
	maxTemplateEntries = 256
)

// TemplateSource identifies each raw declaration used to build an effective
// snapshot. The order is root-to-leaf and hashes bind parent edits on restart.
type TemplateSource struct {
	TemplateID  string `json:"templateId"`
	ContentHash string `json:"contentHash"`
}

// ResolveWorkspaceTemplate takes only owner-owned raw records. It never reads
// other owners and never changes stored declarations or old startup snapshots.
func ResolveWorkspaceTemplate(id string, load func(string) (*WorkspaceTemplate, error)) (*WorkspaceTemplate, error) {
	if strings.TrimSpace(id) == "" || load == nil {
		return nil, common.ErrInvalidArgument
	}
	state := make(map[string]uint8)
	heights := make(map[string]int)
	ordered := make([]*WorkspaceTemplate, 0, maxTemplateNodes)
	totalBytes := 0
	var visit func(string, int) error
	visit = func(current string, depth int) error {
		if state[current] == 1 {
			return fmt.Errorf("%w: Template parent cycle", common.ErrConflict)
		}
		if state[current] == 2 {
			// 共享祖先无需重新合并，但更深路径仍须满足完整链深度上限。
			if depth+heights[current]-1 > maxTemplateDepth {
				return fmt.Errorf("%w: Template inheritance depth limit", common.ErrConflict)
			}
			return nil
		}
		if depth > maxTemplateDepth {
			return fmt.Errorf("%w: Template inheritance depth limit", common.ErrConflict)
		}
		if len(state) >= maxTemplateNodes {
			return fmt.Errorf("%w: Template node limit", common.ErrConflict)
		}
		state[current] = 1
		item, err := load(current)
		if err != nil {
			return fmt.Errorf("%w: Template parent %q unavailable: %v", common.ErrConflict, current, err)
		}
		if item == nil || item.ID != current || item.EffectiveSources != nil || StableWorkspaceTemplateContentHash(item) == "" {
			return common.ErrConflict
		}
		if len(item.ParentTemplateIDs) > maxTemplateNodes {
			return fmt.Errorf("%w: too many Template parents", common.ErrConflict)
		}
		encoded, err := json.Marshal(item)
		if err != nil || len(encoded) > maxTemplateBytes || totalBytes+len(encoded) > maxTemplateBytes || len(item.DoDItems)+len(item.SoulBindings)+len(item.ContextHints.Fields)+len(item.EvidencePolicies) > maxTemplateEntries {
			return fmt.Errorf("%w: Template size limit", common.ErrConflict)
		}
		totalBytes += len(encoded)
		parents := map[string]bool{}
		heights[current] = 1
		for _, parent := range item.ParentTemplateIDs {
			if parent == "" || strings.TrimSpace(parent) != parent || parents[parent] {
				return fmt.Errorf("%w: invalid or duplicate Template parent", common.ErrConflict)
			}
			parents[parent] = true
			if err := visit(parent, depth+1); err != nil {
				return err
			}
			heights[current] = max(heights[current], heights[parent]+1)
		}
		state[current] = 2
		ordered = append(ordered, item)
		if len(ordered) > maxTemplateNodes {
			return fmt.Errorf("%w: Template node limit", common.ErrConflict)
		}
		return nil
	}
	if err := visit(id, 1); err != nil {
		return nil, err
	}
	// Merge raw declarations in stable DFS topological order. Never merge an
	// already-resolved parent: shared diamond ancestors must not duplicate DoD.
	var effective *WorkspaceTemplate
	for _, raw := range ordered {
		var err error
		effective, err = mergeWorkspaceTemplate(effective, raw)
		if err != nil {
			return nil, err
		}
		effective.EffectiveSources = append(effective.EffectiveSources, TemplateSource{TemplateID: raw.ID, ContentHash: StableWorkspaceTemplateContentHash(raw)})
		encoded, err := json.Marshal(effective)
		if err != nil || len(encoded) > maxTemplateBytes || len(effective.DoDItems)+len(effective.SoulBindings)+len(effective.ContextHints.Fields)+len(effective.EvidencePolicies) > maxTemplateEntries {
			return nil, fmt.Errorf("%w: effective Template size limit", common.ErrConflict)
		}
	}
	return effective, nil
}

func mergeWorkspaceTemplate(parent, child *WorkspaceTemplate) (*WorkspaceTemplate, error) {
	merged := *child
	merged.ParentTemplateIDs = append([]string(nil), child.ParentTemplateIDs...)
	merged.DoDItems = append([]string(nil), child.DoDItems...)
	merged.SoulBindings = cloneTemplateBindings(child.SoulBindings)
	merged.ContextHints = child.ContextHints.Clone()
	merged.EvidencePolicies = workorcheventpayload.CloneSlice(child.EvidencePolicies)
	merged.EffectiveSources = nil
	if parent == nil {
		return &merged, nil
	}
	merged.EffectiveSources = append([]TemplateSource(nil), parent.EffectiveSources...)
	var err error
	if merged.GoalText, err = inheritTemplateString(parent.GoalText, child.GoalText); err != nil {
		return nil, err
	}
	if merged.DefaultSoulID, err = inheritTemplateString(parent.DefaultSoulID, child.DefaultSoulID); err != nil {
		return nil, err
	}
	if len(parent.DoDItems) > 0 && len(child.DoDItems) > 0 {
		// DoD requirements are conjunctive, not replaceable. Never silently drop
		// a required parent item, even when the child happens to reorder it.
		merged.DoDItems = append(append([]string(nil), parent.DoDItems...), child.DoDItems...)
	} else if len(child.DoDItems) == 0 {
		merged.DoDItems = append([]string(nil), parent.DoDItems...)
	}
	for stage, soulID := range parent.SoulBindings {
		if value, exists := merged.SoulBindings[stage]; exists && value != soulID {
			return nil, fmt.Errorf("%w: conflicting Template soul binding %q", common.ErrConflict, stage)
		}
		if merged.SoulBindings == nil {
			merged.SoulBindings = make(map[string]string)
		}
		merged.SoulBindings[stage] = soulID
	}
	merged.ContextHints, err = mergeTemplateHints(parent.ContextHints, child.ContextHints)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]string)
	merged.EvidencePolicies = workorcheventpayload.CloneSlice(parent.EvidencePolicies)
	for _, policy := range merged.EvidencePolicies {
		id, ok := policy.String("id")
		if !ok || strings.TrimSpace(id) == "" || seen[id] != "" {
			return nil, fmt.Errorf("%w: inherited evidence policy needs unique id", common.ErrConflict)
		}
		seen[id] = policy.CanonicalString()
	}
	for _, policy := range child.EvidencePolicies {
		id, ok := policy.String("id")
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("%w: child evidence policy needs unique id", common.ErrConflict)
		}
		if old, exists := seen[id]; exists {
			if old != policy.CanonicalString() {
				return nil, fmt.Errorf("%w: evidence policy %q conflict (required rules cannot be weakened)", common.ErrConflict, id)
			}
			continue
		}
		seen[id] = policy.CanonicalString()
		merged.EvidencePolicies = append(merged.EvidencePolicies, policy.Clone())
	}
	return &merged, nil
}

func inheritTemplateString(parent, child string) (string, error) {
	if child == "" {
		return parent, nil
	}
	if parent != "" && parent != child {
		return "", fmt.Errorf("%w: incompatible Template inherited value", common.ErrConflict)
	}
	return child, nil
}

func mergeTemplateHints(parent, child workorcheventpayload.Payload) (workorcheventpayload.Payload, error) {
	merged := parent.Clone()
	for _, field := range child.Fields {
		found := false
		for idx := range merged.Fields {
			if merged.Fields[idx].Name != field.Name {
				continue
			}
			found = true
			if merged.Fields[idx].Value.Kind == workorcheventpayload.ValueObject && field.Value.Kind == workorcheventpayload.ValueObject {
				nested, err := mergeTemplateHints(workorcheventpayload.Payload{Fields: merged.Fields[idx].Value.Object}, workorcheventpayload.Payload{Fields: field.Value.Object})
				if err != nil {
					return workorcheventpayload.Payload{}, err
				}
				merged.Fields[idx].Value.Object = nested.Fields
			} else if !merged.Fields[idx].Value.Equal(field.Value) {
				return workorcheventpayload.Payload{}, fmt.Errorf("%w: incompatible Template context key %q", common.ErrConflict, field.Name)
			}
			break
		}
		if !found {
			merged.Fields = append(merged.Fields, workorcheventpayload.New(field).Fields[0])
		}
	}
	return merged, nil
}

func cloneTemplateBindings(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func inheritedTemplateSourceHash(sources []TemplateSource) string {
	h := sha256.New()
	for _, source := range sources {
		_, _ = fmt.Fprintf(h, "%08x%s%08x%s", len(source.TemplateID), source.TemplateID, len(source.ContentHash), source.ContentHash)
	}
	return hex.EncodeToString(h.Sum(nil))
}
