package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeDefinitionGapResultKind                   = "definition_clarification_required"
	RuntimeDefinitionClarificationSchemaCurrent      = "workorch.runtime_definition_clarification"
	RuntimeDefinitionClarificationSignatureVersionV1 = "v1"
	RuntimeDefinitionClarificationRequiredOutputKind = "definition/clarification"

	RuntimeDefinitionClarificationMaxOffendingRefs      = 64
	RuntimeDefinitionClarificationMaxPrerequisiteClaims = 64
	RuntimeDefinitionClarificationMaxCanonicalBytes     = 64 * 1024
	RuntimeDefinitionClarificationMaxDiagnosticBytes    = 4 * 1024
)

type RuntimeDefinitionGapKind string

const (
	RuntimeDefinitionGapImplicitOneOf          RuntimeDefinitionGapKind = "implicit_one_of"
	RuntimeDefinitionGapUndeclaredPrerequisite RuntimeDefinitionGapKind = "undeclared_prerequisite"
	RuntimeDefinitionGapSemanticAmbiguity      RuntimeDefinitionGapKind = "semantic_ambiguity"
)

// RuntimeDefinitionGapProviderDocument is the bounded factual alternative to
// a semantic-plan document. It contains no workspace/run/ticket/authority
// identity; the Orchestrator derives those values only after the complete
// provider document has been persisted.
type RuntimeDefinitionGapProviderDocument struct {
	DocumentKind             RuntimeProviderDocumentKind             `json:"documentKind"`
	SchemaVersion            string                                  `json:"schemaVersion"`
	ManifestHash             string                                  `json:"manifestHash"`
	DefinitionSeedHash       string                                  `json:"definitionSeedHash"`
	ResultKind               string                                  `json:"resultKind"`
	PlanStep                 RuntimeSemanticPlanStep                 `json:"planStep"`
	BatchID                  string                                  `json:"batchId,omitempty"`
	BatchSequence            *int                                    `json:"batchSequence,omitempty"`
	BatchCoverageWorkItemIDs []common.CoverageWorkItemID             `json:"batchCoverageWorkItemIds,omitempty"`
	GapKind                  RuntimeDefinitionGapKind                `json:"gapKind"`
	OffendingItemRefs        []string                                `json:"offendingItemRefs"`
	PrerequisiteClaims       []RuntimeDefinitionGapPrerequisiteClaim `json:"prerequisiteClaims,omitempty"`
	DiagnosticMessage        string                                  `json:"diagnosticMessage"`
}

// RuntimeDefinitionGapPrerequisiteClaim is the provider's bounded factual
// assertion that one exact owner-fact requirement is absent from the sealed
// Definition for one CWI. It is not authority: pure manifest admission must
// prove the requirement is genuinely undeclared before any owner state can
// observe the gap.
type RuntimeDefinitionGapPrerequisiteClaim struct {
	CoverageWorkItemID common.CoverageWorkItemID `json:"coverageWorkItemId"`
	OwnerModuleID      string                    `json:"ownerModuleId"`
	FactKind           RuntimeOwnerFactKind      `json:"factKind"`
	OperationKind      string                    `json:"operationKind"`
}

func (c RuntimeDefinitionGapPrerequisiteClaim) Normalized() RuntimeDefinitionGapPrerequisiteClaim {
	c.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(c.CoverageWorkItemID)))
	requirement := c.Requirement().Normalized()
	c.OwnerModuleID = requirement.OwnerModuleID
	c.FactKind = requirement.FactKind
	c.OperationKind = requirement.OperationKind
	return c
}

func (c RuntimeDefinitionGapPrerequisiteClaim) Requirement() RuntimeOwnerFactRequirement {
	return RuntimeOwnerFactRequirement{
		OwnerModuleID: c.OwnerModuleID,
		FactKind:      c.FactKind,
		OperationKind: c.OperationKind,
	}
}

func (c RuntimeDefinitionGapPrerequisiteClaim) Validate() error {
	c = c.Normalized()
	if c.CoverageWorkItemID == "" {
		return fmt.Errorf("runtime Definition gap prerequisite claim requires a coverage work item")
	}
	return c.Requirement().Validate()
}

