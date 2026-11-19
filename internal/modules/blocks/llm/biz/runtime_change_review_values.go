package llm

import (
	"fmt"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only exact strings within this request's same-target corpus can be
// shared. Paths and indices address typed slots, never external files or arbitrary JSON.
type runtimeChangeReviewBaselineRef struct {
	Change          int    `json:"change"`
	SourceKind      string `json:"sourceKind"`
	Source          string `json:"source,omitempty"`
	SourceIndex     int    `json:"sourceIndex,omitempty"`
	SourceParameter int    `json:"sourceParameter,omitempty"`
}

type runtimeChangeReviewValueRef struct {
	Change          int    `json:"change"`
	Parameter       int    `json:"parameter"`
	SourceKind      string `json:"sourceKind"`
	Source          string `json:"source,omitempty"`
	SourceIndex     int    `json:"sourceIndex,omitempty"`
	SourceParameter int    `json:"sourceParameter,omitempty"`
}

const changeReviewValueReferenceRule = " changeReviewBaselineRefs replace only empty changes[change].baseline.content and changeReviewValueRefs replace only null operation.Parameters[parameter].String in changes[change]. SourceKind=path names an exact same-request C3/C4 body source; current names current[sourceIndex].content; baseline names changes[sourceIndex].baseline.content (for baseline refs, sourceIndex < change; for value refs, sourceIndex <= change); parameter names an earlier changes[sourceIndex].operation.Parameters[sourceParameter].String (sourceIndex < change). Sources must have the same target. Expand work/file references first, then baseline and value references in ascending change/parameter order; restore complete baselines and operations before checking hashes. No content or historical version edge is omitted."

func changeReviewResolveCorpusSource(c *rc.RuntimeChangeReviewContext, target, expectedHash string, limitChange int, allowSameChangeBaseline bool, sourceKind, sourcePath string, sourceIndex, sourceParameter int, sources []runtimeChangeReviewBodySource) (string, bool) {
	if c == nil || sourceIndex < 0 || sourceParameter < 0 {
		return "", false
	}
	switch sourceKind {
	case "path":
		if sourceIndex != 0 || sourceParameter != 0 || sourcePath == "" {
			return "", false
		}
		f, err := runtimeFileReferenceSource(sourcePath, sources)
		if err != nil || f.TargetRef != target || (expectedHash != "" && f.ContentHash != expectedHash) {
			return "", false
		}
		return f.Content, true
	case "current":
		if sourcePath != "" || sourceIndex >= len(c.Current) || sourceParameter != 0 {
			return "", false
		}
		f := c.Current[sourceIndex]
		if f.TargetRef != target || f.Content == "" || rc.RuntimeValidationInputContentHash(f.Content) != f.ContentHash || (expectedHash != "" && f.ContentHash != expectedHash) {
			return "", false
		}
		return f.Content, true
	case "baseline":
		maxIndex := limitChange - 1
		if allowSameChangeBaseline {
			maxIndex = limitChange
		}
		if sourcePath != "" || sourceIndex > maxIndex || sourceIndex >= len(c.Changes) || sourceParameter != 0 {
			return "", false
		}
		f := c.Changes[sourceIndex].Baseline
		if f == nil || f.TargetRef != target || f.Content == "" || rc.RuntimeValidationInputContentHash(f.Content) != f.ContentHash || (expectedHash != "" && f.ContentHash != expectedHash) {
			return "", false
		}
		return f.Content, true
	case "parameter":
		if sourcePath != "" || sourceIndex >= limitChange || sourceIndex >= len(c.Changes) {
			return "", false
		}
		ch := c.Changes[sourceIndex]
		if ch.TargetRef != target || sourceParameter >= len(ch.Operation.Parameters) {
			return "", false
		}
		p := ch.Operation.Parameters[sourceParameter]
		if p.String == nil || *p.String == "" || (expectedHash != "" && rc.RuntimeValidationInputContentHash(*p.String) != expectedHash) {
			return "", false
		}
		return *p.String, true
	}
	return "", false
}

func compactChangeReviewBaselinesAndValues(c *rc.RuntimeChangeReviewContext, bodySources []runtimeChangeReviewBodySource) ([]runtimeChangeReviewBaselineRef, []runtimeChangeReviewValueRef) {
	if c == nil {
		return nil, nil
	}
	type key struct{ target, body string }
	type canonicalSource struct {
		kind, path       string
		index, parameter int
	}
	// Request-local encoder lookup only; never stored or reused as authority.
	canonical := map[key]canonicalSource{}
	add := func(target, body string, source canonicalSource) {
		if len(body) < 256 {
			return
		}
		k := key{target, body}
		if _, found := canonical[k]; !found {
			canonical[k] = source
		}
	}
	for i, f := range c.Current {
		if rc.RuntimeValidationInputContentHash(f.Content) == f.ContentHash {
			add(f.TargetRef, f.Content, canonicalSource{kind: "current", index: i})
		}
	}
	for _, s := range bodySources {
		if rc.RuntimeValidationInputContentHash(s.file.Content) == s.file.ContentHash {
			add(s.file.TargetRef, s.file.Content, canonicalSource{kind: "path", path: s.ref})
		}
	}
	var baselineRefs []runtimeChangeReviewBaselineRef
	var valueRefs []runtimeChangeReviewValueRef
	for ci := range c.Changes {
		ch := &c.Changes[ci]
		if b := ch.Baseline; b != nil && len(b.Content) >= 256 && b.TargetRef == ch.TargetRef && rc.RuntimeValidationInputContentHash(b.Content) == b.ContentHash {
			k := key{ch.TargetRef, b.Content}
			if s, found := canonical[k]; found {
				baselineRefs = append(baselineRefs, runtimeChangeReviewBaselineRef{
					Change: ci, SourceKind: s.kind, Source: s.path, SourceIndex: s.index, SourceParameter: s.parameter,
				})
				b.Content = ""
			} else {
				canonical[k] = canonicalSource{kind: "baseline", index: ci}
			}
		}
		for pi := range ch.Operation.Parameters {
			p := &ch.Operation.Parameters[pi]
			if p.String == nil || len(*p.String) < 256 || p.Integer != nil || p.Boolean != nil || p.StringList != nil {
				continue
			}
			k := key{ch.TargetRef, *p.String}
			if s, found := canonical[k]; found {
				valueRefs = append(valueRefs, runtimeChangeReviewValueRef{
					Change: ci, Parameter: pi, SourceKind: s.kind, Source: s.path, SourceIndex: s.index, SourceParameter: s.parameter,
				})
				p.String = nil
			} else {
				canonical[k] = canonicalSource{kind: "parameter", index: ci, parameter: pi}
			}
		}
	}
	return baselineRefs, valueRefs
}

func expandChangeReviewBaselinesAndValues(c *rc.RuntimeChangeReviewContext, baselineRefs []runtimeChangeReviewBaselineRef, valueRefs []runtimeChangeReviewValueRef, sources []runtimeChangeReviewBodySource) error {
	if c == nil {
		if len(baselineRefs) != 0 || len(valueRefs) != 0 {
			return fmt.Errorf("change review reference outside request")
		}
		return nil
	}
	bi, vi := 0, 0
	for ci := range c.Changes {
		ch := &c.Changes[ci]
		if bi < len(baselineRefs) && baselineRefs[bi].Change == ci {
			r := baselineRefs[bi]
			if ch.Baseline == nil || ch.Baseline.Content != "" || ch.Baseline.TargetRef != ch.TargetRef {
				return fmt.Errorf("change review baseline reference differs from its exact same-target source")
			}
			body, ok := changeReviewResolveCorpusSource(c, ch.TargetRef, ch.Baseline.ContentHash, ci, false, r.SourceKind, r.Source, r.SourceIndex, r.SourceParameter, sources)
			if !ok {
				return fmt.Errorf("change review baseline reference differs from its exact same-target source")
			}
			ch.Baseline.Content = body
			bi++
		}
		lastParameter := -1
		for vi < len(valueRefs) && valueRefs[vi].Change == ci {
			r := valueRefs[vi]
			if r.Parameter < 0 || r.Parameter >= len(ch.Operation.Parameters) || r.Parameter <= lastParameter {
				return fmt.Errorf("change review value reference is duplicate, unordered or outside request")
			}
			p := &ch.Operation.Parameters[r.Parameter]
			body, ok := changeReviewResolveCorpusSource(c, ch.TargetRef, "", ci, true, r.SourceKind, r.Source, r.SourceIndex, r.SourceParameter, sources)
			if !ok || body == "" || p.String != nil || p.Integer != nil || p.Boolean != nil || p.StringList != nil {
				return fmt.Errorf("change review value reference differs from its exact same-target source")
			}
			value := body
			p.String = &value
			lastParameter = r.Parameter
			vi++
		}
	}
	if bi != len(baselineRefs) || vi != len(valueRefs) {
		return fmt.Errorf("change review value reference is duplicate, unordered or outside request")
	}
	return nil
}
