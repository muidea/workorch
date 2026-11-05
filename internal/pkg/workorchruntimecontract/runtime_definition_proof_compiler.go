package workorchruntimecontract

// Immutable Definition inputs and their compiled declaration only. Current
// catalog acquisition/repair choices, joins and credit remain outside it.
type runtimeDefinitionProofCompilation struct {
	revision string
	item     RuntimeCoverageItem
	source   RuntimeDefinitionFocus
	result   RuntimeSemanticProofObligation
}

func compileRuntimeDefinitionProofInSession(revision string, item RuntimeCoverageItem, source RuntimeDefinitionFocus, session *runtimeSemanticValidationSession) (RuntimeSemanticProofObligation, error) {
	if session != nil {
		for _, entry := range session.definitionProofs {
			if entry.revision == revision && runtimeRawEqual(entry.item, item) && runtimeRawEqual(entry.source, source) {
				return cloneRuntimeSemanticProofObligationRaw(entry.result), nil
			}
		}
	}
	result, err := compileRuntimeSemanticDefinitionProofObligation(revision, item, source)
	if err != nil {
		return RuntimeSemanticProofObligation{}, err
	}
	if session != nil {
		session.definitionProofs = append(session.definitionProofs, runtimeDefinitionProofCompilation{
			revision: revision, item: cloneRuntimeCoverageItemRaw(item), source: cloneRuntimeDefinitionFocusRaw(source),
			result: cloneRuntimeSemanticProofObligationRaw(result),
		})
	}
	return result, nil
}