func normalizeRuntimeDefinitionGapPrerequisiteClaims(values []RuntimeDefinitionGapPrerequisiteClaim) []RuntimeDefinitionGapPrerequisiteClaim {
	ret := append([]RuntimeDefinitionGapPrerequisiteClaim(nil), values...)
	for idx := range ret {
		ret[idx] = ret[idx].Normalized()
	}
	sort.Slice(ret, func(i, j int) bool {
		left := string(ret[i].CoverageWorkItemID) + "|" + RuntimeOwnerFactRequirementRef(ret[i].Requirement())
		right := string(ret[j].CoverageWorkItemID) + "|" + RuntimeOwnerFactRequirementRef(ret[j].Requirement())
		return left < right
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (d RuntimeDefinitionGapProviderDocument) Normalized() RuntimeDefinitionGapProviderDocument {
	d.DocumentKind = RuntimeProviderDocumentKind(strings.TrimSpace(string(d.DocumentKind)))
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.DefinitionSeedHash = strings.TrimSpace(d.DefinitionSeedHash)
	d.ResultKind = strings.TrimSpace(d.ResultKind)
	d.PlanStep = RuntimeSemanticPlanStep(strings.TrimSpace(string(d.PlanStep)))
	d.BatchID = strings.TrimSpace(d.BatchID)
	d.BatchCoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(d.BatchCoverageWorkItemIDs)
	d.GapKind = RuntimeDefinitionGapKind(strings.TrimSpace(string(d.GapKind)))
	d.OffendingItemRefs = normalizeStrings(d.OffendingItemRefs)
	d.PrerequisiteClaims = normalizeRuntimeDefinitionGapPrerequisiteClaims(d.PrerequisiteClaims)
	d.DiagnosticMessage = strings.TrimSpace(d.DiagnosticMessage)
	return d
}

func (d RuntimeDefinitionGapProviderDocument) Clone() RuntimeDefinitionGapProviderDocument {
	d = d.Normalized()
	d.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), d.BatchCoverageWorkItemIDs...)
	d.OffendingItemRefs = append([]string(nil), d.OffendingItemRefs...)
	d.PrerequisiteClaims = append([]RuntimeDefinitionGapPrerequisiteClaim(nil), d.PrerequisiteClaims...)
	if d.BatchSequence != nil {
		value := *d.BatchSequence
		d.BatchSequence = &value
	}
	return d
}

func (d RuntimeDefinitionGapProviderDocument) ValidateAgainst(manifest RuntimeSemanticPlanManifest, expected RuntimeExpectedDocumentContract) error {
	raw := d
	d = d.Normalized()
	manifest = manifest.Normalized()
	expected = expected.Normalized()
	if len(raw.OffendingItemRefs) != len(d.OffendingItemRefs) || len(raw.BatchCoverageWorkItemIDs) != len(d.BatchCoverageWorkItemIDs) {
		return fmt.Errorf("runtime Definition gap document contains duplicate or empty identity values")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if d.DocumentKind != RuntimeProviderDocumentSemanticPlan || d.SchemaVersion != RuntimeSemanticPlanDocumentSchemaCurrent ||
		d.ManifestHash != manifest.Hash || d.DefinitionSeedHash != manifest.DefinitionSeedHash ||
		d.ResultKind != RuntimeDefinitionGapResultKind || d.PlanStep != expected.PlanStep {
		return fmt.Errorf("runtime Definition gap document identity does not match the sealed semantic request")
	}
	switch d.GapKind {
	case RuntimeDefinitionGapImplicitOneOf, RuntimeDefinitionGapUndeclaredPrerequisite, RuntimeDefinitionGapSemanticAmbiguity:
	default:
		return fmt.Errorf("runtime Definition gap document has invalid gap kind %q", d.GapKind)
	}
	if len(d.OffendingItemRefs) == 0 || len(d.OffendingItemRefs) > RuntimeDefinitionClarificationMaxOffendingRefs {
		return fmt.Errorf("runtime Definition gap offending refs exceed the bounded clarification budget")
	}
	if len([]byte(d.DiagnosticMessage)) == 0 || len([]byte(d.DiagnosticMessage)) > RuntimeDefinitionClarificationMaxDiagnosticBytes {
		return fmt.Errorf("runtime Definition gap diagnostic exceeds the bounded clarification budget")
	}
	if err := validateRuntimeFeedbackSafeText("runtime Definition gap diagnostic", d.DiagnosticMessage); err != nil {
		return err
	}
	coverageByRef := make(map[string][]common.CoverageWorkItemID, len(manifest.CoverageItems)*2)
	coverageIDRefs := make(map[string]struct{}, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		ref := string(item.ID)
		coverageIDRefs[ref] = struct{}{}
		coverageByRef[ref] = []common.CoverageWorkItemID{item.ID}
	}
	for _, item := range manifest.CoverageItems {
		ref := strings.TrimSpace(item.SourceID)
		if _, exactCoverageID := coverageIDRefs[ref]; exactCoverageID {
			continue
		}
		duplicate := false
		for _, current := range coverageByRef[ref] {
			if current == item.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			coverageByRef[ref] = append(coverageByRef[ref], item.ID)
		}
	}
	offendingCoverage := make(map[common.CoverageWorkItemID]struct{}, len(d.OffendingItemRefs))
	for _, ref := range d.OffendingItemRefs {
		coverageIDs := coverageByRef[ref]
		if len(coverageIDs) == 0 {
			return fmt.Errorf("runtime Definition gap offending ref %q is outside the sealed manifest", ref)
		}
		for _, coverageID := range coverageIDs {
			offendingCoverage[coverageID] = struct{}{}
		}
	}
	switch d.GapKind {
	case RuntimeDefinitionGapUndeclaredPrerequisite:
		if len(d.PrerequisiteClaims) == 0 {
			return fmt.Errorf("runtime Definition gap undeclared prerequisite requires exact typed prerequisite claims")
		}
		if len(d.PrerequisiteClaims) > RuntimeDefinitionClarificationMaxPrerequisiteClaims {
			return fmt.Errorf("runtime Definition gap prerequisite claims exceed the bounded clarification budget")
		}
		claimedCoverage := make(map[common.CoverageWorkItemID]struct{}, len(d.PrerequisiteClaims))
		seenClaims := make(map[string]struct{}, len(d.PrerequisiteClaims))
		for _, claim := range d.PrerequisiteClaims {
			if err := claim.Validate(); err != nil {
				return err
			}
			if _, ok := offendingCoverage[claim.CoverageWorkItemID]; !ok {
				return fmt.Errorf("runtime Definition gap prerequisite claim coverage %q is not an offending item", claim.CoverageWorkItemID)
			}
			claimRef := string(claim.CoverageWorkItemID) + "|" + RuntimeOwnerFactRequirementRef(claim.Requirement())
			if _, duplicate := seenClaims[claimRef]; duplicate {
				return fmt.Errorf("runtime Definition gap prerequisite claim %q is duplicated", claimRef)
			}
			seenClaims[claimRef] = struct{}{}
			claimedCoverage[claim.CoverageWorkItemID] = struct{}{}
		}
		for coverageID := range offendingCoverage {
			if _, ok := claimedCoverage[coverageID]; !ok {
				return fmt.Errorf("runtime Definition gap offending coverage %q has no exact prerequisite claim", coverageID)
			}
		}
	default:
		if len(d.PrerequisiteClaims) != 0 {
			return fmt.Errorf("runtime Definition gap kind %q cannot carry prerequisite claims", d.GapKind)
		}
	}
	switch expected.PlanStep {
	case RuntimeSemanticPlanStepCoverageOrder:
		if d.BatchID != "" || d.BatchSequence != nil || len(d.BatchCoverageWorkItemIDs) != 0 {
			return fmt.Errorf("coverage-order Definition gap cannot carry proof-batch identity")
		}
	case RuntimeSemanticPlanStepProofBatch:
		if d.BatchID != expected.BatchID || d.BatchSequence == nil || *d.BatchSequence != expected.BatchSequence ||
			!sameCoverageIDsInOrder(d.BatchCoverageWorkItemIDs, expected.BatchCoverageWorkItemIDs) {
			return fmt.Errorf("proof-batch Definition gap does not match the sealed batch identity")
		}
		batchCoverage := make(map[common.CoverageWorkItemID]struct{}, len(expected.BatchCoverageWorkItemIDs))
		for _, coverageID := range expected.BatchCoverageWorkItemIDs {
			batchCoverage[coverageID] = struct{}{}
		}
		for coverageID := range offendingCoverage {
			if _, ok := batchCoverage[coverageID]; !ok {
				return fmt.Errorf("proof-batch Definition gap offending coverage %q is outside the sealed batch", coverageID)
			}
		}
	default:
		return fmt.Errorf("runtime Definition gap has unsupported semantic plan step %q", expected.PlanStep)
	}
	return nil
}

// RuntimeDefinitionGapAdmissionDisposition is a pure, manifest-bound
// classification of one structurally valid provider gap document. It neither
// selects a Runtime transition nor implies a retry or ticket action.
type RuntimeDefinitionGapAdmissionDisposition string

const (
	RuntimeDefinitionGapAdmissionAdmitted      RuntimeDefinitionGapAdmissionDisposition = "admitted"
	RuntimeDefinitionGapAdmissionRejected      RuntimeDefinitionGapAdmissionDisposition = "rejected"
	RuntimeDefinitionGapAdmissionStaleConflict RuntimeDefinitionGapAdmissionDisposition = "stale_conflict"

	RuntimeDefinitionGapAdmissionValidationRepairForbidden = "validation_failure_repair_gap_forbidden"
)

// RuntimeDefinitionGapAdmissionDecision carries a stable classification code;
// Message is diagnostic only and must never be parsed to select a transition.
type RuntimeDefinitionGapAdmissionDecision struct {
	Disposition RuntimeDefinitionGapAdmissionDisposition
	Code        string
	Message     string
}

func (d RuntimeDefinitionGapAdmissionDecision) Admitted() bool {
	return d.Disposition == RuntimeDefinitionGapAdmissionAdmitted
}

func runtimeDefinitionGapAdmissionFailure(disposition RuntimeDefinitionGapAdmissionDisposition, code string, err error) RuntimeDefinitionGapAdmissionDecision {
	decision := RuntimeDefinitionGapAdmissionDecision{Disposition: disposition, Code: strings.TrimSpace(code)}
	if err != nil {
		decision.Message = strings.TrimSpace(err.Error())
	}
	return decision
}

// EvaluateRuntimeDefinitionGapAdmission evaluates only detached manifest,
// expected-document and provider-document values. It owns no state, CAS,
// EventHub, retry accounting or action selection. In particular, a provider
// cannot classify a prerequisite as undeclared when the same sealed manifest
// already declares, satisfies, or carries an exact acquisition for that owner
// fact on the offending CWI.
func EvaluateRuntimeDefinitionGapAdmission(manifest RuntimeSemanticPlanManifest, expected RuntimeExpectedDocumentContract, document RuntimeDefinitionGapProviderDocument) RuntimeDefinitionGapAdmissionDecision {
	if err := manifest.Validate(); err != nil {
		return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "manifest_invalid", err)
	}
	if err := expected.Validate(); err != nil {
		return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "expected_document_invalid", err)
	}
	normalizedManifest := manifest.Normalized()
	normalizedExpected := expected.Normalized()
	if normalizedExpected.DocumentKind != RuntimeProviderDocumentSemanticPlan ||
		normalizedExpected.ManifestHash != normalizedManifest.Hash ||
		normalizedExpected.DefinitionSeedHash != normalizedManifest.DefinitionSeedHash {
		return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionStaleConflict, "expected_document_stale", fmt.Errorf("runtime Definition gap expected document does not match the sealed manifest"))
	}
	if !runtimeDefinitionGapDocumentMatchesExpectedIdentity(document, normalizedManifest, normalizedExpected) {
		return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionStaleConflict, "document_identity_stale", fmt.Errorf("runtime Definition gap document does not match the sealed request identity"))
	}
	if err := document.ValidateAgainst(normalizedManifest, normalizedExpected); err != nil {
		return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "document_invalid", err)
	}
	if err := ValidateRuntimeSemanticRequestContract(normalizedManifest, normalizedExpected.PlanStep, normalizedExpected.BatchCoverageWorkItemIDs); err != nil {
		return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "semantic_request_contract_conflict", err)
	}
	for _, obligation := range normalizedManifest.ProofObligations {
		evidence, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
		if err != nil {
			return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "manifest_invalid", err)
		}
		if evidence.Purpose == RuntimeSemanticEvidenceObligationValidationFailureRepair {
			return runtimeDefinitionGapAdmissionFailure(
				RuntimeDefinitionGapAdmissionRejected,
				RuntimeDefinitionGapAdmissionValidationRepairForbidden,
				fmt.Errorf("runtime validation-failure repair cannot be reclassified as a Definition gap"),
			)
		}
	}
	normalizedDocument := document.Normalized()
	if normalizedDocument.GapKind == RuntimeDefinitionGapUndeclaredPrerequisite {
		obligations := make(map[common.CoverageWorkItemID]RuntimeSemanticProofObligation, len(normalizedManifest.ProofObligations))
		for _, obligation := range normalizedManifest.ProofObligations {
			obligations[obligation.CoverageWorkItemID] = obligation
		}
		catalogs := append(CloneRuntimeOperationAdmissibilities(normalizedManifest.OwnerCatalogs), normalizedManifest.OwnerFactDependencyCatalogs...)
		for _, claim := range normalizedDocument.PrerequisiteClaims {
			obligation := obligations[claim.CoverageWorkItemID]
			requirement := claim.Requirement().Normalized()
			declared := false
			for _, current := range obligation.RequiredOwnerFacts {
				if current.Normalized() == requirement {
					declared = true
					break
				}
			}
			if !declared {
				continue
			}
			if len(RuntimeMissingOwnerFactRequirements([]RuntimeOwnerFactRequirement{requirement}, catalogs)) == 0 {
				return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "owner_fact_requirement_satisfied", fmt.Errorf("runtime Definition gap prerequisite %q is already satisfied by the sealed owner catalog", RuntimeOwnerFactRequirementRef(requirement)))
			}
			if obligation.OwnerFactAcquisition != nil && obligation.OwnerFactAcquisition.Requirement.Normalized() == requirement {
				return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "owner_fact_acquisition_declared", fmt.Errorf("runtime Definition gap prerequisite %q already has a sealed owner-fact acquisition", RuntimeOwnerFactRequirementRef(requirement)))
			}
			return runtimeDefinitionGapAdmissionFailure(RuntimeDefinitionGapAdmissionRejected, "owner_fact_requirement_declared", fmt.Errorf("runtime Definition gap prerequisite %q is already declared by the sealed proof obligation", RuntimeOwnerFactRequirementRef(requirement)))
		}
	}
	return RuntimeDefinitionGapAdmissionDecision{Disposition: RuntimeDefinitionGapAdmissionAdmitted, Code: "gap_admitted"}
}

