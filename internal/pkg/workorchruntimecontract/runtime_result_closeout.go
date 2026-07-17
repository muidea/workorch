package workorchruntimecontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeResultCloseout is the Orchestrator-authored structured terminal outcome.
// Result owner only persists it; it must not re-judge accepted/rejected authority.
type RuntimeResultCloseout struct {
	SchemaVersion                  uint32
	DefinitionRevision             string
	SatisfiedCoverageWorkItemIDs   []common.CoverageWorkItemID
	UnsatisfiedCoverageWorkItemIDs []common.CoverageWorkItemID
	Gaps                           []RuntimeAssessmentGap
	MissingEvidenceKinds           []string
	TargetRefs                     []string
	EvidenceRefs                   []string
	ArtifactRefs                   []common.ArtifactID
	Disposition                    AssessmentDisposition
	FailureClass                   FailureClass
	FailureCode                    string
	FailureSignature               string
	PredecessorRunID               common.RunID
	PredecessorResultID            common.ResultID
}

const RuntimeResultCloseoutSchemaVersionCurrent uint32 = 1

func (c RuntimeResultCloseout) Normalized() RuntimeResultCloseout {
	if c.SchemaVersion == 0 {
		c.SchemaVersion = RuntimeResultCloseoutSchemaVersionCurrent
	}
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.SatisfiedCoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.SatisfiedCoverageWorkItemIDs)
	c.UnsatisfiedCoverageWorkItemIDs = normalizeCoverageWorkItemIDs(c.UnsatisfiedCoverageWorkItemIDs)
	if len(c.Gaps) > 0 {
		gaps := make([]RuntimeAssessmentGap, 0, len(c.Gaps))
		for _, gap := range c.Gaps {
			gap = gap.Normalized()
			if len(gap.CoverageWorkItemIDs) == 0 && gap.Reason == "" {
				continue
			}
			gaps = append(gaps, gap)
		}
		if len(gaps) == 0 {
			c.Gaps = nil
		} else {
			c.Gaps = gaps
		}
	}
	c.MissingEvidenceKinds = normalizeStrings(c.MissingEvidenceKinds)
	c.TargetRefs = normalizeStrings(c.TargetRefs)
	c.EvidenceRefs = normalizeStrings(c.EvidenceRefs)
	if len(c.ArtifactRefs) > 0 {
		refs := make([]common.ArtifactID, 0, len(c.ArtifactRefs))
		for _, ref := range c.ArtifactRefs {
			ref = common.ArtifactID(strings.TrimSpace(string(ref)))
			if ref != "" {
				refs = append(refs, ref)
			}
		}
		if len(refs) == 0 {
			c.ArtifactRefs = nil
		} else {
			c.ArtifactRefs = refs
		}
	}
	c.Disposition = c.Disposition.Normalized()
	c.FailureClass = FailureClass(strings.TrimSpace(string(c.FailureClass)))
	c.FailureCode = strings.TrimSpace(c.FailureCode)
	c.FailureSignature = strings.TrimSpace(c.FailureSignature)
	c.PredecessorRunID = common.RunID(strings.TrimSpace(string(c.PredecessorRunID)))
	c.PredecessorResultID = common.ResultID(strings.TrimSpace(string(c.PredecessorResultID)))
	return c
}
