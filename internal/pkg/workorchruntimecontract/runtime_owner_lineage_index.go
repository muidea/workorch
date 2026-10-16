package workorchruntimecontract

import "reflect"

// runtime-owner-lineage-subgraph-compiler/v1. Only complete, detached raw
// historical nodes are retained. Keys select candidates, never certify them;
// every hit compares the entire node, including nil/empty and original bytes.
// No current Facts, budget, collection success or cross-node join is cached.
type runtimeOwnerLineageIndex struct {
	dispatches    map[string][]RuntimeDispatchRecord
	catalogs      map[string][]RuntimeOperationAdmissibility
	receipts      map[string][]runtimeOwnerLineageReceipt
	confirmations map[string][]RuntimeRepairSurfaceConfirmationRecord
}

type runtimeOwnerLineageReceipt struct {
	raw  RuntimeOperationReceipt
	hash string // pure product of this exact raw node, never a lookup credential
}

func runtimeOwnerLineageIndexFor(s *runtimeSemanticValidationSession) *runtimeOwnerLineageIndex {
	if s == nil || s.ownerHistory == nil {
		return nil
	}
	if s.ownerHistory.lineage == nil {
		s.ownerHistory.lineage = &runtimeOwnerLineageIndex{}
	}
	return s.ownerHistory.lineage
}

func (s *runtimeHistoricalPreimageSession) lineageIndex() *runtimeOwnerLineageIndex {
	if s == nil {
		return nil
	}
	return runtimeOwnerLineageIndexFor(s.semantic)
}

func (c *runtimeOwnerLineageIndex) count() int {
	if c == nil {
		return 0
	}
	n := 0
	for _, v := range c.dispatches {
		n += len(v)
	}
	for _, v := range c.catalogs {
		n += len(v)
	}
	for _, v := range c.receipts {
		n += len(v)
	}
	for _, v := range c.confirmations {
		n += len(v)
	}
	return n
}

func (c *runtimeOwnerLineageIndex) dispatch(d RuntimeDispatchRecord) error {
	// Active execution progress is never retained as a historical node.
	if c == nil || d.State != RuntimeDispatchStateCommitted && d.State != RuntimeDispatchStateInvalidated {
		return d.validateIntrinsicInLineageIndex(c)
	}
	for _, prior := range c.dispatches[d.ID] {
		if reflect.DeepEqual(prior, d) {
			return nil
		}
	}
	if err := d.validateIntrinsicInLineageIndex(c); err != nil {
		return err
	}
	if c.dispatches == nil {
		c.dispatches = make(map[string][]RuntimeDispatchRecord)
	}
	c.dispatches[d.ID] = append(c.dispatches[d.ID], cloneRuntimeDispatchRecordRaw(d))
	return nil
}

func (c *runtimeOwnerLineageIndex) catalog(a RuntimeOperationAdmissibility) error {
	if c == nil {
		return a.Validate()
	}
	for _, prior := range c.catalogs[a.SnapshotID] {
		if reflect.DeepEqual(prior, a) {
			return nil
		}
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if c.catalogs == nil {
		c.catalogs = make(map[string][]RuntimeOperationAdmissibility)
	}
	c.catalogs[a.SnapshotID] = append(c.catalogs[a.SnapshotID], cloneRuntimeOperationAdmissibilityRaw(a))
	return nil
}

func (c *runtimeOwnerLineageIndex) receipt(r RuntimeOperationReceipt) error {
	if c == nil {
		return r.Validate()
	}
	for _, prior := range c.receipts[r.ExecutionKey] {
		if reflect.DeepEqual(prior.raw, r) {
			return nil
		}
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if c.receipts == nil {
		c.receipts = make(map[string][]runtimeOwnerLineageReceipt)
	}
	c.receipts[r.ExecutionKey] = append(c.receipts[r.ExecutionKey], runtimeOwnerLineageReceipt{raw: cloneRuntimeOperationReceiptRaw(r)})
	return nil
}

// Called at the original hash/join position, never instead of validation.
// An uncompiled or different raw receipt takes the independent pure hash path;
// only already validated exact raw nodes can retain a hash compilation product.
func (c *runtimeOwnerLineageIndex) receiptHash(r RuntimeOperationReceipt) string {
	if c != nil {
		for i := range c.receipts[r.ExecutionKey] {
			entry := &c.receipts[r.ExecutionKey][i]
			if reflect.DeepEqual(entry.raw, r) {
				if entry.hash == "" {
					entry.hash = StableRuntimeOperationReceiptHash(r)
				}
				return entry.hash
			}
		}
	}
	return StableRuntimeOperationReceiptHash(r)
}

func (c *runtimeOwnerLineageIndex) confirmation(r RuntimeRepairSurfaceConfirmationRecord) error {
	if c == nil {
		return r.Validate()
	}
	for _, prior := range c.confirmations[r.Signature] {
		if reflect.DeepEqual(prior, r) {
			return nil
		}
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if c.confirmations == nil {
		c.confirmations = make(map[string][]RuntimeRepairSurfaceConfirmationRecord)
	}
	c.confirmations[r.Signature] = append(c.confirmations[r.Signature], cloneRuntimeRepairSurfaceConfirmationRecordRaw(r))
	return nil
}