func runtimeDefinitionGapDocumentMatchesExpectedIdentity(document RuntimeDefinitionGapProviderDocument, manifest RuntimeSemanticPlanManifest, expected RuntimeExpectedDocumentContract) bool {
	document = document.Normalized()
	manifest = manifest.Normalized()
	expected = expected.Normalized()
	if document.DocumentKind != RuntimeProviderDocumentSemanticPlan || document.ManifestHash != manifest.Hash ||
		document.DefinitionSeedHash != manifest.DefinitionSeedHash || document.PlanStep != expected.PlanStep {
		return false
	}
	switch expected.PlanStep {
	case RuntimeSemanticPlanStepCoverageOrder:
		return document.BatchID == "" && document.BatchSequence == nil && len(document.BatchCoverageWorkItemIDs) == 0
	case RuntimeSemanticPlanStepProofBatch:
		return document.BatchID == expected.BatchID && document.BatchSequence != nil && *document.BatchSequence == expected.BatchSequence &&
			sameCoverageIDsInOrder(document.BatchCoverageWorkItemIDs, expected.BatchCoverageWorkItemIDs)
	default:
		return false
	}
}

func sameCoverageIDsInOrder(left, right []common.CoverageWorkItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func RuntimeSemanticPayloadIsDefinitionGap(payload []byte) bool {
	var envelope struct {
		ResultKind string `json:"resultKind"`
	}
	return json.Unmarshal(payload, &envelope) == nil && strings.TrimSpace(envelope.ResultKind) == RuntimeDefinitionGapResultKind
}

func StableRuntimeDefinitionGapProviderDocumentHash(document RuntimeDefinitionGapProviderDocument) string {
	document = document.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-definition-gap-provider-document")
	writeHashString(h, string(document.DocumentKind))
	writeHashString(h, document.SchemaVersion)
	writeHashString(h, document.ManifestHash)
	writeHashString(h, document.DefinitionSeedHash)
	writeHashString(h, document.ResultKind)
	writeHashString(h, string(document.PlanStep))
	writeHashString(h, document.BatchID)
	if document.BatchSequence != nil {
		writeHashString(h, fmt.Sprintf("%d", *document.BatchSequence))
	}
	for _, id := range document.BatchCoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	writeHashString(h, string(document.GapKind))
	writeHashStrings(h, document.OffendingItemRefs)
	for _, claim := range document.PrerequisiteClaims {
		writeHashString(h, string(claim.CoverageWorkItemID))
		writeHashString(h, RuntimeOwnerFactRequirementRef(claim.Requirement()))
	}
	writeHashString(h, document.DiagnosticMessage)
	return "runtime-definition-gap-document-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func RuntimeOriginalDiagnosticFromDefinitionGap(document RuntimeDefinitionGapProviderDocument) RuntimeOriginalDiagnostic {
	document = document.Normalized()
	diagnostic := RuntimeOriginalDiagnostic{
		SchemaVersion:   RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID:   RuntimeModuleLLM,
		OriginalClass:   RuntimeDefinitionGapResultKind,
		OriginalCode:    string(document.GapKind),
		OriginalMessage: document.DiagnosticMessage,
		TargetRefs:      append([]string(nil), document.OffendingItemRefs...),
	}
	diagnostic.Signature = StableRuntimeDefinitionGapProviderDocumentHash(document)
	return diagnostic
}

type RuntimeDefinitionGapObservation struct {
	SchemaVersion        string
	ObservationID        string
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	DefinitionRevision   string
	StartupInputHash     string
	AuthoritySnapshotID  string
	FocusRef             string
	LogicalRequestID     string
	SemanticDocumentHash string
	GapKind              RuntimeDefinitionGapKind
	OffendingItemRefs    []string
	OriginalDiagnostic   RuntimeOriginalDiagnostic
}

func (o RuntimeDefinitionGapObservation) Normalized() RuntimeDefinitionGapObservation {
	o.SchemaVersion = strings.TrimSpace(o.SchemaVersion)
	o.ObservationID = strings.TrimSpace(o.ObservationID)
	o.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(o.WorkspaceID)))
	o.RunID = common.RunID(strings.TrimSpace(string(o.RunID)))
	o.DefinitionRevision = strings.TrimSpace(o.DefinitionRevision)
	o.StartupInputHash = strings.TrimSpace(o.StartupInputHash)
	o.AuthoritySnapshotID = strings.TrimSpace(o.AuthoritySnapshotID)
	o.FocusRef = strings.TrimSpace(o.FocusRef)
	o.LogicalRequestID = strings.TrimSpace(o.LogicalRequestID)
	o.SemanticDocumentHash = strings.TrimSpace(o.SemanticDocumentHash)
	o.GapKind = RuntimeDefinitionGapKind(strings.TrimSpace(string(o.GapKind)))
	o.OffendingItemRefs = normalizeStrings(o.OffendingItemRefs)
	if diagnostic := o.OriginalDiagnostic.Normalized(); diagnostic != nil {
		o.OriginalDiagnostic = *diagnostic
	}
	return o
}

