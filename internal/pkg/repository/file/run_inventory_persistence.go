package file

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memorycontract "github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The existing file cache can retain the original physical Run image until an
// aggregate consumer actually requests it. This is one representation of the
// same file, not a persisted summary or an accepted-business-result cache.
// Inventory consumers never receive a Run assembled from these projections.
type runInventoryPhysicalMap map[common.RunID]runInventoryPhysicalImage

type runInventoryPhysicalImage struct {
	raw               json.RawMessage
	metadata          run.Run
	stateRevision     uint64
	hasState          bool
	hasBootstrap      bool
	memorySelection   *memorycontract.RecallSelectionEvidence
	providerSelection *rc.ProviderInferenceSelection
	aggregate         *run.Run
}

type runInventoryPersistenceAlias run.Run
type runInventoryPersistenceWire struct {
	runInventoryPersistenceAlias
	RuntimeState              inventoryJSONView
	RuntimeBootstrap          inventoryJSONView
	ReplacementIntent         inventoryJSONView
	RetiredReplacementIntents inventoryJSONView
	ExecutionIntentReceipts   inventoryJSONView
}

// Projection views borrow the immutable decoder image instead of copying each
// opaque Runtime/receipt body. No view escapes through an owner result.
type inventoryJSONView []byte

func (v *inventoryJSONView) UnmarshalJSON(raw []byte) error { *v = raw; return nil }

func readRunInventoryPhysicalImage(raw json.RawMessage) (runInventoryPhysicalImage, error) {
	var wire runInventoryPersistenceWire
	if err := decodeCurrentJSON(raw, &wire); err != nil {
		return runInventoryPhysicalImage{}, err
	}
	image := runInventoryPhysicalImage{raw: raw, metadata: run.Run(wire.runInventoryPersistenceAlias)}
	bootstrap := bytes.TrimSpace(wire.RuntimeBootstrap)
	image.hasBootstrap = len(bootstrap) > 0 && !bytes.Equal(bootstrap, []byte("null"))
	if image.hasBootstrap {
		var selection struct {
			Activation struct {
				MemorySelection   *memorycontract.RecallSelectionEvidence
				ProviderSelection rc.ProviderInferenceSelection
			}
		}
		if err := json.Unmarshal(bootstrap, &selection); err != nil {
			return runInventoryPhysicalImage{}, err
		}
		image.memorySelection = selection.Activation.MemorySelection
		provider := selection.Activation.ProviderSelection.Normalized()
		if provider.Validate() == nil {
			image.providerSelection = &provider
		}
	}
	replacement := bytes.TrimSpace(wire.ReplacementIntent)
	if len(replacement) > 0 && !bytes.Equal(replacement, []byte("null")) {
		var selected struct {
			State    run.ReplacementIntentState
			Recovery struct{ State run.ReplacementRecoveryState }
		}
		if err := json.Unmarshal(replacement, &selected); err != nil {
			return runInventoryPhysicalImage{}, err
		}
		image.metadata.ReplacementIntent = &run.ReplacementIntent{State: selected.State, Recovery: run.ReplacementRecoveryLease{State: selected.Recovery.State}}
	}
	state := bytes.TrimSpace(wire.RuntimeState)
	image.hasState = len(state) != 0 && !bytes.Equal(state, []byte("null"))
	if !image.hasState {
		if image.metadata.RuntimeStateHash != "" {
			return runInventoryPhysicalImage{}, run.ErrRuntimeStateIntegrity
		}
		return image, nil
	}
	// Physical integrity is independent of business adjudication. The persisted
	// encoder emits canonical field order; whitespace framing is not part of its
	// JSON fingerprint. No historical Runtime object graph is constructed here.
	h := sha256.New()
	quoted, escaped := false, false
	start := 0
	for i, c := range state {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
		} else if c == '"' {
			quoted = true
		} else if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			_, _ = h.Write(state[start:i])
			start = i + 1
		}
	}
	_, _ = h.Write(state[start:])
	if image.metadata.RuntimeStateHash == "" || image.metadata.RuntimeStateHash != "runtime-state-"+hex.EncodeToString(h.Sum(nil)) {
		return runInventoryPhysicalImage{}, run.ErrRuntimeStateIntegrity
	}
	var coordinate struct{ StateRevision uint64 }
	if err := json.Unmarshal(state, &coordinate); err != nil {
		return runInventoryPhysicalImage{}, err
	}
	image.stateRevision = coordinate.StateRevision
	return image, nil
}

