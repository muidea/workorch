package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const DefinitionAuthoringPreparationDiagnosticLimit = 32
const DefinitionAuthoringPreparationCauseLimit = 64 * 1024
const DefinitionAuthoringPreparationCauseNotSavedReason = "preparation diagnostic cause exceeds supported storage boundary"

type DefinitionAuthoringPreparationStage string

const (
	DefinitionAuthoringPreparationDerivation DefinitionAuthoringPreparationStage = "derivation"
	DefinitionAuthoringPreparationProposal   DefinitionAuthoringPreparationStage = "proposal"
	DefinitionAuthoringPreparationReview     DefinitionAuthoringPreparationStage = "review"
)

// 前置诊断仅描述 target request reserve 之前的失败。
// 它不消耗 provider 预算，也不授权调用或准入。
type DefinitionAuthoringPreparationDiagnostic struct {
	DiagnosticID              string
	WorkspaceID               common.WorkspaceID
	InputHash                 string
	TemplateSnapshotHash      string
	ObservedWorkspaceRevision uint64
	Stage                     DefinitionAuthoringPreparationStage
	TargetRequestID           string
	DerivationPermitID        string
	ReviewKind                string
	ProposalHash              string
	PhaseCode                 string
	ResourceRef               string
	ProviderBindingHash       string
	Cause                     []byte
	CauseSHA256               string
	CauseBytes                uint64
	CauseSaved                bool
	NotSavedReason            string
}

func NewDefinitionAuthoringPreparationDiagnostic(identity DefinitionAuthoringPreparationDiagnostic, cause []byte) DefinitionAuthoringPreparationDiagnostic {
	d := identity
	d.Cause = nil
	d.CauseBytes = uint64(len(cause))
	h := sha256.Sum256(cause)
	d.CauseSHA256 = hex.EncodeToString(h[:])
	d.CauseSaved = len(cause) <= DefinitionAuthoringPreparationCauseLimit
	d.NotSavedReason = ""
	if d.CauseSaved {
		d.Cause = append([]byte{}, cause...)
	} else {
		d.NotSavedReason = DefinitionAuthoringPreparationCauseNotSavedReason
	}
	d.DiagnosticID = d.ExpectedDiagnosticID()
	return d
}

func (d DefinitionAuthoringPreparationDiagnostic) ExpectedDiagnosticID() string {
	h := sha256.New()
	// ObservedWorkspaceRevision 不参与幂等 ID；InputHash 已包含封存输入 revision，防止 ABA。
	writeDefinitionHash(h, "workspace-authoring-preparation-diagnostic-v1", string(d.WorkspaceID), d.InputHash, d.TemplateSnapshotHash, string(d.Stage), d.TargetRequestID, d.DerivationPermitID, d.ReviewKind, d.ProposalHash, d.PhaseCode, d.ResourceRef, d.ProviderBindingHash, d.CauseSHA256, strconv.FormatUint(d.CauseBytes, 10), strconv.FormatBool(d.CauseSaved), d.NotSavedReason)
	return "preparation-diagnostic-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (d DefinitionAuthoringPreparationDiagnostic) Clone() DefinitionAuthoringPreparationDiagnostic {
	if d.Cause != nil {
		d.Cause = append([]byte{}, d.Cause...)
	}
	return d
}

func preparationIdentityText(value string, limit int, required bool) bool {
	return (!required || value != "") && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && strings.TrimSpace(value) == value
}

func (d DefinitionAuthoringPreparationDiagnostic) Validate() error { return d.validate(false) }

