package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/application"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	repository "github.com/muidea/workorch/internal/initiators/workorchruntime"
	repositorycommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	"github.com/muidea/workorch/internal/services/workorch"
)

const productionLifecycleScenario = "WORKORCH_TEST_PRODUCTION_LIFECYCLE_SCENARIO"

// 独立进程隔离全局 plugin 阶段与配置；只加载 main.go 选择的真实参与者。
func TestProductionAssemblyFailureRecovery(t *testing.T) {
	for _, scenario := range []string{"background-timeout", "nested-send-timeout", "unsubscribe-error", "setup-error", "setup-panic"} {
		t.Run(scenario, func(t *testing.T) {
			configDir := writeWorkorchTestConfig(t, reserveLoopbackAddr(t), nil)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProductionAssemblyFailureRecoveryHelper$", "-test.timeout=25s")
			cmd.Env = append(os.Environ(), productionLifecycleScenario+"="+scenario, "WORKORCH_TEST_PRODUCTION_CONFIG="+configDir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("production %s: %v\n%s", scenario, err, out)
			}
		})
	}
}

type productionFaultHub struct {
	magicevent.DrainingHub
	mu               sync.Mutex
	setupFault       bool
	setupPanic       bool
	unsubscribeFault bool
	failure          error
	subscribeCalls   int
	unsubscribeCalls int
}

func (h *productionFaultHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.mu.Lock()
	h.subscribeCalls++
	fail := h.setupFault && h.subscribeCalls == 3
	panicOnFailure := h.setupPanic
	h.mu.Unlock()
	if fail {
		if panicOnFailure {
			panic(cd.WrapError(cd.Unexpected, h.failure, "production subscription panic"))
		}
		return cd.WrapError(cd.Unexpected, h.failure, "production subscription rejected")
	}
	return h.DrainingHub.Subscribe(topic, observer)
}

func (h *productionFaultHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.mu.Lock()
	h.unsubscribeCalls++
	fail := h.unsubscribeFault
	h.mu.Unlock()
	if fail {
		return cd.WrapError(cd.Unexpected, h.failure, "production unsubscribe rejected")
	}
	return h.DrainingHub.Unsubscribe(topic, observer)
}

func TestProductionAssemblyFailureRecoveryHelper(t *testing.T) {
	scenario := os.Getenv(productionLifecycleScenario)
	if scenario == "" {
		t.Skip("isolated child helper")
	}
	ctx := context.Background()
	configDir := os.Getenv("WORKORCH_TEST_PRODUCTION_CONFIG")
	workorch.ConfigureDirectory(configDir)
	failure := errors.New("injected production lifecycle failure")
	hub := &productionFaultHub{DrainingHub: magicevent.NewHub(64).(magicevent.DrainingHub), failure: failure}
	setupFailure := strings.HasPrefix(scenario, "setup-")
	hub.setupFault, hub.setupPanic, hub.unsubscribeFault = setupFailure, scenario == "setup-panic", setupFailure
	app := application.NewApplication(application.Options{
		ConfigDir: configDir, ServiceName: "workorch", EventHub: hub,
		Ownership: application.RuntimeOwnership{EventHub: true},
	})
	svc := frameworkservice.DefaultService()
	startupErr := app.Startup(ctx, svc)
	if setupFailure {
		if !errors.Is(startupErr, failure) {
			t.Fatalf("startup cause=%v", startupErr)
		}
	} else {
		if startupErr != nil {
			t.Fatal(startupErr)
		}
		if err := app.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := initiator.GetEntity(repositorycommon.InitiatorID, (*repository.RepositoryInitiator)(nil))
	if err != nil {
		t.Fatal(err)
	}
	original := provider.RepositoryProvider()
	if original == nil {
		t.Fatal("startup or failed cleanup discarded provider")
	}
	var release chan struct{}
	var callbackDone chan struct{}
	switch scenario {
	case "background-timeout":
		entered := make(chan struct{})
		release, callbackDone = make(chan struct{}), make(chan struct{})
		if err := app.BackgroundRoutine().AsyncFunction(func() { close(entered); <-release; close(callbackDone) }); err != nil {
			t.Fatal(err)
		}
		<-entered
	case "nested-send-timeout":
		entered := make(chan struct{})
		release, callbackDone = make(chan struct{}), make(chan struct{})
		inner := magicevent.NewSimpleObserver("lifecycle-inner", hub)
		outer := magicevent.NewSimpleObserver("lifecycle-outer", hub)
		if err := inner.Subscribe("lifecycle-inner", func(_ magicevent.Event, result magicevent.Result) {
			close(entered)
			<-release
			result.Set(struct{}{}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		if err := outer.Subscribe("lifecycle-outer", func(ev magicevent.Event, _ magicevent.Result) {
			defer close(callbackDone)
			result := hub.Send(magicevent.NewEventWithContext("lifecycle-inner", "test", "lifecycle-inner", nil, ev.Context(), nil))
			if err := result.Error(); err != nil {
				t.Errorf("nested Send=%v", err)
			}
		}); err != nil {
			t.Fatal(err)
		}
		hub.Post(magicevent.NewEventWithContext("lifecycle-outer", "test", "lifecycle-outer", nil, ctx, nil))
		<-entered
	case "unsubscribe-error":
		hub.mu.Lock()
		hub.unsubscribeFault = true
		hub.mu.Unlock()
	}
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	stopErr := app.ShutdownChecked(short)
	cancel()
	if stopErr == nil {
		t.Fatal("incomplete shutdown reported success")
	}
	if provider.RepositoryProvider() != original {
		t.Fatal("failed shutdown released or replaced provider")
	}
	if scenario == "background-timeout" || scenario == "nested-send-timeout" {
		assertProductionSoulQuery(t, hub)
		select {
		case <-callbackDone:
			t.Fatal("blocked callback reported exit")
		default:
		}
		close(release)
		<-callbackDone
	}
	hub.mu.Lock()
	hub.unsubscribeFault = false
	hub.mu.Unlock()
	if err := app.ShutdownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if provider.RepositoryProvider() != nil {
		t.Fatal("completed shutdown retained provider")
	}
	hub.mu.Lock()
	unsubscribes := hub.unsubscribeCalls
	hub.mu.Unlock()
	if err := app.ShutdownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	hub.mu.Lock()
	repeated := hub.unsubscribeCalls
	hub.mu.Unlock()
	if repeated != unsubscribes {
		t.Fatal("idempotent shutdown repeated release")
	}
}

func assertProductionSoulQuery(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	ev := magicevent.NewEventWithContext(soulevents.TopicSoulCommandList, "application/workorchd/context-registry", soulevents.CommandDestination, nil, context.Background(), soulevents.ListSoulsCommand{Limit: 10})
	data, err := hub.Send(ev).Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := data.(soulevents.ListSoulsResult); !ok {
		t.Fatalf("retained owner query=%T", data)
	}
}
