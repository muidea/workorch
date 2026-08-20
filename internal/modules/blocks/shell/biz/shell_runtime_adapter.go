package shell

import (
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	DefaultCommandTimeout           = 60 * time.Second
	MaxCommandTimeout               = 10 * time.Minute
	MaxCommandOutputBytes           = 64 * 1024
	maxObservedWorkspaceFiles       = 8192
	maxObservedWorkspaceBytes int64 = 256 * 1024 * 1024
)

type RuntimeAdapter struct {
	timeout      time.Duration
	envAllowlist map[string]struct{}
}

const shellValidationTargetSelector = "workspace-validation-target"

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{
		timeout: DefaultCommandTimeout,
		envAllowlist: map[string]struct{}{
			"GOCACHE":    {},
			"GOMODCACHE": {},
			"HOME":       {},
			"PATH":       {},
			"TMPDIR":     {},
		},
	}
}

func NewRuntimeAdapterWithTimeout(timeout time.Duration) *RuntimeAdapter {
	adapter := NewRuntimeAdapter()
	adapter.timeout = timeout
	return adapter
}

func NewRuntimeAdapterFromTimeoutString(timeoutValue string) (*RuntimeAdapter, error) {
	adapter := NewRuntimeAdapter()
	timeoutValue = strings.TrimSpace(timeoutValue)
	if timeoutValue == "" {
		return adapter, nil
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must be a valid duration", common.ErrInvalidArgument)
	}
	if timeout < time.Second {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must be at least 1s", common.ErrInvalidArgument)
	}
	if timeout > MaxCommandTimeout {
		return nil, fmt.Errorf("%w: runtime.shellCommandTimeout must not exceed %s", common.ErrInvalidArgument, MaxCommandTimeout)
	}
	adapter.timeout = timeout
	return adapter, nil
}
