package shell

import (
	"slices"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The same owner compiler used by dispatch supplies exact native selectors.
// This function performs no I/O and makes no semantic sufficiency judgment.
func shellProjectReviewExecutionPlan(subject *rc.RuntimeValidationInputReviewSubject, binding rc.RuntimeValidationProofRequirementBinding) (*rc.RuntimeValidationExecutionPlan, error) {
	if len(binding.CheckAttestations) == 0 || !shellProjectObservationProtocol(binding.CheckAttestations[0].ObservationProtocolRef) {
		return nil, nil
	}
	pkg, err := shellIsolatedValidationPackage(binding.CheckAttestations)
	if err != nil {
		return nil, err
	}
	request := RuntimeCommandRequest{Command: "go", Args: []string{"test", "-v", "./..."}, ValidationCheckAttestations: binding.CheckAttestations}
	plan := &rc.RuntimeValidationExecutionPlan{}
	if subject.Recipe != nil {
		if subject.Recipe.Command != "" {
			request.Command, request.Args = subject.Recipe.Command, slices.Clone(subject.Recipe.Args)
		}
		plan.TimeoutMillis = subject.Recipe.TimeoutMillis
	}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		return nil, err
	}
	protocol := binding.CheckAttestations[0].ObservationProtocolRef
	requireBehavior := shellProjectRequiresBehavior(protocol)
	identities := shellProjectExecutionStageIdentitiesForProtocol(pkg, protocol)
	for i, spec := range shellProjectFixedStages(request.Args, pkg, requireBehavior) {
		stage := rc.RuntimeValidationPlannedStage{Identity: identities[i], Command: "go", Args: slices.Clone(spec.Args)}
		if i == 2 {
			stage.Instrumentation = []string{"race"}
		}
		if i == 3 {
			if requireBehavior {
				stage.Instrumentation = []string{"coverage_behavior", "process_behavior"}
			}
			stage.RequiredCheckRefs = normalizeShellStrings(binding.RequiredCheckRefs)
		}
		if spec.JSON && (shellProjectMultiFailureProtocol(protocol) || shellNativeChannelMethodRequired(subject.Files)) {
			stage.Instrumentation = append(stage.Instrumentation, "native_source_channel")
			slices.Sort(stage.Instrumentation)
		}
		plan.Stages = append(plan.Stages, stage)
	}
	return plan, nil
}
