package workorchruntimecontract

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

// ValidateRaw 先拒绝会被规范化隐藏的差异，再校验正式材料及 hash。
func (c RuntimeResultCloseout) ValidateRaw() error {
	if !reflect.DeepEqual(c, c.Normalized()) {
		return fmt.Errorf("runtime result closeout must be raw canonical")
	}
	return c.Validate()
}

// CloneRaw 保全原始材料及 nil/empty 身份，不执行规范化或重新派生 hash。
func (c RuntimeResultCloseout) CloneRaw() RuntimeResultCloseout {
	c.OriginalDiagnostic = cloneRuntimeStateRawPointer(c.OriginalDiagnostic, cloneRuntimeOriginalDiagnosticRaw)
	c.SourceObligations = cloneRuntimeStateRawPointer(c.SourceObligations, func(value RuntimeSourceObligationInventory) RuntimeSourceObligationInventory { return value.Clone() })
	c.RequirementOutcomes = cloneRuntimeStateRawSlice(c.RequirementOutcomes, func(value RuntimeRequirementOutcome) RuntimeRequirementOutcome {
		value.MissingEvidenceKinds = cloneRuntimeStateRawSlice(value.MissingEvidenceKinds, func(item string) string { return item })
		value.EvidenceItems = cloneRuntimeStateRawSlice(value.EvidenceItems, func(item RuntimeRequirementEvidenceItem) RuntimeRequirementEvidenceItem { return item })
		value.AcceptedEvidenceRefs = cloneRuntimeStateRawSlice(value.AcceptedEvidenceRefs, func(item string) string { return item })
		value.SourceObligations = cloneRuntimeStateRawSlice(value.SourceObligations, func(item RuntimeSourceObligationOutcome) RuntimeSourceObligationOutcome {
			item.AcceptedEvidenceRefs = cloneRuntimeStateRawSlice(item.AcceptedEvidenceRefs, func(ref string) string { return ref })
			item.CurrentBindingRef = cloneRuntimeStateRawPointer(item.CurrentBindingRef, func(ref RuntimeCWIBindingRef) RuntimeCWIBindingRef { return ref })
			return item
		})
		return value
	})
	c.AcceptedEvidenceRefs = cloneRuntimeStateRawSlice(c.AcceptedEvidenceRefs, func(item string) string { return item })
	c.DeliverableRefs = cloneRuntimeStateRawSlice(c.DeliverableRefs, func(item string) string { return item })
	c.OwnerRevisionSet.Entries = cloneRuntimeStateRawSlice(c.OwnerRevisionSet.Entries, func(item RuntimeOwnerRevisionRef) RuntimeOwnerRevisionRef { return item })
	c.KnownLimitations = cloneRuntimeStateRawSlice(c.KnownLimitations, func(item string) string { return item })
	c.SatisfiedCoverageWorkItemIDs = cloneRuntimeStateRawSlice(c.SatisfiedCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	c.UnsatisfiedCoverageWorkItemIDs = cloneRuntimeStateRawSlice(c.UnsatisfiedCoverageWorkItemIDs, func(item common.CoverageWorkItemID) common.CoverageWorkItemID { return item })
	c.Gaps = cloneRuntimeStateRawSlice(c.Gaps, cloneRuntimeAssessmentGapRaw)
	c.MissingEvidenceKinds = cloneRuntimeStateRawSlice(c.MissingEvidenceKinds, func(item string) string { return item })
	c.TargetRefs = cloneRuntimeStateRawSlice(c.TargetRefs, func(item string) string { return item })
	c.EvidenceRefs = cloneRuntimeStateRawSlice(c.EvidenceRefs, func(item string) string { return item })
	c.ArtifactRefs = cloneRuntimeStateRawSlice(c.ArtifactRefs, func(item common.ArtifactID) common.ArtifactID { return item })
	c.OperationOutcomes = cloneRuntimeStateRawSlice(c.OperationOutcomes, func(item RuntimeOperationCloseout) RuntimeOperationCloseout { return item })
	c.RecoveryPolicy = cloneRuntimeRecoveryPolicyRaw(c.RecoveryPolicy)
	c.RecoveryLedger = cloneRuntimeRecoveryLedgerRaw(c.RecoveryLedger)
	return c
}

// ValidateRaw 保留原始回执身份，拒绝规范化会掩盖的变化。
func (a RuntimeDeliveryAcknowledgement) ValidateRaw() error {
	if !reflect.DeepEqual(a, a.Normalized()) {
		return fmt.Errorf("runtime delivery acknowledgement must be raw canonical")
	}
	return a.Validate()
}

// CloneRaw 复制 owner 实际持久化的材料，不用当前 Runtime 状态替换历史回执。
func (a RuntimeDeliveryAcknowledgement) CloneRaw() RuntimeDeliveryAcknowledgement {
	return cloneRuntimeDeliveryAcknowledgementRaw(a)
}
