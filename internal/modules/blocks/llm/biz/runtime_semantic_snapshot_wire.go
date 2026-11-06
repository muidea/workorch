package llm

import (
	"bytes"
	"encoding/json"
	"fmt"

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
	ProtocolRef          string
	Snapshots            []runtimeSemanticSnapshotBody
	SnapshotObservations []rc.TargetSnapshot
	ManifestRevision     string
}

type runtimeSemanticWorkWire struct {
	Content     *runtimeSemanticContentWire          `json:"content"`
	PriorRounds []runtimeSemanticWorkRoundProjection `json:"priorRounds"`
	Intent      *rc.RuntimeImplementationIntent      `json:"intent,omitempty"`
}

func (p runtimeSemanticWorkProjection) MarshalJSON() ([]byte, error) {
	w := runtimeSemanticWorkWire{PriorRounds: p.PriorRounds, Intent: p.Intent}
	if p.Content != nil {
		c := p.Content
		w.Content = &runtimeSemanticContentWire{ProtocolRef: c.ProtocolRef, ManifestRevision: c.ManifestRevision}
		if c.Snapshots != nil {
			w.Content.Snapshots = make([]runtimeSemanticSnapshotBody, len(c.Snapshots))
			w.Content.SnapshotObservations = make([]rc.TargetSnapshot, len(c.Snapshots))
		}
		for i, s := range c.Snapshots {
			w.Content.Snapshots[i] = runtimeSemanticSnapshotBody{TargetRef: s.TargetRef, Content: s.Content}
			s.Content = ""
			w.Content.SnapshotObservations[i] = s
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
		if len(c.Snapshots) != len(c.SnapshotObservations) || (c.Snapshots == nil) != (c.SnapshotObservations == nil) {
			return fmt.Errorf("semantic snapshot observations do not match complete body inventory")
		}
		v.Content = &rc.RuntimeImplementationWorkContent{ProtocolRef: c.ProtocolRef, ManifestRevision: c.ManifestRevision}
		if c.Snapshots != nil {
			v.Content.Snapshots = make([]rc.TargetSnapshot, len(c.Snapshots))
		}
		for i, body := range c.Snapshots {
			s := c.SnapshotObservations[i]
			if s.Content != "" || s.TargetRef != body.TargetRef {
				return fmt.Errorf("semantic snapshot observation has mixed content or different target")
			}
			s.Content = body.Content
			v.Content.Snapshots[i] = s
		}
	}
	*p = v
	return nil
}
