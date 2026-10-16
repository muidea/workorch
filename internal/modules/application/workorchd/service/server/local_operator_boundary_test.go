package server

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalOperatorListenerRejectsNonLoopbackBeforeStartup(t *testing.T) {
	for _, addr := range []string{"", ":8080", "0.0.0.0:8080", "[::]:8080", "192.0.2.1:8080", "example.com:8080", "localhost:http", " localhost:8080", "127.0.0.1:08080", "127.0.0.1:65536"} {
		if _, err := localOperatorListenerAddress(addr); err == nil {
			t.Fatalf("accepted noncanonical/non-loopback address %q", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:0", "127.0.0.2:8080", "[::1]:8080", "localhost:8080", "[::ffff:127.0.0.1]:8080"} {
		if _, err := localOperatorListenerAddress(addr); err != nil {
			t.Fatalf("valid loopback %q: %v", addr, err)
		}
	}
	server := NewHTTPServer(Config{Addr: "0.0.0.0:0"})
	if err := server.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("invalid listener reached application validation: %v", err)
	}
	if err := server.Start(); err == nil || server.listener != nil {
		t.Fatal("non-loopback listener admitted")
	}
}

func TestLocalOperatorHTTPBoundaryRejectsSpoofingAndCrossSiteBeforeBusiness(t *testing.T) {
	for _, scenario := range []string{"local", "ipv6", "tls", "local-cli", "remote", "missing-peer", "wildcard-listener", "dns-host", "forwarded", "real-ip", "forwarded-host", "cross-origin", "same-site", "null-origin", "empty-origin", "multiple-origin", "origin-userinfo", "wrong-port", "origin-path", "empty-host-port", "empty-origin-port", "unknown-fetch-site", "empty-fetch-site", "multiple-fetch-site"} {
		t.Run(scenario, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/v1/workorch/workspaces", strings.NewReader("原始\r\n\x00\xff输入"))
			request.RemoteAddr = "127.0.0.1:32100"
			request.Header.Set("Origin", "http://127.0.0.1:8080")
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			request.Header.Set("X-Workorch-Actor", "external-user")
			request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}))
			switch scenario {
			case "local-cli":
				request.Header.Del("Origin")
				request.Header.Del("Sec-Fetch-Site")
			case "ipv6":
				request.RemoteAddr = "[::1]:32100"
				request.Host = "[::1]:8080"
				request.Header.Set("Origin", "http://[::1]:8080")
			case "tls":
				request.TLS = &tls.ConnectionState{}
				request.Header.Set("Origin", "https://127.0.0.1:8080")
			case "remote":
				request.RemoteAddr = "192.0.2.1:32100"
			case "missing-peer":
				request.RemoteAddr = ""
			case "wildcard-listener":
				request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.IPv4zero, Port: 8080}))
			case "dns-host":
				request.Host = "attacker.example:8080"
				request.Header.Set("Origin", "http://attacker.example:8080")
			case "forwarded":
				request.Header.Set("Forwarded", "for=127.0.0.1")
			case "real-ip":
				request.Header.Set("X-Real-IP", "127.0.0.1")
			case "forwarded-host":
				request.Header.Set("X-Forwarded-Host", "127.0.0.1")
			case "cross-origin":
				request.Header.Set("Origin", "http://evil.example")
			case "same-site":
				request.Header.Set("Sec-Fetch-Site", "same-site")
			case "null-origin":
				request.Header.Set("Origin", "null")
			case "multiple-origin":
				request.Header.Add("Origin", "http://127.0.0.1:8080")
			case "empty-origin":
				request.Header.Set("Origin", "")
			case "empty-host-port":
				request.Host = "127.0.0.1:"
			case "empty-origin-port":
				request.Host = "127.0.0.1"
				request.Header.Set("Origin", "http://127.0.0.1:")
			case "origin-userinfo":
				request.Header.Set("Origin", "http://actor@127.0.0.1:8080")
			case "wrong-port":
				request.Header.Set("Origin", "http://127.0.0.1:9090")
			case "origin-path":
				request.Header.Set("Origin", "http://127.0.0.1:8080/path")
			case "unknown-fetch-site":
				request.Header.Set("Sec-Fetch-Site", "unknown")
			case "empty-fetch-site":
				request.Header.Set("Sec-Fetch-Site", "")
			case "multiple-fetch-site":
				request.Header.Add("Sec-Fetch-Site", "cross-site")
			}
			called := false
			handler := localOperatorHTTPBoundary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				identity, ok := localOperatorForRequest(r)
				if !ok || identity.SubjectID != "local-machine-operator" {
					t.Fatal("caller actor was promoted into trusted identity")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != "原始\r\n\x00\xff输入" {
					t.Fatal("authentication modified original input")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			allowed := scenario == "local" || scenario == "ipv6" || scenario == "tls" || scenario == "local-cli"
			if called != allowed || allowed && response.Code != http.StatusNoContent || !allowed && (response.Code != http.StatusForbidden || response.Header().Get("Cache-Control") != "no-store") {
				t.Fatalf("boundary calls=%v code=%d", called, response.Code)
			}
		})
	}
}

func TestProductionHTTPServerIncludesLocalOperatorBoundary(t *testing.T) {
	server := NewHTTPServer(Config{})
	for _, peer := range []string{"127.0.0.1:32100", "192.0.2.1:32100"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/healthz", nil)
		request.RemoteAddr = peer
		response := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(response, request)
		want := http.StatusOK
		if strings.HasPrefix(peer, "192.") {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Fatalf("production peer %s status=%d want=%d", peer, response.Code, want)
		}
	}
}
