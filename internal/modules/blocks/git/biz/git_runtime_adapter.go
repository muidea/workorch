package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	DefaultCommandTimeout = 30 * time.Second
	MaxCommandTimeout     = 10 * time.Minute
	maxOutputBytes        = 64 * 1024
	maxDiffBytes          = 128 * 1024
	maxChangedFiles       = 500
	defaultLogLimit       = 20
	maxLogLimit           = 100
)

type RuntimeAdapter struct {
	timeout      time.Duration
	envAllowlist map[string]struct{}
}

// SupportedRuntimeOperationVariants is the Git owner's executable operation
// surface. The descriptor remains the parameter/effect authority; this list is
// only an enumeration guard so an owner switch cannot silently drift from it.
func SupportedRuntimeOperationVariants() []runtimecontract.RuntimeOperationVariantRef {
	return []runtimecontract.RuntimeOperationVariantRef{
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationStatus},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationDiff},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationLog},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationShow},
		{Kind: gitcommon.StepKindRead, VariantID: gitcommon.OperationBranch},
		{Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationInit},
		{Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationAdd},
		{Kind: gitcommon.StepKindWrite, VariantID: gitcommon.OperationCommit},
	}
}

type gitCommandResult struct {
	Stdout         string
	Stderr         string
	ExitCode       int
	DurationMillis int64
	Truncated      bool
}

type repoContext struct {
	DeliverableDirectory string
	RepoRoot             string
	Branch               string
	HeadCommit           string
	DetachedHead         bool
	UnbornHead           bool
}

func NewRuntimeAdapter(writeEnabled bool) *RuntimeAdapter {
	_ = writeEnabled
	return &RuntimeAdapter{
		timeout: DefaultCommandTimeout,
		envAllowlist: map[string]struct{}{
			"HOME":   {},
			"PATH":   {},
			"TMPDIR": {},
			"LANG":   {},
		},
	}
}

func NewRuntimeAdapterWithTimeout(writeEnabled bool, timeout time.Duration) *RuntimeAdapter {
	adapter := NewRuntimeAdapter(writeEnabled)
	adapter.timeout = timeout
	return adapter
}

func NewRuntimeAdapterFromTimeoutString(writeEnabled bool, timeoutValue string) (*RuntimeAdapter, error) {
	adapter := NewRuntimeAdapter(writeEnabled)
	timeoutValue = strings.TrimSpace(timeoutValue)
	if timeoutValue == "" {
		return adapter, nil
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return nil, fmt.Errorf("%w: runtime.git.defaultTimeout must be a valid duration", common.ErrInvalidArgument)
	}
	if timeout < time.Second {
		return nil, fmt.Errorf("%w: runtime.git.defaultTimeout must be at least 1s", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return nil, fmt.Errorf("%w: runtime.git.defaultTimeout must not exceed %s", common.ErrInvalidArgument, MaxCommandTimeout)
	}
	adapter.timeout = timeout
	return adapter, nil
}

func (a *RuntimeAdapter) ExecuteOperation(ctx context.Context, request RuntimeOperationRequest) (RuntimeOperationResult, error) {
	operation := strings.ToLower(strings.TrimSpace(request.Operation))
	if operation == "" {
		return a.rejectedResult(request, operation, gitcommon.ErrorInvalidRequest, common.ErrInvalidArgument)
	}
	timeout, err := gitCommandTimeout(request.TimeoutMillis, a.timeout)
	if err != nil {
		return a.rejectedResult(request, operation, gitcommon.ErrorInvalidRequest, err)
	}
	if operation == gitcommon.OperationInit {
		if request.Kind != gitcommon.StepKindWrite {
			return a.rejectedResult(request, operation, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: git init requires git/write", common.ErrInvalidArgument))
		}
		return a.executeInit(ctx, request, timeout)
	}
	repo, err := a.resolveRepo(ctx, request, timeout)
	if err != nil {
		return a.failedResult(request, operation, gitcommon.ErrorNotGitRepository, err)
	}
	switch request.Kind {
	case gitcommon.StepKindRead:
		return a.executeRead(ctx, request, repo, operation, timeout)
	case gitcommon.StepKindWrite:
		return a.executeWrite(ctx, request, repo, operation, timeout)
	default:
		return a.rejectedResultWithRepo(request, operation, repo, gitcommon.ErrorOperationDenied, fmt.Errorf("%w: unsupported git step kind %q", common.ErrInvalidArgument, request.Kind))
	}
}
