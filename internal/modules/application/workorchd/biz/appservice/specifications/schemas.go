package specifications

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"strings"
)

func authoringSpecificationOutputSchema(purpose string) string {
	if strings.HasPrefix(purpose, "review/") {
		return specificationOutputSchema(purpose)
	}
	return workspace.DefinitionAuthoringMappingSchema
}
func authoringSpecificationOutputContract(purpose, instructions string) string {
	if strings.HasPrefix(purpose, "review/") {
		return specificationOutputContract(purpose, instructions)
	}
	return instructions + "\n当前输出协议 " + workspace.DefinitionAuthoringMappingProtocol + "；基础字段必填、大小写精确、禁止未知/重复/null字段；ExecutionProofMappings 仅对版本化跨源 Profile opt-in；EvidencePurposeMappings 是独立的版本化历史行为／当前验收来源提案。两项存在时其字段全部必填，256KiB/32层完整文档边界；local Workspace strict decoder/compiler 是最终权威，无旧形状 fallback。\nJSON Schema:\n" + workspace.DefinitionAuthoringMappingSchema
}

// 版本化手写输出合同只约束结构；owner 语义校验、独立审查及确认仍不可省略。
const derivationOutputSchema = `{"type":"object","additionalProperties":false,"required":["AuthoringInputHash","TemplateSnapshotHash","AdditionalItems"],"properties":{"AuthoringInputHash":{"type":"string"},"TemplateSnapshotHash":{"type":"string"},"AdditionalItems":{"type":"array","maxItems":128,"items":{"type":"string"}}}}`

func derivationOutputContract() string {
	return derivationInstructions + "\n输出合同 workorch.derived_acceptance_output.v1：禁止未知字段；所有字段必须提供；AdditionalItems 必须是字符串数组，不得为 null，无补充条件时使用 []。结构合法不替代原始要求、来源和预算校验。\nJSON Schema:\n" + derivationOutputSchema
}

