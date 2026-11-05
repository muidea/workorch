package workorchruntimecontract

import "strings"

// Frozen normalization before owner-private generation borrowing.
func referenceSemanticLedgerNormalization(l RuntimeSemanticPlanningLedger) RuntimeSemanticPlanningLedger {
	l.SchemaVersion = strings.TrimSpace(l.SchemaVersion)
	l.Phase = RuntimeSemanticPlanningPhase(strings.TrimSpace(string(l.Phase)))
	l.DefinitionSeed = l.DefinitionSeed.Clone()
	l.StablePrefixHash = strings.TrimSpace(l.StablePrefixHash)
	l.LogicalRequestID = strings.TrimSpace(l.LogicalRequestID)
	if l.CorrectionDirective != nil {
		value := l.CorrectionDirective.Clone()
		l.CorrectionDirective = &value
	}
	if l.Manifest != nil {
		value := l.Manifest.Clone()
		l.Manifest = &value
	}
	if l.CompiledOrder != nil {
		value := *l.CompiledOrder
		l.CompiledOrder = &value
	}
	if l.CoverageOrderDocument != nil {
		value := l.CoverageOrderDocument.Clone()
		l.CoverageOrderDocument = &value
	}
	if l.Document != nil {
		value := l.Document.Clone()
		l.Document = &value
	}
	l.DocumentHash = strings.TrimSpace(l.DocumentHash)
	l.Batches = normalizeRuntimeSemanticPlanBatches(l.Batches)
	l.ActiveBatchID = strings.TrimSpace(l.ActiveBatchID)
	if l.NextBatchSequence < 0 {
		l.NextBatchSequence = 0
	}
	l.PendingCoverageIDs = normalizeOrderedCoverageWorkItemIDs(l.PendingCoverageIDs)
	l.CompletedBatchIDs = normalizeStrings(l.CompletedBatchIDs)
	l.BatchFeasibilityResults = normalizeRuntimeSemanticPlanBatchFeasibilityResults(l.BatchFeasibilityResults)
	l.BatchDocuments = normalizeRuntimeSemanticPlanDocuments(l.BatchDocuments)
	if l.MergedDocument != nil {
		value := l.MergedDocument.Clone()
		l.MergedDocument = &value
	}
	l.MergedDocumentHash = strings.TrimSpace(l.MergedDocumentHash)
	l.FeasibilityBatchID = strings.TrimSpace(l.FeasibilityBatchID)
	l.Feasibilities = normalizeRuntimeSemanticFeasibilities(l.Feasibilities)
	l.FeasibilitySetHash = strings.TrimSpace(l.FeasibilitySetHash)
	l.SealedBindingSetHash = strings.TrimSpace(l.SealedBindingSetHash)
	// Sealed lineage is a positional chain (manifest → order → document →
	// feasibility batch → feasibility set → binding set), not an unordered
	// collection. Preserve order while trimming values so validation can reject
	// a permuted chain instead of silently sorting it into acceptance.
	l.SealedLineageRefs = normalizeOrderedStrings(l.SealedLineageRefs)
	for idx := range l.Rejections {
		if value := l.Rejections[idx].Normalized(); value != nil {
			l.Rejections[idx] = *value
		}
	}
	for idx := range l.FeasibilityGaps {
		l.FeasibilityGaps[idx] = l.FeasibilityGaps[idx].Normalized()
	}
	l.Failure = l.Failure.Normalized()
	if l.HistoryArchive != nil {
		archive := &RuntimeSemanticHistoryArchive{ProtocolRef: l.HistoryArchive.ProtocolRef, GenerationCount: l.HistoryArchive.GenerationCount}
		if l.HistoryArchive.Segments != nil {
			archive.Segments = make([][]RuntimeSemanticPlanningGeneration, len(l.HistoryArchive.Segments))
			for i, segment := range l.HistoryArchive.Segments {
				if segment != nil {
					archive.Segments[i] = make([]RuntimeSemanticPlanningGeneration, len(segment))
					for j, generation := range segment {
						archive.Segments[i][j] = generation.Clone()
					}
				}
			}
		}
		l.HistoryArchive = archive
	}

	if len(l.History) > 0 {
		values := make([]RuntimeSemanticPlanningGeneration, 0, len(l.History))
		for _, value := range l.History {
			values = append(values, value.Normalized())
		}
		// History is append-only semantic lineage. Preserve its order; sorting
		// would turn a causal chain into a presentation list.
		l.History = values
	}
	return l
}