func (o RuntimeDefinitionGapObservation) Clone() RuntimeDefinitionGapObservation {
	ret := o.Normalized()
	ret.OffendingItemRefs = append([]string(nil), ret.OffendingItemRefs...)
	if diagnostic := o.OriginalDiagnostic.CloneOriginal(); diagnostic != nil {
		ret.OriginalDiagnostic = *diagnostic
	}
	return ret
}

func (o RuntimeDefinitionGapObservation) Validate() error {
	o = o.Normalized()
	if o.SchemaVersion != RuntimeDefinitionClarificationSchemaCurrent || o.ObservationID == "" || o.WorkspaceID == "" || o.RunID == "" ||
		o.DefinitionRevision == "" || o.StartupInputHash == "" || o.AuthoritySnapshotID == "" || o.FocusRef == "" ||
		o.LogicalRequestID == "" || o.SemanticDocumentHash == "" || len(o.OffendingItemRefs) == 0 {
		return fmt.Errorf("runtime Definition gap observation is incomplete")
	}
	switch o.GapKind {
	case RuntimeDefinitionGapImplicitOneOf, RuntimeDefinitionGapUndeclaredPrerequisite, RuntimeDefinitionGapSemanticAmbiguity:
	default:
		return fmt.Errorf("runtime Definition gap observation kind is invalid")
	}
	if err := o.OriginalDiagnostic.Validate(); err != nil {
		return err
	}
	if o.OriginalDiagnostic.OwnerModuleID != RuntimeModuleLLM || o.OriginalDiagnostic.OriginalClass != RuntimeDefinitionGapResultKind ||
		o.OriginalDiagnostic.OriginalCode != string(o.GapKind) || !sameStrings(normalizeStrings(o.OriginalDiagnostic.TargetRefs), o.OffendingItemRefs) {
		return fmt.Errorf("runtime Definition gap observation diagnostic identity mismatch")
	}
	if o.ObservationID != StableRuntimeDefinitionGapObservationID(o) {
		return fmt.Errorf("runtime Definition gap observation identity mismatch")
	}
	return nil
}

