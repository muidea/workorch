package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeEvidenceApplicabilityObservationV1 = "evidence-applicability-observation/v1"
const RuntimeEvidenceApplicabilityObservationLimit = 4096

// Decision-only projection of an existing SM-2 current-read obligation. It is
// not an ordinary acquisition requirement and cannot enter Pending/Satisfied
// facts or the generic PrerequisiteObserved ingress.
const RuntimeEvidenceApplicabilityPrerequisiteV1 PrerequisiteKind = "evidence_applicability/v1"

// The set of distinct source-bound factual results is semantic authority.
// Query/CAS fences and repeated identical results are transaction metadata;
// they do not change equivalent authority or revive a stale original source.
func StableRuntimeEvidenceApplicabilitySemanticHash(values []RuntimeEvidenceApplicabilityObservation) string {
	if len(values) == 0 {
		return ""
	}
	type result struct {
		EvidenceID          common.EvidenceRecordID
		EvidenceHash        string
		ReceiptHash         string
		Missing             bool
		CurrentDependencies *RuntimeOwnerRevisionSet
	}
	unique := make(map[string]struct{}, len(values))
	lastBySource := make(map[common.EvidenceRecordID]int, 32)
	buf := make([]byte, 0, 512)
	for i, observation := range values {
		if observation.Request.PurposeRef == RuntimeHistoricalEffectProofV1 {
			// Source validation independently checks this against the original
			// receipt. An audit of an old effect postimage is not current
			// resource authority and cannot invalidate an ordinary candidate.
			continue
		}
		if prevIdx, ok := lastBySource[observation.Request.SourceEvidenceID]; ok {
			prev := &values[prevIdx]
			if prev.Request.SourceEvidenceHash == observation.Request.SourceEvidenceHash &&
				prev.Request.SourceReceiptHash == observation.Request.SourceReceiptHash &&
				prev.Missing == observation.Missing &&
				equalRuntimeOwnerRevisionSetPtr(prev.CurrentDependencies, observation.CurrentDependencies) {
				continue
			}
		}
		lastBySource[observation.Request.SourceEvidenceID] = i
		if encoded, ok := appendEvidenceApplicabilitySemanticResultJSON(buf[:0], observation); ok {
			buf = encoded[:0]
			unique[string(encoded)] = struct{}{}
			continue
		}
		body, _ := json.Marshal(result{observation.Request.SourceEvidenceID, observation.Request.SourceEvidenceHash, observation.Request.SourceReceiptHash, observation.Missing, observation.CurrentDependencies})
		unique[string(body)] = struct{}{}
	}
	if len(unique) == 0 {
		return ""
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	writeHashString(h, "evidence-applicability-semantic/v1")
	for _, key := range keys {
		writeHashString(h, key)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func isSafeJSONASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c >= 0x7f || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			return false
		}
	}
	return true
}

func appendSafeJSONString(buf []byte, s string) ([]byte, bool) {
	if !isSafeJSONASCII(s) {
		return nil, false
	}
	buf = append(buf, '"')
	buf = append(buf, s...)
	buf = append(buf, '"')
	return buf, true
}

