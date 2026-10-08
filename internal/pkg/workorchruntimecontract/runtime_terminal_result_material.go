package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

// RuntimeTerminalResultMaterial 是 SM-4 从已裁决 aggregate 派生的 owner-neutral 终态材料。
// 字段顺序、类型及无 JSON tag 的编码形状保持与 Result receiving-owner 材料完全一致；
// 此值不包含 Result lifecycle 字段，也不提供 Hub、I/O、CAS 或验收裁决权限。
type RuntimeTerminalResultMaterial struct {
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	GoalID          common.GoalID
	DoDID           common.DoDID
	Title           string
	Summary         string
	SourceType      string
	SourceRef       string
	EvidenceRefs    []string
	ArtifactRefs    []common.ArtifactID
	DoDAssessment   workorchresultcontract.DoDAssessment
	SummaryDetail   workorchresultcontract.ResultSummary
	RuntimeCloseout *RuntimeResultCloseout
}

const RuntimeTerminalResultSourceTypeDoDAssessment = "dod_assessment"

// CloneRaw 保全全部原值及 nil/empty 身份，不规范化、不派生 hash。
func (m RuntimeTerminalResultMaterial) CloneRaw() RuntimeTerminalResultMaterial {
	m.EvidenceRefs = cloneRuntimeStateRawSlice(m.EvidenceRefs, func(item string) string { return item })
	m.ArtifactRefs = cloneRuntimeStateRawSlice(m.ArtifactRefs, func(item common.ArtifactID) common.ArtifactID { return item })
	m.DoDAssessment = cloneRuntimeTerminalAssessmentRaw(m.DoDAssessment)
	m.SummaryDetail = cloneRuntimeTerminalSummaryRaw(m.SummaryDetail)
	m.RuntimeCloseout = cloneRuntimeStateRawPointer(m.RuntimeCloseout, func(item RuntimeResultCloseout) RuntimeResultCloseout { return item.CloneRaw() })
	return m
}

func cloneRuntimeTerminalAssessmentItemRaw(item workorchresultcontract.DoDAssessmentItem) workorchresultcontract.DoDAssessmentItem {
	item.EvidenceRefs = cloneRuntimeStateRawSlice(item.EvidenceRefs, func(value string) string { return value })
	return item
}

func cloneRuntimeTerminalAssessmentRaw(item workorchresultcontract.DoDAssessment) workorchresultcontract.DoDAssessment {
	item.Items = cloneRuntimeStateRawSlice(item.Items, cloneRuntimeTerminalAssessmentItemRaw)
	item.GoalCoverage = cloneRuntimeStateRawPointer(item.GoalCoverage, cloneRuntimeTerminalCoverageRaw)
	item.AcceptanceDimensions = cloneRuntimeStateRawPointer(item.AcceptanceDimensions, cloneRuntimeTerminalCoverageRaw)
	item.DeliverableCoverage = cloneRuntimeStateRawPointer(item.DeliverableCoverage, cloneRuntimeTerminalCoverageRaw)
	return item
}

func cloneRuntimeTerminalCoverageRaw(item workorchresultcontract.DoDCoverage) workorchresultcontract.DoDCoverage {
	item.Items = cloneRuntimeStateRawSlice(item.Items, func(value workorchresultcontract.DoDCoverageItem) workorchresultcontract.DoDCoverageItem {
		value.EvidenceRefs = cloneRuntimeStateRawSlice(value.EvidenceRefs, func(ref string) string { return ref })
		value.ArtifactRefs = cloneRuntimeStateRawSlice(value.ArtifactRefs, func(ref common.ArtifactID) common.ArtifactID { return ref })
		return value
	})
	return item
}

