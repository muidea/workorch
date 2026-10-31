package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

const RuntimeImplementationWorkContentV1 = "implementation-work-content/v1"
const RuntimeImplementationWorkContentMaxBytes = 128 * 1024

// These are exact, detached read-only inputs, sealed with the scope observation.
// They never become receipt, readiness, evidence, or completion authority.
type RuntimeImplementationWorkContent struct {
	ProtocolRef      string
	ManifestRevision string
	Snapshots        []TargetSnapshot
}

func (c *RuntimeImplementationWorkContent) Clone() *RuntimeImplementationWorkContent {
	if c == nil {
		return nil
	}
	v := *c
	v.Snapshots = slices.Clone(c.Snapshots)
	return &v
}
func (c RuntimeImplementationWorkContent) Hash() string {
	return implementationWorkRawHash("implementation-work-content:", c)
}
func (c RuntimeImplementationWorkContent) Validate(scope RuntimeImplementationWorkScopeObservation) error {
	if c.ProtocolRef != RuntimeImplementationWorkContentV1 || c.ManifestRevision != scope.Selection.ObservedRevision || c.ManifestRevision == "" || len(c.Snapshots) != len(scope.Selection.TargetRefs) {
		return fmt.Errorf("implementation work content requires exact complete scope revision and targets")
	}
	total := 0
	for i, s := range c.Snapshots {
		h := sha256.Sum256([]byte(s.Content))
		total += len(s.Content)
		if s.TargetRef != scope.Selection.TargetRefs[i] || !s.Complete() || s.Size != int64(len(s.Content)) || s.ContentHash != hex.EncodeToString(h[:]) || !utf8.ValidString(s.Content) {
			return fmt.Errorf("implementation work content missing, altered or incomplete for target %q", s.TargetRef)
		}
	}
	if total > RuntimeImplementationWorkContentMaxBytes {
		return fmt.Errorf("implementation work content exceeds explicit byte capacity")
	}
	return nil
}

// A minimal semantic explanation binds a purpose proposal to its exact original
// obligation and read-only input. Code validates identity, never its sufficiency.
type RuntimeImplementationIntent struct {
	DeclarationRef string `json:"declarationRef"`
	ContentHash    string `json:"contentHash"`
	RemainingWork  string `json:"remainingWork"`
	Reason         string `json:"reason"`
}

func (i *RuntimeImplementationIntent) Clone() *RuntimeImplementationIntent {
	if i == nil {
		return nil
	}
	v := *i
	return &v
}
func validateRuntimeImplementationIntent(i *RuntimeImplementationIntent, w *RuntimeSemanticImplementationWork, declaration string) error {
	if w == nil || !w.PurposeChoiceOnly || w.Scope.Content == nil {
		if i != nil {
			return fmt.Errorf("implementation intent outside content-bound purpose request")
		}
		return nil
	}
	if i == nil || i.DeclarationRef != declaration || i.ContentHash != w.Scope.Content.Hash() || strings.TrimSpace(i.RemainingWork) == "" || strings.TrimSpace(i.Reason) == "" || len(i.RemainingWork) > 2048 || len(i.Reason) > 2048 {
		return fmt.Errorf("implementation purpose requires exact obligation/content and bounded remaining-work explanation")
	}
	return nil
}

// Work context is a projection, not another completion ledger. PriorRounds is
// complete for this CWI (bounded by its original policy), unlike the 16-receipt
// general history suffix. Original goals/criteria remain in C1/C3.
type RuntimeSemanticWorkContext struct {
	Content     *RuntimeImplementationWorkContent `json:"content"`
	PriorRounds []RuntimeSemanticWorkRound        `json:"priorRounds"`
	Intent      *RuntimeImplementationIntent      `json:"intent,omitempty"`
}
type RuntimeSemanticWorkRound struct {
	Ordinal        int                          `json:"ordinal"`
	DispatchID     string                       `json:"dispatchId"`
	ReceiptSetHash string                       `json:"receiptSetHash"`
	Targets        []RuntimeSemanticWorkTarget  `json:"targets"`
	Intent         *RuntimeImplementationIntent `json:"intent,omitempty"`
}
type RuntimeSemanticWorkTarget struct {
	OperationID string   `json:"operationId"`
	TargetRefs  []string `json:"targetRefs"`
	ReceiptRef  string   `json:"receiptRef"`
	ReceiptHash string   `json:"receiptHash"`
	BeforeHash  string   `json:"beforeHash"`
	AfterHash   string   `json:"afterHash"`
	Changed     bool     `json:"changed"`
}

func (c *RuntimeSemanticWorkContext) Clone() *RuntimeSemanticWorkContext {
	if c == nil {
		return nil
	}
	v := *c
	v.Content = c.Content.Clone()
	v.Intent = c.Intent.Clone()
	v.PriorRounds = slices.Clone(c.PriorRounds)
	for i := range v.PriorRounds {
		v.PriorRounds[i].Intent = c.PriorRounds[i].Intent.Clone()
		v.PriorRounds[i].Targets = slices.Clone(c.PriorRounds[i].Targets)
		for j := range v.PriorRounds[i].Targets {
			v.PriorRounds[i].Targets[j].TargetRefs = slices.Clone(c.PriorRounds[i].Targets[j].TargetRefs)
		}
	}
	return &v
}
func (c *RuntimeSemanticWorkContext) Validate(m RuntimeSemanticPlanManifest) error {
	var work *RuntimeSemanticImplementationWork
	for _, o := range m.ProofObligations {
		if o.ImplementationWork != nil && o.ImplementationWork.Scope.Content != nil {
			if work != nil {
				return fmt.Errorf("ambiguous semantic work context")
			}
			work = o.ImplementationWork
		}
	}
	if work == nil {
		if c != nil {
			return fmt.Errorf("unexpected semantic work content")
		}
		return nil
	}
	if c == nil || !reflect.DeepEqual(c.Content, work.Scope.Content) || len(c.PriorRounds) != work.Ordinal-1 {
		return fmt.Errorf("semantic work context missing exact source or complete prior rounds")
	}
	for i, r := range c.PriorRounds {
		if r.Ordinal != i+1 || r.DispatchID == "" || r.ReceiptSetHash == "" || len(r.Targets) == 0 || len(r.Targets) > work.Scope.Request.Policy.MaxTargetsPerRound {
			return fmt.Errorf("semantic work context has incomplete original rounds")
		}
		for _, t := range r.Targets {
			if t.OperationID == "" || len(t.TargetRefs) == 0 || t.ReceiptRef == "" || t.ReceiptHash == "" {
				return fmt.Errorf("semantic work context lacks trusted target receipt")
			}
		}
	}
	return nil
}
