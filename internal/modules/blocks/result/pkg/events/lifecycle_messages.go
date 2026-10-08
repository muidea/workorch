package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RUNTIME-SMO result lifecycle contracts.
//
// Result owner alone constructs and persists accepted/rejected Result from a
// sealed terminal delivery fact. Callers never set Result.State as a target.

// ResultMaterial is immutable outcome content sealed by the Orchestrator. It
// deliberately excludes Result owner lifecycle fields (State, timestamps,
// AcceptedBy and TerminalDelivery); Result Biz derives those after validating
// the terminal fact.
type ResultMaterial struct {
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
	RuntimeCloseout *runtimecontract.RuntimeResultCloseout
}

func (m ResultMaterial) Normalized() ResultMaterial {
	m.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(m.WorkspaceID)))
	m.RunID = common.RunID(strings.TrimSpace(string(m.RunID)))
	m.GoalID = common.GoalID(strings.TrimSpace(string(m.GoalID)))
	m.DoDID = common.DoDID(strings.TrimSpace(string(m.DoDID)))
	m.Title = strings.TrimSpace(m.Title)
	m.Summary = strings.TrimSpace(m.Summary)
	m.SourceType = strings.TrimSpace(m.SourceType)
	m.SourceRef = strings.TrimSpace(m.SourceRef)
	m.EvidenceRefs = normalizedStringRefs(m.EvidenceRefs)
	m.ArtifactRefs = normalizedArtifactRefs(m.ArtifactRefs)
	m.DoDAssessment = normalizeDoDAssessmentMaterial(m.DoDAssessment)
	m.SummaryDetail = normalizeResultSummaryMaterial(m.SummaryDetail)
	if m.RuntimeCloseout != nil {
		closeout := m.RuntimeCloseout.Normalized()
		closeout.ArtifactRefs = normalizedArtifactRefs(closeout.ArtifactRefs)
		m.RuntimeCloseout = &closeout
	}
	return m
}

func normalizeDoDAssessmentMaterial(item workorchresultcontract.DoDAssessment) workorchresultcontract.DoDAssessment {
	item = item.Clone()
	for idx := range item.Items {
		item.Items[idx].EvidenceRefs = normalizedStringRefs(item.Items[idx].EvidenceRefs)
	}
	for _, coverage := range []*workorchresultcontract.DoDCoverage{
		item.GoalCoverage,
		item.AcceptanceDimensions,
		item.DeliverableCoverage,
	} {
		if coverage == nil {
			continue
		}
		for idx := range coverage.Items {
			coverage.Items[idx].EvidenceRefs = normalizedStringRefs(coverage.Items[idx].EvidenceRefs)
			coverage.Items[idx].ArtifactRefs = normalizedArtifactRefs(coverage.Items[idx].ArtifactRefs)
		}
	}
	return item
}

func normalizeResultSummaryMaterial(item workorchresultcontract.ResultSummary) workorchresultcontract.ResultSummary {
	item = item.Clone()
	item.EvidenceRefs = normalizedStringRefs(item.EvidenceRefs)
	item.ArtifactRefs = normalizedArtifactRefs(item.ArtifactRefs)
	for idx := range item.DoDItems {
		item.DoDItems[idx].EvidenceRefs = normalizedStringRefs(item.DoDItems[idx].EvidenceRefs)
	}
	return item
}

func (m ResultMaterial) Validate() error {
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
	if err := validateArtifactReferenceClosure(m); err != nil {
		return err
	}
	if _, err := CanonicalResultMaterialHash(m); err != nil {
		return fmt.Errorf("%w: result material is not canonical: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func validateArtifactReferenceClosure(material ResultMaterial) error {
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
	for _, coverage := range []*workorchresultcontract.DoDCoverage{
		material.DoDAssessment.GoalCoverage,
		material.DoDAssessment.AcceptanceDimensions,
		material.DoDAssessment.DeliverableCoverage,
	} {
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

// CanonicalResultMaterialHash binds the complete normalized sealed material
// to the owner-local Result lifecycle fence. JSON is used only as a
// deterministic encoding of this concrete typed value; it is never used as a
// cross-owner transport or authority reconstruction bridge.
func CanonicalResultMaterialHash(material ResultMaterial) (string, error) {
	encoded, err := json.Marshal(material.Normalized())
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "result-material-sha256:" + hex.EncodeToString(digest[:]), nil
}

func normalizedStringRefs(values []string) []string {
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

func normalizedArtifactRefs(values []common.ArtifactID) []common.ArtifactID {
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

// RuntimeTerminalDeliveryFact is an immutable SM-4 terminal delivery fact for
// the Result owner. The payload carries sealed terminal identity and immutable
// outcome material; Result Biz decides accepted/rejected.
type RuntimeTerminalDeliveryFact struct {
	runtimecontract.RuntimeTerminalDeliveryFact
	Material ResultMaterial
}

func (c RuntimeTerminalDeliveryFact) Normalized() RuntimeTerminalDeliveryFact {
	c.RuntimeTerminalDeliveryFact = c.RuntimeTerminalDeliveryFact.Normalized()
	c.Material = c.Material.Normalized()
	return c
}

func (c RuntimeTerminalDeliveryFact) Validate() error {
	if err := c.Material.Validate(); err != nil {
		return err
	}
	c = c.Normalized()
	if c.SourceOwner != "kernel/orchestrator" {
		return fmt.Errorf("%w: result terminal delivery requires workspace, run and source", common.ErrInvalidArgument)
	}
	if err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil {
		return fmt.Errorf("%w: result terminal delivery fact: %v", common.ErrInvalidArgument, err)
	}
	if err := c.Material.Validate(); err != nil {
		return err
	}
	if c.Material.WorkspaceID != c.WorkspaceID || c.Material.RunID != c.RunID {
		return fmt.Errorf("%w: result material scope does not match terminal delivery", common.ErrConflict)
	}
	if c.Material.RuntimeCloseout != nil && c.Material.RuntimeCloseout.CloseoutID != "" {
		closeout := c.Material.RuntimeCloseout.Normalized()
		if closeout.DefinitionRevision != c.DefinitionRevision || closeout.TerminalOutcome != c.Outcome {
			return fmt.Errorf("%w: formal runtime closeout does not match terminal delivery authority", common.ErrConflict)
		}
	}
	return nil
}

func (c RuntimeTerminalDeliveryFact) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return c.Normalized().RuntimeTerminalDeliveryFact.Identity()
}

// ResultLifecycleCommandResult is the synchronous result of a terminal delivery.
type ResultLifecycleCommandResult struct {
	Result       *resultmodel.Result
	Transitioned bool
	DeliveryAck  *runtimecontract.RuntimeDeliveryAcknowledgement
}
