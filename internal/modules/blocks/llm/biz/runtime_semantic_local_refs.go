package llm

import (
	"encoding/json"
	"fmt"
	"reflect"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These aliases have a closed, typed address space inside one C3. They never
// resolve a mutable file, another request, an external hash or an owner store.
type runtimeSemanticFileRef struct {
	Work     int `json:"work"` // -1: implementationPurpose; otherwise proofObligations index
	File     int `json:"file"`
	Snapshot int `json:"snapshot"`
}
type runtimeSemanticIntentRef struct {
	Work  int `json:"work"`
	Round int `json:"round"`
}
type runtimeSemanticAuthorityInline runtimeSemanticAuthorityProjection
type runtimeSemanticAuthorityWire struct {
	runtimeSemanticAuthorityInline
	FileRefs       []runtimeSemanticFileRef           `json:"fileRefs,omitempty"`
	IntentRefs     []runtimeSemanticIntentRef         `json:"intentRefs,omitempty"`
	GenerationRefs []runtimeSemanticGenerationFileRef `json:"generationRefs,omitempty"`
	ReferenceRule  string                             `json:"referenceRule,omitempty"`
}

const semanticLocalReferenceRule = "fileRefs replace only empty inputRevision.Binding.InputReview.Files[file].content with exact workContext.content.Snapshots[snapshot].Content (matching target and hash). intentRefs replace only null previousRound.intentBinding.intent with exact workContext.priorRounds[round].intent (matching ordinal, dispatch and receipt set). work=-1 names manifest.implementationPurpose; other work indexes name manifest.proofObligations[work].implementationWork. generationRefs replace only empty validationGenerationContexts[generation].context.Files[file].content from exactly one local snapshot, sourceReview=[proof,review,file], or earlier sourceGeneration/sourceFile with matching target/hash. Expand these same-C3 aliases before review; all original conflict reports, before-content, criteria and lineage remain mandatory."

func semanticLocalWork(p *runtimeSemanticAuthorityProjection, index int) *runtimeSemanticImplementationWorkProjection {
	if index == -1 {
		return p.Manifest.ImplementationPurpose
	}
	if index < 0 || index >= len(p.Manifest.ProofObligations) {
		return nil
	}
	return p.Manifest.ProofObligations[index].ImplementationWork
}
func semanticLocalWorkIndexes(p *runtimeSemanticAuthorityProjection) []int {
	ret := []int{-1}
	for i := range p.Manifest.ProofObligations {
		ret = append(ret, i)
	}
	return ret
}
func (p runtimeSemanticAuthorityProjection) MarshalJSON() ([]byte, error) {
	// Detach all nested values. No provider projection may alter its manifest,
	// original rejected review or the complete trusted prior-round ledger.
	raw, err := json.Marshal(runtimeSemanticAuthorityInline(p))
	if err != nil {
		return nil, err
	}
	var clone runtimeSemanticAuthorityInline
	if err = json.Unmarshal(raw, &clone); err != nil {
		return nil, err
	}
	p = runtimeSemanticAuthorityProjection(clone)
	wire := runtimeSemanticAuthorityWire{}
	for _, wi := range semanticLocalWorkIndexes(&p) {
		w := semanticLocalWork(&p, wi)
		if w == nil || p.WorkContext == nil {
			continue
		}
		if v := w.InputRevision; v != nil && v.Binding.InputReview != nil && p.WorkContext.Content != nil {
			for fi := range v.Binding.InputReview.Files {
				f := &v.Binding.InputReview.Files[fi]
				if len(f.Content) < 256 {
					continue
				}
				for si, s := range p.WorkContext.Content.Snapshots {
					if f.TargetRef == s.TargetRef && f.ContentHash == s.ContentHash && f.Content == s.Content {
						f.Content = ""
						wire.FileRefs = append(wire.FileRefs, runtimeSemanticFileRef{wi, fi, si})
						break
					}
				}
			}
		}
		if prev := w.PreviousRound; prev != nil && prev.IntentBinding != nil && prev.IntentBinding.Intent != nil {
			for ri, r := range p.WorkContext.PriorRounds {
				if prev.Ordinal == r.Ordinal && prev.DispatchID == r.DispatchID && prev.ReceiptSetHash == r.ReceiptSetHash && reflect.DeepEqual(prev.IntentBinding.Intent, r.Intent) {
					prev.IntentBinding.Intent = nil
					wire.IntentRefs = append(wire.IntentRefs, runtimeSemanticIntentRef{wi, ri})
					break
				}
			}
		}
	}
	wire.GenerationRefs = compactSemanticGenerationFiles(&p)
	wire.runtimeSemanticAuthorityInline = runtimeSemanticAuthorityInline(p)
	if len(wire.FileRefs)+len(wire.IntentRefs)+len(wire.GenerationRefs) > 0 {
		wire.ReferenceRule = semanticLocalReferenceRule
	}
	return json.Marshal(wire)
}
func (p *runtimeSemanticAuthorityProjection) UnmarshalJSON(raw []byte) error {
	var wire runtimeSemanticAuthorityWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	value := runtimeSemanticAuthorityProjection(wire.runtimeSemanticAuthorityInline)
	if len(wire.FileRefs)+len(wire.IntentRefs) > 0 && (wire.ReferenceRule != semanticLocalReferenceRule || value.WorkContext == nil) {
		return fmt.Errorf("semantic local references require exact rule and current work context")
	}
	for _, ref := range wire.FileRefs {
		w := semanticLocalWork(&value, ref.Work)
		if w == nil || w.InputRevision == nil || w.InputRevision.Binding.InputReview == nil || value.WorkContext.Content == nil {
			return fmt.Errorf("semantic file reference has no original input")
		}
		files := w.InputRevision.Binding.InputReview.Files
		snapshots := value.WorkContext.Content.Snapshots
		if ref.File < 0 || ref.File >= len(files) || ref.Snapshot < 0 || ref.Snapshot >= len(snapshots) {
			return fmt.Errorf("semantic file reference is outside local scope")
		}
		f, s := &files[ref.File], snapshots[ref.Snapshot]
		if f.Content != "" || s.Content == "" || f.TargetRef != s.TargetRef || f.ContentHash != s.ContentHash {
			return fmt.Errorf("semantic file reference differs from original target/hash or mixes content")
		}
		f.Content = s.Content
	}
	for _, ref := range wire.IntentRefs {
		w := semanticLocalWork(&value, ref.Work)
		if w == nil || w.PreviousRound == nil || w.PreviousRound.IntentBinding == nil || ref.Round < 0 || ref.Round >= len(value.WorkContext.PriorRounds) {
			return fmt.Errorf("semantic intent reference outside original round")
		}
		prev, r := w.PreviousRound, value.WorkContext.PriorRounds[ref.Round]
		if prev.IntentBinding.Intent != nil || r.Intent == nil || prev.Ordinal != r.Ordinal || prev.DispatchID != r.DispatchID || prev.ReceiptSetHash != r.ReceiptSetHash {
			return fmt.Errorf("semantic intent reference has different lineage or mixed value")
		}
		prev.IntentBinding.Intent = r.Intent.Clone()
	}
	if len(wire.GenerationRefs) > 0 && wire.ReferenceRule != semanticLocalReferenceRule {
		return fmt.Errorf("generation references require exact local rule")
	}
	if err := expandSemanticGenerationFiles(&value, wire.GenerationRefs); err != nil {
		return err
	}
	*p = value
	return nil
}

// Ordinary ordering has no authority to inspect or propose implementation.
// Its selected coverage/relations remain in the manifest; keep the full owner
// context intact and narrow only the provider-visible projection.
func runtimeSemanticVisibleWorkContext(c rc.RuntimeSemanticPlanRequestContext, m rc.RuntimeSemanticPlanManifest) *rc.RuntimeSemanticWorkContext {
	if c.PlanStep == rc.RuntimeSemanticPlanStepCoverageOrder && !rc.RuntimeSemanticPurposeChoice(m, c.PlanStep) {
		return nil
	}
	return c.WorkContext.Clone()
}