func appendEvidenceApplicabilitySemanticResultJSON(buf []byte, observation RuntimeEvidenceApplicabilityObservation) ([]byte, bool) {
	var ok bool
	buf = append(buf, `{"EvidenceID":`...)
	if buf, ok = appendSafeJSONString(buf, string(observation.Request.SourceEvidenceID)); !ok {
		return nil, false
	}
	buf = append(buf, `,"EvidenceHash":`...)
	if buf, ok = appendSafeJSONString(buf, observation.Request.SourceEvidenceHash); !ok {
		return nil, false
	}
	buf = append(buf, `,"ReceiptHash":`...)
	if buf, ok = appendSafeJSONString(buf, observation.Request.SourceReceiptHash); !ok {
		return nil, false
	}
	buf = append(buf, `,"Missing":`...)
	buf = strconv.AppendBool(buf, observation.Missing)
	buf = append(buf, `,"CurrentDependencies":`...)
	deps := observation.CurrentDependencies
	if deps == nil {
		buf = append(buf, `null}`...)
		return buf, true
	}
	buf = append(buf, `{"DefinitionRevision":`...)
	if buf, ok = appendSafeJSONString(buf, deps.DefinitionRevision); !ok {
		return nil, false
	}
	buf = append(buf, `,"Entries":`...)
	if deps.Entries == nil {
		buf = append(buf, `null`...)
	} else {
		buf = append(buf, '[')
		for i := range deps.Entries {
			if i > 0 {
				buf = append(buf, ',')
			}
			entry := &deps.Entries[i]
			buf = append(buf, `{"OwnerKind":`...)
			if buf, ok = appendSafeJSONString(buf, entry.OwnerKind); !ok {
				return nil, false
			}
			buf = append(buf, `,"OwnerInstanceID":`...)
			if buf, ok = appendSafeJSONString(buf, entry.OwnerInstanceID); !ok {
				return nil, false
			}
			buf = append(buf, `,"ResourceKind":`...)
			if buf, ok = appendSafeJSONString(buf, entry.ResourceKind); !ok {
				return nil, false
			}
			buf = append(buf, `,"ResourceID":`...)
			if buf, ok = appendSafeJSONString(buf, entry.ResourceID); !ok {
				return nil, false
			}
			buf = append(buf, `,"SnapshotID":`...)
			if buf, ok = appendSafeJSONString(buf, entry.SnapshotID); !ok {
				return nil, false
			}
			buf = append(buf, `,"Revision":`...)
			if buf, ok = appendSafeJSONString(buf, entry.Revision); !ok {
				return nil, false
			}
			buf = append(buf, `,"Fence":`...)
			buf = strconv.AppendUint(buf, entry.Fence, 10)
			buf = append(buf, `,"DependencyHash":`...)
			if buf, ok = appendSafeJSONString(buf, entry.DependencyHash); !ok {
				return nil, false
			}
			buf = append(buf, '}')
		}
		buf = append(buf, ']')
	}
	buf = append(buf, `,"SetHash":`...)
	if buf, ok = appendSafeJSONString(buf, deps.SetHash); !ok {
		return nil, false
	}
	buf = append(buf, `}}`...)
	return buf, true
}

// A read-only owner observation request is independently compiled from the
// original committed fact/receipt and the exact aggregate revision. It grants
// neither another effect nor evidence credit. Query replay before its CAS is safe.
type RuntimeEvidenceApplicabilityRequest struct {
	ProtocolRef           string
	RequestID             string
	IssuedAtStateRevision uint64
	EvidenceRevision      string
	SourceEvidenceID      common.EvidenceRecordID
	SourceEvidenceHash    string
	SourceReceiptHash     string
	DispatchID            string
	OperationID           string
	// A new Resume/reconciliation generation must reobserve even when no new
	// receipt advanced EvidenceRevision. This is a read fence, not semantics.
	ReconciliationRequestHash string `json:",omitempty"`
	// Exact original receipt purpose, never a caller-selected sampling mode.
	// Empty means this request concerns a current resource rather than a historical source.
	PurposeRef string `json:",omitempty"`
}

// Missing is an actual owner observation, not a failed query. A failed query
// never creates this record. CurrentDependencies is detached factual data;
// only the receiving SM-2 transaction may judge the original dependency snapshot.
type RuntimeEvidenceApplicabilityObservation struct {
	Request                 RuntimeEvidenceApplicabilityRequest
	RecordedAtStateRevision uint64
	Missing                 bool
	CurrentDependencies     *RuntimeOwnerRevisionSet `json:",omitempty"`
}

func (r RuntimeEvidenceApplicabilityRequest) WithID() RuntimeEvidenceApplicabilityRequest {
	r.RequestID = ""
	body, _ := json.Marshal(r)
	digest := sha256.Sum256(append([]byte("runtime-evidence-applicability-request/v1\x00"), body...))
	r.RequestID = "evidence-current-" + hex.EncodeToString(digest[:])
	return r
}