const proposalOutputSchema = `{
 "type":"object","additionalProperties":false,"required":["specification"],"properties":{"specification":{
 "type":"object","additionalProperties":false,
 "required":["Version","GoalItemRef","GoalTextHash","Resources","Dimensions","Requirements","Obligations","Verifications","Reviews"],
 "properties":{
 "Version":{"type":"integer","enum":[1]},"GoalItemRef":{"type":"string"},"GoalTextHash":{"type":"string"},
 "Resources":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["ID","Ref","SnapshotHash","Required","Available"],"properties":{"ID":{"type":"string"},"Ref":{"type":"string"},"SnapshotHash":{"type":"string"},"Required":{"type":"boolean"},"Available":{"type":"boolean"}}}},
 "Dimensions":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["ID","Required","Activated","ResourceIDs"],"properties":{"ID":{"type":"string"},"Required":{"type":"boolean"},"Activated":{"type":"boolean"},"ResourceIDs":{"type":"array","items":{"type":"string"}}}}},
 "Requirements":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["ID","DimensionID","Required","SourceKind","SourceItemRefs","ResourceIDs","Text"],"properties":{"ID":{"type":"string"},"DimensionID":{"type":"string"},"Required":{"type":"boolean"},"SourceKind":{"type":"string","enum":["USER_EXPLICIT","TEMPLATE_REQUIRED","TEMPLATE_CONDITIONAL","DERIVED","DISCOVERED"]},"SourceItemRefs":{"type":"array","items":{"type":"string"}},"ResourceIDs":{"type":"array","items":{"type":"string"}},"Text":{"type":"string"}}}},
 "Obligations":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["ID","RequirementID","DoDItemRefs","Text"],"properties":{"ID":{"type":"string"},"RequirementID":{"type":"string"},"DoDItemRefs":{"type":"array","items":{"type":"string"}},"Text":{"type":"string"}}}},
 "Verifications":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["ID","ObligationID","Declaration","Status","ProfileID","EvidenceRequirementID","Scope"],"properties":{"ID":{"type":"string"},"ObligationID":{"type":"string"},"Declaration":{"type":"string"},"Status":{"type":"string","enum":["DECLARED","UNVERIFIED"]},"ProfileID":{"type":"string"},"EvidenceRequirementID":{"type":"string"},"Scope":{"type":"string"}}}},
 "ExecutionProofMappings":{"type":"array","maxItems":512,"items":{"type":"object","additionalProperties":false,"required":["ContractRef","ConsumerItemRef","RuleRef","SourceItemRef"],"properties":{"ContractRef":{"type":"string","enum":["runtime/cross-source-execution-proof/v1","runtime/goal-source-execution-proof/v2"]},"ConsumerItemRef":{"type":"string"},"RuleRef":{"type":"string"},"SourceItemRef":{"type":"string"},"ConsumerGoalText":{"type":"string","minLength":1,"maxLength":4096}},"allOf":[{"if":{"properties":{"ContractRef":{"const":"runtime/goal-source-execution-proof/v2"}}},"then":{"required":["ConsumerGoalText"]},"else":{"not":{"required":["ConsumerGoalText"]}}}]}},
 "EvidencePurposeMappings":{"type":"array","maxItems":512,"items":{"type":"object","additionalProperties":false,"required":["ContractRef","HistoricalItemRef","AcceptanceItemRef"],"properties":{"ContractRef":{"type":"string","enum":["runtime/cross-cwi-evidence-purpose/v1","runtime/profile-deliverable-evidence-purpose/v2"]},"HistoricalItemRef":{"type":"string"},"AcceptanceItemRef":{"type":"string"},"HistoricalProfileID":{"type":"string"},"HistoricalDeliverableID":{"type":"string"},"HistoricalProfileHash":{"type":"string"},"HistoricalText":{"type":"string"}},"allOf":[{"if":{"properties":{"ContractRef":{"const":"runtime/profile-deliverable-evidence-purpose/v2"}}},"then":{"required":["HistoricalProfileID","HistoricalDeliverableID","HistoricalProfileHash","HistoricalText"]},"else":{"not":{"anyOf":[{"required":["HistoricalProfileID"]},{"required":["HistoricalDeliverableID"]},{"required":["HistoricalProfileHash"]},{"required":["HistoricalText"]}]}}}]}},
 "Reviews":{"type":"array","maxItems":0,"items":REVIEW_SCHEMA}
 }}}}`

const reviewObjectSchema = `{"type":"object","additionalProperties":false,"required":["Kind","Status","SpecificationHash","Reason","EvidenceRefs","RequirementIDs"],"properties":{"Kind":{"type":"string","enum":["dimension_coverage","negative_path","regression","resource_environment","traceability","verifier_feasibility"]},"Status":{"type":"string","enum":["PASS","FAIL","UNKNOWN"]},"SpecificationHash":{"type":"string"},"Reason":{"type":"string"},"EvidenceRefs":{"type":"array","items":{"type":"string"}},"RequirementIDs":{"type":"array","items":{"type":"string"}}}}`

func specificationOutputSchema(purpose string) string {
	if strings.HasPrefix(purpose, "review/") {
		return `{"type":"object","additionalProperties":false,"required":["review"],"properties":{"review":` + reviewObjectSchema + `}}`
	}
	return strings.ReplaceAll(proposalOutputSchema, "REVIEW_SCHEMA", reviewObjectSchema)
}

func specificationOutputContract(purpose, instructions string) string {
	return instructions + "\n输出合同 workorch.goal_specification_output.v1（所有字段必须存在且精确区分大小写；所有对象禁止未知字段、重复字段；标量和数组均不得为 null；仅接受合法 UTF-8 与成对 Unicode surrogate；数组必须是数组；proposal 的 Reviews 必须为 []；Verification 为平面字段；SourceItemRefs 为数组）。all_of 是所有必需条件共同成立的语义，不是 JSON 字段；不得添加 ConditionMode、Authority 或 ProfileAssessment。仅约束结构，不替代来源、Profile、独立审查、确认与准入校验。\nJSON Schema:\n" + specificationOutputSchema(purpose)
}
