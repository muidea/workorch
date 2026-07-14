package web

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOperationRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	StepID                        common.StepID
	Kind                          string
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	ProviderID                    string
	Query                         string
	Locale                        string
	Recency                       string
	Domains                       []string
	ExcludeDomains                []string
	MaxResults                    int64
	SafeSearch                    bool
	ApprovalID                    common.ApprovalID
	URL                           string
	MaxBytes                      int64
	ExtractMode                   string
	TargetRefs                    []string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
}

type RuntimeOperationResult struct {
	Status          string
	Stage           string
	ProviderID      string
	Query           string
	URL             string
	FinalURL        string
	Title           string
	Content         string
	Summary         string
	ContentType     string
	ContentHash     string
	RetrievedAt     string
	PublishedAt     string
	Source          string
	Size            int64
	ResultCount     int64
	Truncated       bool
	ApprovalID      common.ApprovalID
	ApprovalNeeded  bool
	DurationMillis  int64
	FailureCode     string
	FailureMessage  string
	TargetRefs      []string
	Results         []WebSearchResult
	ApprovalContext workorcheventpayload.Payload
	Evidence        runtimecontract.RuntimeOperationEvidence
}

type WebSearchResult struct {
	Title       string
	URL         string
	Snippet     string
	Source      string
	PublishedAt string
	Rank        int64
	ContentHash string
}

func firstWebNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