func StableRuntimeDefinitionGapObservationID(observation RuntimeDefinitionGapObservation) string {
	o := observation.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-definition-gap-observation")
	writeHashString(h, string(o.WorkspaceID))
	writeHashString(h, string(o.RunID))
	writeHashString(h, o.DefinitionRevision)
	writeHashString(h, o.StartupInputHash)
	writeHashString(h, o.AuthoritySnapshotID)
	writeHashString(h, o.FocusRef)
	writeHashString(h, o.LogicalRequestID)
	writeHashString(h, o.SemanticDocumentHash)
	writeHashString(h, string(o.GapKind))
	writeHashStrings(h, o.OffendingItemRefs)
	writeHashString(h, o.OriginalDiagnostic.Signature)
	return "runtime-definition-gap-observation-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeDefinitionClarificationDispatch struct {
	SchemaVersion                string
	DispatchID                   string
	ObservationID                string
	WorkspaceID                  common.WorkspaceID
	RunID                        common.RunID
	DefinitionRevision           string
	StartupInputHash             string
	AuthoritySnapshotID          string
	FocusRef                     string
	LogicalRequestID             string
	SemanticDocumentHash         string
	GapKind                      RuntimeDefinitionGapKind
	OffendingItemRefs            []string
	CanonicalGapSignatureVersion string
	CanonicalGapSignature        string
	CanonicalGapSignatureHash    string
}