func (r RuntimeEvidenceApplicabilityRequest) ValidateRaw() error {
	if r.ProtocolRef != RuntimeEvidenceApplicabilityObservationV1 || r.IssuedAtStateRevision == 0 || r.RequestID != r.WithID().RequestID {
		return fmt.Errorf("invalid evidence applicability request protocol/revision/identity")
	}
	for _, value := range []string{r.EvidenceRevision, string(r.SourceEvidenceID), r.SourceEvidenceHash, r.SourceReceiptHash, r.DispatchID, r.OperationID} {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("evidence applicability request requires exact original source")
		}
	}
	if r.ReconciliationRequestHash != strings.TrimSpace(r.ReconciliationRequestHash) {
		return fmt.Errorf("evidence applicability reconciliation fence must be raw canonical")
	}
	if r.PurposeRef != "" && r.PurposeRef != RuntimeHistoricalEffectProofV1 {
		return fmt.Errorf("evidence applicability request has unsupported original purpose")
	}
	return nil
}

func (o RuntimeEvidenceApplicabilityObservation) CloneRaw() RuntimeEvidenceApplicabilityObservation {
	o.CurrentDependencies = CloneRuntimeOwnerRevisionSet(o.CurrentDependencies)
	return o
}

func CloneRuntimeEvidenceApplicabilityObservations(values []RuntimeEvidenceApplicabilityObservation) []RuntimeEvidenceApplicabilityObservation {
	if values == nil {
		return nil
	}
	ret := make([]RuntimeEvidenceApplicabilityObservation, len(values))
	for index, value := range values {
		ret[index] = value.CloneRaw()
	}
	return ret
}

func (o RuntimeEvidenceApplicabilityObservation) ValidateRaw() error {
	if err := o.Request.ValidateRaw(); err != nil {
		return err
	}
	if o.RecordedAtStateRevision != o.Request.IssuedAtStateRevision+1 || (o.Missing == (o.CurrentDependencies != nil)) {
		return fmt.Errorf("evidence applicability observation has invalid CAS or missing/observed pair")
	}
	if err := validateRuntimeOwnerRevisionSetRaw(o.CurrentDependencies); err != nil {
		return err
	}
	if o.CurrentDependencies != nil && len(o.CurrentDependencies.Entries) == 0 {
		return fmt.Errorf("empty observation cannot prove current dependencies")
	}
	return nil
}

