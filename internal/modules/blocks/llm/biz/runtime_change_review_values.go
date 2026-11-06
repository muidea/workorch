package llm

import (
	"fmt"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only exact strings within this request's same-target mutation corpus can be
// shared. Indices address typed slots, never external files or arbitrary JSON.
type runtimeChangeReviewValueRef struct {
	Change          int    `json:"change"`
	Parameter       int    `json:"parameter"`
	SourceKind      string `json:"sourceKind"`
	SourceIndex     int    `json:"sourceIndex"`
	SourceParameter int    `json:"sourceParameter,omitempty"`
}

const changeReviewValueReferenceRule = " changeReviewValueRefs replace only null operation.Parameters[parameter].String in changes[change]. SourceKind=current names current[sourceIndex].content; baseline names changes[sourceIndex].baseline.content; parameter names an earlier changes[sourceIndex].operation.Parameters[sourceParameter].String. Sources must have the same target. Expand file references first, then value references in change/parameter order; restore complete operations before checking operationHash. No content or historical version edge is omitted."

func changeReviewValueSource(c *rc.RuntimeChangeReviewContext, r runtimeChangeReviewValueRef) (target, value string, ok bool) {
	if c == nil || r.SourceIndex < 0 || r.SourceParameter < 0 {
		return
	}
	switch r.SourceKind {
	case "current":
		if r.SourceIndex < len(c.Current) && r.SourceParameter == 0 {
			f := c.Current[r.SourceIndex]
			return f.TargetRef, f.Content, f.ContentHash == rc.RuntimeValidationInputContentHash(f.Content)
		}
	case "baseline":
		if r.SourceIndex < len(c.Changes) && r.SourceParameter == 0 {
			if f := c.Changes[r.SourceIndex].Baseline; f != nil {
				return f.TargetRef, f.Content, f.ContentHash == rc.RuntimeValidationInputContentHash(f.Content)
			}
		}
	case "parameter":
		if r.SourceIndex < r.Change && r.SourceIndex < len(c.Changes) {
			ch := c.Changes[r.SourceIndex]
			if r.SourceParameter < len(ch.Operation.Parameters) {
				p := ch.Operation.Parameters[r.SourceParameter]
				if p.String != nil {
					return ch.TargetRef, *p.String, true
				}
			}
		}
	}
	return
}

func compactChangeReviewValues(c *rc.RuntimeChangeReviewContext) []runtimeChangeReviewValueRef {
	if c == nil {
		return nil
	}
	type key struct{ target, body string }
	// Request-local encoder lookup only; never stored or reused as authority.
	sources := map[key]runtimeChangeReviewValueRef{}
	add := func(target, body string, source runtimeChangeReviewValueRef) {
		if len(body) < 256 {
			return
		}
		k := key{target, body}
		if _, found := sources[k]; !found {
			sources[k] = source
		}
	}
	for i, f := range c.Current {
		add(f.TargetRef, f.Content, runtimeChangeReviewValueRef{SourceKind: "current", SourceIndex: i})
	}
	for i, ch := range c.Changes {
		if f := ch.Baseline; f != nil {
			add(f.TargetRef, f.Content, runtimeChangeReviewValueRef{SourceKind: "baseline", SourceIndex: i})
		}
	}
	var refs []runtimeChangeReviewValueRef
	for ci := range c.Changes {
		ch := &c.Changes[ci]
		for pi := range ch.Operation.Parameters {
			p := &ch.Operation.Parameters[pi]
			if p.String == nil || len(*p.String) < 256 || p.Integer != nil || p.Boolean != nil || p.StringList != nil {
				continue
			}
			k := key{ch.TargetRef, *p.String}
			if r, found := sources[k]; found && (r.SourceKind != "parameter" || r.SourceIndex < ci) {
				r.Change, r.Parameter = ci, pi
				refs = append(refs, r)
				p.String = nil
			} else if !found {
				sources[k] = runtimeChangeReviewValueRef{SourceKind: "parameter", SourceIndex: ci, SourceParameter: pi}
			}
		}
	}
	return refs
}

func expandChangeReviewValues(c *rc.RuntimeChangeReviewContext, refs []runtimeChangeReviewValueRef) error {
	lastChange, lastParameter := -1, -1
	for _, r := range refs {
		if c == nil || r.Change < 0 || r.Change >= len(c.Changes) || r.Parameter < 0 || r.Parameter >= len(c.Changes[r.Change].Operation.Parameters) || r.Change < lastChange || (r.Change == lastChange && r.Parameter <= lastParameter) {
			return fmt.Errorf("change review value reference is duplicate, unordered or outside request")
		}
		ch := &c.Changes[r.Change]
		p := &ch.Operation.Parameters[r.Parameter]
		target, body, ok := changeReviewValueSource(c, r)
		if !ok || target != ch.TargetRef || body == "" || p.String != nil || p.Integer != nil || p.Boolean != nil || p.StringList != nil {
			return fmt.Errorf("change review value reference differs from its exact same-target source")
		}
		p.String = &body
		lastChange, lastParameter = r.Change, r.Parameter
	}
	return nil
}
