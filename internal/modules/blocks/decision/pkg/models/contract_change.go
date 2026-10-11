package decision

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	ContractChangeDecisionKind      = "contract_change"
	ContractChangeSchemaVersion     = 1
	ContractChangeRevisionLimit     = 32
	ContractChangeDocumentByteLimit = 128 * 1024
	ContractChangeHistoryByteLimit  = 1024 * 1024
)

// 登记材料不携带批准、应用或启动状态；未知和缺证据按原文记录。
type ContractChangeProposal struct {
	SchemaVersion                    int
	WorkspaceID                      common.WorkspaceID
	RunID                            common.RunID
	BaseDefinitionRevision           string
	BaseStartupInputHash             string
	BaseInputRevision                uint64
	BaseInputHash                    string
	ProposedBy                       string
	TriggerFacts                     string
	Discovery                        string
	RootCauseOrUnknown               string
	WhyChange                        string
	EvidenceRefs                     []string
	EvidenceGaps                     string
	Differences                      []ContractChangeDifference
	Alternatives                     string
	NoChangeConsequences             string
	UnaffectedContinuationConditions string
	Impacts                          []ContractChangeImpact
	EffectsAndReceipts               string
	OutstandingCosts                 string
	ReservedBudget                   string
	AdditionalCostDurationRisk       string
	RequiredAuthorization            string
	DecisionScope                    string
	ApplicationConditions            string
	RecoveryRestrictions             string
	FinalCandidate                   *ContractChangeCandidate
}

type ContractChangeDifference struct {
	ItemID         string
	Kind           string
	Action         string
	OriginalText   string
	ProposedText   string
	SemanticChange string
}

// ProposedHandling 仅保存提出者的处理建议，不裁定已有证据或闭包的适用性。
type ContractChangeImpact struct {
	SubjectID        string
	Kind             string
	ProposedHandling string
	Reason           string
}

type ContractChangeCandidate struct {
	ID       string
	Revision uint64
	Hash     string
}

// 每次登记请求及材料共同封存，重试从原版本恢复，不另建提案账本。
type ContractChangeRevision struct {
	RequestID    string
	Revision     uint64
	Hash         string
	RegisteredAt string
	Proposal     ContractChangeProposal
}

func (p ContractChangeProposal) Clone() ContractChangeProposal {
	if p.EvidenceRefs != nil {
		p.EvidenceRefs = append([]string{}, p.EvidenceRefs...)
	}
	if p.Differences != nil {
		p.Differences = append([]ContractChangeDifference{}, p.Differences...)
	}
	if p.Impacts != nil {
		p.Impacts = append([]ContractChangeImpact{}, p.Impacts...)
	}
	if p.FinalCandidate != nil {
		v := *p.FinalCandidate
		p.FinalCandidate = &v
	}
	return p
}

func CloneContractChangeRevisions(items []ContractChangeRevision) []ContractChangeRevision {
	if items == nil {
		return nil
	}
	ret := append([]ContractChangeRevision{}, items...)
	for n := range ret {
		ret[n].Proposal = ret[n].Proposal.Clone()
	}
	return ret
}

