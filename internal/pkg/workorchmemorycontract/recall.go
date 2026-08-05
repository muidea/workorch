// Package workorchmemorycontract contains immutable, owner-neutral memory
// recall values shared by the Memory owner, EventHub contracts and read
// projections. It owns no topic, command, repository or selection behavior.
package workorchmemorycontract

import "github.com/muidea/workorch/internal/pkg/common"

// RecallHit records the bounded facts for one selected memory item.
type RecallHit struct {
	ID              common.MemoryID `json:"id"`
	Summary         string          `json:"summary,omitempty"`
	VectorScore     float64         `json:"vectorScore,omitempty"`
	VectorDimension int             `json:"vectorDimension,omitempty"`
}

// RecallSelectionScore records the already-computed score for one candidate.
type RecallSelectionScore struct {
	ID     common.MemoryID `json:"id"`
	Score  int             `json:"score"`
	Reason string          `json:"reason,omitempty"`
}

// TypeQuotaEntry is the map-free representation of one recall type quota.
type TypeQuotaEntry struct {
	Type  string `json:"type"`
	Quota int    `json:"quota"`
}
