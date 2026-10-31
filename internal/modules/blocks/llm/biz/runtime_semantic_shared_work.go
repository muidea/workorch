package llm

import (
	"fmt"
	"reflect"
	"strings"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const semanticWorkContextRef = "C3.workContext"
const semanticSharedWorkInstruction = " For contextEncoding=semantic/shared-work, workContext is the complete C3 work context (C4.workContextRef points here). Each prior-round target contains either its full value or receiptSequence referencing the same dispatch in C4.executionHistory.receipts; expand that receipt to operationId, targetRefs, receiptRef/hash, beforeHash, afterHash and changed. No history, original requirement or current resource content is omitted by these exact local references."

// This wire-only view is lossless. A receiptSequence is a local reference to
// C4.executionHistory.receipts, never an owner receipt or authority substitute.
type runtimeSemanticWorkProjection struct {
	Content     *rc.RuntimeImplementationWorkContent `json:"content"`
	PriorRounds []runtimeSemanticWorkRoundProjection `json:"priorRounds"`
	Intent      *rc.RuntimeImplementationIntent      `json:"intent,omitempty"`
}

type runtimeSemanticWorkRoundProjection struct {
	Ordinal        int                                   `json:"ordinal"`
	DispatchID     string                                `json:"dispatchId"`
	ReceiptSetHash string                                `json:"receiptSetHash"`
	Targets        []runtimeSemanticWorkTargetProjection `json:"targets"`
	Intent         *rc.RuntimeImplementationIntent       `json:"intent,omitempty"`
}

type runtimeSemanticWorkTargetProjection struct {
	ReceiptSequence int                           `json:"receiptSequence,omitempty"`
	Value           *rc.RuntimeSemanticWorkTarget `json:"value,omitempty"`
}

func semanticWorkTargetFromReceipt(r rc.RuntimeSemanticReceiptObservation) rc.RuntimeSemanticWorkTarget {
	return rc.RuntimeSemanticWorkTarget{OperationID: r.OperationID, TargetRefs: r.TargetRefs,
		ReceiptRef: r.ReceiptRef, ReceiptHash: r.ReceiptHash, BeforeHash: r.BeforeHash, AfterHash: r.AfterHash, Changed: r.Changed}
}

func compactSemanticWorkContext(c *rc.RuntimeSemanticWorkContext, h rc.RuntimeSemanticExecutionHistory) *runtimeSemanticWorkProjection {
	if c == nil {
		return nil
	}
	c = c.Clone()
	v := &runtimeSemanticWorkProjection{Content: c.Content, Intent: c.Intent}
	if c.PriorRounds != nil {
		v.PriorRounds = make([]runtimeSemanticWorkRoundProjection, len(c.PriorRounds))
	}
	for n, round := range c.PriorRounds {
		r := runtimeSemanticWorkRoundProjection{Ordinal: round.Ordinal, DispatchID: round.DispatchID, ReceiptSetHash: round.ReceiptSetHash, Intent: round.Intent}
		if round.Targets != nil {
			r.Targets = make([]runtimeSemanticWorkTargetProjection, len(round.Targets))
		}
		for j, target := range round.Targets {
			value := target
			t := runtimeSemanticWorkTargetProjection{Value: &value}
			matches, sequence := 0, 0
			for _, receipt := range h.Receipts {
				if receipt.DispatchID == round.DispatchID && reflect.DeepEqual(target, semanticWorkTargetFromReceipt(receipt)) {
					matches++
					sequence = receipt.ReceiptSequence
				}
			}
			if matches == 1 && sequence > 0 {
				t = runtimeSemanticWorkTargetProjection{ReceiptSequence: sequence}
			}
			r.Targets[j] = t
		}
		v.PriorRounds[n] = r
	}
	return v
}

func expandSemanticWorkContext(v *runtimeSemanticWorkProjection, h rc.RuntimeSemanticExecutionHistory) (*rc.RuntimeSemanticWorkContext, error) {
	if v == nil {
		return nil, nil
	}
	c := &rc.RuntimeSemanticWorkContext{Content: v.Content.Clone(), Intent: v.Intent.Clone()}
	if v.PriorRounds != nil {
		c.PriorRounds = make([]rc.RuntimeSemanticWorkRound, len(v.PriorRounds))
	}
	for n, round := range v.PriorRounds {
		r := rc.RuntimeSemanticWorkRound{Ordinal: round.Ordinal, DispatchID: round.DispatchID, ReceiptSetHash: round.ReceiptSetHash, Intent: round.Intent.Clone()}
		if round.Targets != nil {
			r.Targets = make([]rc.RuntimeSemanticWorkTarget, len(round.Targets))
		}
		for j, target := range round.Targets {
			if target.ReceiptSequence == 0 && target.Value != nil {
				r.Targets[j] = *target.Value
				continue
			}
			if target.ReceiptSequence <= 0 || target.Value != nil {
				return nil, fmt.Errorf("semantic work target requires exactly one inline value or local receipt reference")
			}
			matches := 0
			for _, receipt := range h.Receipts {
				if receipt.ReceiptSequence == target.ReceiptSequence && receipt.DispatchID == round.DispatchID {
					matches++
					r.Targets[j] = semanticWorkTargetFromReceipt(receipt)
				}
			}
			if matches != 1 {
				return nil, fmt.Errorf("semantic work local receipt reference is missing, ambiguous or outside the original dispatch")
			}
		}
		c.PriorRounds[n] = r
	}
	return c.Clone(), nil
}

func runtimeSemanticSharedWorkOutputInstruction(c rc.RuntimeSemanticPlanRequestContext, m rc.RuntimeSemanticPlanManifest) string {
	if c.WorkContext == nil {
		return runtimeSemanticOutputInstruction(c, m)
	}
	return strings.ReplaceAll(runtimeSemanticOutputInstruction(c, m), "C4 workContext", "C3 workContext") + semanticSharedWorkInstruction
}
