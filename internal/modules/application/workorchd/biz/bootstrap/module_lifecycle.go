package bootstrap

import (
	"context"
	"errors"
	"fmt"
)

type runtimeModuleLifecycle struct {
	modules []lifecycle
}

func newRuntimeModuleLifecycle(modules []lifecycle) (*runtimeModuleLifecycle, error) {
	ret := &runtimeModuleLifecycle{}
	seen := map[string]struct{}{}
	for _, item := range modules {
		if item == nil {
			continue
		}
		id := item.ID()
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("duplicate module id: %s", id)
		}
		seen[id] = struct{}{}
		ret.modules = append(ret.modules, item)
	}
	return ret, nil
}

func (m *runtimeModuleLifecycle) Run() error {
	if m == nil {
		return nil
	}
	for _, item := range m.modules {
		if item == nil {
			continue
		}
		if err := item.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (m *runtimeModuleLifecycle) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	var shutdownErr error
	for idx := len(m.modules) - 1; idx >= 0; idx-- {
		item := m.modules[idx]
		if item == nil {
			continue
		}
		if ctx == nil {
			ctx = context.Background()
		}
		if err := item.Teardown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	m.modules = nil
	return shutdownErr
}
