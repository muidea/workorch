package biz

import workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"

// synthetic provider fixture 显式提供当前 wire 合同的全部数组，不修复生产输出。
func authoringWireSpecificationFixture(spec workspace.GoalSpecification) workspace.GoalSpecification {
	spec = spec.Clone()
	if spec.Resources == nil {
		spec.Resources = []workspace.GoalSpecificationResource{}
	}
	if spec.Dimensions == nil {
		spec.Dimensions = []workspace.GoalSpecificationDimension{}
	}
	if spec.Requirements == nil {
		spec.Requirements = []workspace.GoalSpecificationRequirement{}
	}
	if spec.Obligations == nil {
		spec.Obligations = []workspace.GoalSpecificationObligation{}
	}
	if spec.Verifications == nil {
		spec.Verifications = []workspace.GoalSpecificationVerification{}
	}
	if spec.Reviews == nil {
		spec.Reviews = []workspace.GoalSpecificationCompletenessReview{}
	}
	for i := range spec.Dimensions {
		if spec.Dimensions[i].ResourceIDs == nil {
			spec.Dimensions[i].ResourceIDs = []string{}
		}
	}
	for i := range spec.Requirements {
		if spec.Requirements[i].SourceItemRefs == nil {
			spec.Requirements[i].SourceItemRefs = []string{}
		}
		if spec.Requirements[i].ResourceIDs == nil {
			spec.Requirements[i].ResourceIDs = []string{}
		}
	}
	for i := range spec.Obligations {
		if spec.Obligations[i].DoDItemRefs == nil {
			spec.Obligations[i].DoDItemRefs = []string{}
		}
	}
	return spec
}
