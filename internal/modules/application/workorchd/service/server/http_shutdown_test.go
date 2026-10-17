package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestHTTPServerStreamReplayCancellationStillWaitsForRealReaderExit(t *testing.T) {
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	state := newServerStateBase(Config{}, nil, applicationOperations{runOperations: runOperations{
		listEventRecords: func(ctx context.Context, _, _ string, _ workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-release
			return nil, ctx.Err()
		},
	}})
	srv := newHTTPServerWithState(Config{}, state)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.serve(listener) }()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/api/v1/workorch/events/stream?view=full&lastEventId=event-1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("replay never entered reader")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := srv.CloseTransportState(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation is not reader completion: %v", err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("shutdown did not cancel stream replay")
	}
	select {
	case <-state.backgroundCtx.Done():
		t.Fatal("canceled replay released shared dependencies before reader exit")
	default:
	}
	close(release)
	ctx, retryCancel := context.WithTimeout(context.Background(), time.Second)
	defer retryCancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPServerDrainEndsOpenEventStreamsBeforeReleasingState(t *testing.T) {
	state := newServerStateBase(Config{}, nil, applicationOperations{})
	srv := newHTTPServerWithState(Config{}, state)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.serve(listener) }()
	client := &http.Client{Timeout: 3 * time.Second}
	for range 2 {
		response, err := client.Get("http://" + listener.Addr().String() + "/api/v1/workorch/events/stream")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		line, err := bufio.NewReader(response.Body).ReadString('\n')
		if err != nil || line != ": connected\n" {
			t.Fatalf("stream handshake: %q %v", line, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Quiesce(ctx); err != nil {
		t.Fatalf("open browser streams prevented quiescence: %v", err)
	}
	if err := <-serveDone; err != nil {
		t.Fatal(err)
	}
	state.streamSubscribersMu.Lock()
	subscribers := len(state.streamSubscribers)
	state.streamSubscribersMu.Unlock()
	if subscribers != 0 {
		t.Fatalf("stream handlers have not unsubscribed: %d", subscribers)
	}
	select {
	case <-state.backgroundCtx.Done():
		t.Fatal("quiescence prematurely released transport dependencies")
	default:
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPServerDrainWaitsForHandlerAfterHijackedConnectionCloses(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	state := newServerStateBase(Config{}, nil, applicationOperations{})
	srv := newHTTPServerWithHandler(Config{}, state, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
		close(started)
		<-release
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.serve(listener) }()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not hijack connection")
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	srv.mu.Lock()
	connections, handlers := len(srv.connections), srv.activeHandlers
	srv.mu.Unlock()
	if connections != 0 || handlers != 1 {
		t.Fatalf("hijacked closed connection incorrectly completed handler: connections=%d handlers=%d", connections, handlers)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := srv.CloseTransportState(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseTransportState = %v, want actual-handler deadline", err)
	}
	select {
	case <-state.backgroundCtx.Done():
		t.Fatal("connection closure released handler dependencies")
	default:
	}
	close(release)
	ctx, cancelRetry := context.WithTimeout(context.Background(), time.Second)
	defer cancelRetry()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := <-serveDone; err != nil {
		t.Fatalf("serve: %v", err)
	}
}

func TestHTTPServerDrainTracksAcceptedConnectionBeforeAnyHandler(t *testing.T) {
	var handlers atomic.Int64
	srv := newHTTPServerWithHandler(Config{}, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		handlers.Add(1)
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.serve(listener) }()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: localhost\r\n")
	deadline := time.After(time.Second)
	for {
		srv.mu.Lock()
		accepted := len(srv.connections) == 1
		srv.mu.Unlock()
		if accepted {
			break
		}
		select {
		case <-deadline:
			t.Fatal("connection was not registered")
		case <-time.After(time.Millisecond):
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := srv.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain accepted partial-header connection = %v", err)
	}
	if handlers.Load() != 0 {
		t.Fatal("incomplete request was admitted as business handler")
	}
	_ = client.Close()
	ctx, retryCancel := context.WithTimeout(context.Background(), time.Second)
	defer retryCancel()
	if err := srv.Drain(ctx); err != nil {
		t.Fatalf("connection retry drain: %v", err)
	}
	if err := <-serveDone; err != nil {
		t.Fatalf("serve: %v", err)
	}
}

func TestHTTPServerHandlerGatePreservesRequestAndRejectsLateAdmission(t *testing.T) {
	var calls atomic.Int64
	request := httptest.NewRequest(http.MethodPost, "/original?q=a%2Bb", strings.NewReader("原始输入"))
	request.Header.Set("Authorization", "Bearer original")
	writer := httptest.NewRecorder()
	srv := newHTTPServerWithHandler(Config{}, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if w != writer || r != request {
			t.Error("wrapper changed request or ResponseWriter identity")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "原始输入" || r.Header.Get("Authorization") != "Bearer original" || r.URL.RawQuery != "q=a%2Bb" {
			t.Error("wrapper changed business request contents")
		}
	}))
	srv.http.Handler.ServeHTTP(writer, request)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.BeginShutdown(ctx); err != nil {
		t.Fatalf("canceled BeginShutdown: %v", err)
	}
	writer = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatalf("late admission: code=%d calls=%d", writer.Code, calls.Load())
	}
	if err := srv.Drain(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Drain = %v", err)
	}
	if err := srv.Quiesce(context.Background()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start after BeginShutdown: %v", err)
	}
}

type closeRetryListener struct {
	net.Listener
	closeCalls int
	closeErr   error
}

func (l *closeRetryListener) Close() error {
	l.closeCalls++
	if l.closeCalls == 1 {
		return l.closeErr
	}
	return nil
}

func TestHTTPServerBeginShutdownRetainsListenerCloseFailureAndRetriesOnce(t *testing.T) {
	closeErr := errors.New("listener close failed")
	listener := &closeRetryListener{closeErr: closeErr}
	srv := newHTTPServerWithHandler(Config{}, nil, http.NotFoundHandler())
	srv.listener = &shutdownListener{Listener: listener, server: srv}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.BeginShutdown(ctx); !errors.Is(err, closeErr) {
		t.Fatalf("BeginShutdown = %v, want listener failure", err)
	}
	if err := srv.BeginShutdown(context.Background()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if listener.closeCalls != 2 {
		t.Fatalf("listener closed %d times, want failed attempt + one successful attempt", listener.closeCalls)
	}
}

func TestHTTPServerHandlerAdmissionRacesDrainWithoutLostExits(t *testing.T) {
	for range 25 {
		var active atomic.Int64
		srv := newHTTPServerWithHandler(Config{}, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			active.Add(1)
			defer active.Add(-1)
		}))
		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				srv.http.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			})
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := srv.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
		cancel()
		if active.Load() != 0 {
			t.Fatal("Drain returned while admitted business handler was running")
		}
		wg.Wait()
	}
}

func TestHTTPServerHandlerPanicPropagatesAndReleasesExitLedger(t *testing.T) {
	panicValue := &struct{ reason string }{"unknown panic"}
	srv := newHTTPServerWithHandler(Config{}, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(panicValue)
	}))
	func() {
		defer func() {
			if recovered := recover(); recovered != panicValue {
				t.Fatalf("recovered = %v, want original unknown panic", recovered)
			}
		}()
		srv.http.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	if err := srv.Drain(context.Background()); err != nil {
		t.Fatalf("panic left drain ledger unfinished: %v", err)
	}
}

