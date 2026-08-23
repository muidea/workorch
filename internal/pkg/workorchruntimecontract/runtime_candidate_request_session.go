package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCandidateRequestSessionState is the persisted lifecycle of provider
// I/O before a complete candidate exists. It is intentionally separate from
// RuntimeCandidateAttemptState: transport and incomplete documents must never
// consume the schema/semantic correction budget.
type RuntimeCandidateRequestSessionState string

const (
	RuntimeCandidateRequestSessionPrepared         RuntimeCandidateRequestSessionState = "prepared"
	RuntimeCandidateRequestSessionRecoveryPending  RuntimeCandidateRequestSessionState = "recovery_pending"
	RuntimeCandidateRequestSessionTransportPending RuntimeCandidateRequestSessionState = "transport_retry_pending"
	RuntimeCandidateRequestSessionDocumentComplete RuntimeCandidateRequestSessionState = "document_complete"
	RuntimeCandidateRequestSessionTerminal         RuntimeCandidateRequestSessionState = "terminal"
)

// RuntimeCandidateRequestOutcomeClass is the last durable provider/session
// result. It is deliberately distinct from candidate schema/contract failure:
// no value here may allocate or fail a candidate attempt.
type RuntimeCandidateRequestOutcomeClass string

const (
	RuntimeCandidateRequestOutcomeNone                      RuntimeCandidateRequestOutcomeClass = ""
	RuntimeCandidateRequestOutcomeDocumentIncomplete        RuntimeCandidateRequestOutcomeClass = "document_incomplete"
	RuntimeCandidateRequestOutcomeDocumentComplete          RuntimeCandidateRequestOutcomeClass = "document_complete"
	RuntimeCandidateRequestOutcomeProviderTransient         RuntimeCandidateRequestOutcomeClass = "provider_transient"
	RuntimeCandidateRequestOutcomeProviderAuthentication    RuntimeCandidateRequestOutcomeClass = "provider_authentication"
	RuntimeCandidateRequestOutcomeProviderConfiguration     RuntimeCandidateRequestOutcomeClass = "provider_configuration"
	RuntimeCandidateRequestOutcomeProviderProtocol          RuntimeCandidateRequestOutcomeClass = "provider_protocol"
	RuntimeCandidateRequestOutcomeProviderSubmissionUnknown RuntimeCandidateRequestOutcomeClass = "provider_submission_unknown"
	RuntimeCandidateRequestOutcomePrerequisiteStale         RuntimeCandidateRequestOutcomeClass = "prerequisite_stale"
	RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted RuntimeCandidateRequestOutcomeClass = "candidate_document_recovery_exhausted"
)

// RuntimeCandidateRequestOutcome is bounded audit-safe metadata. It never
// contains a provider body, prompt, credential, endpoint, or free-form error.
type RuntimeCandidateRequestOutcome struct {
	Class     RuntimeCandidateRequestOutcomeClass
	Code      string
	Signature string
}

func (o RuntimeCandidateRequestOutcome) Normalized() RuntimeCandidateRequestOutcome {
	o.Class = RuntimeCandidateRequestOutcomeClass(strings.TrimSpace(string(o.Class)))
	o.Code = strings.TrimSpace(o.Code)
	o.Signature = strings.TrimSpace(o.Signature)
	return o
}