func (d RuntimeDefinitionClarificationDispatch) Normalized() RuntimeDefinitionClarificationDispatch {
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.DispatchID = strings.TrimSpace(d.DispatchID)
	d.ObservationID = strings.TrimSpace(d.ObservationID)
	d.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(d.WorkspaceID)))
	d.RunID = common.RunID(strings.TrimSpace(string(d.RunID)))
	d.DefinitionRevision = strings.TrimSpace(d.DefinitionRevision)
	d.StartupInputHash = strings.TrimSpace(d.StartupInputHash)
	d.AuthoritySnapshotID = strings.TrimSpace(d.AuthoritySnapshotID)
	d.FocusRef = strings.TrimSpace(d.FocusRef)
	d.LogicalRequestID = strings.TrimSpace(d.LogicalRequestID)
	d.SemanticDocumentHash = strings.TrimSpace(d.SemanticDocumentHash)
	d.GapKind = RuntimeDefinitionGapKind(strings.TrimSpace(string(d.GapKind)))
	d.OffendingItemRefs = normalizeStrings(d.OffendingItemRefs)
	d.CanonicalGapSignatureVersion = strings.TrimSpace(d.CanonicalGapSignatureVersion)
	d.CanonicalGapSignature = strings.TrimSpace(d.CanonicalGapSignature)
	d.CanonicalGapSignatureHash = strings.TrimSpace(d.CanonicalGapSignatureHash)
	return d
}

func (d RuntimeDefinitionClarificationDispatch) Clone() RuntimeDefinitionClarificationDispatch {
	d = d.Normalized()
	d.OffendingItemRefs = append([]string(nil), d.OffendingItemRefs...)
	return d
}

func CompileRuntimeDefinitionClarificationDispatch(observation RuntimeDefinitionGapObservation, authoritySnapshotID string) (RuntimeDefinitionClarificationDispatch, error) {
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil {
		return RuntimeDefinitionClarificationDispatch{}, err
	}
	dispatch := RuntimeDefinitionClarificationDispatch{
		SchemaVersion: RuntimeDefinitionClarificationSchemaCurrent,
		ObservationID: observation.ObservationID,
		WorkspaceID:   observation.WorkspaceID, RunID: observation.RunID,
		DefinitionRevision: observation.DefinitionRevision, StartupInputHash: observation.StartupInputHash,
		AuthoritySnapshotID: strings.TrimSpace(authoritySnapshotID), FocusRef: observation.FocusRef,
		LogicalRequestID: observation.LogicalRequestID, SemanticDocumentHash: observation.SemanticDocumentHash,
		GapKind: observation.GapKind, OffendingItemRefs: append([]string(nil), observation.OffendingItemRefs...),
		CanonicalGapSignatureVersion: RuntimeDefinitionClarificationSignatureVersionV1,
	}
	dispatch.DispatchID = StableRuntimeDefinitionClarificationDispatchID(dispatch)
	dispatch.CanonicalGapSignature = RuntimeDefinitionClarificationCanonicalSignature(dispatch)
	dispatch.CanonicalGapSignatureHash = stableRuntimeDefinitionClarificationSignatureHash(dispatch.CanonicalGapSignatureVersion, dispatch.CanonicalGapSignature)
	if err := dispatch.ValidateAgainst(observation); err != nil {
		return RuntimeDefinitionClarificationDispatch{}, err
	}
	return dispatch, nil
}