func (image runInventoryPhysicalImage) inventory() run.RunInventoryItem {
	fact := run.InventoryFromRun(&image.metadata)
	fact.HasRuntimeState, fact.RuntimeStateRevision = image.hasState, image.stateRevision
	fact.HasRuntimeBootstrap = image.hasBootstrap
	fact.ActivationMemorySelection = memorycontract.CloneRecallSelectionEvidence(image.memorySelection)
	if image.providerSelection != nil {
		s := *image.providerSelection
		fact.SealedLLMSelection = &s
	}
	return fact
}

func loadRunInventory(path string) ([]run.RunInventoryItem, error) {
	cache := mapCacheEntry(path)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if !cache.loaded {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		originals := map[common.RunID]json.RawMessage{}
		if err == nil {
			if err := json.Unmarshal(data, &originals); err != nil {
				return nil, err
			}
		}
		physical := make(runInventoryPhysicalMap, len(originals))
		for id, raw := range originals {
			image, err := readRunInventoryPhysicalImage(raw)
			if err != nil {
				return nil, fmt.Errorf("load Run %q inventory: %w", id, err)
			}
			physical[id] = image
		}
		cache.items, cache.loaded = physical, true
	}
	switch items := cache.items.(type) {
	case map[common.RunID]run.Run:
		facts := make([]run.RunInventoryItem, 0, len(items))
		for _, item := range items {
			facts = append(facts, run.InventoryFromRun(&item))
		}
		return facts, nil
	case runInventoryPhysicalMap:
		facts := make([]run.RunInventoryItem, 0, len(items))
		for _, item := range items {
			facts = append(facts, item.inventory())
		}
		return facts, nil
	default:
		return nil, fmt.Errorf("file repository cache type mismatch for %s", path)
	}
}

// Materialization occurs only for a full aggregate read/write. It consumes the
// exact original bytes and the existing complete physical/shape gate. It never
// rebuilds Runtime semantic authority or modifies the persisted file.
func materializeRunInventoryImages(images runInventoryPhysicalMap) (map[common.RunID]run.Run, error) {
	items := make(map[common.RunID]run.Run, len(images))
	for id, image := range images {
		if image.aggregate != nil {
			items[id] = *image.aggregate
			continue
		}
		var item run.Run
		if err := decodeCurrentJSON(image.raw, &item); err != nil {
			return nil, err
		}
		items[id] = item
	}
	if err := validateRunMapIntegrity(items); err != nil {
		return nil, err
	}
	return items, nil
}

// An exact aggregate read materializes only its selected original image. The
// other historical images remain opaque in the same repository file cache.
func loadRunByID(path string, workspaceID common.WorkspaceID, id common.RunID) (*run.Run, error) {
	original, err := loadRunOriginalByID(path, workspaceID, id)
	if err != nil {
		return nil, err
	}
	return cloneRun(original), nil
}

// Borrow only while holding the owning repository lock. Metadata consumers
// project their detached result before releasing that lock; no original escapes.
func loadRunOriginalByID(path string, workspaceID common.WorkspaceID, id common.RunID) (*run.Run, error) {
	if _, err := loadRunInventory(path); err != nil {
		return nil, err
	}
	cache := mapCacheEntry(path)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	switch items := cache.items.(type) {
	case map[common.RunID]run.Run:
		item, found := items[id]
		if !found || workspaceID != "" && item.WorkspaceID != workspaceID {
			return nil, common.ErrNotFound
		}
		return &item, nil
	case runInventoryPhysicalMap:
		image, found := items[id]
		if !found || workspaceID != "" && image.metadata.WorkspaceID != workspaceID {
			return nil, common.ErrNotFound
		}
		if image.aggregate == nil {
			var item run.Run
			if err := decodeCurrentJSON(image.raw, &item); err != nil {
				return nil, err
			}
			if err := validateRunMapIntegrity(map[common.RunID]run.Run{id: item}); err != nil {
				return nil, err
			}
			image.aggregate = &item
			image.raw = nil
			items[id] = image
		}
		return image.aggregate, nil
	default:
		return nil, fmt.Errorf("file repository cache type mismatch for %s", path)
	}
}