func TestServiceCheckedShutdownQueuedRunRetainsStateUntilRealFunctionExit(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, routine),
		done: make(chan error, 1),
	}
	configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)
	if err := svc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := svc.ShutdownChecked(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ShutdownChecked = %v, want queued server task deadline", err)
	}
	select {
	case <-svc.http.state.backgroundCtx.Done():
		t.Fatal("queued task timeout released transport state")
	default:
	}
	routine.function()
	// Another consumer may read Done without consuming the drain completion.
	<-svc.Done()
	if err := svc.Quiesce(context.Background()); err != nil {
		t.Fatalf("Quiesce retry: %v", err)
	}
	if err := svc.CloseTransportStateChecked(context.Background()); err != nil {
		t.Fatalf("CloseTransportStateChecked: %v", err)
	}
	if err := svc.ShutdownChecked(context.Background()); err != nil {
		t.Fatalf("repeated ShutdownChecked: %v", err)
	}
}

func TestServiceCheckedDrainReleasesExitedServerButRetainsRunError(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	svc := &Service{
		http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "invalid address", Backend: "memory"}, routine),
		done: make(chan error, 1),
	}
	configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)
	var stopped workorchdevents.ServiceStoppedData
	svc.SetServiceStoppedNotifier(func(data workorchdevents.ServiceStoppedData) { stopped = data })
	if err := svc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	routine.function()
	serveErr := <-svc.Done()
	if serveErr == nil {
		t.Fatal("invalid address did not fail HTTP startup")
	}
	if stopped.State != "failed" || stopped.ErrorMessage != serveErr.Error() || !errors.Is(svc.runErr, serveErr) {
		t.Fatalf("run failure not observable: stopped=%+v runErr=%v Done=%v", stopped, svc.runErr, serveErr)
	}
	for range 2 {
		if err := svc.ShutdownChecked(context.Background()); err != nil {
			t.Fatalf("checked cleanup of exited server = %v", err)
		}
	}
	if !errors.Is(svc.runErr, serveErr) {
		t.Fatal("checked cleanup erased original run error")
	}
	select {
	case <-svc.http.state.backgroundCtx.Done():
	default:
		t.Fatal("checked cleanup did not free truly exited server dependencies")
	}
}

