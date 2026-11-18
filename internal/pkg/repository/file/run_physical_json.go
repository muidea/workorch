package file

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memorycontract "github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

//go:generate go run ../../../../scripts/generate-run-physical-json.go

func marshalRunPhysicalSingle(v *run.Run, frozen bool) ([]byte, error) {
	var e runPhysicalJSONEncoder
	if frozen {
		e.frozen = v
	}
	e.run(v)
	if e.err != nil {
		return nil, e.err
	}
	return e.out.Bytes(), nil
}

func estimateRunPhysicalJSONSize(written *run.Run) int {
	if written == nil || written.RuntimeState == nil {
		return 4096
	}
	s := written.RuntimeState
	return len(s.CompletedDispatches)*420_000 +
		s.SemanticPlanning.HistoryCount()*1_100_000 +
		len(s.CandidateRequestSessions)*45_000 +
		len(s.Facts.EvidenceApplicabilityObservations)*6_000 +
		2_097_152
}

func marshalRunInventoryPhysicalWrite(images runInventoryPhysicalMap, written *run.Run) ([]byte, error) {
	keys := make([]string, 0, len(images))
	estimatedSize := 64
	for key, img := range images {
		keys = append(keys, string(key))
		estimatedSize += len(key) + 16 + len(img.raw)
		if key == written.ID {
			if len(img.raw) == 0 {
				estimatedSize += estimateRunPhysicalJSONSize(written)
			} else {
				estimatedSize += 1572864
			}
		}
	}
	sort.Strings(keys)
	prevImg, hasPrevImg := images[written.ID]
	e := runPhysicalJSONEncoder{frozen: written}
	if hasPrevImg && prevImg.aggregate != nil && prevImg.aggregate.RuntimeState != nil &&
		len(prevImg.stateRaw) != 0 && prevImg.stateSpans != nil {
		e.frozenPrevState = prevImg.aggregate.RuntimeState
		e.frozenPrevStateRaw = prevImg.stateRaw
		e.frozenPrevStateSpans = prevImg.stateSpans
	}
	if hasPrevImg && prevImg.aggregate != nil && prevImg.aggregate.RuntimeBootstrap != nil &&
		len(prevImg.bootstrapRaw) != 0 {
		e.frozenPrevBootstrap = prevImg.aggregate.RuntimeBootstrap
		e.frozenPrevBootstrapRaw = prevImg.bootstrapRaw
	}
	if hasPrevImg && cap(prevImg.prevAlloc) >= estimatedSize {
		e.out = *bytes.NewBuffer(prevImg.prevAlloc[:0])
	} else {
		targetCap := estimatedSize + 8*1024*1024
		if hasPrevImg && cap(prevImg.alloc) > targetCap {
			targetCap = cap(prevImg.alloc)
		}
		e.out = *bytes.NewBuffer(make([]byte, 0, targetCap))
	}
	writtenStart, writtenEnd := -1, -1
	e.text("{")
	for i, key := range keys {
		if i != 0 {
			e.text(",")
		}
		e.value(key)
		e.text(":")
		if e.err != nil {
			return nil, e.err
		}
		id := common.RunID(key)
		if id == written.ID {
			writtenStart = e.out.Len()
			e.run(written)
			if e.err != nil {
				return nil, e.err
			}
			writtenEnd = e.out.Len()
			continue
		}
		img := images[id]
		if img.rawCanonical && len(img.raw) != 0 {
			e.out.Write(img.raw)
			continue
		}
		if img.aggregate != nil {
			raw, err := marshalRunPhysicalSingle(img.aggregate, false)
			if err != nil {
				return nil, err
			}
			img.raw = raw
			img.rawCanonical = true
			images[id] = img
			e.out.Write(raw)
			continue
		}
		if len(img.raw) == 0 {
			return nil, fmt.Errorf("run %q physical image is empty", key)
		}
		var compacted bytes.Buffer
		if err := json.Compact(&compacted, img.raw); err != nil {
			return nil, err
		}
		img.raw = compacted.Bytes()
		img.rawCanonical = true
		images[id] = img
		e.out.Write(img.raw)
	}
	e.text("}")
	if e.err != nil {
		return nil, e.err
	}
	body := e.out.Bytes()
	if writtenStart >= 0 {
		writtenImg := images[written.ID]
		writtenImg.raw = body[writtenStart:writtenEnd]
		writtenImg.rawCanonical = true
		writtenImg.alloc = body
		writtenImg.prevAlloc = nil
		if cap(prevImg.alloc) > 0 {
			allocHead := prevImg.alloc[:1]
			if len(body) == 0 || &allocHead[0] != &body[0] {
				writtenImg.prevAlloc = prevImg.alloc[:0]
			}
		}
		if e.frozenBootstrapEnd > e.frozenBootstrapStart {
			writtenImg.bootstrapRaw = body[e.frozenBootstrapStart:e.frozenBootstrapEnd]
		} else {
			writtenImg.bootstrapRaw = nil
		}
		if e.frozenStateEnd > e.frozenStateStart {
			writtenImg.stateRaw = body[e.frozenStateStart:e.frozenStateEnd]
			writtenImg.stateSpans = e.frozenStateSpans
		} else {
			writtenImg.stateRaw = nil
			writtenImg.stateSpans = nil
		}
		writtenImg.metadata = *written
		writtenImg.metadata.RuntimeState = nil
		writtenImg.metadata.RuntimeBootstrap = nil
		writtenImg.metadata.RetiredReplacementIntents = nil
		writtenImg.metadata.ExecutionIntentReceipts = nil
		if written.ReplacementIntent != nil {
			writtenImg.metadata.ReplacementIntent = &run.ReplacementIntent{
				State:    written.ReplacementIntent.State,
				Recovery: run.ReplacementRecoveryLease{State: written.ReplacementIntent.Recovery.State},
			}
		}
		if written.RuntimeBootstrap != nil {
			writtenImg.hasBootstrap = true
			writtenImg.memorySelection = memorycontract.CloneRecallSelectionEvidence(written.RuntimeBootstrap.Activation.MemorySelection)
			selection := written.RuntimeBootstrap.Activation.ProviderSelection.Normalized()
			if selection.Validate() == nil {
				writtenImg.providerSelection = &selection
			} else {
				writtenImg.providerSelection = nil
			}
		} else {
			writtenImg.hasBootstrap = false
			writtenImg.memorySelection = nil
			writtenImg.providerSelection = nil
		}
		if written.RuntimeState != nil {
			writtenImg.hasState = true
			writtenImg.stateRevision = written.RuntimeState.StateRevision
		} else {
			writtenImg.hasState = false
			writtenImg.stateRevision = 0
		}
		writtenImg.aggregate = written
		images[written.ID] = writtenImg
	}
	return body, nil
}

