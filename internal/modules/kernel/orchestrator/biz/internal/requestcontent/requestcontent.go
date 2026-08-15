// Package requestcontent contains immutable, deterministic request-content
// projections. It has no repository, EventHub, provider or Runtime state
// access; callers decide when a projection is persisted or sent.
package requestcontent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	maxSemanticCatalogs      = 32
	maxSemanticContextValues = 256
)

// SemanticPlanRequestInput is the detached input available when an
// Orchestrator drive is about to call the LLM owner. It contains only sealed
// manifest facts and the exact persisted correction lineage.
type SemanticPlanRequestInput struct {
	Manifest                 runtimecontract.RuntimeSemanticPlanManifest
	StablePrefixHash         string
	Attempt                  int
	PreviousRejection        *runtimecontract.RuntimeOriginalDiagnostic
	CorrectionDirective      *runtimecontract.RuntimeSemanticCorrectionDirective
	RemainingRecoveryAttempt int
}

// SemanticPlanRequestContent is the provider-safe volatile projection. It is
// intentionally not a Runtime contract or state record. Catalog and target
// revision values are descriptive facts; ExistingTarget lists owner-published
// admissible existing-target surfaces and never selects one.
type SemanticPlanRequestContent struct {
	Attempt                  int
	StablePrefixHash         string
	ManifestHash             string
	DefinitionSeedHash       string
	CatalogSnapshotIDs       []string
	CatalogObservedRevisions []string
	ConstraintRefs           []string
	PreviousFailureClass     string
	PreviousFailureCode      string
	PreviousFailureSignature string
	PreviousTargetRefs       []string
	PreviousExpectedRevision string
	PreviousObservedRevision string
	ExistingTarget           semanticplan.ExistingTargetContext
	RemainingRecoveryAttempt int
}

// BuildSemanticPlanRequestContent validates correction lineage and builds a
// bounded immutable projection. Attempt-1 never derives a correction or retry
// decision; it only verifies the values already sealed by SM-3.
func BuildSemanticPlanRequestContent(input SemanticPlanRequestInput) (SemanticPlanRequestContent, error) {
	manifest := input.Manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return SemanticPlanRequestContent{}, err
	}
	if strings.TrimSpace(input.StablePrefixHash) == "" {
		return SemanticPlanRequestContent{}, fmt.Errorf("semantic request content requires stable prefix hash")
	}
	if input.Attempt < 0 || input.Attempt > 1 {
		return SemanticPlanRequestContent{}, fmt.Errorf("semantic request content attempt must be 0 or 1")
	}
	if input.RemainingRecoveryAttempt < 0 {
		return SemanticPlanRequestContent{}, fmt.Errorf("semantic request content recovery budget cannot be negative")
	}
	if input.Attempt == 0 && (input.PreviousRejection != nil || input.CorrectionDirective != nil) {
		return SemanticPlanRequestContent{}, fmt.Errorf("initial semantic request content cannot carry correction lineage")
	}
	if input.Attempt == 1 {
		if input.PreviousRejection == nil || input.CorrectionDirective == nil {
			return SemanticPlanRequestContent{}, fmt.Errorf("semantic request content attempt-1 requires rejection and correction")
		}
		if err := input.PreviousRejection.Validate(); err != nil {
			return SemanticPlanRequestContent{}, err
		}
		if err := input.CorrectionDirective.ValidateAgainst(manifest, *input.PreviousRejection); err != nil {
			return SemanticPlanRequestContent{}, err
		}
		if input.CorrectionDirective.StablePrefixHash != strings.TrimSpace(input.StablePrefixHash) {
			return SemanticPlanRequestContent{}, fmt.Errorf("semantic request content correction prefix is stale")
		}
	}
	content := SemanticPlanRequestContent{
		Attempt: input.Attempt, StablePrefixHash: strings.TrimSpace(input.StablePrefixHash), ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, RemainingRecoveryAttempt: input.RemainingRecoveryAttempt,
	}
	for _, catalog := range manifest.OwnerCatalogs {
		catalog = catalog.Normalized()
		content.CatalogSnapshotIDs = appendUnique(content.CatalogSnapshotIDs, catalog.SnapshotID)
		content.CatalogObservedRevisions = appendUnique(content.CatalogObservedRevisions, catalog.ObservedRevision)
		content.ConstraintRefs = appendUnique(content.ConstraintRefs, catalog.ConstraintRefs...)
		for _, variant := range catalog.OperationVariants {
			content.ConstraintRefs = appendUnique(content.ConstraintRefs, variant.ConstraintRefs...)
		}
	}
	if len(content.CatalogSnapshotIDs) > maxSemanticCatalogs || len(content.CatalogObservedRevisions) > maxSemanticContextValues || len(content.ConstraintRefs) > maxSemanticContextValues {
		return SemanticPlanRequestContent{}, fmt.Errorf("semantic request content exceeds bounded catalog context")
	}
	if input.PreviousRejection != nil {
		previous := input.PreviousRejection.CloneOriginal()
		content.PreviousFailureClass = strings.TrimSpace(previous.OriginalClass)
		content.PreviousFailureCode = strings.TrimSpace(previous.OriginalCode)
		content.PreviousFailureSignature = strings.TrimSpace(previous.Signature)
		content.PreviousTargetRefs = appendUnique(content.PreviousTargetRefs, previous.TargetRefs...)
		content.PreviousExpectedRevision = strings.TrimSpace(previous.ExpectedRevision)
		content.PreviousObservedRevision = strings.TrimSpace(previous.ObservedRevision)
		existing, err := semanticplan.BuildExistingTargetContext(manifest, previous)
		if err != nil {
			return SemanticPlanRequestContent{}, err
		}
		content.ExistingTarget = existing
	}
	return content, nil
}

// Validate re-checks the bounded projection before it is handed to another
// adapter. It does not re-open or mutate any owner state.
func (c SemanticPlanRequestContent) Validate() error {
	if c.Attempt < 0 || c.Attempt > 1 || c.StablePrefixHash == "" || c.ManifestHash == "" || c.DefinitionSeedHash == "" || c.RemainingRecoveryAttempt < 0 {
		return fmt.Errorf("semantic request content identity is incomplete")
	}
	if len(c.CatalogSnapshotIDs) > maxSemanticCatalogs || len(c.CatalogObservedRevisions) > maxSemanticContextValues || len(c.ConstraintRefs) > maxSemanticContextValues {
		return fmt.Errorf("semantic request content exceeds bounded context")
	}
	if c.Attempt == 0 && (c.PreviousFailureCode != "" || c.PreviousFailureSignature != "" || len(c.PreviousTargetRefs) != 0) {
		return fmt.Errorf("initial semantic request content contains failure lineage")
	}
	return nil
}

// StableHash is an audit/cache identity for the volatile projection. It is
// intentionally distinct from the stable C0-C2 prefix hash.
func (c SemanticPlanRequestContent) StableHash() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ContentHash is used by the opaque persisted request-content carrier. It is
// kept here so raw document hashing and semantic volatile hashing share one
// stateless package without importing repository code.
func ContentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func ContentTail(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	if len(value) > max {
		return value[len(value)-max:]
	}
	return value
}

func appendUnique(values []string, additions ...string) []string {
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}
