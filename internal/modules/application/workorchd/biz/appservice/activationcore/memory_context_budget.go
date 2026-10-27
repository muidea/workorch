package activationcore

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/memoryselection"
)

// Kept as the activation API; the shared pure compiler owns the unchanged policy.
const (
	RuntimeMemoryContextMaxItems           = memoryselection.RuntimeMemoryContextMaxItems
	RuntimeMemoryContextMaxBytes           = memoryselection.RuntimeMemoryContextMaxBytes
	RuntimeMemoryContextMaxEstimatedTokens = memoryselection.RuntimeMemoryContextMaxEstimatedTokens
	RuntimeMemorySelectionCandidateLimit   = memoryselection.RuntimeMemorySelectionCandidateLimit
	RuntimeMemorySelectionPolicyVersion    = memoryselection.RuntimeMemorySelectionPolicyVersion
)

func SelectRuntimeMemoryContext(items []*memorymodel.MemoryItem) (memoryselection.RuntimeMemorySelection, error) {
	return memoryselection.SelectRuntimeMemoryContext(items)
}
