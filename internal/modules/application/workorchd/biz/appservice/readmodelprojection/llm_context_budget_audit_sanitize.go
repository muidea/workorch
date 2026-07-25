package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// sanitizeLLMContextBudgetAudit keeps the read model on the bounded audit
// contract. Event records are persisted data and therefore must not turn an
// unknown nested field into a second prompt or owner-payload transport.
//
// The allow-list mirrors llmaudit.ContextBudget.Payload. It intentionally
// excludes raw prompt/profile/cache-plan objects; section identity is carried
// through the bounded sectionHashes projection instead.
func sanitizeLLMContextBudgetAudit(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(input.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	fields := make([]workorcheventpayload.Field, 0, len(llmContextBudgetAuditFieldNames)+1)
	for _, name := range llmContextBudgetAuditFieldNames {
		if value, ok := input.Field(name); ok {
			fields = append(fields, workorcheventpayload.Field{Name: name, Value: value})
		}
	}
	if value, ok := input.Field("sectionHashes"); ok && value.Kind == workorcheventpayload.ValueArray {
		sections := make([]workorcheventpayload.Value, 0, len(value.Array))
		for _, item := range value.Array {
			if item.Kind != workorcheventpayload.ValueObject {
				continue
			}
			section := workorcheventpayload.Payload{Fields: item.Object}
			id, ok := section.String("id")
			if !ok || id == "" {
				continue
			}
			sectionFields := []workorcheventpayload.Field{workorcheventpayload.StringField("id", id)}
			if hash, ok := section.String("hash"); ok {
				sectionFields = append(sectionFields, workorcheventpayload.StringField("hash", hash))
			}
			if size, ok := section.Int64("sizeBytes"); ok {
				sectionFields = append(sectionFields, workorcheventpayload.IntField("sizeBytes", size))
			}
			if refs, ok := section.StringSlice("sourceRefs"); ok {
				sectionFields = append(sectionFields, workorcheventpayload.StringArrayField("sourceRefs", refs))
			}
			sections = append(sections, workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: workorcheventpayload.New(sectionFields...).Fields})
		}
		if len(sections) > 0 {
			fields = append(fields, workorcheventpayload.Field{Name: "sectionHashes", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: sections}})
		}
	}
	return workorcheventpayload.New(fields...)
}

var llmContextBudgetAuditFieldNames = []string{
	"providerId", "modelId", "capabilityRevision", "configurationRevision",
	"contextWindowTokens", "providerMaxOutputTokens", "effectiveMaxOutputTokens",
	"maxBytes", "actualBytes", "maxTokens", "estimatedInputTokens",
	"estimatedOutputTokens", "actualTokens", "maxRequestBytes", "profileMode",
	"capabilitySource", "providerSubmitted", "overBudget", "truncated",
	"stablePrefixHash", "renderedStablePrefixHash", "cacheStrategy",
	"contextGateAction", "contextGateReason", "budgetFailureClass",
	"includedSections", "normalizedSections", "compressedSections", "omittedSections",
	"redactedPaths", "protectedFields", "integrityWarnings", "sourceRefs",
}
