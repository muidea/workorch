package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

// DefinitionDerivedAcceptance 是非权威派生结果；不能替代规格审查或正式 DoD。
// 用户和模板明确要求原样保留，模型只能追加验收项。
type DefinitionDerivedAcceptance struct {
	AuthoringInputHash   string
	TemplateSnapshotHash string
	AdditionalItems      []string
}

func (p DefinitionDerivedAcceptance) ValidateFor(input DefinitionAuthoringInput, template *WorkspaceTemplate) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if template == nil || template.ID != input.Normalized().TemplateID || p.AuthoringInputHash != input.InputHash() || p.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(template) {
		return fmt.Errorf("derived acceptance does not match authoring input and template")
	}
	if len(p.AdditionalItems) > 128 || len(p.AdditionalItems)+len(input.AcceptanceRequirements)+len(template.DoDItems) == 0 {
		return fmt.Errorf("derived acceptance requires bounded nonempty items")
	}
	total := 0
	for _, items := range [][]string{input.AcceptanceRequirements, template.DoDItems, p.AdditionalItems} {
		for _, item := range items {
			if strings.TrimSpace(item) == "" || len(item) > 4096 || !utf8.ValidString(item) || strings.ContainsRune(item, 0) {
				return fmt.Errorf("derived acceptance contains invalid item")
			}
			total += len(item)
		}
	}
	if total > 128*1024 {
		return fmt.Errorf("derived acceptance exceeds byte budget")
	}
	return nil
}

// CandidateItems 生成独立候选引用，不伪装成已持久化的正式 DoD revision。
func (p DefinitionDerivedAcceptance) CandidateItems(input DefinitionAuthoringInput, template *WorkspaceTemplate) ([]DefinitionCandidateItem, error) {
	if err := p.ValidateFor(input, template); err != nil {
		return nil, err
	}
	items := make([]DefinitionCandidateItem, 0, len(input.AcceptanceRequirements)+len(template.DoDItems)+len(p.AdditionalItems))
	appendItems := func(kind, source string, texts []string) {
		for n, text := range texts {
			items = append(items, DefinitionCandidateItem{CandidateItemRef: fmt.Sprintf("derived-dod:%s:%06d", kind, n+1), Text: strings.TrimSpace(text), SourceIdentity: source})
		}
	}
	appendItems("user", input.InputHash(), input.AcceptanceRequirements)
	appendItems("template", p.TemplateSnapshotHash, template.DoDItems)
	appendItems("generated", p.ContentHash(), p.AdditionalItems)
	return items, nil
}

func (p DefinitionDerivedAcceptance) ContentHash() string {
	h := sha256.New()
	writeDefinitionHash(h, "workspace-derived-acceptance-v1", p.AuthoringInputHash, p.TemplateSnapshotHash, fmt.Sprint(len(p.AdditionalItems)))
	for _, item := range p.AdditionalItems {
		writeDefinitionHash(h, strings.TrimSpace(item))
	}
	return "derived-acceptance-sha256:" + hex.EncodeToString(h.Sum(nil))
}