// marshalRunRuntimeWrite keeps the physical representation identical to
// json.Marshal(items). Only the isolated state encoded and hashed in this CAS
// is inserted directly; other Runs retain their complete original values.
// There are no persistent references, semantic caches or cross-call buffers.
func marshalRunRuntimeWrite(items map[common.RunID]run.Run, written *run.Run) ([]byte, error) {
	e := runPhysicalJSONEncoder{frozen: written}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	e.text("{")
	for i, key := range keys {
		if i != 0 {
			e.text(",")
		}
		e.value(key)
		e.text(":")
		if common.RunID(key) == written.ID {
			e.run(written)
		} else {
			other := items[common.RunID(key)]
			e.run(&other)
		}
	}
	e.text("}")
	if e.err != nil {
		return nil, e.err
	}
	return e.out.Bytes(), nil
}

type runPhysicalJSONEncoder struct {
	out                    bytes.Buffer
	err                    error
	frozen                 *run.Run
	frozenPrevBootstrap    *rc.RuntimeBootstrap
	frozenPrevBootstrapRaw []byte
	frozenBootstrapStart   int
	frozenBootstrapEnd     int
	frozenPrevState        *rc.RuntimeStateRecord
	frozenPrevStateRaw     []byte
	frozenPrevStateSpans   *rc.RuntimeStateJSONSpans
	frozenStateStart       int
	frozenStateEnd         int
	frozenStateSpans       *rc.RuntimeStateJSONSpans
}

func (e *runPhysicalJSONEncoder) text(s string) {
	if e.err == nil {
		e.out.WriteString(s)
	}
}
func (e *runPhysicalJSONEncoder) value(v any) {
	if e.err != nil {
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		e.err = err
		return
	}
	e.out.Write(body)
}
func (e *runPhysicalJSONEncoder) bootstrap(v *run.Run) {
	if e.err != nil {
		return
	}
	if v == e.frozen && v.RuntimeBootstrap != nil {
		start := e.out.Len()
		if e.frozenPrevBootstrap != nil && len(e.frozenPrevBootstrapRaw) > 0 &&
			rc.RuntimeRawEqual(*v.RuntimeBootstrap, *e.frozenPrevBootstrap) {
			e.out.Write(e.frozenPrevBootstrapRaw)
		} else {
			e.value(v.RuntimeBootstrap)
		}
		e.frozenBootstrapStart, e.frozenBootstrapEnd = start, e.out.Len()
		return
	}
	e.value(v.RuntimeBootstrap)
}
func (e *runPhysicalJSONEncoder) state(v *run.Run) {
	if e.err != nil {
		return
	}
	if v == e.frozen {
		if v.RuntimeState != nil {
			stateStart := e.out.Len()
			v.RuntimeStateHash, e.frozenStateSpans, e.err = run.AppendRuntimeStateWriteWithPreimage(
				&e.out, v, e.frozenPrevState, e.frozenPrevStateRaw, e.frozenPrevStateSpans,
			)
			e.frozenStateStart, e.frozenStateEnd = stateStart, e.out.Len()
			return
		}
		v.RuntimeStateHash, e.err = run.AppendRuntimeStateWrite(&e.out, v)
		return
	}
	e.value(v.RuntimeState)
}
