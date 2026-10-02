package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DefinitionAuthoringInput 是尚未派生 DoD 的创作输入，不可作为准入候选。
// 资源仅含用户引用；可用性和快照必须由能力 owner 另行提供。
type DefinitionAuthoringInput struct {
	Version                uint32
	WorkspaceID            common.WorkspaceID
	Revision               uint64
	GoalText               string
	AcceptanceRequirements []string
	TemplateID             string
	ResourceRefs           []string
}

type DefinitionAuthoringRecord struct {
	Input                 DefinitionAuthoringInput
	InputHash             string
	TemplateSnapshotHash  string
	TemplateSnapshot      WorkspaceTemplate
	DerivationRequests    []DefinitionDerivationRequest
	SpecificationRequests []DefinitionAuthoringSpecificationRequest
	SaveReceipts          []DefinitionAuthoringSaveReceipt
}

func (r DefinitionAuthoringRecord) Clone() DefinitionAuthoringRecord {
	r.SaveReceipts = append([]DefinitionAuthoringSaveReceipt(nil), r.SaveReceipts...)
	r.Input = r.Input.Clone()
	r.TemplateSnapshot = r.TemplateSnapshot.Clone()
	r.DerivationRequests = append([]DefinitionDerivationRequest(nil), r.DerivationRequests...)
	for n := range r.DerivationRequests {
		r.DerivationRequests[n] = r.DerivationRequests[n].Clone()
	}
	r.SpecificationRequests = append([]DefinitionAuthoringSpecificationRequest(nil), r.SpecificationRequests...)
	for n := range r.SpecificationRequests {
		r.SpecificationRequests[n] = r.SpecificationRequests[n].Clone()
	}
	return r
}

func (r DefinitionAuthoringRecord) Validate() error {
	if err := r.Input.Validate(); err != nil {
		return err
	}
	if r.InputHash != r.Input.InputHash() || r.TemplateSnapshot.ID != r.Input.Normalized().TemplateID || r.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(&r.TemplateSnapshot) {
		return fmt.Errorf("authoring record identity mismatch")
	}
	if len(r.SaveReceipts) > DefinitionAuthoringSaveReceiptLimit {
		return fmt.Errorf("authoring save receipt limit exceeded")
	}
	seen := make(map[string]bool, len(r.SaveReceipts))
	for _, receipt := range r.SaveReceipts {
		if receipt.Validate() != nil || seen[receipt.RequestID] {
			return fmt.Errorf("authoring save receipt identity mismatch")
		}
		seen[receipt.RequestID] = true
	}
	if err := validateDerivationRequests(r.DerivationRequests, r.Input); err != nil {
		return err
	}
	return validateAuthoringSpecificationRequests(r)
}

const DefinitionAuthoringInputVersionCurrent uint32 = 1

func (i DefinitionAuthoringInput) Clone() DefinitionAuthoringInput {
	i.AcceptanceRequirements = append([]string(nil), i.AcceptanceRequirements...)
	i.ResourceRefs = append([]string(nil), i.ResourceRefs...)
	return i
}

func (i DefinitionAuthoringInput) Normalized() DefinitionAuthoringInput {
	i = i.Clone()
	i.GoalText = strings.TrimSpace(i.GoalText)
	i.TemplateID = strings.TrimSpace(i.TemplateID)
	for n := range i.AcceptanceRequirements {
		i.AcceptanceRequirements[n] = strings.TrimSpace(i.AcceptanceRequirements[n])
	}
	for n := range i.ResourceRefs {
		i.ResourceRefs[n] = strings.TrimSpace(i.ResourceRefs[n])
	}
	return i
}

func (i DefinitionAuthoringInput) Validate() error {
	i = i.Normalized()
	if i.Version != DefinitionAuthoringInputVersionCurrent || i.WorkspaceID == "" || i.Revision == 0 {
		return fmt.Errorf("definition authoring input requires version, workspace and revision")
	}
	validText := func(value string, limit int) bool {
		return value != "" && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
	}
	if !validText(string(i.WorkspaceID), 256) || !validText(i.GoalText, 32*1024) || !validText(i.TemplateID, 256) {
		return fmt.Errorf("definition authoring input has invalid goal or scope")
	}
	if len(i.AcceptanceRequirements) > 128 || len(i.ResourceRefs) > DefinitionAuthoringResourceLimit {
		return fmt.Errorf("definition authoring input exceeds item bounds")
	}
	total := len(i.GoalText) + len(i.TemplateID) + len(i.WorkspaceID)
	for _, requirement := range i.AcceptanceRequirements {
		if !validText(requirement, 4096) {
			return fmt.Errorf("definition authoring acceptance requirement is empty or invalid")
		}
		total += len(requirement)
	}
	seen := map[string]bool{}
	for _, ref := range i.ResourceRefs {
		if !validText(ref, 1024) || seen[ref] {
			return fmt.Errorf("definition authoring resource reference is invalid or duplicated")
		}
		seen[ref] = true
		total += len(ref)
	}
	if total > 128*1024 {
		return fmt.Errorf("definition authoring input exceeds byte budget")
	}
	return nil
}

// InputHash 包含 revision，防止 A→B→A 的旧确认绑定新输入。
// 输入顺序亦被封存，不能通过排序或去重静默丢弃用户要求。
func (i DefinitionAuthoringInput) InputHash() string {
	i = i.Normalized()
	h := sha256.New()
	writeDefinitionHash(h, "workspace-definition-authoring-v1", fmt.Sprint(i.Version), string(i.WorkspaceID), fmt.Sprint(i.Revision), i.GoalText, i.TemplateID)
	writeDefinitionHash(h, fmt.Sprint(len(i.AcceptanceRequirements)))
	writeDefinitionHash(h, i.AcceptanceRequirements...)
	writeDefinitionHash(h, fmt.Sprint(len(i.ResourceRefs)))
	writeDefinitionHash(h, i.ResourceRefs...)
	return "definition-authoring-sha256:" + hex.EncodeToString(h.Sum(nil))
}
