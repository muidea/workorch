package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
)

const runtimeStateSHA256CheckpointStride = 256 * 1024 // 256 KiB (multiple of 64-byte SHA-256 block size)

type runtimeStateSHA256Checkpoint struct {
	Offset int
	State  []byte
}

// RuntimeStateJSONByteSpan records an exact UTF-8 JSON value byte range
// [Start, End) relative to the beginning of a canonical RuntimeStateRecord JSON
// payload. It is an encoding optimization boundary only, never business state.
type RuntimeStateJSONByteSpan struct {
	Start            int
	End              int
	LastElementStart int
	Present          bool
}

// RuntimeStateJSONSpans captures top-level field, large-ledger slice value
// byte spans, and SHA-256 prefix checkpoints produced by
// WriteRuntimeStateRecordJSONWithPreimage and SumRuntimeStateJSONWithPreimage so
// a later CAS over an unchanged preimage can copy unmodified JSON spans and
// resume prefix hashing verbatim.
type RuntimeStateJSONSpans struct {
	Fields                                 [30]RuntimeStateJSONByteSpan
	SHA256Checkpoints                      []runtimeStateSHA256Checkpoint
	UnchangedPrefixBytes                   int
	PreimagePrefixTracked                  bool
	PreimageRawLen                         int
	FactsDefinitionSeed                    RuntimeStateJSONByteSpan
	FactsDefinitionFocuses                 RuntimeStateJSONByteSpan
	FactsFocus                             RuntimeStateJSONByteSpan
	FactsRemainingFocuses                  RuntimeStateJSONByteSpan
	FactsCompletedFocuses                  RuntimeStateJSONByteSpan
	FactsEvidence                          RuntimeStateJSONByteSpan
	FactsEvidenceApplicabilityObservations RuntimeStateJSONByteSpan
	FactsAssessments                       RuntimeStateJSONByteSpan
	FactsPrerequisites                     RuntimeStateJSONByteSpan
	FactsCapabilities                      RuntimeStateJSONByteSpan
	FactsResources                         RuntimeStateJSONByteSpan
	FactsActivation                        RuntimeStateJSONByteSpan
	FactsReconciliationContract            RuntimeStateJSONByteSpan
	FactsCoverage                          RuntimeStateJSONByteSpan
	FactsCWISuspensions                    RuntimeStateJSONByteSpan
	FactsSlices                            RuntimeStateJSONByteSpan
	BudgetPolicy                           RuntimeStateJSONByteSpan
	BudgetAllocations                      RuntimeStateJSONByteSpan
	BudgetConsumptions                     RuntimeStateJSONByteSpan
	BudgetReconciliations                  RuntimeStateJSONByteSpan
	ReconciliationHistory                  RuntimeStateJSONByteSpan
	ReconciliationRequest                  RuntimeStateJSONByteSpan
	ReconciliationBatch                    RuntimeStateJSONByteSpan
	ReconciliationResults                  RuntimeStateJSONByteSpan
	ReconciliationReopened                 RuntimeStateJSONByteSpan
	SemanticManifest                       RuntimeStateJSONByteSpan
	SemanticCoverageOrderDocument          RuntimeStateJSONByteSpan
	SemanticDocument                       RuntimeStateJSONByteSpan
	SemanticBatches                        RuntimeStateJSONByteSpan
	SemanticBatchFeasibilityResults        RuntimeStateJSONByteSpan
	SemanticBatchDocuments                 RuntimeStateJSONByteSpan
	SemanticMergedDocument                 RuntimeStateJSONByteSpan
	SemanticFeasibilities                  RuntimeStateJSONByteSpan
	SemanticHistory                        RuntimeStateJSONByteSpan
	SemanticHistoryArchive                 RuntimeStateJSONByteSpan
}