func TestServiceCheckedShutdownPartialStartupAndCancellation(t *testing.T) {
	svc := NewService(Config{Backend: "memory"})
	if err := svc.StartupWithApplicationPorts(context.Background(), "test", ApplicationPorts{}, &recordingBackgroundRoutine{}); err == nil {
		t.Fatal("invalid ports unexpectedly started")
	}
	original := svc.http
	if err := svc.StartupWithApplicationPorts(context.Background(), "test", ApplicationPorts{}, &recordingBackgroundRoutine{}); err == nil {
		t.Fatal("partial startup state was allowed to be overwritten")
	}
	if svc.http != original {
		t.Fatal("partial startup lost its cleanup handle")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.ShutdownChecked(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ShutdownChecked = %v", err)
	}
	select {
	case <-original.state.backgroundCtx.Done():
		t.Fatal("canceled shutdown released transport dependencies")
	default:
	}
	if err := svc.ShutdownChecked(context.Background()); err != nil {
		t.Fatalf("partial startup retry shutdown: %v", err)
	}
	if err := svc.ShutdownChecked(context.Background()); err != nil {
		t.Fatalf("repeated partial startup shutdown: %v", err)
	}
}

func TestServerStateCloseTimeoutRetainsCleanupLedgerAndBlocksNewTasks(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newServerStateBase(Config{}, routine, applicationOperations{})
	var cleanupCalls int
	state.eventUnsubscribes = []func(){func() { cleanupCalls++ }}
	if !state.scheduleBackground("accepted", func(context.Context) {}) {
		t.Fatal("task not accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := state.close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close = %v", err)
	}
	if cleanupCalls != 0 || len(state.eventUnsubscribes) != 1 {
		t.Fatal("timeout released or discarded cleanup ledger")
	}
	if state.scheduleBackground("late", func(context.Context) {}) {
		t.Fatal("accepted task after close gate")
	}
	routine.function()
	if err := state.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := state.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup called %d times, want once", cleanupCalls)
	}
}

func TestServiceRunAdmissionRacesBeginShutdown(t *testing.T) {
	for range 20 {
		routine := &recordingBackgroundRoutine{}
		svc := &Service{
			http: newIsolatedTestHTTPServerWithBackground(Config{Addr: "127.0.0.1:0", Backend: "memory"}, routine),
			done: make(chan error, 1),
		}
		configureIsolatedTestSession(t, svc.http.state, svc.http.state.config)
		var wg sync.WaitGroup
		wg.Go(func() { _ = svc.Run(context.Background()) })
		wg.Go(func() {
			if err := svc.BeginShutdown(context.Background()); err != nil {
				t.Errorf("BeginShutdown: %v", err)
			}
		})
		wg.Wait()
		if routine.function != nil {
			routine.function()
		}
		if err := svc.ShutdownChecked(context.Background()); err != nil {
			t.Fatalf("ShutdownChecked after admission race: %v", err)
		}
		if err := svc.Run(context.Background()); err == nil {
			t.Fatal("Run accepted after completed shutdown")
		}
	}
}

func TestServerStateConcurrentCloseHonorsWaitingCallerCancellation(t *testing.T) {
	routine := &recordingBackgroundRoutine{}
	state := newServerStateBase(Config{}, routine, applicationOperations{})
	if !state.scheduleBackground("accepted", func(context.Context) {}) {
		t.Fatal("task not accepted")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- state.close(context.Background()) }()
	select {
	case <-state.backgroundCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("first close did not enter task drain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := state.close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent close = %v, want waiting caller deadline", err)
	}
	routine.function()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("first close did not observe real task exit")
	}
}

func TestServerStateUnknownCleanupPanicKeepsEntryForRetry(t *testing.T) {
	state := newServerStateBase(Config{}, nil, applicationOperations{})
	panicValue := &struct{}{}
	var calls int
	state.eventUnsubscribes = []func(){func() {
		calls++
		if calls == 1 {
			panic(panicValue)
		}
	}}
	func() {
		defer func() {
			if recovered := recover(); recovered != panicValue {
				t.Fatalf("cleanup panic = %v", recovered)
			}
		}()
		_ = state.close(context.Background())
	}()
	if state.closed || len(state.eventUnsubscribes) != 1 {
		t.Fatal("panic incorrectly completed cleanup")
	}
	if err := state.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !state.closed {
		t.Fatal("cleanup retry did not retain unfinished entry")
	}
}