func (d RuntimeDefinitionClarificationDispatch) ValidateAgainst(observation RuntimeDefinitionGapObservation) error {
	d = d.Normalized()
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil {
		return err
	}
	if err := d.Validate(); err != nil {
		return err
	}
	if d.ObservationID != observation.ObservationID ||
		d.WorkspaceID != observation.WorkspaceID || d.RunID != observation.RunID || d.DefinitionRevision != observation.DefinitionRevision ||
		d.StartupInputHash != observation.StartupInputHash || d.AuthoritySnapshotID == "" || d.FocusRef != observation.FocusRef ||
		d.LogicalRequestID != observation.LogicalRequestID || d.SemanticDocumentHash != observation.SemanticDocumentHash ||
		d.GapKind != observation.GapKind || !sameStrings(d.OffendingItemRefs, observation.OffendingItemRefs) {
		return fmt.Errorf("runtime Definition clarification dispatch does not match its observation")
	}
	return nil
}

func (d RuntimeDefinitionClarificationDispatch) Validate() error {
	d = d.Normalized()
	if d.SchemaVersion != RuntimeDefinitionClarificationSchemaCurrent || d.DispatchID == "" || d.ObservationID == "" || d.WorkspaceID == "" || d.RunID == "" ||
		d.DefinitionRevision == "" || d.StartupInputHash == "" || d.AuthoritySnapshotID == "" || d.FocusRef == "" || d.LogicalRequestID == "" ||
		d.SemanticDocumentHash == "" || len(d.OffendingItemRefs) == 0 || d.CanonicalGapSignatureVersion != RuntimeDefinitionClarificationSignatureVersionV1 {
		return fmt.Errorf("runtime Definition clarification dispatch is incomplete")
	}
	switch d.GapKind {
	case RuntimeDefinitionGapImplicitOneOf, RuntimeDefinitionGapUndeclaredPrerequisite, RuntimeDefinitionGapSemanticAmbiguity:
	default:
		return fmt.Errorf("runtime Definition clarification dispatch kind is invalid")
	}
	if d.DispatchID != StableRuntimeDefinitionClarificationDispatchID(d) || d.CanonicalGapSignature != RuntimeDefinitionClarificationCanonicalSignature(d) ||
		d.CanonicalGapSignatureHash != stableRuntimeDefinitionClarificationSignatureHash(d.CanonicalGapSignatureVersion, d.CanonicalGapSignature) {
		return fmt.Errorf("runtime Definition clarification dispatch signature mismatch")
	}
	if len([]byte(d.CanonicalGapSignature)) > RuntimeDefinitionClarificationMaxCanonicalBytes {
		return fmt.Errorf("runtime Definition clarification signature exceeds its bounded budget")
	}
	return nil
}

func StableRuntimeDefinitionClarificationDispatchID(dispatch RuntimeDefinitionClarificationDispatch) string {
	d := dispatch.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-definition-clarification-dispatch")
	writeHashString(h, d.ObservationID)
	writeHashString(h, d.AuthoritySnapshotID)
	return "runtime-definition-clarification-dispatch-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func RuntimeDefinitionClarificationCanonicalSignature(dispatch RuntimeDefinitionClarificationDispatch) string {
	d := dispatch.Normalized()
	parts := []string{
		string(d.GapKind), string(d.WorkspaceID), string(d.RunID), d.DefinitionRevision, d.StartupInputHash,
		d.AuthoritySnapshotID, d.DispatchID, d.FocusRef, d.LogicalRequestID, d.SemanticDocumentHash,
	}
	parts = append(parts, d.OffendingItemRefs...)
	var builder strings.Builder
	for _, part := range parts {
		_, _ = fmt.Fprintf(&builder, "%08x%s", len(part), part)
	}
	return builder.String()
}

func stableRuntimeDefinitionClarificationSignatureHash(version, signature string) string {
	h := sha256.New()
	for _, part := range []string{strings.TrimSpace(version), strings.TrimSpace(signature)} {
		_, _ = fmt.Fprintf(h, "%08x%s", len(part), part)
	}
	return "runtime-signature-sha256:" + hex.EncodeToString(h.Sum(nil))
}

type RuntimeDefinitionClarificationRequestRecord struct {
	DispatchID         string
	TicketID           common.TicketID
	RequestBindingHash string
}

