package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	policyAllowed        = "allowed"
	policyDenied         = "denied"
	policyApproval       = "approval_required"
	approvalReasonPrefix = "web access requires approval"
	approvalContextQuery = "query"
	approvalContextURL   = "url"
)

type ProviderConfigLoader func() (string, map[string]webmodels.ProviderConfig, error)
type ProviderConfigRevision func() uint64

type ApprovalGetter interface {
	Get(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
}

type ScopedApprovalGetter interface {
	GetForRun(context.Context, common.ApprovalID, common.RunID) (*approvalmodel.Approval, error)
}

type providerRuntimeCache struct {
	ready             bool
	revision          uint64
	defaultProviderID string
	providers         map[string]webmodels.ProviderConfig
}

type RuntimeAdapter struct {
	defaultProviderID string
	providers         map[string]webmodels.ProviderConfig
	configLoader      ProviderConfigLoader
	configRevision    ProviderConfigRevision
	approvals         ApprovalGetter
	client            *http.Client
	resolver          *net.Resolver
	cacheMu           sync.RWMutex
	cache             providerRuntimeCache
}

func NewRuntimeAdapter(defaultProviderID string, providers map[string]webmodels.ProviderConfig, approvals ApprovalGetter) *RuntimeAdapter {
	defaultProviderID = strings.TrimSpace(defaultProviderID)
	if defaultProviderID == "" {
		defaultProviderID = webproviderconfig.DefaultProviderIDFromProviders(providers)
	}
	return &RuntimeAdapter{
		defaultProviderID: defaultProviderID,
		providers:         webproviderconfig.NormalizeProviders(providers),
		approvals:         approvals,
		client:            http.DefaultClient,
		resolver:          net.DefaultResolver,
	}
}

func NewRuntimeAdapterFromConfigLoaderWithRevision(loader ProviderConfigLoader, revision ProviderConfigRevision, approvals ApprovalGetter) *RuntimeAdapter {
	return &RuntimeAdapter{
		defaultProviderID: webproviderconfig.DefaultProviderID,
		configLoader:      loader,
		configRevision:    revision,
		approvals:         approvals,
		client:            http.DefaultClient,
		resolver:          net.DefaultResolver,
	}
}

func NewRuntimeAdapterWithHTTPClient(defaultProviderID string, providers map[string]webmodels.ProviderConfig, approvals ApprovalGetter, client *http.Client) *RuntimeAdapter {
	adapter := NewRuntimeAdapter(defaultProviderID, providers, approvals)
	if client != nil {
		adapter.client = client
	}
	return adapter
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeOperationRequest) (RuntimeOperationResult, error) {
	if a == nil {
		return RuntimeOperationResult{}, common.ErrInvalidArgument
	}
	provider, err := a.selectProviderID(request.ProviderID)
	if err != nil {
		return failedWebResult(request, provider, webcommon.ErrorProviderUnavailable, err), err
	}
	switch strings.TrimSpace(request.Kind) {
	case webcommon.StepKindSearch:
		result, executeErr := a.search(ctx, request, provider)
		return requireWebApprovalFailure(result, executeErr)
	case webcommon.StepKindFetch:
		result, executeErr := a.fetch(ctx, request, provider)
		return requireWebApprovalFailure(result, executeErr)
	default:
		err := fmt.Errorf("%w: unsupported web step kind %q", common.ErrInvalidArgument, request.Kind)
		return failedWebResult(request, provider, webcommon.ErrorInvalidRequest, err), err
	}
}

// OperationAdmissibility returns the current, bounded provider selector set and
// a stable revision for Runtime's read-only candidate-admission inventory.
// It only reads provider configuration; it never performs an HTTP request.
func (a *RuntimeAdapter) OperationAdmissibility() ([]string, string, error) {
	if a == nil {
		return nil, "", common.ErrInvalidArgument
	}
	defaultID, providers, err := a.providerConfigs()
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	payload, err := json.Marshal(struct {
		DefaultProviderID string                              `json:"defaultProviderId"`
		Providers         map[string]webmodels.ProviderConfig `json:"providers"`
	}{DefaultProviderID: defaultID, Providers: providers})
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(payload)
	return ids, "web-admissibility-" + hex.EncodeToString(sum[:])[:20], nil
}

// ProbeOperation checks only local/provider-policy prerequisites. It does not
// contact a search/fetch endpoint, create a receipt, or commit any evidence.
// A fetch may perform DNS classification, which is read-only and uses the same
// SSRF policy as execution so an invalid target cannot be sealed.
func (a *RuntimeAdapter) ProbeOperation(ctx context.Context, request RuntimeOperationRequest) error {
	if a == nil {
		return common.ErrInvalidArgument
	}
	provider, err := a.selectProviderID(request.ProviderID)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(request.Kind) {
	case webcommon.StepKindSearch:
		if strings.TrimSpace(request.Query) == "" {
			return fmt.Errorf("%w: web/search query is required", common.ErrInvalidArgument)
		}
		return nil
	case webcommon.StepKindFetch:
		if strings.TrimSpace(request.URL) == "" {
			return fmt.Errorf("%w: web/fetch url is required", common.ErrInvalidArgument)
		}
		if decision := a.classifyURL(ctx, request.URL, provider, true); decision.Decision == policyDenied {
			return fmt.Errorf("%w: web/fetch URL is not allowed: %s", common.ErrInvalidArgument, strings.Join(decision.Reasons, ","))
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported web step kind %q", common.ErrInvalidArgument, request.Kind)
	}
}