// SumRuntimeStateJSONWithPreimage computes the exact SHA-256 digest of currRaw
// while resuming from the longest byte-identical 256 KiB prefix checkpoint of
// prevRaw recorded in prevSpans, and stores updated prefix checkpoints in spans.
func SumRuntimeStateJSONWithPreimage(
	currRaw []byte,
	prevRaw []byte,
	prevSpans *RuntimeStateJSONSpans,
	spans *RuntimeStateJSONSpans,
) [32]byte {
	matchedChunks := 0
	if prevSpans != nil && len(prevSpans.SHA256Checkpoints) > 0 && len(prevRaw) > 0 {
		maxChunks := min(len(currRaw), len(prevRaw)) / runtimeStateSHA256CheckpointStride
		limit := min(maxChunks, len(prevSpans.SHA256Checkpoints))
		if spans != nil && spans.PreimagePrefixTracked && spans.PreimageRawLen == len(prevRaw) {
			trackedLimit := min(spans.UnchangedPrefixBytes/runtimeStateSHA256CheckpointStride, limit)
			for matchedChunks < trackedLimit {
				end := (matchedChunks + 1) * runtimeStateSHA256CheckpointStride
				if prevSpans.SHA256Checkpoints[matchedChunks].Offset != end {
					break
				}
				matchedChunks++
			}
		} else {
			for matchedChunks < limit {
				start := matchedChunks * runtimeStateSHA256CheckpointStride
				end := start + runtimeStateSHA256CheckpointStride
				checkpoint := prevSpans.SHA256Checkpoints[matchedChunks]
				if checkpoint.Offset != end || !bytes.Equal(currRaw[start:end], prevRaw[start:end]) {
					break
				}
				matchedChunks++
			}
		}
	}
	h := sha256.New()
	offset := 0
	var newCheckpoints []runtimeStateSHA256Checkpoint
	if totalPossible := len(currRaw) / runtimeStateSHA256CheckpointStride; totalPossible > 0 {
		newCheckpoints = make([]runtimeStateSHA256Checkpoint, 0, totalPossible)
	}
	if matchedChunks > 0 {
		if unmarshaler, ok := h.(encoding.BinaryUnmarshaler); ok {
			if err := unmarshaler.UnmarshalBinary(prevSpans.SHA256Checkpoints[matchedChunks-1].State); err == nil {
				offset = matchedChunks * runtimeStateSHA256CheckpointStride
				newCheckpoints = append(newCheckpoints, prevSpans.SHA256Checkpoints[:matchedChunks]...)
			} else {
				h.Reset()
			}
		}
	}
	marshaler, hasBinaryMarshaler := h.(encoding.BinaryMarshaler)
	for offset+runtimeStateSHA256CheckpointStride <= len(currRaw) {
		end := offset + runtimeStateSHA256CheckpointStride
		_, _ = h.Write(currRaw[offset:end])
		if hasBinaryMarshaler {
			if state, err := marshaler.MarshalBinary(); err == nil {
				newCheckpoints = append(newCheckpoints, runtimeStateSHA256Checkpoint{
					Offset: end,
					State:  state,
				})
			}
		}
		offset = end
	}
	if offset < len(currRaw) {
		_, _ = h.Write(currRaw[offset:])
	}
	if spans != nil {
		spans.SHA256Checkpoints = newCheckpoints
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// WriteRuntimeStateRecordJSON writes the exact JSON representation produced by
// encoding/json for RuntimeStateRecord while streaming large slices element by
// element so repeated CAS hashes and physical writes do not grow a 30+ MB
// intermediate encoding buffer on each transition.
func WriteRuntimeStateRecordJSON(w io.Writer, state RuntimeStateRecord) error {
	_, err := WriteRuntimeStateRecordJSONWithPreimage(w, state, nil, nil, nil)
	return err
}

// WriteRuntimeStateRecordJSONWithPreimage writes exact encoding/json output for
// state and reuses unmodified field/slice spans from prevRaw when a matching
// formal preimage and span index are supplied by the physical repository.
func WriteRuntimeStateRecordJSONWithPreimage(
	w io.Writer,
	state RuntimeStateRecord,
	prev *RuntimeStateRecord,
	prevRaw []byte,
	prevSpans *RuntimeStateJSONSpans,
) (RuntimeStateJSONSpans, error) {
	if w == nil {
		return RuntimeStateJSONSpans{}, fmt.Errorf("runtime state JSON writer is nil")
	}
	hasPreimage := prev != nil && prevSpans != nil && len(prevRaw) > 0
	s := runtimeStateStreamEncoder{
		w:         w,
		prev:      prev,
		prevRaw:   prevRaw,
		prevSpans: prevSpans,
		diverged:  !hasPreimage,
	}
	if hasPreimage {
		s.spans.PreimagePrefixTracked = true
		s.spans.PreimageRawLen = len(prevRaw)
	}
	s.trim = runtimeNewlineTrimWriter{encoder: &s}
	s.enc = json.NewEncoder(&s.trim)
	s.writeRuntimeStateRecord(&state)
	return s.spans, s.err
}

type runtimeNewlineTrimWriter struct {
	encoder *runtimeStateStreamEncoder
}

func (tw *runtimeNewlineTrimWriter) Write(p []byte) (int, error) {
	if len(p) == 0 || p[len(p)-1] != '\n' {
		return 0, fmt.Errorf("json encoder omitted framing newline")
	}
	enc := tw.encoder
	chunk := p[:len(p)-1]
	pos := enc.written
	if !enc.diverged {
		if pos+len(chunk) <= len(enc.prevRaw) && bytes.Equal(enc.prevRaw[pos:pos+len(chunk)], chunk) {
			enc.spans.UnchangedPrefixBytes = pos + len(chunk)
		} else {
			enc.diverged = true
		}
	}
	n, err := enc.w.Write(chunk)
	enc.written += n
	if err != nil {
		return len(p), err
	}
	return len(p), nil
}

type runtimeStateStreamEncoder struct {
	w         io.Writer
	trim      runtimeNewlineTrimWriter
	enc       *json.Encoder
	written   int
	prev      *RuntimeStateRecord
	prevRaw   []byte
	prevSpans *RuntimeStateJSONSpans
	spans     RuntimeStateJSONSpans
	diverged  bool
	err       error
}

func (s *runtimeStateStreamEncoder) writeString(text string) {
	if s.err != nil {
		return
	}
	pos := s.written
	if !s.diverged {
		if pos+len(text) <= len(s.prevRaw) && string(s.prevRaw[pos:pos+len(text)]) == text {
			s.spans.UnchangedPrefixBytes = pos + len(text)
		} else {
			s.diverged = true
		}
	}
	n, err := io.WriteString(s.w, text)
	s.written += n
	s.err = err
}

func (s *runtimeStateStreamEncoder) writePreimageSpan(start, end int) {
	if s.err != nil || end <= start {
		return
	}
	pos := s.written
	if !s.diverged {
		if pos == start && end <= len(s.prevRaw) {
			s.spans.UnchangedPrefixBytes = end
		} else {
			s.diverged = true
		}
	}
	n, err := s.w.Write(s.prevRaw[start:end])
	s.written += n
	s.err = err
}

func (s *runtimeStateStreamEncoder) writeValue(v any) {
	if s.err != nil {
		return
	}
	if err := s.enc.Encode(v); err != nil {
		s.err = err
	}
}

func (s *runtimeStateStreamEncoder) writeFieldPrefix(first *bool, field string) {
	if s.err != nil {
		return
	}
	if !*first {
		s.writeString(",")
	}
	*first = false
	s.writeString(`"`)
	s.writeString(field)
	s.writeString(`":`)
}

func (s *runtimeStateStreamEncoder) writeField(first *bool, field string, v any) RuntimeStateJSONByteSpan {
	s.writeFieldPrefix(first, field)
	start := s.written
	s.writeValue(v)
	return RuntimeStateJSONByteSpan{Start: start, End: s.written, Present: s.err == nil}
}

func (s *runtimeStateStreamEncoder) canReuseSpan(span RuntimeStateJSONByteSpan) bool {
	return s.prev != nil && s.prevSpans != nil && span.Present &&
		span.Start >= 0 && span.End >= span.Start && span.End <= len(s.prevRaw)
}

func writeSubfieldWithPreimage[T any](s *runtimeStateStreamEncoder, first *bool, field string, curr *T, prevVal *T, prevSpan RuntimeStateJSONByteSpan) RuntimeStateJSONByteSpan {
	s.writeFieldPrefix(first, field)
	start := s.written
	if prevVal != nil && s.prev != nil && s.prevSpans != nil && s.canReuseSpan(prevSpan) && runtimeRawEqual(curr, prevVal) {
		s.writePreimageSpan(prevSpan.Start, prevSpan.End)
		return RuntimeStateJSONByteSpan{Start: start, End: s.written, Present: s.err == nil}
	}
	s.writeValue(curr)
	return RuntimeStateJSONByteSpan{Start: start, End: s.written, Present: s.err == nil}
}

func writeStructFieldWithPreimage[T any](s *runtimeStateStreamEncoder, first *bool, idx int, field string, curr *T, prevVal *T) {
	var prevSpan RuntimeStateJSONByteSpan
	if s.prevSpans != nil {
		prevSpan = s.prevSpans.Fields[idx]
	}
	s.spans.Fields[idx] = writeSubfieldWithPreimage(s, first, field, curr, prevVal, prevSpan)
}

func writePtrSubfieldWithPreimage[T any](s *runtimeStateStreamEncoder, first *bool, field string, curr *T, prevVal *T, hasPrev bool, prevSpan RuntimeStateJSONByteSpan) RuntimeStateJSONByteSpan {
	s.writeFieldPrefix(first, field)
	start := s.written
	if hasPrev && s.canReuseSpan(prevSpan) &&
		((curr == nil && prevVal == nil) || (curr != nil && prevVal != nil && runtimeRawEqual(curr, prevVal))) {
		s.writePreimageSpan(prevSpan.Start, prevSpan.End)
		return RuntimeStateJSONByteSpan{Start: start, End: s.written, Present: s.err == nil}
	}
	s.writeValue(curr)
	return RuntimeStateJSONByteSpan{Start: start, End: s.written, Present: s.err == nil}
}

func writePtrFieldWithPreimage[T any](s *runtimeStateStreamEncoder, first *bool, idx int, field string, curr *T, prevVal *T) {
	var prevSpan RuntimeStateJSONByteSpan
	if s.prevSpans != nil {
		prevSpan = s.prevSpans.Fields[idx]
	}
	s.spans.Fields[idx] = writePtrSubfieldWithPreimage(s, first, field, curr, prevVal, s.prev != nil && s.prevSpans != nil, prevSpan)
}

func writeStreamSlice[T any](s *runtimeStateStreamEncoder, items []T) int {
	if s.err != nil {
		return 0
	}
	if items == nil {
		s.writeString("null")
		return 0
	}
	if len(items) == 0 {
		s.writeString("[]")
		return 0
	}
	s.writeString("[")
	lastElemStart := 0
	for i := range items {
		if i > 0 {
			s.writeString(",")
		}
		if i == len(items)-1 {
			lastElemStart = s.written
		}
		s.writeValue(&items[i])
	}
	s.writeString("]")
	if s.err != nil {
		return 0
	}
	return lastElemStart
}

func writeSliceWithPreimage[T any](s *runtimeStateStreamEncoder, items []T, prevItems []T, prevSpan RuntimeStateJSONByteSpan) RuntimeStateJSONByteSpan {
	start := s.written
	if s.canReuseSpan(prevSpan) {
		if len(items) == len(prevItems) && (items == nil) == (prevItems == nil) &&
			(len(items) == 0 || runtimeRawEqual(items, prevItems)) {
			s.writePreimageSpan(prevSpan.Start, prevSpan.End)
			if s.err != nil {
				return RuntimeStateJSONByteSpan{}
			}
			return shiftSpan(prevSpan, start-prevSpan.Start)
		}
		if len(prevItems) > 0 && len(items) > len(prevItems) &&
			prevSpan.End-prevSpan.Start >= 2 &&
			s.prevRaw[prevSpan.Start] == '[' && s.prevRaw[prevSpan.End-1] == ']' &&
			runtimeRawEqual(items[:len(prevItems)], prevItems) {
			s.writePreimageSpan(prevSpan.Start, prevSpan.End-1)
			lastElemStart := 0
			for i := len(prevItems); i < len(items); i++ {
				s.writeString(",")
				if i == len(items)-1 {
					lastElemStart = s.written
				}
				s.writeValue(&items[i])
			}
			s.writeString("]")
			return RuntimeStateJSONByteSpan{Start: start, End: s.written, LastElementStart: lastElemStart, Present: s.err == nil}
		}
		if len(prevItems) >= 2 && len(items) >= len(prevItems) &&
			prevSpan.LastElementStart > prevSpan.Start && prevSpan.LastElementStart < prevSpan.End &&
			s.prevRaw[prevSpan.Start] == '[' && s.prevRaw[prevSpan.End-1] == ']' &&
			runtimeRawEqual(items[:len(prevItems)-1], prevItems[:len(prevItems)-1]) {
			s.writePreimageSpan(prevSpan.Start, prevSpan.LastElementStart)
			lastElemStart := 0
			for i := len(prevItems) - 1; i < len(items); i++ {
				if i > len(prevItems)-1 {
					s.writeString(",")
				}
				if i == len(items)-1 {
					lastElemStart = s.written
				}
				s.writeValue(&items[i])
			}
			s.writeString("]")
			return RuntimeStateJSONByteSpan{Start: start, End: s.written, LastElementStart: lastElemStart, Present: s.err == nil}
		}
	}
	lastElemStart := writeStreamSlice(s, items)
	return RuntimeStateJSONByteSpan{Start: start, End: s.written, LastElementStart: lastElemStart, Present: s.err == nil}
}

func shiftSpan(span RuntimeStateJSONByteSpan, delta int) RuntimeStateJSONByteSpan {
	if !span.Present {
		return RuntimeStateJSONByteSpan{}
	}
	lastStart := 0
	if span.LastElementStart > 0 {
		lastStart = span.LastElementStart + delta
	}
	return RuntimeStateJSONByteSpan{Start: span.Start + delta, End: span.End + delta, LastElementStart: lastStart, Present: true}
}

func (s *runtimeStateStreamEncoder) writeRuntimeStateRecord(state *RuntimeStateRecord) {
	s.writeString("{")
	first := true
	prev := s.prev
	hasPrev := prev != nil && s.prevSpans != nil

	if hasPrev {
		writeStructFieldWithPreimage(s, &first, 0, "Definition", &state.Definition, &prev.Definition)
		s.spans.Fields[1] = writeSliceFieldSpan(s, &first, "CoverageProofLineage", state.CoverageProofLineage, prev.CoverageProofLineage, true, s.prevSpans.Fields[1])
		s.spans.Fields[2] = writeSliceFieldSpan(s, &first, "OperationReceipts", state.OperationReceipts, prev.OperationReceipts, true, s.prevSpans.Fields[2])
		s.spans.Fields[3] = writeSliceFieldSpan(s, &first, "ProcessedEventIDs", state.ProcessedEventIDs, prev.ProcessedEventIDs, true, s.prevSpans.Fields[3])
		s.spans.Fields[4] = writeSliceFieldSpan(s, &first, "CompletedDispatches", state.CompletedDispatches, prev.CompletedDispatches, true, s.prevSpans.Fields[4])
	} else {
		s.spans.Fields[0] = s.writeField(&first, "Definition", &state.Definition)
		s.spans.Fields[1] = writeSliceFieldSpan(s, &first, "CoverageProofLineage", state.CoverageProofLineage, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[2] = writeSliceFieldSpan(s, &first, "OperationReceipts", state.OperationReceipts, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[3] = writeSliceFieldSpan(s, &first, "ProcessedEventIDs", state.ProcessedEventIDs, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[4] = writeSliceFieldSpan(s, &first, "CompletedDispatches", state.CompletedDispatches, nil, false, RuntimeStateJSONByteSpan{})
	}

	s.writeFieldPrefix(&first, "Reconciliation")
	recStart := s.written
	if hasPrev && s.canReuseSpan(s.prevSpans.Fields[5]) && runtimeRawEqual(&state.Reconciliation, &prev.Reconciliation) {
		prevSpan := s.prevSpans.Fields[5]
		s.writePreimageSpan(prevSpan.Start, prevSpan.End)
		delta := recStart - prevSpan.Start
		s.spans.ReconciliationHistory = shiftSpan(s.prevSpans.ReconciliationHistory, delta)
		s.spans.ReconciliationRequest = shiftSpan(s.prevSpans.ReconciliationRequest, delta)
		s.spans.ReconciliationBatch = shiftSpan(s.prevSpans.ReconciliationBatch, delta)
		s.spans.ReconciliationResults = shiftSpan(s.prevSpans.ReconciliationResults, delta)
		s.spans.ReconciliationReopened = shiftSpan(s.prevSpans.ReconciliationReopened, delta)
	} else {
		s.writeCWIReconciliationLedger(&state.Reconciliation)
	}
	s.spans.Fields[5] = RuntimeStateJSONByteSpan{Start: recStart, End: s.written, Present: s.err == nil}

	if hasPrev {
		s.spans.Fields[6] = writeSliceFieldSpan(s, &first, "CandidateAttempts", state.CandidateAttempts, prev.CandidateAttempts, true, s.prevSpans.Fields[6])
		s.spans.Fields[7] = writeSliceFieldSpan(s, &first, "CandidateRequestSessions", state.CandidateRequestSessions, prev.CandidateRequestSessions, true, s.prevSpans.Fields[7])
	} else {
		s.spans.Fields[6] = writeSliceFieldSpan(s, &first, "CandidateAttempts", state.CandidateAttempts, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[7] = writeSliceFieldSpan(s, &first, "CandidateRequestSessions", state.CandidateRequestSessions, nil, false, RuntimeStateJSONByteSpan{})
	}

	s.writeFieldPrefix(&first, "SemanticPlanning")
	semStart := s.written
	if hasPrev && s.canReuseSpan(s.prevSpans.Fields[8]) && runtimeRawEqual(&state.SemanticPlanning, &prev.SemanticPlanning) {
		prevSpan := s.prevSpans.Fields[8]
		s.writePreimageSpan(prevSpan.Start, prevSpan.End)
		delta := semStart - prevSpan.Start
		s.spans.SemanticManifest = shiftSpan(s.prevSpans.SemanticManifest, delta)
		s.spans.SemanticCoverageOrderDocument = shiftSpan(s.prevSpans.SemanticCoverageOrderDocument, delta)
		s.spans.SemanticDocument = shiftSpan(s.prevSpans.SemanticDocument, delta)
		s.spans.SemanticBatches = shiftSpan(s.prevSpans.SemanticBatches, delta)
		s.spans.SemanticBatchFeasibilityResults = shiftSpan(s.prevSpans.SemanticBatchFeasibilityResults, delta)
		s.spans.SemanticBatchDocuments = shiftSpan(s.prevSpans.SemanticBatchDocuments, delta)
		s.spans.SemanticMergedDocument = shiftSpan(s.prevSpans.SemanticMergedDocument, delta)
		s.spans.SemanticFeasibilities = shiftSpan(s.prevSpans.SemanticFeasibilities, delta)
		s.spans.SemanticHistory = shiftSpan(s.prevSpans.SemanticHistory, delta)
		s.spans.SemanticHistoryArchive = shiftSpan(s.prevSpans.SemanticHistoryArchive, delta)
	} else {
		s.writeSemanticPlanningLedger(&state.SemanticPlanning)
	}
	s.spans.Fields[8] = RuntimeStateJSONByteSpan{Start: semStart, End: s.written, Present: s.err == nil}

	s.writeFieldPrefix(&first, "Facts")
	factsStart := s.written
	if hasPrev && s.canReuseSpan(s.prevSpans.Fields[9]) && runtimeRawEqual(&state.Facts, &prev.Facts) {
		prevSpan := s.prevSpans.Fields[9]
		s.writePreimageSpan(prevSpan.Start, prevSpan.End)
		delta := factsStart - prevSpan.Start
		s.spans.FactsDefinitionSeed = shiftSpan(s.prevSpans.FactsDefinitionSeed, delta)
		s.spans.FactsDefinitionFocuses = shiftSpan(s.prevSpans.FactsDefinitionFocuses, delta)
		s.spans.FactsFocus = shiftSpan(s.prevSpans.FactsFocus, delta)
		s.spans.FactsRemainingFocuses = shiftSpan(s.prevSpans.FactsRemainingFocuses, delta)
		s.spans.FactsCompletedFocuses = shiftSpan(s.prevSpans.FactsCompletedFocuses, delta)
		s.spans.FactsEvidence = shiftSpan(s.prevSpans.FactsEvidence, delta)
		s.spans.FactsEvidenceApplicabilityObservations = shiftSpan(s.prevSpans.FactsEvidenceApplicabilityObservations, delta)
		s.spans.FactsAssessments = shiftSpan(s.prevSpans.FactsAssessments, delta)
		s.spans.FactsPrerequisites = shiftSpan(s.prevSpans.FactsPrerequisites, delta)
		s.spans.FactsCapabilities = shiftSpan(s.prevSpans.FactsCapabilities, delta)
		s.spans.FactsResources = shiftSpan(s.prevSpans.FactsResources, delta)
		s.spans.FactsActivation = shiftSpan(s.prevSpans.FactsActivation, delta)
		s.spans.FactsReconciliationContract = shiftSpan(s.prevSpans.FactsReconciliationContract, delta)
		s.spans.FactsCoverage = shiftSpan(s.prevSpans.FactsCoverage, delta)
		s.spans.FactsCWISuspensions = shiftSpan(s.prevSpans.FactsCWISuspensions, delta)
		s.spans.FactsSlices = shiftSpan(s.prevSpans.FactsSlices, delta)
	} else {
		s.writeRuntimeFacts(&state.Facts)
	}
	s.spans.Fields[9] = RuntimeStateJSONByteSpan{Start: factsStart, End: s.written, Present: s.err == nil}

	s.writeFieldPrefix(&first, "ExecutionBudget")
	budStart := s.written
	if hasPrev && s.canReuseSpan(s.prevSpans.Fields[10]) && runtimeRawEqual(&state.ExecutionBudget, &prev.ExecutionBudget) {
		prevSpan := s.prevSpans.Fields[10]
		s.writePreimageSpan(prevSpan.Start, prevSpan.End)
		delta := budStart - prevSpan.Start
		s.spans.BudgetPolicy = shiftSpan(s.prevSpans.BudgetPolicy, delta)
		s.spans.BudgetAllocations = shiftSpan(s.prevSpans.BudgetAllocations, delta)
		s.spans.BudgetConsumptions = shiftSpan(s.prevSpans.BudgetConsumptions, delta)
		s.spans.BudgetReconciliations = shiftSpan(s.prevSpans.BudgetReconciliations, delta)
	} else {
		s.writeExecutionBudgetLedger(&state.ExecutionBudget)
	}
	s.spans.Fields[10] = RuntimeStateJSONByteSpan{Start: budStart, End: s.written, Present: s.err == nil}

	if hasPrev {
		s.spans.Fields[11] = writeSliceFieldSpan(s, &first, "RepairSurfaceConfirmations", state.RepairSurfaceConfirmations, prev.RepairSurfaceConfirmations, true, s.prevSpans.Fields[11])
		s.spans.Fields[12] = writeSliceFieldSpan(s, &first, "Diagnostics", state.Diagnostics, prev.Diagnostics, true, s.prevSpans.Fields[12])
		s.spans.Fields[13] = writeSliceFieldSpan(s, &first, "PrerequisiteInvalidations", state.PrerequisiteInvalidations, prev.PrerequisiteInvalidations, true, s.prevSpans.Fields[13])
		s.spans.Fields[14] = writeSliceFieldSpan(s, &first, "GovernanceRequests", state.GovernanceRequests, prev.GovernanceRequests, true, s.prevSpans.Fields[14])
		s.spans.Fields[15] = writeSliceFieldSpan(s, &first, "GovernanceResolutions", state.GovernanceResolutions, prev.GovernanceResolutions, true, s.prevSpans.Fields[15])
		s.spans.Fields[16] = writeSliceFieldSpan(s, &first, "OperationGovernanceBindings", state.OperationGovernanceBindings, prev.OperationGovernanceBindings, true, s.prevSpans.Fields[16])
		s.spans.Fields[17] = writeSliceFieldSpan(s, &first, "OperationRecoveryAuthorizations", state.OperationRecoveryAuthorizations, prev.OperationRecoveryAuthorizations, true, s.prevSpans.Fields[17])
		s.spans.Fields[18] = writeSliceFieldSpan(s, &first, "RecoverableWaits", state.RecoverableWaits, prev.RecoverableWaits, true, s.prevSpans.Fields[18])
		s.spans.Fields[19] = writeSliceFieldSpan(s, &first, "DeliveryAcknowledgements", state.DeliveryAcknowledgements, prev.DeliveryAcknowledgements, true, s.prevSpans.Fields[19])
		writeStructFieldWithPreimage(s, &first, 20, "ImplementationWork", &state.ImplementationWork, &prev.ImplementationWork)
		writePtrFieldWithPreimage(s, &first, 21, "DefinitionClarification", state.DefinitionClarification, prev.DefinitionClarification)
		writePtrFieldWithPreimage(s, &first, 22, "TerminalMaterialSeal", state.TerminalMaterialSeal, prev.TerminalMaterialSeal)
		writePtrFieldWithPreimage(s, &first, 23, "ActiveDispatch", state.ActiveDispatch, prev.ActiveDispatch)
		writeStructFieldWithPreimage(s, &first, 24, "Decision", &state.Decision, &prev.Decision)
		writePtrFieldWithPreimage(s, &first, 25, "DriveLease", state.DriveLease, prev.DriveLease)
		writePtrFieldWithPreimage(s, &first, 26, "Termination", state.Termination, prev.Termination)
	} else {
		s.spans.Fields[11] = writeSliceFieldSpan(s, &first, "RepairSurfaceConfirmations", state.RepairSurfaceConfirmations, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[12] = writeSliceFieldSpan(s, &first, "Diagnostics", state.Diagnostics, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[13] = writeSliceFieldSpan(s, &first, "PrerequisiteInvalidations", state.PrerequisiteInvalidations, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[14] = writeSliceFieldSpan(s, &first, "GovernanceRequests", state.GovernanceRequests, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[15] = writeSliceFieldSpan(s, &first, "GovernanceResolutions", state.GovernanceResolutions, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[16] = writeSliceFieldSpan(s, &first, "OperationGovernanceBindings", state.OperationGovernanceBindings, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[17] = writeSliceFieldSpan(s, &first, "OperationRecoveryAuthorizations", state.OperationRecoveryAuthorizations, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[18] = writeSliceFieldSpan(s, &first, "RecoverableWaits", state.RecoverableWaits, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[19] = writeSliceFieldSpan(s, &first, "DeliveryAcknowledgements", state.DeliveryAcknowledgements, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.Fields[20] = s.writeField(&first, "ImplementationWork", &state.ImplementationWork)
		s.spans.Fields[21] = s.writeField(&first, "DefinitionClarification", state.DefinitionClarification)
		s.spans.Fields[22] = s.writeField(&first, "TerminalMaterialSeal", state.TerminalMaterialSeal)
		s.spans.Fields[23] = s.writeField(&first, "ActiveDispatch", state.ActiveDispatch)
		s.spans.Fields[24] = s.writeField(&first, "Decision", &state.Decision)
		s.spans.Fields[25] = s.writeField(&first, "DriveLease", state.DriveLease)
		s.spans.Fields[26] = s.writeField(&first, "Termination", state.Termination)
	}
	if state.ResultFeedbackVersion != 0 {
		s.spans.Fields[27] = s.writeField(&first, "ResultFeedbackVersion", &state.ResultFeedbackVersion)
	}
	s.spans.Fields[28] = s.writeField(&first, "StateRevision", &state.StateRevision)
	s.spans.Fields[29] = s.writeField(&first, "EvidenceSequence", &state.EvidenceSequence)
	s.writeString("}")
}

func writeSliceFieldSpan[T any](s *runtimeStateStreamEncoder, first *bool, field string, items []T, prevItems []T, hasPrev bool, prevSpan RuntimeStateJSONByteSpan) RuntimeStateJSONByteSpan {
	s.writeFieldPrefix(first, field)
	if hasPrev {
		return writeSliceWithPreimage(s, items, prevItems, prevSpan)
	}
	start := s.written
	lastElemStart := writeStreamSlice(s, items)
	return RuntimeStateJSONByteSpan{Start: start, End: s.written, LastElementStart: lastElemStart, Present: s.err == nil}
}

func (s *runtimeStateStreamEncoder) writeCWIReconciliationLedger(ledger *RuntimeCWIReconciliationLedger) {
	s.writeString("{")
	first := true
	var prevLedger *RuntimeCWIReconciliationLedger
	var prevSpans *RuntimeStateJSONSpans
	if s.prev != nil && s.prevSpans != nil {
		prevLedger = &s.prev.Reconciliation
		prevSpans = s.prevSpans
	}
	if prevLedger != nil {
		s.spans.ReconciliationHistory = writeSliceFieldSpan(s, &first, "History", ledger.History, prevLedger.History, true, prevSpans.ReconciliationHistory)
		s.spans.ReconciliationRequest = writePtrSubfieldWithPreimage(s, &first, "Request", ledger.Request, prevLedger.Request, true, prevSpans.ReconciliationRequest)
		s.writeField(&first, "RequestHash", &ledger.RequestHash)
		s.spans.ReconciliationBatch = writePtrSubfieldWithPreimage(s, &first, "Batch", ledger.Batch, prevLedger.Batch, true, prevSpans.ReconciliationBatch)
		s.spans.ReconciliationResults = writeSliceFieldSpan(s, &first, "Results", ledger.Results, prevLedger.Results, true, prevSpans.ReconciliationResults)
		s.spans.ReconciliationReopened = writeSliceFieldSpan(s, &first, "Reopened", ledger.Reopened, prevLedger.Reopened, true, prevSpans.ReconciliationReopened)
	} else {
		s.spans.ReconciliationHistory = writeSliceFieldSpan(s, &first, "History", ledger.History, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.ReconciliationRequest = writePtrSubfieldWithPreimage(s, &first, "Request", ledger.Request, nil, false, RuntimeStateJSONByteSpan{})
		s.writeField(&first, "RequestHash", &ledger.RequestHash)
		s.spans.ReconciliationBatch = writePtrSubfieldWithPreimage(s, &first, "Batch", ledger.Batch, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.ReconciliationResults = writeSliceFieldSpan(s, &first, "Results", ledger.Results, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.ReconciliationReopened = writeSliceFieldSpan(s, &first, "Reopened", ledger.Reopened, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeString("}")
}

func (s *runtimeStateStreamEncoder) writeExecutionBudgetLedger(ledger *RuntimeExecutionBudgetLedger) {
	s.writeString("{")
	first := true
	var prevLedger *RuntimeExecutionBudgetLedger
	var prevSpans *RuntimeStateJSONSpans
	if s.prev != nil && s.prevSpans != nil {
		prevLedger = &s.prev.ExecutionBudget
		prevSpans = s.prevSpans
	}
	if prevLedger != nil {
		s.spans.BudgetPolicy = writeSubfieldWithPreimage(s, &first, "Policy", &ledger.Policy, &prevLedger.Policy, prevSpans.BudgetPolicy)
		s.spans.BudgetConsumptions = writeSliceFieldSpan(s, &first, "Consumptions", ledger.Consumptions, prevLedger.Consumptions, true, prevSpans.BudgetConsumptions)
		s.spans.BudgetReconciliations = writeSliceFieldSpan(s, &first, "Reconciliations", ledger.Reconciliations, prevLedger.Reconciliations, true, prevSpans.BudgetReconciliations)
		s.spans.BudgetAllocations = writeSliceFieldSpan(s, &first, "Allocations", ledger.Allocations, prevLedger.Allocations, true, prevSpans.BudgetAllocations)
	} else {
		s.spans.BudgetPolicy = s.writeField(&first, "Policy", &ledger.Policy)
		s.spans.BudgetConsumptions = writeSliceFieldSpan(s, &first, "Consumptions", ledger.Consumptions, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.BudgetReconciliations = writeSliceFieldSpan(s, &first, "Reconciliations", ledger.Reconciliations, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.BudgetAllocations = writeSliceFieldSpan(s, &first, "Allocations", ledger.Allocations, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeString("}")
}

func (s *runtimeStateStreamEncoder) writeRuntimeFacts(facts *RuntimeFacts) {
	s.writeString("{")
	first := true
	var prevFacts *RuntimeFacts
	var prevSpans *RuntimeStateJSONSpans
	if s.prev != nil && s.prevSpans != nil {
		prevFacts = &s.prev.Facts
		prevSpans = s.prevSpans
	}
	s.writeField(&first, "WorkspaceID", &facts.WorkspaceID)
	s.writeField(&first, "RunID", &facts.RunID)
	s.writeField(&first, "DefinitionRevision", &facts.DefinitionRevision)
	s.writeField(&first, "CWILedgerSchemaVersion", &facts.CWILedgerSchemaVersion)
	if prevFacts != nil {
		s.spans.FactsDefinitionSeed = writeSubfieldWithPreimage(s, &first, "DefinitionSeed", &facts.DefinitionSeed, &prevFacts.DefinitionSeed, prevSpans.FactsDefinitionSeed)
		s.spans.FactsDefinitionFocuses = writeSliceFieldSpan(s, &first, "DefinitionFocuses", facts.DefinitionFocuses, prevFacts.DefinitionFocuses, true, prevSpans.FactsDefinitionFocuses)
		s.spans.FactsActivation = writeSubfieldWithPreimage(s, &first, "Activation", &facts.Activation, &prevFacts.Activation, prevSpans.FactsActivation)
	} else {
		s.spans.FactsDefinitionSeed = s.writeField(&first, "DefinitionSeed", &facts.DefinitionSeed)
		s.spans.FactsDefinitionFocuses = writeSliceFieldSpan(s, &first, "DefinitionFocuses", facts.DefinitionFocuses, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.FactsActivation = s.writeField(&first, "Activation", &facts.Activation)
	}
	s.writeField(&first, "ConvergencePolicy", &facts.ConvergencePolicy)
	s.writeField(&first, "RecoveryPolicy", &facts.RecoveryPolicy)
	s.writeField(&first, "ExecutionBudgetPolicy", &facts.ExecutionBudgetPolicy)
	if prevFacts != nil {
		s.spans.FactsSlices = writeSliceFieldSpan(s, &first, "Slices", facts.Slices, prevFacts.Slices, true, prevSpans.FactsSlices)
		s.spans.FactsCompletedFocuses = writeSliceFieldSpan(s, &first, "CompletedFocuses", facts.CompletedFocuses, prevFacts.CompletedFocuses, true, prevSpans.FactsCompletedFocuses)
		s.spans.FactsAssessments = writeSliceFieldSpan(s, &first, "Assessments", facts.Assessments, prevFacts.Assessments, true, prevSpans.FactsAssessments)
		s.spans.FactsEvidence = writeSubfieldWithPreimage(s, &first, "Evidence", &facts.Evidence, &prevFacts.Evidence, prevSpans.FactsEvidence)
	} else {
		s.spans.FactsSlices = writeSliceFieldSpan(s, &first, "Slices", facts.Slices, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.FactsCompletedFocuses = writeSliceFieldSpan(s, &first, "CompletedFocuses", facts.CompletedFocuses, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.FactsAssessments = writeSliceFieldSpan(s, &first, "Assessments", facts.Assessments, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.FactsEvidence = s.writeField(&first, "Evidence", &facts.Evidence)
	}
	if len(facts.EvidenceApplicabilityObservations) != 0 {
		hasPrevObs := prevFacts != nil && len(prevFacts.EvidenceApplicabilityObservations) != 0
		var prevObs []RuntimeEvidenceApplicabilityObservation
		var prevObsSpan RuntimeStateJSONByteSpan
		if hasPrevObs {
			prevObs = prevFacts.EvidenceApplicabilityObservations
			prevObsSpan = prevSpans.FactsEvidenceApplicabilityObservations
		}
		s.spans.FactsEvidenceApplicabilityObservations = writeSliceFieldSpan(
			s,
			&first,
			"EvidenceApplicabilityObservations",
			facts.EvidenceApplicabilityObservations,
			prevObs,
			hasPrevObs,
			prevObsSpan,
		)
	}
	if prevFacts != nil {
		s.spans.FactsCapabilities = writeSubfieldWithPreimage(s, &first, "Capabilities", &facts.Capabilities, &prevFacts.Capabilities, prevSpans.FactsCapabilities)
		s.spans.FactsReconciliationContract = writeSubfieldWithPreimage(s, &first, "ReconciliationContract", &facts.ReconciliationContract, &prevFacts.ReconciliationContract, prevSpans.FactsReconciliationContract)
		s.spans.FactsRemainingFocuses = writeSliceFieldSpan(s, &first, "RemainingFocuses", facts.RemainingFocuses, prevFacts.RemainingFocuses, true, prevSpans.FactsRemainingFocuses)
		s.spans.FactsCoverage = writeSliceFieldSpan(s, &first, "Coverage", facts.Coverage, prevFacts.Coverage, true, prevSpans.FactsCoverage)
	} else {
		s.spans.FactsCapabilities = s.writeField(&first, "Capabilities", &facts.Capabilities)
		s.spans.FactsReconciliationContract = s.writeField(&first, "ReconciliationContract", &facts.ReconciliationContract)
		s.spans.FactsRemainingFocuses = writeSliceFieldSpan(s, &first, "RemainingFocuses", facts.RemainingFocuses, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.FactsCoverage = writeSliceFieldSpan(s, &first, "Coverage", facts.Coverage, nil, false, RuntimeStateJSONByteSpan{})
	}
	if len(facts.CWISuspensions) != 0 {
		hasPrevSusp := prevFacts != nil && len(prevFacts.CWISuspensions) != 0
		var prevSusp []RuntimeCWISuspension
		var prevSuspSpan RuntimeStateJSONByteSpan
		if hasPrevSusp {
			prevSusp = prevFacts.CWISuspensions
			prevSuspSpan = prevSpans.FactsCWISuspensions
		}
		s.spans.FactsCWISuspensions = writeSliceFieldSpan(
			s,
			&first,
			"CWISuspensions",
			facts.CWISuspensions,
			prevSusp,
			hasPrevSusp,
			prevSuspSpan,
		)
	}
	if prevFacts != nil {
		s.spans.FactsResources = writeSubfieldWithPreimage(s, &first, "Resources", &facts.Resources, &prevFacts.Resources, prevSpans.FactsResources)
		s.spans.FactsPrerequisites = writeSubfieldWithPreimage(s, &first, "Prerequisites", &facts.Prerequisites, &prevFacts.Prerequisites, prevSpans.FactsPrerequisites)
		s.writeField(&first, "EvidenceRevision", &facts.EvidenceRevision)
		s.spans.FactsFocus = writeSubfieldWithPreimage(s, &first, "Focus", &facts.Focus, &prevFacts.Focus, prevSpans.FactsFocus)
	} else {
		s.spans.FactsResources = s.writeField(&first, "Resources", &facts.Resources)
		s.spans.FactsPrerequisites = s.writeField(&first, "Prerequisites", &facts.Prerequisites)
		s.writeField(&first, "EvidenceRevision", &facts.EvidenceRevision)
		s.spans.FactsFocus = s.writeField(&first, "Focus", &facts.Focus)
	}
	s.writeField(&first, "Feasibility", facts.Feasibility)
	s.writeField(&first, "Failure", facts.Failure)
	s.writeField(&first, "Repair", &facts.Repair)
	s.writeField(&first, "RepairDirective", facts.RepairDirective)
	s.writeField(&first, "Recovery", &facts.Recovery)
	s.writeField(&first, "StateChange", &facts.StateChange)
	s.writeString("}")
}

func (s *runtimeStateStreamEncoder) writeSemanticPlanningLedger(ledger *RuntimeSemanticPlanningLedger) {
	s.writeString("{")
	first := true
	var prevLedger *RuntimeSemanticPlanningLedger
	var prevSpans *RuntimeStateJSONSpans
	if s.prev != nil && s.prevSpans != nil {
		prevLedger = &s.prev.SemanticPlanning
		prevSpans = s.prevSpans
	}
	s.writeField(&first, "SchemaVersion", &ledger.SchemaVersion)
	s.writeField(&first, "DefinitionSeedHash", &ledger.DefinitionSeedHash)
	if ledger.HistoryArchive != nil {
		s.writeFieldPrefix(&first, "HistoryArchive")
		archStart := s.written
		if prevLedger != nil && prevLedger.HistoryArchive != nil &&
			s.canReuseSpan(prevSpans.SemanticHistoryArchive) &&
			runtimeRawEqual(ledger.HistoryArchive, prevLedger.HistoryArchive) {
			s.writePreimageSpan(prevSpans.SemanticHistoryArchive.Start, prevSpans.SemanticHistoryArchive.End)
		} else {
			s.writeSemanticHistoryArchive(ledger.HistoryArchive)
		}
		s.spans.SemanticHistoryArchive = RuntimeStateJSONByteSpan{Start: archStart, End: s.written, Present: s.err == nil}
	}
	s.writeFieldPrefix(&first, "History")
	if prevLedger != nil {
		s.spans.SemanticHistory = writeSliceWithPreimage(s, ledger.History, prevLedger.History, prevSpans.SemanticHistory)
	} else {
		start := s.written
		lastElemStart := writeStreamSlice(s, ledger.History)
		s.spans.SemanticHistory = RuntimeStateJSONByteSpan{Start: start, End: s.written, LastElementStart: lastElemStart, Present: s.err == nil}
	}
	if prevLedger != nil {
		s.spans.SemanticManifest = writePtrSubfieldWithPreimage(s, &first, "Manifest", ledger.Manifest, prevLedger.Manifest, true, prevSpans.SemanticManifest)
	} else {
		s.spans.SemanticManifest = writePtrSubfieldWithPreimage(s, &first, "Manifest", ledger.Manifest, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeField(&first, "StablePrefixHash", &ledger.StablePrefixHash)
	if ledger.CompiledOrder != nil {
		s.writeField(&first, "CompiledOrder", ledger.CompiledOrder)
	}
	if len(ledger.CompiledProofBatches) != 0 {
		s.writeFieldPrefix(&first, "CompiledProofBatches")
		writeStreamSlice(s, ledger.CompiledProofBatches)
	}
	if prevLedger != nil {
		s.spans.SemanticCoverageOrderDocument = writePtrSubfieldWithPreimage(s, &first, "CoverageOrderDocument", ledger.CoverageOrderDocument, prevLedger.CoverageOrderDocument, true, prevSpans.SemanticCoverageOrderDocument)
	} else {
		s.spans.SemanticCoverageOrderDocument = writePtrSubfieldWithPreimage(s, &first, "CoverageOrderDocument", ledger.CoverageOrderDocument, nil, false, RuntimeStateJSONByteSpan{})
	}
	if ledger.FailureInputReview != nil {
		s.writeField(&first, "FailureInputReview", ledger.FailureInputReview)
	}
	if prevLedger != nil {
		s.spans.SemanticBatchDocuments = writeSliceFieldSpan(s, &first, "BatchDocuments", ledger.BatchDocuments, prevLedger.BatchDocuments, true, prevSpans.SemanticBatchDocuments)
		s.spans.SemanticBatchFeasibilityResults = writeSliceFieldSpan(s, &first, "BatchFeasibilityResults", ledger.BatchFeasibilityResults, prevLedger.BatchFeasibilityResults, true, prevSpans.SemanticBatchFeasibilityResults)
		s.spans.SemanticMergedDocument = writePtrSubfieldWithPreimage(s, &first, "MergedDocument", ledger.MergedDocument, prevLedger.MergedDocument, true, prevSpans.SemanticMergedDocument)
	} else {
		s.spans.SemanticBatchDocuments = writeSliceFieldSpan(s, &first, "BatchDocuments", ledger.BatchDocuments, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.SemanticBatchFeasibilityResults = writeSliceFieldSpan(s, &first, "BatchFeasibilityResults", ledger.BatchFeasibilityResults, nil, false, RuntimeStateJSONByteSpan{})
		s.spans.SemanticMergedDocument = writePtrSubfieldWithPreimage(s, &first, "MergedDocument", ledger.MergedDocument, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeField(&first, "MergedDocumentHash", &ledger.MergedDocumentHash)
	if prevLedger != nil {
		s.spans.SemanticDocument = writePtrSubfieldWithPreimage(s, &first, "Document", ledger.Document, prevLedger.Document, true, prevSpans.SemanticDocument)
	} else {
		s.spans.SemanticDocument = writePtrSubfieldWithPreimage(s, &first, "Document", ledger.Document, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeField(&first, "DocumentHash", &ledger.DocumentHash)
	if prevLedger != nil {
		s.spans.SemanticFeasibilities = writeSliceFieldSpan(s, &first, "Feasibilities", ledger.Feasibilities, prevLedger.Feasibilities, true, prevSpans.SemanticFeasibilities)
	} else {
		s.spans.SemanticFeasibilities = writeSliceFieldSpan(s, &first, "Feasibilities", ledger.Feasibilities, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeField(&first, "FeasibilitySetHash", &ledger.FeasibilitySetHash)
	s.writeField(&first, "SealedBindingSetHash", &ledger.SealedBindingSetHash)
	s.writeFieldPrefix(&first, "SealedLineageRefs")
	writeStreamSlice(s, ledger.SealedLineageRefs)
	if prevLedger != nil {
		s.spans.SemanticBatches = writeSliceFieldSpan(s, &first, "Batches", ledger.Batches, prevLedger.Batches, true, prevSpans.SemanticBatches)
	} else {
		s.spans.SemanticBatches = writeSliceFieldSpan(s, &first, "Batches", ledger.Batches, nil, false, RuntimeStateJSONByteSpan{})
	}
	s.writeField(&first, "Phase", &ledger.Phase)
	s.writeField(&first, "Attempt", &ledger.Attempt)
	s.writeField(&first, "LogicalRequestID", &ledger.LogicalRequestID)
	s.writeField(&first, "CorrectionDirective", ledger.CorrectionDirective)
	s.writeField(&first, "ActiveBatchID", &ledger.ActiveBatchID)
	s.writeField(&first, "NextBatchSequence", &ledger.NextBatchSequence)
	s.writeFieldPrefix(&first, "PendingCoverageIDs")
	writeStreamSlice(s, ledger.PendingCoverageIDs)
	s.writeFieldPrefix(&first, "CompletedBatchIDs")
	writeStreamSlice(s, ledger.CompletedBatchIDs)
	s.writeField(&first, "FeasibilityBatchID", &ledger.FeasibilityBatchID)
	s.writeFieldPrefix(&first, "Rejections")
	writeStreamSlice(s, ledger.Rejections)
	s.writeFieldPrefix(&first, "FeasibilityGaps")
	writeStreamSlice(s, ledger.FeasibilityGaps)
	s.writeField(&first, "CatalogRefreshAttempts", &ledger.CatalogRefreshAttempts)
	s.writeField(&first, "Failure", ledger.Failure)
	s.writeString("}")
}

func (s *runtimeStateStreamEncoder) writeSemanticHistoryArchive(archive *RuntimeSemanticHistoryArchive) {
	s.writeString("{")
	first := true
	s.writeField(&first, "ProtocolRef", &archive.ProtocolRef)
	s.writeField(&first, "GenerationCount", &archive.GenerationCount)
	s.writeFieldPrefix(&first, "Segments")
	if archive.Segments == nil {
		s.writeString("null")
	} else if len(archive.Segments) == 0 {
		s.writeString("[]")
	} else {
		s.writeString("[")
		for i := range archive.Segments {
			if i > 0 {
				s.writeString(",")
			}
			writeStreamSlice(s, archive.Segments[i])
		}
		s.writeString("]")
	}
	s.writeString("}")
}
