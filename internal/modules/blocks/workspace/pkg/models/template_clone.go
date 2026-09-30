package workspace

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// Clone 将有效模板从 owner 的可变记录中分离，用于持久创作快照。
func (t WorkspaceTemplate) Clone() WorkspaceTemplate {
	t.ParentTemplateIDs = append([]string(nil), t.ParentTemplateIDs...)
	t.EffectiveSources = append([]TemplateSource(nil), t.EffectiveSources...)
	t.DoDItems = append([]string(nil), t.DoDItems...)
	t.SoulBindings = cloneTemplateBindings(t.SoulBindings)
	t.ContextHints = t.ContextHints.Clone()
	t.EvidencePolicies = workorcheventpayload.CloneSlice(t.EvidencePolicies)
	return t
}
