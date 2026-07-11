package common

const ModuleID = "blocks/result"

const (
	SourceTypeDoDAssessment   = "dod_assessment"
	SourceTypeAcceptanceSlice = "acceptance_slice"
	SourceTypeOperator        = "operator"
	SourceTypeArtifact        = "artifact"
)

const (
	ArtifactTypeText        = "text"
	ArtifactTypeFile        = "file"
	ArtifactTypePatch       = "patch"
	ArtifactTypeExternalRef = "external_ref"
	ArtifactTypeReport      = "report"
)

const (
	ArtifactCategorySource          = "source"
	ArtifactCategoryConfiguration   = "configuration"
	ArtifactCategoryDocumentation   = "documentation"
	ArtifactCategoryTestReport      = "test_report"
	ArtifactCategoryDeliverable     = "deliverable"
	ArtifactCategoryRuntimeEvidence = "runtime_evidence"
	ArtifactCategoryFinalSummary    = "final_summary"
)
