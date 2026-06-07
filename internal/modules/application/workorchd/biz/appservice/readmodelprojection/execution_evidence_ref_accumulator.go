package readmodelprojection

type executionEvidenceRefs struct {
	evidenceByStep      map[string][]string
	decisionsByStep     map[string][]string
	approvalsByStep     map[string][]string
	toolCallsByStep     map[string][]string
	patchesByStep       map[string][]string
	validationsByStep   map[string][]string
	resultsByStep       map[string][]string
	artifactsByStep     map[string][]string
	memoryRecallsByStep map[string][]string
	memoryIDsByStep     map[string][]string
}

func newExecutionEvidenceRefs() executionEvidenceRefs {
	return executionEvidenceRefs{
		evidenceByStep:      map[string][]string{},
		decisionsByStep:     map[string][]string{},
		approvalsByStep:     map[string][]string{},
		toolCallsByStep:     map[string][]string{},
		patchesByStep:       map[string][]string{},
		validationsByStep:   map[string][]string{},
		resultsByStep:       map[string][]string{},
		artifactsByStep:     map[string][]string{},
		memoryRecallsByStep: map[string][]string{},
		memoryIDsByStep:     map[string][]string{},
	}
}
