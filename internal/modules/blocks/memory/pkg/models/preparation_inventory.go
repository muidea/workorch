package memory

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const PreparationInventoryVersion = "workorch-preparation-memory-inventory/v1"
const PreparationInventoryMaxItems = 256
const PreparationInventoryMaxBytes = 1024 * 1024

// Complete local inventory at one point, not selection, recall, freshness or
// readiness. No embedding, search, fallback or normalization is permitted.
type PreparationInventory struct {
	Version     string
	WorkspaceID common.WorkspaceID
	OwnerEpoch  string
	ObservedAt  time.Time
	Items       []*MemoryItem
}

func (r PreparationInventory) Clone() PreparationInventory {
	r.Items = slices.Clone(r.Items)
	for i, item := range r.Items {
		if item != nil {
			copy := *item
			copy.Tags, copy.Vector = slices.Clone(item.Tags), slices.Clone(item.Vector)
			r.Items[i] = &copy
		}
	}
	return r
}

func ValidPreparationWorkspaceID(id common.WorkspaceID) bool {
	s := string(id)
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}

func (r PreparationInventory) ValidateFor(id common.WorkspaceID) error {
	const prefix = "memory-preparation-epoch:"
	epoch := strings.TrimPrefix(r.OwnerEpoch, prefix)
	if !ValidPreparationWorkspaceID(id) || r.WorkspaceID != id || r.Version != PreparationInventoryVersion || r.ObservedAt.IsZero() || !strings.HasPrefix(r.OwnerEpoch, prefix) || len(epoch) < 26 || len(epoch) > 100 || strings.Trim(epoch, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" || r.Items == nil || len(r.Items) > PreparationInventoryMaxItems {
		return common.ErrConflict
	}
	var previous common.MemoryID
	materialBytes := 0
	for _, item := range r.Items {
		if item == nil || item.ID == "" || len(item.ID) > 256 || strings.TrimSpace(string(item.ID)) != string(item.ID) || strings.ContainsAny(string(item.ID), "\x00\r\n") || item.WorkspaceID != id || previous != "" && item.ID <= previous || !modulecommon.IsSupportedMemoryType(item.MemoryType) || !modulecommon.IsSupportedSourceType(item.SourceType) || len(item.Tags) > 256 || len(item.Vector) > modulecommon.MemoryVectorMaxDimension || math.IsNaN(item.VectorScore) || math.IsInf(item.VectorScore, 0) {
			return common.ErrConflict
		}
		for _, s := range append([]string{string(item.ID), string(item.RunID), item.SourceRef, item.Content, item.Summary, item.Visibility, item.EmbeddingProviderID, item.EmbeddingModel, item.EmbeddingText}, item.Tags...) {
			if !utf8.ValidString(s) || len(s) > PreparationInventoryMaxBytes {
				return common.ErrConflict
			}
			materialBytes += len(s)
		}
		materialBytes += len(item.Vector) * 8
		if materialBytes > PreparationInventoryMaxBytes {
			return common.ErrConflict
		}
		for _, v := range item.Vector {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return common.ErrConflict
			}
		}
		previous = item.ID
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > PreparationInventoryMaxBytes {
		return common.ErrConflict
	}
	return nil
}