func cloneRuntimeTerminalSummaryRaw(item workorchresultcontract.ResultSummary) workorchresultcontract.ResultSummary {
	item.DoDItems = cloneRuntimeStateRawSlice(item.DoDItems, cloneRuntimeTerminalAssessmentItemRaw)
	item.EvidenceRefs = cloneRuntimeStateRawSlice(item.EvidenceRefs, func(value string) string { return value })
	item.ArtifactRefs = cloneRuntimeStateRawSlice(item.ArtifactRefs, func(value common.ArtifactID) common.ArtifactID { return value })
	item.KnownLimitations = cloneRuntimeStateRawSlice(item.KnownLimitations, func(value string) string { return value })
	item.ResidualRisks = cloneRuntimeStateRawSlice(item.ResidualRisks, func(value string) string { return value })
	item.NextActions = cloneRuntimeStateRawSlice(item.NextActions, func(value string) string { return value })
	item.Diagnostics.Fields = cloneRuntimeStateRawSlice(item.Diagnostics.Fields, cloneRuntimeTerminalDiagnosticFieldRaw)
	return item
}

func cloneRuntimeTerminalDiagnosticFieldRaw(item workorcheventpayload.Field) workorcheventpayload.Field {
	item.Value = cloneRuntimeTerminalDiagnosticValueRaw(item.Value)
	return item
}

func cloneRuntimeTerminalDiagnosticValueRaw(item workorcheventpayload.Value) workorcheventpayload.Value {
	item.Object = cloneRuntimeStateRawSlice(item.Object, cloneRuntimeTerminalDiagnosticFieldRaw)
	item.Array = cloneRuntimeStateRawSlice(item.Array, cloneRuntimeTerminalDiagnosticValueRaw)
	return item
}

// Normalized 复用既有 ResultMaterial 的确定性规范，不改写原始诊断。
func (m RuntimeTerminalResultMaterial) Normalized() RuntimeTerminalResultMaterial {
	m.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(m.WorkspaceID)))
	m.RunID = common.RunID(strings.TrimSpace(string(m.RunID)))
	m.GoalID = common.GoalID(strings.TrimSpace(string(m.GoalID)))
	m.DoDID = common.DoDID(strings.TrimSpace(string(m.DoDID)))
	m.Title = strings.TrimSpace(m.Title)
	m.Summary = strings.TrimSpace(m.Summary)
	m.SourceType = strings.TrimSpace(m.SourceType)
	m.SourceRef = strings.TrimSpace(m.SourceRef)
	m.EvidenceRefs = normalizedRuntimeTerminalStringRefs(m.EvidenceRefs)
	m.ArtifactRefs = normalizedRuntimeTerminalArtifactRefs(m.ArtifactRefs)
	m.DoDAssessment = normalizeRuntimeTerminalAssessment(m.DoDAssessment)
	m.SummaryDetail = normalizeRuntimeTerminalSummary(m.SummaryDetail)
	if m.RuntimeCloseout != nil {
		closeout := m.RuntimeCloseout.Normalized()
		closeout.ArtifactRefs = normalizedRuntimeTerminalArtifactRefs(closeout.ArtifactRefs)
		m.RuntimeCloseout = &closeout
	}
	return m
}

func normalizeRuntimeTerminalAssessment(item workorchresultcontract.DoDAssessment) workorchresultcontract.DoDAssessment {
	item = item.Clone()
	for idx := range item.Items {
		item.Items[idx].EvidenceRefs = normalizedRuntimeTerminalStringRefs(item.Items[idx].EvidenceRefs)
	}
	for _, coverage := range []*workorchresultcontract.DoDCoverage{item.GoalCoverage, item.AcceptanceDimensions, item.DeliverableCoverage} {
		if coverage == nil {
			continue
		}
		for idx := range coverage.Items {
			coverage.Items[idx].EvidenceRefs = normalizedRuntimeTerminalStringRefs(coverage.Items[idx].EvidenceRefs)
			coverage.Items[idx].ArtifactRefs = normalizedRuntimeTerminalArtifactRefs(coverage.Items[idx].ArtifactRefs)
		}
	}
	return item
}

func normalizeRuntimeTerminalSummary(item workorchresultcontract.ResultSummary) workorchresultcontract.ResultSummary {
	item = item.Clone()
	item.EvidenceRefs = normalizedRuntimeTerminalStringRefs(item.EvidenceRefs)
	item.ArtifactRefs = normalizedRuntimeTerminalArtifactRefs(item.ArtifactRefs)
	for idx := range item.DoDItems {
		item.DoDItems[idx].EvidenceRefs = normalizedRuntimeTerminalStringRefs(item.DoDItems[idx].EvidenceRefs)
	}
	return item
}