// ValidateMetadata 不读取未投递的原文，不能代替 owner 的 bytes/hash 校验。
func (d DefinitionAuthoringPreparationDiagnostic) ValidateMetadata() error { return d.validate(true) }
func (d DefinitionAuthoringPreparationDiagnostic) Metadata() DefinitionAuthoringPreparationDiagnostic {
	d.Cause = nil
	return d
}
func (d DefinitionAuthoringPreparationDiagnostic) validate(view bool) error {
	if view && d.Cause != nil {
		return fmt.Errorf("preparation metadata contains raw bytes")
	}
	if !preparationIdentityText(string(d.WorkspaceID), 256, true) || !preparationIdentityText(d.InputHash, 256, true) || !preparationIdentityText(d.TemplateSnapshotHash, 256, true) || d.ObservedWorkspaceRevision == 0 || !preparationIdentityText(d.TargetRequestID, 128, true) || !preparationIdentityText(d.PhaseCode, 128, true) || !preparationIdentityText(d.ResourceRef, 1024, false) || !preparationIdentityText(d.ProviderBindingHash, 256, false) || !preparationIdentityText(d.DerivationPermitID, 256, false) || !preparationIdentityText(d.ProposalHash, 256, false) {
		return fmt.Errorf("preparation diagnostic identity invalid")
	}
	switch d.Stage {
	case DefinitionAuthoringPreparationDerivation:
		if d.DerivationPermitID != "" || d.ReviewKind != "" || d.ProposalHash != "" {
			return fmt.Errorf("derivation preparation diagnostic claims specification identity")
		}
	case DefinitionAuthoringPreparationProposal:
		if d.DerivationPermitID == "" || d.ReviewKind != "" || d.ProposalHash != "" {
			return fmt.Errorf("proposal preparation diagnostic identity invalid")
		}
	case DefinitionAuthoringPreparationReview:
		if d.DerivationPermitID == "" || !GoalSpecificationReviewKind(d.ReviewKind) || d.ProposalHash == "" {
			return fmt.Errorf("review preparation diagnostic identity invalid")
		}
	default:
		return fmt.Errorf("preparation diagnostic stage invalid")
	}
	switch d.PhaseCode {
	case "resources":
		if d.Stage != DefinitionAuthoringPreparationDerivation {
			return fmt.Errorf("resource preparation diagnostic stage invalid")
		}
	case "config", "render", "request_budget", "attempt_budget":
	default:
		return fmt.Errorf("preparation diagnostic phase code invalid")
	}
	if len(d.CauseSHA256) != sha256.Size*2 || strings.ToLower(d.CauseSHA256) != d.CauseSHA256 {
		return fmt.Errorf("preparation diagnostic cause identity invalid")
	}
	if _, err := hex.DecodeString(d.CauseSHA256); err != nil {
		return fmt.Errorf("preparation diagnostic cause hash invalid")
	}
	if d.CauseSaved {
		if d.CauseBytes > DefinitionAuthoringPreparationCauseLimit || d.NotSavedReason != "" || !view && (d.Cause == nil || uint64(len(d.Cause)) != d.CauseBytes) {
			return fmt.Errorf("preparation diagnostic saved cause byte boundary invalid")
		}
		if !view {
			h := sha256.Sum256(d.Cause)
			if hex.EncodeToString(h[:]) != d.CauseSHA256 {
				return fmt.Errorf("preparation diagnostic cause hash mismatch")
			}
		}
	} else if d.Cause != nil || d.CauseBytes <= DefinitionAuthoringPreparationCauseLimit || d.NotSavedReason != DefinitionAuthoringPreparationCauseNotSavedReason {
		return fmt.Errorf("preparation diagnostic withheld cause boundary invalid")
	}
	if d.DiagnosticID != d.ExpectedDiagnosticID() {
		return fmt.Errorf("preparation diagnostic ID mismatch")
	}
	return nil
}

// 容量拒绝没有已保存回执或诊断 ID，不能伪称可查询。
type PreparationDiagnosticLimitError struct{ Limit uint32 }

func (e *PreparationDiagnosticLimitError) Error() string {
	return fmt.Sprintf("preparation diagnostic lineage limit exhausted (limit %d)", e.Limit)
}
func (e *PreparationDiagnosticLimitError) Unwrap() error { return common.ErrConflict }