// ValidateRuntimeEvidenceApplicabilityObservationSource independently binds a
// stored observation to the original evidence/receipt. Self-consistent request
// hashes never replace this source comparison. The enclosing aggregate gates
// independently validate receipt execution, owner, dispatch and fact lineage.
func ValidateRuntimeEvidenceApplicabilityObservationSource(o RuntimeEvidenceApplicabilityObservation, facts RuntimeFacts, receipts []RuntimeOperationReceipt) error {
	if err := o.ValidateRaw(); err != nil {
		return err
	}
	var fact EvidenceFact
	matches := 0
	for _, value := range facts.Evidence.Accepted {
		if value.ID == o.Request.SourceEvidenceID {
			fact = value
			matches++
		}
	}
	if matches != 1 || fact.ApplicabilityDependencies == nil || fact.Revision > o.Request.EvidenceRevision ||
		fact.DispatchID != o.Request.DispatchID || fact.OperationID != o.Request.OperationID || StableRuntimeEvidenceFactHash(fact) != o.Request.SourceEvidenceHash {
		return fmt.Errorf("evidence applicability observation has no exact original accepted fact")
	}
	if o.Request.PurposeRef != fact.ApplicabilityDependencies.PurposeRef {
		return fmt.Errorf("evidence applicability request differs from original receipt purpose")
	}
	if err := ValidateRuntimeEvidenceDependencyPair(fact.OwnerRevisionSet, fact.ApplicabilityDependencies, facts.DefinitionRevision); err != nil {
		return err
	}
	matches = 0
	for _, receipt := range receipts {
		if receipt.OperationID == fact.OperationID &&
			fact.SourceRef == "runtime-receipt/"+receipt.ExecutionKey && fact.ID == common.EvidenceRecordID("runtime-receipt|"+o.Request.SourceReceiptHash) &&
			(receipt.Outcome == RuntimeOperationOutcomeAccepted || receipt.Outcome == RuntimeOperationOutcomeNoOp) &&
			RuntimeRawEqual(receipt.Evidence.OwnerRevisionSet, fact.OwnerRevisionSet) && RuntimeRawEqual(receipt.Evidence.ApplicabilityDependencies, fact.ApplicabilityDependencies) &&
			StableRuntimeOperationReceiptHash(receipt) == o.Request.SourceReceiptHash {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("evidence applicability observation has no exact original receipt")
	}
	if o.CurrentDependencies != nil {
		original := fact.ApplicabilityDependencies.OwnerRevisionSet
		observed := *o.CurrentDependencies
		if original.DefinitionRevision != observed.DefinitionRevision || len(original.Entries) != len(observed.Entries) {
			return fmt.Errorf("evidence applicability observation changed dependency identity")
		}
		for index, entry := range original.Entries {
			if ownerRevisionResourceKey(entry) != ownerRevisionResourceKey(observed.Entries[index]) {
				return fmt.Errorf("evidence applicability observation changed resource identity")
			}
		}
	}
	return nil
}

// runtimeEvidenceApplicability is a call-local query result, never a retained
// owner admission or credit. One pass joins the original fact to all applicable
// observations. A later current observation cannot erase an earlier stale one.
type runtimeEvidenceApplicability struct {
	stale, observed, atRevision, fresh bool
}

func queryRuntimeEvidenceApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, cutoff, reconciliationRequestHash string) runtimeEvidenceApplicability {
	var result runtimeEvidenceApplicability
	var factHash string
	dependenciesValidated := false
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID != fact.ID || (cutoff != "" && observation.Request.EvidenceRevision > cutoff) {
			continue
		}
		// Hash the complete original fact once, only if an observation can
		// consume it. No digest substitutes for owner source admission.
		if factHash == "" {
			factHash = StableRuntimeEvidenceFactHash(fact)
		}
		if observation.Request.SourceEvidenceHash != factHash {
			continue
		}
		if observation.Missing || observation.CurrentDependencies == nil {
			result.stale = true
			return result
		}
		var matches bool
		var err error
		if dependenciesValidated && equalRuntimeOwnerRevisionSet(&fact.ApplicabilityDependencies.OwnerRevisionSet, observation.CurrentDependencies) {
			matches = true
		} else {
			matches, err = RuntimeOwnerRevisionSetMatchesCurrent(fact.ApplicabilityDependencies.OwnerRevisionSet, *observation.CurrentDependencies)
			if err == nil {
				dependenciesValidated = true
			}
		}
		if err != nil || !matches {
			result.stale = true
			return result
		}
		result.observed = true
		if observation.Request.EvidenceRevision == cutoff {
			result.atRevision = true
			if observation.Request.ReconciliationRequestHash == reconciliationRequestHash {
				result.fresh = true
			}
		}
	}
	return result
}

// RuntimeEvidenceHasStaleApplicability is a pure audit predicate. Missing
// observations do not prove currentness. Once an exact source was observed
// stale, a later byte restoration cannot silently revive its old credit.
func RuntimeEvidenceHasStaleApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, cutoff string) bool {
	if fact.ApplicabilityDependencies == nil {
		return false
	}
	return queryRuntimeEvidenceApplicability(fact, observations, cutoff, "").stale
}

// Evidence with resource dependencies requires an actual matching SM-2 owner
// observation. Dependency-free evidence has no resource to reobserve; nil
// never fabricates an observation for dependent evidence.
func RuntimeEvidenceHasObservedCurrentApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, cutoff string) bool {
	if fact.ApplicabilityDependencies == nil {
		return true
	}
	result := queryRuntimeEvidenceApplicability(fact, observations, cutoff, "")
	return !result.stale && result.observed
}

