package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DefinitionChangeReviewedIdentity 只标识持久完成的候选材料，不是批准或当前资源 readiness。
type DefinitionChangeReviewedIdentity struct {
	ID                   string
	Revision             uint64
	Hash                 string
	InputHash            string
	TemplateSnapshotHash string
	DerivationPermitID   string
	ProposalHash         string
	SpecificationHash    string
}

func (c DefinitionChangeCandidate) ReviewedIdentity(w *Workspace) (DefinitionChangeReviewedIdentity, error) {
	if c.Validate(w) != nil {
		return DefinitionChangeReviewedIdentity{}, common.ErrConflict
	}
	return c.reviewedIdentity(w.ID, false)
}

// View 只用于 detached 回执一致性校验；不能替代 owner 对原 mapping bytes 的重验。
func (c DefinitionChangeCandidate) ReviewedIdentityView(id common.WorkspaceID) (DefinitionChangeReviewedIdentity, error) {
	return c.reviewedIdentity(id, true)
}

func (c DefinitionChangeCandidate) reviewedIdentity(id common.WorkspaceID, view bool) (DefinitionChangeReviewedIdentity, error) {
	if c.Record.Input.WorkspaceID != id {
		return DefinitionChangeReviewedIdentity{}, common.ErrConflict
	}
	var derivation *DefinitionDerivationRequest
	for n := range c.Record.DerivationRequests {
		request := &c.Record.DerivationRequests[n]
		if request.Input.InputHash() != c.Record.InputHash {
			continue
		}
		if request.Proposal == nil || request.FailureCode != "" || derivation != nil {
			return DefinitionChangeReviewedIdentity{}, common.ErrConflict
		}
		derivation = request
	}
	if derivation == nil {
		return DefinitionChangeReviewedIdentity{}, common.ErrConflict
	}
	proposalHash := ""
	for _, request := range c.Record.SpecificationRequests {
		if request.DerivationPermitID != derivation.PermitID {
			continue
		}
		if request.FailureCode != "" || request.Proposal == nil && request.Review == nil {
			return DefinitionChangeReviewedIdentity{}, common.ErrConflict
		}
		if request.Proposal != nil {
			if proposalHash != "" {
				return DefinitionChangeReviewedIdentity{}, common.ErrConflict
			}
			proposalHash = request.Proposal.ContentHash()
		}
	}
	var specification GoalSpecification
	var err error
	if view {
		_, specification, err = c.Record.ConfirmableSpecificationView(derivation.PermitID, proposalHash)
	} else {
		_, specification, err = c.Record.ConfirmableSpecification(derivation.PermitID, proposalHash)
	}
	if err != nil {
		return DefinitionChangeReviewedIdentity{}, err
	}
	identityHash := sha256.New()
	writeDefinitionHash(identityHash, "definition-change-candidate-id-v1", string(id), string(c.TicketID), c.RequestID, c.BaseInputHash)
	candidateID := "definition-change-" + hex.EncodeToString(identityHash.Sum(nil))
	h := sha256.New()
	writeDefinitionHash(h, "definition-change-reviewed-v1", string(id), string(c.TicketID), candidateID, strconv.FormatUint(c.Record.Input.Revision, 10), c.BaseDefinitionRevision, c.BaseStartupInputHash, c.BaseInputHash, c.Record.InputHash, c.Record.TemplateSnapshotHash, derivation.PermitID, derivation.ResourceSnapshotHash, proposalHash, specification.AdmissionHash())
	return DefinitionChangeReviewedIdentity{ID: candidateID, Revision: c.Record.Input.Revision, Hash: "definition-change-reviewed-sha256:" + hex.EncodeToString(h.Sum(nil)), InputHash: c.Record.InputHash, TemplateSnapshotHash: c.Record.TemplateSnapshotHash, DerivationPermitID: derivation.PermitID, ProposalHash: proposalHash, SpecificationHash: specification.AdmissionHash()}, nil
}