func validatePreparationDiagnostics(r DefinitionAuthoringRecord) error {
	return validatePreparationDiagnosticsMode(r, false)
}
func validatePreparationDiagnosticsMode(r DefinitionAuthoringRecord, view bool) error {
	if len(r.PreparationDiagnostics) > DefinitionAuthoringPreparationDiagnosticLimit {
		return fmt.Errorf("preparation diagnostic lineage limit exceeded")
	}
	seen := make(map[string]bool, len(r.PreparationDiagnostics))
	for _, d := range r.PreparationDiagnostics {
		if d.validate(view) != nil || d.WorkspaceID != r.Input.WorkspaceID || seen[d.DiagnosticID] {
			return fmt.Errorf("preparation diagnostic lineage scope or identity invalid")
		}
		knownInput := d.InputHash == r.InputHash && d.TemplateSnapshotHash == r.TemplateSnapshotHash
		for _, receipt := range r.SaveReceipts {
			if receipt.InputHash == d.InputHash && receipt.TemplateSnapshotHash == d.TemplateSnapshotHash {
				knownInput = true
			}
		}
		for _, request := range r.DerivationRequests {
			if request.Input.InputHash() == d.InputHash && WorkspaceTemplateSnapshotHash(&request.Template) == d.TemplateSnapshotHash {
				knownInput = true
			}
		}
		if !knownInput {
			return fmt.Errorf("preparation diagnostic has no owner input lineage")
		}
		if d.Stage != DefinitionAuthoringPreparationDerivation {
			derivation, err := r.derivationForSpecification(d.DerivationPermitID)
			if err != nil || derivation.Input.InputHash() != d.InputHash || WorkspaceTemplateSnapshotHash(&derivation.Template) != d.TemplateSnapshotHash || d.PhaseCode != "config" && d.ProviderBindingHash != "" && d.ProviderBindingHash != derivation.ProviderBindingHash {
				return fmt.Errorf("preparation diagnostic derivation lineage invalid")
			}
			if d.Stage == DefinitionAuthoringPreparationReview && r.completedSpecificationProposal(d.DerivationPermitID, d.ProposalHash) == nil {
				return fmt.Errorf("preparation diagnostic review lineage invalid")
			}
		}
		seen[d.DiagnosticID] = true
	}
	return nil
}

// ValidatePreparationDiagnosticHistory 对整条 lineage 去重限额，并反证继承与归档副本保留首写原值。
func (w *Workspace) ValidatePreparationDiagnosticHistory() error {
	return w.validatePreparationDiagnosticHistory(false)
}
func (w *Workspace) ValidatePreparationDiagnosticHistoryView() error {
	return w.validatePreparationDiagnosticHistory(true)
}
func (w *Workspace) validatePreparationDiagnosticHistory(view bool) error {
	if w == nil || len(w.DefinitionAuthoringHistory) > DefinitionDerivationRequestLimit {
		return common.ErrInvalidArgument
	}
	if w.DefinitionAuthoring == nil {
		if len(w.DefinitionAuthoringHistory) > 0 {
			return common.ErrConflict
		}
		return nil
	}
	records := []DefinitionAuthoringRecord{*w.DefinitionAuthoring}
	for _, archived := range w.DefinitionAuthoringHistory {
		records = append(records, archived.Record)
	}
	seen := make(map[string]DefinitionAuthoringPreparationDiagnostic)
	for _, record := range records {
		if validatePreparationDiagnosticsMode(record, view) != nil {
			return common.ErrConflict
		}
		for _, diagnostic := range record.PreparationDiagnostics {
			if diagnostic.WorkspaceID != w.ID || diagnostic.ObservedWorkspaceRevision > w.Revision {
				return common.ErrConflict
			}
			if previous, ok := seen[diagnostic.DiagnosticID]; ok && !reflect.DeepEqual(previous, diagnostic) {
				return common.ErrConflict
			}
			seen[diagnostic.DiagnosticID] = diagnostic
		}
	}
	if len(seen) > DefinitionAuthoringPreparationDiagnosticLimit {
		return common.ErrConflict
	}
	for _, archived := range w.DefinitionAuthoringHistory {
		for _, diagnostic := range archived.Record.PreparationDiagnostics {
			found := false
			for _, current := range w.DefinitionAuthoring.PreparationDiagnostics {
				if reflect.DeepEqual(current, diagnostic) {
					found = true
					break
				}
			}
			if !found {
				return common.ErrConflict
			}
		}
	}
	return nil
}

func (r DefinitionAuthoringRecord) RecordPreparationDiagnostic(d DefinitionAuthoringPreparationDiagnostic) (DefinitionAuthoringRecord, DefinitionAuthoringPreparationDiagnostic, bool, error) {
	if err := r.Validate(); err != nil {
		return DefinitionAuthoringRecord{}, d, false, err
	}
	if d.Validate() != nil || d.WorkspaceID != r.Input.WorkspaceID {
		return DefinitionAuthoringRecord{}, d, false, common.ErrInvalidArgument
	}
	for _, previous := range r.PreparationDiagnostics {
		if previous.DiagnosticID == d.DiagnosticID {
			return r.Clone(), previous.Clone(), true, nil
		}
	}
	if d.InputHash != r.InputHash || d.TemplateSnapshotHash != r.TemplateSnapshotHash {
		return DefinitionAuthoringRecord{}, d, false, common.ErrConflict
	}
	if err := r.validatePreparationTarget(d); err != nil {
		return DefinitionAuthoringRecord{}, d, false, err
	}
	if len(r.PreparationDiagnostics) >= DefinitionAuthoringPreparationDiagnosticLimit {
		return DefinitionAuthoringRecord{}, d, false, &PreparationDiagnosticLimitError{Limit: DefinitionAuthoringPreparationDiagnosticLimit}
	}
	next := r.Clone()
	next.PreparationDiagnostics = append(next.PreparationDiagnostics, d.Clone())
	return next, d.Clone(), false, nil
}