func (r RuntimeDefinitionClarificationRequestRecord) Normalized() RuntimeDefinitionClarificationRequestRecord {
	r.DispatchID = strings.TrimSpace(r.DispatchID)
	r.TicketID = common.TicketID(strings.TrimSpace(string(r.TicketID)))
	r.RequestBindingHash = strings.TrimSpace(r.RequestBindingHash)
	return r
}

func (r RuntimeDefinitionClarificationRequestRecord) ValidateAgainst(dispatch RuntimeDefinitionClarificationDispatch) error {
	r = r.Normalized()
	dispatch = dispatch.Normalized()
	if r.DispatchID == "" || r.TicketID == "" || r.RequestBindingHash == "" || r.DispatchID != dispatch.DispatchID {
		return fmt.Errorf("runtime Definition clarification request record is incomplete or stale")
	}
	return nil
}

type RuntimeDefinitionClarificationState struct {
	Observation           RuntimeDefinitionGapObservation
	RequestContractRepair *RuntimeSemanticRequestRepairProof `json:",omitempty"`
	Dispatch              *RuntimeDefinitionClarificationDispatch
	Request               *RuntimeDefinitionClarificationRequestRecord
}

func (s RuntimeDefinitionClarificationState) Clone() RuntimeDefinitionClarificationState {
	ret := RuntimeDefinitionClarificationState{Observation: s.Observation.Clone()}
	if s.RequestContractRepair != nil {
		value := s.RequestContractRepair.Clone()
		ret.RequestContractRepair = &value
	}
	if s.Dispatch != nil {
		value := s.Dispatch.Clone()
		ret.Dispatch = &value
	}
	if s.Request != nil {
		value := s.Request.Normalized()
		ret.Request = &value
	}
	return ret
}

func (s RuntimeDefinitionClarificationState) Validate() error {
	s = s.Clone()
	if err := s.Observation.Validate(); err != nil {
		return err
	}
	if s.Dispatch == nil {
		if s.Request != nil || s.RequestContractRepair != nil {
			return fmt.Errorf("runtime Definition clarification request cannot precede dispatch")
		}
		return nil
	}
	if err := s.Dispatch.ValidateAgainst(s.Observation); err != nil {
		return err
	}
	if s.RequestContractRepair != nil {
		if err := s.RequestContractRepair.Validate(); err != nil {
			return err
		}
		if !s.RequestContractRepair.Matches(s) {
			return fmt.Errorf("request contract repair differs from original clarification")
		}
	}
	if s.Request != nil {
		return s.Request.ValidateAgainst(*s.Dispatch)
	}
	return nil
}

// DecodeRuntimeDefinitionGapPayloadWithManifest is intentionally separate
// from normal semantic-plan admission. Callers use it only when resultKind is
// the explicit factual gap discriminator; arbitrary schema failures remain
// candidate correction input and cannot create a ticket.
func DecodeRuntimeDefinitionGapPayloadWithManifest(manifest RuntimeSemanticPlanManifest, expected RuntimeExpectedDocumentContract, payload []byte) (RuntimeDefinitionGapProviderDocument, error) {
	if err := manifest.Validate(); err != nil {
		return RuntimeDefinitionGapProviderDocument{}, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return RuntimeDefinitionGapProviderDocument{}, err
	}
	allowed := []string{"batchCoverageWorkItemIds", "batchId", "batchSequence", "definitionSeedHash", "diagnosticMessage", "documentKind", "gapKind", "manifestHash", "offendingItemRefs", "planStep", "prerequisiteClaims", "resultKind", "schemaVersion"}
	required := []string{"definitionSeedHash", "diagnosticMessage", "documentKind", "gapKind", "manifestHash", "offendingItemRefs", "planStep", "resultKind", "schemaVersion"}
	var gapKind RuntimeDefinitionGapKind
	if rawGapKind, found := envelope["gapKind"]; found {
		_ = json.Unmarshal(rawGapKind, &gapKind)
	}
	if RuntimeDefinitionGapKind(strings.TrimSpace(string(gapKind))) == RuntimeDefinitionGapUndeclaredPrerequisite {
		required = append(required, "prerequisiteClaims")
	}
	if expected.Normalized().PlanStep == RuntimeSemanticPlanStepProofBatch {
		required = append(required, "batchCoverageWorkItemIds", "batchId", "batchSequence")
	}
	if err := validateRuntimeSemanticObjectFields("$", envelope, allowed, required); err != nil {
		return RuntimeDefinitionGapProviderDocument{}, err
	}
	var document RuntimeDefinitionGapProviderDocument
	if err := strictDecodeRuntimeCandidatePayload(payload, &document); err != nil {
		return RuntimeDefinitionGapProviderDocument{}, err
	}
	if err := document.ValidateAgainst(manifest, expected); err != nil {
		return RuntimeDefinitionGapProviderDocument{}, err
	}
	return document.Clone(), nil
}

func RuntimeDefinitionClarificationGapKinds() []string {
	values := []string{string(RuntimeDefinitionGapImplicitOneOf), string(RuntimeDefinitionGapUndeclaredPrerequisite), string(RuntimeDefinitionGapSemanticAmbiguity)}
	sort.Strings(values)
	return values
}