func (o RuntimeCandidateRequestOutcome) Validate() error {
	o = o.Normalized()
	switch o.Class {
	case RuntimeCandidateRequestOutcomeNone:
		return nil
	case RuntimeCandidateRequestOutcomeDocumentIncomplete,
		RuntimeCandidateRequestOutcomeDocumentComplete,
		RuntimeCandidateRequestOutcomeProviderTransient,
		RuntimeCandidateRequestOutcomeProviderAuthentication,
		RuntimeCandidateRequestOutcomeProviderConfiguration,
		RuntimeCandidateRequestOutcomeProviderProtocol,
		RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
		RuntimeCandidateRequestOutcomePrerequisiteStale,
		RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted:
		if o.Class != RuntimeCandidateRequestOutcomeDocumentComplete && o.Code == "" {
			return common.ErrInvalidArgument
		}
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

// RuntimeCandidateRequestSession is the SM-3 provider request ledger. A
// correction after a complete candidate rejection creates a distinct session
// fingerprint; whole-document recovery keeps the same ID.
type RuntimeCandidateRequestSession struct {
	LogicalRequestID   string
	DocumentKind       RuntimeProviderDocumentKind
	ExpectedDocument   *RuntimeExpectedDocumentContract
	Transaction        AuthorityTransaction
	StablePrefixHash   string
	ManifestHash       string
	CorrectionHash     string
	State              RuntimeCandidateRequestSessionState
	NextSequence       int
	IncompleteOutcomes int
	TransportOutcomes  int
	// LastDocumentFraming stores only owner-neutral response framing. The
	// authoritative physical-response fact is emitted by the LLM owner.
	LastDocumentFraming *RuntimeProviderDocumentFraming
	LastPhysicalCallID  string
	ProviderSubmitted   bool
	LastOutcome         RuntimeCandidateRequestOutcome
}

// RuntimeCandidateRequestIdentity is the canonical identity carried across
// Orchestrator -> LLM -> provider boundaries for one physical request. It is
// intentionally a pure value: session state and submission ordering remain in
// the Orchestrator aggregate controller.
type RuntimeCandidateRequestIdentity struct {
	LogicalRequestID string `json:"logicalRequestId"`
	Sequence         int    `json:"sequence"`
	PhysicalCallID   string `json:"physicalCallId"`
}

func (i RuntimeCandidateRequestIdentity) Normalized() RuntimeCandidateRequestIdentity {
	i.LogicalRequestID = strings.TrimSpace(i.LogicalRequestID)
	i.PhysicalCallID = strings.TrimSpace(i.PhysicalCallID)
	return i
}

func (i RuntimeCandidateRequestIdentity) Validate() error {
	i = i.Normalized()
	if i.LogicalRequestID == "" || i.Sequence <= 0 || i.PhysicalCallID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

func SameRuntimeCandidateRequestIdentity(left, right RuntimeCandidateRequestIdentity) bool {
	return left.Normalized() == right.Normalized()
}

func (s RuntimeCandidateRequestSession) Normalized() RuntimeCandidateRequestSession {
	s.LogicalRequestID = strings.TrimSpace(s.LogicalRequestID)
	s.DocumentKind = RuntimeProviderDocumentKind(strings.TrimSpace(string(s.DocumentKind)))
	s.StablePrefixHash = strings.TrimSpace(s.StablePrefixHash)
	s.ManifestHash = strings.TrimSpace(s.ManifestHash)
	s.CorrectionHash = strings.TrimSpace(s.CorrectionHash)
	s.State = RuntimeCandidateRequestSessionState(strings.TrimSpace(string(s.State)))
	s.LastPhysicalCallID = strings.TrimSpace(s.LastPhysicalCallID)
	s.LastOutcome = s.LastOutcome.Normalized()
	if s.ExpectedDocument != nil {
		value := s.ExpectedDocument.Normalized()
		s.ExpectedDocument = &value
	}
	if s.LastDocumentFraming != nil {
		value := s.LastDocumentFraming.Normalized()
		s.LastDocumentFraming = &value
	}
	return s
}

func (s RuntimeCandidateRequestSession) Validate() error {
	s = s.Normalized()
	if s.LogicalRequestID == "" || (s.DocumentKind != RuntimeProviderDocumentSemanticPlan && s.DocumentKind != RuntimeProviderDocumentOperations) || s.StablePrefixHash == "" || s.ManifestHash == "" ||
		strings.TrimSpace(s.Transaction.AuthoritySnapshotID) == "" || s.NextSequence <= 0 || s.IncompleteOutcomes < 0 || s.TransportOutcomes < 0 {
		return common.ErrInvalidArgument
	}
	if err := s.LastOutcome.Validate(); err != nil {
		return err
	}
	if s.ExpectedDocument != nil {
		if err := s.ExpectedDocument.Validate(); err != nil || s.ExpectedDocument.DocumentKind != s.DocumentKind || s.ExpectedDocument.LogicalRequestID != s.LogicalRequestID || s.ExpectedDocument.ManifestHash != s.ManifestHash || s.ExpectedDocument.StablePrefixHash != s.StablePrefixHash {
			return common.ErrInvalidArgument
		}
	}
	if s.LastDocumentFraming != nil {
		if s.ExpectedDocument == nil {
			return common.ErrInvalidArgument
		}
		if err := s.LastDocumentFraming.Validate(); err != nil || s.LastDocumentFraming.ExpectedDocumentHash != s.ExpectedDocument.Hash || s.LastDocumentFraming.Disposition != RuntimeProviderDocumentIncomplete {
			return common.ErrInvalidArgument
		}
	}
	switch s.State {
	case RuntimeCandidateRequestSessionPrepared, RuntimeCandidateRequestSessionRecoveryPending,
		RuntimeCandidateRequestSessionTransportPending, RuntimeCandidateRequestSessionDocumentComplete,
		RuntimeCandidateRequestSessionTerminal:
	default:
		return common.ErrInvalidArgument
	}
	if s.State == RuntimeCandidateRequestSessionRecoveryPending && s.IncompleteOutcomes == 0 {
		return common.ErrInvalidArgument
	}
	if s.ProviderSubmitted && s.LastPhysicalCallID == "" {
		return common.ErrInvalidArgument
	}
	switch s.State {
	case RuntimeCandidateRequestSessionPrepared:
		if s.LastOutcome.Class != RuntimeCandidateRequestOutcomeNone {
			return common.ErrInvalidArgument
		}
	case RuntimeCandidateRequestSessionRecoveryPending:
		if s.LastOutcome.Class != RuntimeCandidateRequestOutcomeDocumentIncomplete || s.LastDocumentFraming == nil {
			return common.ErrInvalidArgument
		}
	case RuntimeCandidateRequestSessionTransportPending:
		if s.LastOutcome.Class != RuntimeCandidateRequestOutcomeProviderTransient {
			return common.ErrInvalidArgument
		}
	case RuntimeCandidateRequestSessionDocumentComplete:
		if s.LastOutcome.Class != RuntimeCandidateRequestOutcomeDocumentComplete || !s.ProviderSubmitted {
			return common.ErrInvalidArgument
		}
	case RuntimeCandidateRequestSessionTerminal:
		switch s.LastOutcome.Class {
		case RuntimeCandidateRequestOutcomeProviderAuthentication,
			RuntimeCandidateRequestOutcomeProviderConfiguration,
			RuntimeCandidateRequestOutcomeProviderTransient,
			RuntimeCandidateRequestOutcomeProviderProtocol,
			RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
			RuntimeCandidateRequestOutcomePrerequisiteStale,
			RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted:
		default:
			return common.ErrInvalidArgument
		}
	}
	if s.NextSequence != s.IncompleteOutcomes+s.TransportOutcomes+1 {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s RuntimeCandidateRequestSession) Clone() RuntimeCandidateRequestSession {
	s = s.Normalized()
	if s.ExpectedDocument != nil {
		value := s.ExpectedDocument.Clone()
		s.ExpectedDocument = &value
	}
	if s.LastDocumentFraming != nil {
		value := s.LastDocumentFraming.Normalized()
		s.LastDocumentFraming = &value
	}
	return s
}

// StableRuntimeCandidateRequestSessionID identifies one candidate document.
// It intentionally excludes candidate attempt number: an incomplete initial
// response has no candidate attempt. CorrectionHash differentiates a later
// C4 correction document without relying on an attempt ordinal.
func StableRuntimeCandidateRequestSessionID(documentKind RuntimeProviderDocumentKind, workspaceID common.WorkspaceID, runID common.RunID, transaction AuthorityTransaction, stablePrefixHash, manifestHash, correctionHash string) string {
	h := sha256.New()
	writeHashString(h, "runtime-candidate-request-session")
	writeHashString(h, string(documentKind))
	writeHashString(h, string(workspaceID))
	writeHashString(h, string(runID))
	writeHashString(h, transaction.AuthoritySnapshotID)
	writeHashString(h, transaction.RequiredOutputHash)
	writeHashString(h, strings.TrimSpace(stablePrefixHash))
	writeHashString(h, strings.TrimSpace(manifestHash))
	writeHashString(h, strings.TrimSpace(correctionHash))
	return "candidate-request-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeCandidatePhysicalCallID identifies one physical submission in
// a logical request session. It is deterministic so an adapter can use it as
// an idempotency key when the provider supports one; it carries no attempt
// number and contains no provider credential or response content.
func StableRuntimeCandidatePhysicalCallID(logicalRequestID string, sequence int) string {
	h := sha256.New()
	writeHashString(h, "runtime-candidate-physical-call")
	writeHashString(h, strings.TrimSpace(logicalRequestID))
	writeHashString(h, fmt.Sprintf("%d", sequence))
	return "candidate-call-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeCandidateRequestContentID identifies the Orchestrator-owned
// opaque content record for one logical request session. It deliberately uses
// the logical request ID rather than candidate attempt so raw provider output
// remains available before a candidate exists.
func StableRuntimeCandidateRequestContentID(logicalRequestID string) common.ArtifactContinuationID {
	h := sha256.New()
	writeHashString(h, "runtime-candidate-request-content")
	writeHashString(h, strings.TrimSpace(logicalRequestID))
	return common.ArtifactContinuationID("candidate-request-content-" + hex.EncodeToString(h.Sum(nil))[:20])
}

// StableRuntimeCandidateRequestCorrectionHash differentiates a correction
// document from an initial request without using a candidate-attempt ordinal.
// Reason is deliberately excluded because it can contain unstable or verbose
// provider text; the typed failure identity is the correction contract.
func StableRuntimeCandidateRequestCorrectionHash(failure *RuntimeCandidateFailure) string {
	if failure == nil {
		return ""
	}
	h := sha256.New()
	writeHashString(h, "runtime-candidate-request-correction")
	writeHashString(h, string(failure.Class))
	writeHashString(h, strings.TrimSpace(failure.Code))
	writeHashString(h, strings.TrimSpace(failure.Signature))
	writeHashStrings(h, normalizeStrings(failure.TargetRefs))
	if diagnostic := failure.Diagnostic; diagnostic != nil {
		value := diagnostic.Normalized()
		writeHashString(h, value.OperationID)
		writeHashString(h, value.Path)
		writeHashString(h, value.Actual)
		writeHashStrings(h, value.Allowed)
		writeHashStrings(h, value.Required)
		writeHashStrings(h, value.Forbidden)
		writeHashString(h, value.ManifestHash)
		writeHashString(h, value.RejectedShapeHash)
		for _, objective := range value.Objectives {
			writeHashString(h, string(objective))
		}
	}
	return "candidate-correction-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func ValidateRuntimeCandidateRequestSessions(values []RuntimeCandidateRequestSession) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = value.Normalized()
		if err := value.Validate(); err != nil {
			return fmt.Errorf("runtime candidate request session: %w", err)
		}
		if _, found := seen[value.LogicalRequestID]; found {
			return fmt.Errorf("runtime candidate request session %q is duplicated", value.LogicalRequestID)
		}
		seen[value.LogicalRequestID] = struct{}{}
	}
	return nil
}