// ValidatePreparationDiagnosticTarget 在新增诊断前反证 current target；历史诊断可先于后续 reserve。
func (r DefinitionAuthoringRecord) ValidatePreparationDiagnosticTarget(d DefinitionAuthoringPreparationDiagnostic) error {
	if d.InputHash != r.InputHash || d.TemplateSnapshotHash != r.TemplateSnapshotHash {
		return common.ErrConflict
	}
	return r.validatePreparationTarget(d)
}

func (r DefinitionAuthoringRecord) validatePreparationTarget(d DefinitionAuthoringPreparationDiagnostic) error {
	for _, request := range r.DerivationRequests {
		if request.RequestID == d.TargetRequestID {
			return common.ErrConflict
		}
	}
	for _, request := range r.SpecificationRequests {
		if request.RequestID == d.TargetRequestID {
			return common.ErrConflict
		}
	}
	if d.ResourceRef != "" && !slices.Contains(r.Input.Normalized().ResourceRefs, d.ResourceRef) {
		return common.ErrConflict
	}
	if d.Stage != DefinitionAuthoringPreparationDerivation {
		derivation, err := r.derivationForSpecification(d.DerivationPermitID)
		if err != nil || derivation.Input.InputHash() != r.InputHash || WorkspaceTemplateSnapshotHash(&derivation.Template) != r.TemplateSnapshotHash || d.PhaseCode != "config" && d.ProviderBindingHash != "" && d.ProviderBindingHash != derivation.ProviderBindingHash {
			return common.ErrConflict
		}
		if d.Stage == DefinitionAuthoringPreparationReview && r.completedSpecificationProposal(d.DerivationPermitID, d.ProposalHash) == nil {
			return common.ErrConflict
		}
	}
	if d.PhaseCode == "request_budget" || d.PhaseCode == "attempt_budget" {
		if d.ProviderBindingHash == "" {
			return common.ErrConflict
		}
		count, limit, attempts := len(r.DerivationRequests), DefinitionDerivationRequestLimit, 0
		attemptLimit := DefinitionDerivationAttemptLimit
		if d.Stage == DefinitionAuthoringPreparationDerivation {
			for _, request := range r.DerivationRequests {
				if request.Input.InputHash() == r.InputHash && WorkspaceTemplateSnapshotHash(&request.Template) == r.TemplateSnapshotHash {
					attempts++
				}
			}
		} else {
			count, limit, attemptLimit = len(r.SpecificationRequests), DefinitionAuthoringSpecificationRequestLimit, DefinitionAuthoringSpecificationAttemptLimit
			phase := DefinitionSpecificationProposal
			if d.Stage == DefinitionAuthoringPreparationReview {
				phase = DefinitionSpecificationCritic
			}
			for _, request := range r.SpecificationRequests {
				if request.DerivationPermitID == d.DerivationPermitID && request.Phase == phase && request.ReviewKind == d.ReviewKind {
					attempts++
				}
			}
		}
		if d.PhaseCode == "request_budget" && count < limit || d.PhaseCode == "attempt_budget" && (count >= limit || attempts < attemptLimit) {
			return common.ErrConflict
		}
	}
	return nil
}

// 仅确定性的 reserve 预算 guard 能产生此拒绝。它没有 PermitID，
// stale/conflict/response-loss 绝不能重新标为预算拒绝。
type DefinitionAuthoringBudgetRefusalError struct {
	Stage     DefinitionAuthoringPreparationStage
	RequestID string
	Budget    string
	Limit     uint32
	Cause     error
}

func (e *DefinitionAuthoringBudgetRefusalError) Error() string {
	return e.Cause.Error() + "; no provider permit issued"
}
func preparationStageForSpecification(phase DefinitionSpecificationPhase) DefinitionAuthoringPreparationStage {
	if phase == DefinitionSpecificationCritic {
		return DefinitionAuthoringPreparationReview
	}
	return DefinitionAuthoringPreparationProposal
}

func (e *DefinitionAuthoringBudgetRefusalError) Unwrap() error { return e.Cause }
