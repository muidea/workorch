package common

const ModuleID = "kernel/result"

const (
	SourceTypeDoDAssessment = "dod_assessment"
	SourceTypeOperator      = "operator"
	SourceTypeArtifact      = "artifact"
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
	ArtifactCategoryRuntimeEvidence = "runtime_evidence"
	ArtifactCategoryFinalSummary    = "final_summary"
)
