package llm

import (
	"fmt"
	"reflect"
	"strings"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const semanticWorkContextRefV5 = "C3.workContext"
const semanticSharedWorkInstructionV5 = " For contextEncoding=semantic-render/shared-work/v5, workContext is the complete C3 work context (C4.workContextRef points here). Each prior-round target contains either its full value or receiptSequence referencing the same dispatch in C4.executionHistory.receipts; expand that receipt to operationId, targetRefs, receiptRef/hash, beforeHash, afterHash and changed. No history, original requirement or current resource content is omitted by these exact local references."

// This wire-only view is lossless. A receiptSequence is a local reference to
// C4.executionHistory.receipts, never an owner receipt or authority substitute.
type runtimeSemanticWorkProjectionV5 struct {
	Content     *rc.RuntimeImplementationWorkContent `json:"content"`
	PriorRounds []runtimeSemanticWorkRoundV5         `json:"priorRounds"`
	Intent      *rc.RuntimeImplementationIntent      `json:"intent,omitempty"`
}

type runtimeSemanticWorkRoundV5 struct {
	Ordinal        int                             `json:"ordinal"`
	DispatchID     string                          `json:"dispatchId"`
	ReceiptSetHash string                          `json:"receiptSetHash"`
	Targets        []runtimeSemanticWorkTargetV5   `json:"targets"`
	Intent         *rc.RuntimeImplementationIntent `json:"intent,omitempty"`
}

type runtimeSemanticWorkTargetV5 struct {
	ReceiptSequence int                           `json:"receiptSequence,omitempty"`
	Value           *rc.RuntimeSemanticWorkTarget `json:"value,omitempty"`
}

func semanticWorkTargetFromReceipt(r rc.RuntimeSemanticReceiptObservation) rc.RuntimeSemanticWorkTarget {
	return rc.RuntimeSemanticWorkTarget{OperationID: r.OperationID, TargetRefs: r.TargetRefs,
		ReceiptRef: r.ReceiptRef, ReceiptHash: r.ReceiptHash, BeforeHash: r.BeforeHash, AfterHash: r.AfterHash, Changed: r.Changed}
}

func compactSemanticWorkContext(c *rc.RuntimeSemanticWorkContext, h rc.RuntimeSemanticExecutionHistory) *runtimeSemanticWorkProjectionV5 {
	if c == nil {
		return nil
	}
	c = c.Clone()
	v := &runtimeSemanticWorkProjectionV5{Content: c.Content, Intent: c.Intent}
	if c.PriorRounds != nil {
		v.PriorRounds = make([]runtimeSemanticWorkRoundV5, len(c.PriorRounds))
	}
	for n, round := range c.PriorRounds {
		r := runtimeSemanticWorkRoundV5{Ordinal: round.Ordinal, DispatchID: round.DispatchID, ReceiptSetHash: round.ReceiptSetHash, Intent: round.Intent}
		if round.Targets != nil {
			r.Targets = make([]runtimeSemanticWorkTargetV5, len(round.Targets))
		}
		for j, target := range round.Targets {
			value := target
			t := runtimeSemanticWorkTargetV5{Value: &value}
			matches, sequence := 0, 0
			for _, receipt := range h.Receipts {
				if receipt.DispatchID == round.DispatchID && reflect.DeepEqual(target, semanticWorkTargetFromReceipt(receipt)) {
					matches++
					sequence = receipt.ReceiptSequence
				}
			}
			if matches == 1 && sequence > 0 {
				t = runtimeSemanticWorkTargetV5{ReceiptSequence: sequence}
			}
			r.Targets[j] = t
		}
		v.PriorRounds[n] = r
	}
	return v
}

func expandSemanticWorkContext(v *runtimeSemanticWorkProjectionV5, h rc.RuntimeSemanticExecutionHistory) (*rc.RuntimeSemanticWorkContext, error) {
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
	return strings.ReplaceAll(runtimeSemanticOutputInstruction(c, m), "C4 workContext", "C3 workContext") + semanticSharedWorkInstructionV5
}