// ValidateRaw 用于封存与重载边界，先拒绝规范化会隐藏的材料差异。
func (m RuntimeTerminalResultMaterial) ValidateRaw() error {
	if !reflect.DeepEqual(m, m.Normalized()) {
		return fmt.Errorf("%w: runtime terminal result material must be raw canonical", common.ErrInvalidArgument)
	}
	return m.Validate()
}

// Validate 保持 receiving-owner 的既有规则：closeout 原值校验先于规范化。
func (m RuntimeTerminalResultMaterial) Validate() error {
	if m.RuntimeCloseout != nil {
		if err := m.RuntimeCloseout.ValidateRaw(); err != nil {
			return fmt.Errorf("%w: invalid raw runtime closeout: %v", common.ErrInvalidArgument, err)
		}
	}
	m = m.Normalized()
	if m.WorkspaceID == "" || m.RunID == "" || m.GoalID == "" || m.DoDID == "" || m.Summary == "" {
		return fmt.Errorf("%w: result material requires scope, definition and summary", common.ErrInvalidArgument)
	}
	if m.RuntimeCloseout != nil {
		if err := m.RuntimeCloseout.Validate(); err != nil {
			return fmt.Errorf("%w: invalid runtime closeout: %v", common.ErrInvalidArgument, err)
		}
	}
	if err := validateRuntimeTerminalArtifactReferenceClosure(m); err != nil {
		return err
	}
	if _, err := m.CanonicalHash(); err != nil {
		return fmt.Errorf("%w: result material is not canonical: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func validateRuntimeTerminalArtifactReferenceClosure(material RuntimeTerminalResultMaterial) error {
	known := make(map[common.ArtifactID]struct{}, len(material.ArtifactRefs))
	for _, ref := range material.ArtifactRefs {
		known[common.ArtifactID(strings.TrimSpace(string(ref)))] = struct{}{}
	}
	check := func(refs []common.ArtifactID) error {
		for _, ref := range refs {
			ref = common.ArtifactID(strings.TrimSpace(string(ref)))
			if ref == "" {
				continue
			}
			if _, ok := known[ref]; !ok {
				return fmt.Errorf("%w: nested artifact reference %q is absent from terminal material", common.ErrConflict, ref)
			}
		}
		return nil
	}
	if err := check(material.SummaryDetail.ArtifactRefs); err != nil {
		return err
	}
	if material.RuntimeCloseout != nil {
		if err := check(material.RuntimeCloseout.ArtifactRefs); err != nil {
			return err
		}
	}
	for _, coverage := range []*workorchresultcontract.DoDCoverage{material.DoDAssessment.GoalCoverage, material.DoDAssessment.AcceptanceDimensions, material.DoDAssessment.DeliverableCoverage} {
		if coverage == nil {
			continue
		}
		for _, item := range coverage.Items {
			if err := check(item.ArtifactRefs); err != nil {
				return err
			}
		}
	}
	return nil
}

// CanonicalHash 保持旧 ResultMaterial 的 JSON 形状及 hash 前缀。
// JSON 仅用于 concrete value 的确定性编码，不用于跨 owner 传输或重建权威。
func (m RuntimeTerminalResultMaterial) CanonicalHash() (string, error) {
	encoded, err := json.Marshal(m.Normalized())
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "result-material-sha256:" + hex.EncodeToString(digest[:]), nil
}

func normalizedRuntimeTerminalStringRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizedRuntimeTerminalArtifactRefs(values []common.ArtifactID) []common.ArtifactID {
	seen := make(map[common.ArtifactID]struct{}, len(values))
	ret := make([]common.ArtifactID, 0, len(values))
	for _, value := range values {
		value = common.ArtifactID(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Slice(ret, func(i, j int) bool { return string(ret[i]) < string(ret[j]) })
	if len(ret) == 0 {
		return nil
	}
	return ret
}
