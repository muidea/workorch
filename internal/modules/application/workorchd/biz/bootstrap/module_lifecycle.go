package bootstrap

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
)

type modulePluginLifecycle struct {
	manager *plugincommon.PluginMgr
	modules []*modulePluginAdapter
}

type modulePluginAdapter struct {
	module lifecycle
	ctx    context.Context
}

func newModulePluginLifecycle(modules []lifecycle) (*modulePluginLifecycle, error) {
	manager := plugincommon.NewPluginMgr("workorch-runtime-module")
	ret := &modulePluginLifecycle{manager: manager}
	for _, item := range modules {
		adapter := &modulePluginAdapter{module: item}
		if err := manager.Register(adapter); err != nil {
			return nil, err
		}
		ret.modules = append(ret.modules, adapter)
	}
	return ret, nil
}

func (m *modulePluginLifecycle) Run() error {
	if m == nil || m.manager == nil {
		return nil
	}
	if err := m.manager.Run(context.Background()); err != nil {
		return err
	}
	return nil
}

func (m *modulePluginLifecycle) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	for idx := len(m.modules) - 1; idx >= 0; idx-- {
		if err := m.modules[idx].shutdown(ctx); err != nil {
			return err
		}
	}
	m.modules = nil
	return nil
}

func (m *modulePluginAdapter) ID() string {
	if m == nil || m.module == nil {
		return ""
	}
	return m.module.ID()
}

func (m *modulePluginAdapter) Run(_ context.Context) *cd.Error {
	if m == nil || m.module == nil {
		return nil
	}
	if err := m.module.Run(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *modulePluginAdapter) Teardown(ctx context.Context) *cd.Error {
	if err := m.shutdown(ctx); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *modulePluginAdapter) shutdown(ctx context.Context) error {
	if m == nil || m.module == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return m.module.Teardown(ctx)
}