// RuntimeEvidenceHasFreshApplicability is the consumption fence, not the
// historical audit predicate above. A matching old observation cannot satisfy
// another evidence revision or Resume/reconciliation generation. Historical
// assessment consumers pass their immutable pre-seal generation and prefix.
func RuntimeEvidenceHasFreshApplicability(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, evidenceRevision, reconciliationRequestHash string) bool {
	if fact.ApplicabilityDependencies == nil {
		return true
	}
	if evidenceRevision == "" {
		return false
	}
	required := false
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID == fact.ID &&
			observation.Request.EvidenceRevision == evidenceRevision &&
			observation.Request.ReconciliationRequestHash == reconciliationRequestHash {
			required = true
			break
		}
	}
	if !required {
		return false
	}
	result := queryRuntimeEvidenceApplicability(fact, observations, evidenceRevision, reconciliationRequestHash)
	return !result.stale && result.fresh
}

// Facts-only proof predicates have no reconciliation ledger authority. Require
// an exact evidence revision here; aggregate admission additionally checks the
// exact reconciliation generation with RuntimeEvidenceHasFreshApplicability.
func RuntimeEvidenceHasObservedApplicabilityAtRevision(fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, evidenceRevision string) bool {
	if fact.ApplicabilityDependencies == nil {
		return true
	}
	if evidenceRevision == "" {
		return false
	}
	// No exact-revision candidate means this query cannot succeed. Preserve
	// the original cheap negative path before hashing any earlier evidence.
	required := false
	for _, observation := range observations {
		if observation.Request.SourceEvidenceID == fact.ID && observation.Request.EvidenceRevision == evidenceRevision {
			required = true
			break
		}
	}
	if !required {
		return false
	}
	result := queryRuntimeEvidenceApplicability(fact, observations, evidenceRevision, "")
	return !result.stale && result.atRevision
}

func ValidateRuntimeEvidenceApplicabilityObservations(facts RuntimeFacts, receipts []RuntimeOperationReceipt, stateRevision uint64, ledger RuntimeCWIReconciliationLedger) error {
	return validateRuntimeEvidenceApplicabilityObservationTransition(facts, receipts, stateRevision, ledger, nil)
}

// A formal preimage already owns the source judgment of its exact observations.
// The enclosing committed-history gate protects original evidence and receipts;
// collection, current revision and reconciliation-reference gates still run.
// New or changed observations must prove their complete source independently.
func validateRuntimeEvidenceApplicabilityObservationTransition(facts RuntimeFacts, receipts []RuntimeOperationReceipt, stateRevision uint64, ledger RuntimeCWIReconciliationLedger, previous *RuntimeStateRecord) error {
	if len(facts.EvidenceApplicabilityObservations) > RuntimeEvidenceApplicabilityObservationLimit {
		return fmt.Errorf("evidence applicability observation ledger exceeds bound")
	}
	seen := make(map[string]bool)
	var prior uint64
	for index, observation := range facts.EvidenceApplicabilityObservations {
		if observation.RecordedAtStateRevision <= prior || observation.RecordedAtStateRevision > stateRevision || observation.Request.EvidenceRevision > facts.EvidenceRevision || seen[observation.Request.RequestID] {
			return fmt.Errorf("evidence applicability observation ledger has stale/duplicate/reordered fence")
		}
		committed := previous != nil && index < len(previous.Facts.EvidenceApplicabilityObservations) &&
			equalRuntimeEvidenceApplicabilityObservation(&observation, &previous.Facts.EvidenceApplicabilityObservations[index])
		if !committed {
			if err := ValidateRuntimeEvidenceApplicabilityObservationSource(observation, facts, receipts); err != nil {
				return err
			}
		}
		if observation.Request.ReconciliationRequestHash != "" {
			found := false
			if ledger.Request != nil && ledger.RequestHash == observation.Request.ReconciliationRequestHash {
				found = true
			}
			for _, historical := range ledger.History {
				if historical.RequestHash == observation.Request.ReconciliationRequestHash {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("evidence applicability observation has no original reconciliation read fence")
			}
		}
		seen[observation.Request.RequestID] = true
		prior = observation.RecordedAtStateRevision
	}
	return nil
}
