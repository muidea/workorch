package workorchruntimecontract

import (
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeChangeReviewContext is an exact request-local observation, never a
// second receipt ledger or an acceptance result. Complete means the complete
// mutation chain for these owner-selected current targets, not all Workspace
// resources. Original receipts remain owned by the Orchestrator.
type RuntimeChangeReviewContext struct {
	WorkspaceID        common.WorkspaceID                 `json:"workspaceId"`
	RunID              common.RunID                       `json:"runId"`
	DefinitionRevision string                             `json:"definitionRevision"`
	Current            []RuntimeValidationInputReviewFile `json:"current"`
	Changes            []RuntimeChangeReviewOperation     `json:"changes"`
}

type RuntimeChangeReviewOperation struct {
	Operation     RuntimeOperation `json:"operation"`
	OperationHash string           `json:"operationHash"`
	ExecutionKey  string           `json:"executionKey"`
	Fence         uint64           `json:"fence"`
	DispatchID    string           `json:"dispatchId"`
	OperationID   string           `json:"operationId"`
	OwnerModuleID string           `json:"ownerModuleId"`
	ReceiptRef    string           `json:"receiptRef"`
	ReceiptHash   string           `json:"receiptHash"`
	TargetRef     string           `json:"targetRef"`
	BeforeHash    string           `json:"beforeHash"`
	AfterHash     string           `json:"afterHash"`
	// The first mutation of an existing target, or a later owner-observed
	// preimage differing from the last receipt, needs its exact before-bytes.
	// Current facts need not equal a historical after-image.
	Baseline *RuntimeValidationInputReviewFile `json:"baseline,omitempty"`
}

func (c *RuntimeChangeReviewContext) Clone() *RuntimeChangeReviewContext {
	if c == nil {
		return nil
	}
	v := *c
	v.Current = slices.Clone(c.Current)
	v.Changes = slices.Clone(c.Changes)
	for i := range v.Changes {
		v.Changes[i].Operation = v.Changes[i].Operation.Clone()
		if b := v.Changes[i].Baseline; b != nil {
			x := *b
			v.Changes[i].Baseline = &x
		}
	}
	return &v
}
func (c *RuntimeChangeReviewContext) Validate(workspace common.WorkspaceID, run common.RunID, definition string) error {
	if c == nil {
		return nil
	}
	if c.WorkspaceID != workspace || c.RunID != run || c.DefinitionRevision != definition || len(c.Current) == 0 || workspace == "" || run == "" || definition == "" {
		return fmt.Errorf("change review source identity mismatch")
	}
	current := map[string]string{}
	last := map[string]string{}
	keys := map[string]bool{}
	for _, f := range c.Current {
		if !runtimeProspectiveTargetRefValid(f.TargetRef) || current[f.TargetRef] != "" || f.ContentHash != RuntimeValidationInputContentHash(f.Content) {
			return fmt.Errorf("change review current content missing or altered")
		}
		current[f.TargetRef] = f.ContentHash
	}
	for _, v := range c.Changes {
		if v.Operation.ID != v.OperationID || v.OperationHash == "" || v.OperationHash != StableRuntimeOperationHash(v.Operation) || !slices.Contains(v.Operation.TargetRefs, v.TargetRef) || v.ExecutionKey == "" || v.Fence == 0 || keys[v.ExecutionKey] || v.DispatchID == "" || v.OperationID == "" || v.OwnerModuleID == "" || v.ReceiptHash == "" || v.ReceiptRef != "runtime-receipt/"+v.ExecutionKey || current[v.TargetRef] == "" || v.BeforeHash == v.AfterHash || v.AfterHash == "" {
			return fmt.Errorf("change review requires unique original changed receipts and scoped targets")
		}
		keys[v.ExecutionKey] = true
		prior, seen := last[v.TargetRef]
		if v.BeforeHash != "" && (!seen || prior != v.BeforeHash) {
			if v.Baseline == nil || v.Baseline.TargetRef != v.TargetRef || v.Baseline.ContentHash != v.BeforeHash || RuntimeValidationInputContentHash(v.Baseline.Content) != v.BeforeHash {
				return fmt.Errorf("change review exact preimage unavailable or altered")
			}
		} else if v.Baseline != nil {
			return fmt.Errorf("change review has a fabricated or redundant preimage")
		} else if seen && prior != v.BeforeHash {
			return fmt.Errorf("change review recreation lacks its intervening source")
		}
		last[v.TargetRef] = v.AfterHash
	}
	return nil
}
