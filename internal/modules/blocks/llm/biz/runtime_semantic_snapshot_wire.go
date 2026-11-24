package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Keep all snapshot values and their original order. Observation metadata is
// placed after the complete body inventory, so a fresh observation of unchanged
// bytes does not interrupt reuse of the following file bodies.
type runtimeSemanticSnapshotBody struct {
	TargetRef string
	Content   string
}

type runtimeSemanticContentWire struct {
	ProtocolRef                string
	Snapshots                  []runtimeSemanticSnapshotBody
	ReviewSnapshots            []runtimeSemanticSnapshotBody `json:"reviewSnapshots,omitempty"`
	SnapshotObservations       []rc.TargetSnapshot
	ReviewSnapshotObservations []rc.TargetSnapshot `json:"reviewSnapshotObservations,omitempty"`
	ExcludedTargetRefs         []string            `json:"excludedTargetRefs,omitempty"`
	ManifestRevision           string
}

type runtimeSemanticWorkWire struct {
	Content     *runtimeSemanticContentWire          `json:"content"`
	PriorRounds []runtimeSemanticWorkRoundProjection `json:"priorRounds"`
	Intent      *rc.RuntimeImplementationIntent      `json:"intent,omitempty"`
}

func splitSemanticSnapshotWire(items []rc.TargetSnapshot) ([]runtimeSemanticSnapshotBody, []rc.TargetSnapshot) {
	if items == nil {
		return nil, nil
	}
	bodies := make([]runtimeSemanticSnapshotBody, len(items))
	obs := make([]rc.TargetSnapshot, len(items))
	for i, s := range items {
		bodies[i] = runtimeSemanticSnapshotBody{TargetRef: s.TargetRef, Content: s.Content}
		s.Content = ""
		obs[i] = s
	}
	return bodies, obs
}

func restoreSemanticSnapshotWire(bodies []runtimeSemanticSnapshotBody, observations []rc.TargetSnapshot) ([]rc.TargetSnapshot, error) {
	if len(bodies) != len(observations) || (bodies == nil) != (observations == nil) {
		return nil, fmt.Errorf("semantic snapshot observations do not match complete body inventory")
	}
	if bodies == nil {
		return nil, nil
	}
	restored := make([]rc.TargetSnapshot, len(bodies))
	for i, body := range bodies {
		s := observations[i]
		if s.Content != "" || s.TargetRef != body.TargetRef {
			return nil, fmt.Errorf("semantic snapshot observation has mixed content or different target")
		}
		s.Content = body.Content
		restored[i] = s
	}
	return restored, nil
}

func (p runtimeSemanticWorkProjection) MarshalJSON() ([]byte, error) {
	w := runtimeSemanticWorkWire{PriorRounds: p.PriorRounds, Intent: p.Intent}
	if p.Content != nil {
		c := p.Content
		w.Content = &runtimeSemanticContentWire{
			ProtocolRef:        c.ProtocolRef,
			ExcludedTargetRefs: slices.Clone(c.ExcludedTargetRefs),
			ManifestRevision:   c.ManifestRevision,
		}
		if len(w.Content.ExcludedTargetRefs) == 0 {
			w.Content.ExcludedTargetRefs = nil
		}
		w.Content.Snapshots, w.Content.SnapshotObservations = splitSemanticSnapshotWire(c.Snapshots)
		if len(c.ReviewSnapshots) > 0 {
			w.Content.ReviewSnapshots, w.Content.ReviewSnapshotObservations = splitSemanticSnapshotWire(c.ReviewSnapshots)
		}
	}
	return json.Marshal(w)
}

func (p *runtimeSemanticWorkProjection) UnmarshalJSON(raw []byte) error {
	var w runtimeSemanticWorkWire
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&w); err != nil {
		return err
	}
	v := runtimeSemanticWorkProjection{PriorRounds: w.PriorRounds, Intent: w.Intent}
	if w.Content != nil {
		c := w.Content
		if c.ReviewSnapshots != nil && len(c.ReviewSnapshots) == 0 {
			return fmt.Errorf("semantic review snapshots must be omitted when empty")
		}
		snapshots, err := restoreSemanticSnapshotWire(c.Snapshots, c.SnapshotObservations)
		if err != nil {
			return err
		}
		reviewSnapshots, err := restoreSemanticSnapshotWire(c.ReviewSnapshots, c.ReviewSnapshotObservations)
		if err != nil {
			return err
		}
		v.Content = &rc.RuntimeImplementationWorkContent{
			ProtocolRef:        c.ProtocolRef,
			ExcludedTargetRefs: slices.Clone(c.ExcludedTargetRefs),
			ManifestRevision:   c.ManifestRevision,
			Snapshots:          snapshots,
			ReviewSnapshots:    reviewSnapshots,
		}
		if len(v.Content.ExcludedTargetRefs) == 0 {
			v.Content.ExcludedTargetRefs = nil
		}
	}
	*p = v
	return nil
}