func (p ContractChangeProposal) Hash() string {
	body, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (p ContractChangeProposal) Validate() error {
	invalid := func(reason string) error {
		return fmt.Errorf("%w: contract change %s", common.ErrInvalidArgument, reason)
	}
	if p.SchemaVersion != ContractChangeSchemaVersion || p.WorkspaceID == "" ||
		((p.BaseDefinitionRevision == "") != (p.BaseStartupInputHash == "")) ||
		((p.BaseInputRevision == 0) != (p.BaseInputHash == "")) ||
		(p.BaseDefinitionRevision == "" && p.BaseInputRevision == 0) {
		return invalid("requires exact baseline and schema")
	}
	if p.BaseDefinitionRevision != "" && (strings.TrimSpace(p.BaseDefinitionRevision) == "" || strings.TrimSpace(p.BaseStartupInputHash) == "") ||
		p.BaseInputRevision != 0 && strings.TrimSpace(p.BaseInputHash) == "" || p.RunID != "" && p.BaseDefinitionRevision == "" {
		return invalid("requires nonempty sealed Run baseline")
	}
	texts := []string{p.ProposedBy, p.TriggerFacts, p.Discovery, p.RootCauseOrUnknown, p.WhyChange,
		p.Alternatives, p.NoChangeConsequences, p.UnaffectedContinuationConditions, p.EffectsAndReceipts,
		p.OutstandingCosts, p.ReservedBudget, p.AdditionalCostDurationRisk, p.RequiredAuthorization,
		p.DecisionScope, p.ApplicationConditions, p.RecoveryRestrictions}
	for _, value := range texts {
		if strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
			return invalid("requires complete raw material or explicit unknown")
		}
	}
	if len(p.EvidenceRefs) == 0 && strings.TrimSpace(p.EvidenceGaps) == "" {
		return invalid("requires evidence or explicit gaps")
	}
	if len(p.EvidenceRefs) > 128 || len(p.Differences) == 0 || len(p.Differences) > 128 || len(p.Impacts) == 0 || len(p.Impacts) > 256 {
		return invalid("exceeds item boundary or lacks differences/impacts")
	}
	for _, ref := range p.EvidenceRefs {
		if strings.TrimSpace(ref) == "" || !utf8.ValidString(ref) {
			return invalid("invalid evidence reference")
		}
	}
	seen := map[string]bool{}
	for _, d := range p.Differences {
		if strings.TrimSpace(d.ItemID) == "" || seen[d.ItemID] || strings.TrimSpace(d.SemanticChange) == "" {
			return invalid("invalid or duplicate difference")
		}
		seen[d.ItemID] = true
		switch d.Kind {
		case "goal", "criterion", "constraint":
		default:
			return invalid("unknown difference kind")
		}
		switch d.Action {
		case "add":
			if d.OriginalText != "" || strings.TrimSpace(d.ProposedText) == "" {
				return invalid("invalid added text")
			}
		case "remove":
			if strings.TrimSpace(d.OriginalText) == "" || d.ProposedText != "" {
				return invalid("invalid removed text")
			}
		case "modify":
			if strings.TrimSpace(d.OriginalText) == "" || strings.TrimSpace(d.ProposedText) == "" || d.OriginalText == d.ProposedText {
				return invalid("invalid modified text")
			}
		default:
			return invalid("unknown difference action")
		}
	}
	seen = map[string]bool{}
	for _, impact := range p.Impacts {
		key := impact.Kind + "\x00" + impact.SubjectID
		if strings.TrimSpace(impact.SubjectID) == "" || seen[key] || strings.TrimSpace(impact.Reason) == "" {
			return invalid("invalid or duplicate impact")
		}
		seen[key] = true
		switch impact.Kind {
		case "requirement", "obligation", "criterion", "verification_policy", "review_mapping", "definition_run_binding", "plan_cwi", "resource_environment", "artifact_evidence_closure", "delivery_regression":
		default:
			return invalid("unknown impact kind")
		}
		switch impact.ProposedHandling {
		case "retain", "recheck", "invalidate_reopen", "unknown":
		default:
			return invalid("unknown proposed impact handling")
		}
	}
	if c := p.FinalCandidate; c != nil && (strings.TrimSpace(c.ID) == "" || c.Revision == 0 || strings.TrimSpace(c.Hash) == "") {
		return invalid("incomplete final candidate")
	}
	body, err := json.Marshal(p)
	if err != nil || len(body) > ContractChangeDocumentByteLimit {
		return invalid("document exceeds storage boundary; nothing registered")
	}
	// Marshal 会替换无效 UTF-8，因此先逐字符串验证，不能用编码后的正文掩盖原值损失。
	if !utf8.ValidString(p.EvidenceGaps) || !utf8.ValidString(string(p.WorkspaceID)) || !utf8.ValidString(string(p.RunID)) ||
		!utf8.ValidString(p.BaseDefinitionRevision) || !utf8.ValidString(p.BaseStartupInputHash) || !utf8.ValidString(p.BaseInputHash) {
		return invalid("invalid UTF-8")
	}
	for _, d := range p.Differences {
		for _, text := range []string{d.ItemID, d.OriginalText, d.ProposedText, d.SemanticChange} {
			if !utf8.ValidString(text) {
				return invalid("invalid difference UTF-8")
			}
		}
	}
	for _, i := range p.Impacts {
		if !utf8.ValidString(i.SubjectID) || !utf8.ValidString(i.Reason) {
			return invalid("invalid impact UTF-8")
		}
	}
	if c := p.FinalCandidate; c != nil && (!utf8.ValidString(c.ID) || !utf8.ValidString(c.Hash)) {
		return invalid("invalid candidate UTF-8")
	}
	return nil
}

func (t *Ticket) IsContractChange() bool {
	return t != nil && (t.DecisionKind == ContractChangeDecisionKind || t.ContractChangeRevisions != nil)
}

func (t *Ticket) ValidateContractChange() error {
	if t == nil || t.ID == "" || t.WorkspaceID == "" || t.DecisionKind != ContractChangeDecisionKind ||
		t.StepID != "" || t.State != common.DecisionTicketStateOpen || t.ResolvedAt != "" || t.ResolvedBy != "" ||
		t.CreatedNotificationPublishedAt != "" || t.ResolutionNotificationPublishedAt != "" || t.DefinitionClarification != nil || t.RuntimeGovernanceProposal != nil ||
		len(t.AllowedRecoveryActions) != 0 || len(t.Context.Fields) != 0 || len(t.AutoGovernance.Fields) != 0 ||
		t.FailureClass != "" || t.FailureSignature != "" || t.Question != "" ||
		len(t.ContractChangeRevisions) == 0 || len(t.ContractChangeRevisions) > ContractChangeRevisionLimit {
		return common.ErrConflict
	}
	requests := map[string]bool{}
	base := t.ContractChangeRevisions[0].Proposal
	if t.Reason != base.WhyChange {
		return common.ErrConflict
	}
	for n, r := range t.ContractChangeRevisions {
		if strings.TrimSpace(r.RequestID) == "" || !utf8.ValidString(r.RequestID) || requests[r.RequestID] || r.Revision != uint64(n+1) ||
			r.RegisteredAt == "" || r.Hash != r.Proposal.Hash() || r.Proposal.Validate() != nil || r.Proposal.WorkspaceID != t.WorkspaceID || r.Proposal.RunID != t.RunID ||
			r.Proposal.BaseDefinitionRevision != base.BaseDefinitionRevision || r.Proposal.BaseStartupInputHash != base.BaseStartupInputHash ||
			r.Proposal.BaseInputRevision != base.BaseInputRevision || r.Proposal.BaseInputHash != base.BaseInputHash {
			return common.ErrConflict
		}
		if _, err := time.Parse(time.RFC3339Nano, r.RegisteredAt); err != nil {
			return common.ErrConflict
		}
		if len(r.RequestID) > 256 {
			return common.ErrConflict
		}
		requests[r.RequestID] = true
	}
	body, err := json.Marshal(t.ContractChangeRevisions)
	if err != nil || len(body) > ContractChangeHistoryByteLimit {
		return common.ErrConflict
	}
	return nil
}
