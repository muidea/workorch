package web

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (a *RuntimeAdapter) fetch(ctx context.Context, request RuntimeOperationRequest, provider webmodels.ProviderConfig) (RuntimeOperationResult, error) {
	if err := validateWebOperationRequest(request); err != nil {
		return failedWebResult(request, provider, webcommon.ErrorInvalidRequest, err), err
	}
	rawURL := strings.TrimSpace(request.URL)
	maxBytes := boundedFetchBytes(request, provider)
	response, errCode, err := a.doSafeFetchRequest(ctx, provider, request, maxBytes)
	if err != nil {
		var governanceRequired *webFetchGovernanceRequiredError
		if errors.As(err, &governanceRequired) {
			ret := approvalRequiredFetchResult(request, provider, rawURL, governanceRequired.TargetURL, governanceRequired.Policy)
			ret.DurationMillis = response.Duration.Milliseconds()
			return ret, err
		}
		ret := failedWebResult(request, provider, errCode, err)
		ret.URL = rawURL
		ret.DurationMillis = response.Duration.Milliseconds()
		return ret, err
	}

	content := normalizeFetchedContent(response.Content)
	title, summary, summaryTruncated := summarizeFetchedContent(content, response.ContentType)
	parsed, parseErr := url.Parse(response.FinalURL)
	if parseErr != nil || parsed.Hostname() == "" {
		err := fmt.Errorf("%w: Web fetch returned an invalid final URL", common.ErrInvalidArgument)
		ret := failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, err)
		ret.DurationMillis = response.Duration.Milliseconds()
		return ret, err
	}
	source, _ := boundWebText(parsed.Hostname(), webSourceMaxBytes)
	if summary == "" {
		summary = "fetched bounded Web content"
		if source != "" {
			summary += " from " + source
		}
	}
	truncated := response.Truncated || summaryTruncated
	payload := webFetchEvidencePayload{
		ProviderID:  provider.ID,
		URL:         rawURL,
		FinalURL:    response.FinalURL,
		Title:       title,
		Content:     content,
		Summary:     summary,
		ContentType: response.ContentType,
		Source:      source,
		Size:        int64(len([]byte(content))),
		Truncated:   truncated,
	}
	rawRef, evidenceHash, _, persistErr := persistWebEvidence(request.WorkspaceInternalDirectory, webcommon.StepKindFetch, payload)
	if persistErr != nil {
		ret := failedWebResult(request, provider, webcommon.ErrorBadGatewayResponse, persistErr)
		ret.DurationMillis = response.Duration.Milliseconds()
		return ret, persistErr
	}
	sourceRef := "workorch://web/source/" + stableHash(response.FinalURL)
	evidenceRefs := webEvidenceRefs(rawRef, sourceRef)
	tags := []string{"web/fetch", "content/normalized"}
	if truncated {
		tags = append(tags, "content/truncated")
	}
	retrievedAt := time.Now().UTC().Format(time.RFC3339Nano)
	ret := RuntimeOperationResult{
		Status: "completed", Stage: "web_fetch", ProviderID: provider.ID,
		URL: rawURL, FinalURL: response.FinalURL, Title: title, Content: content, Summary: summary,
		ContentType: response.ContentType, ContentHash: evidenceHash, RetrievedAt: retrievedAt,
		Source: source, Size: int64(len([]byte(content))), Truncated: truncated,
		DurationMillis: response.Duration.Milliseconds(), TargetRefs: append([]string(nil), request.TargetRefs...),
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: summary, ContentHash: evidenceHash, EvidenceRefs: evidenceRefs,
			Quality: common.EvidenceQualityValid, EvidenceReason: "owner-normalized Web fetch evidence", EvidenceTags: tags,
		},
	}
	return ret, nil
}
