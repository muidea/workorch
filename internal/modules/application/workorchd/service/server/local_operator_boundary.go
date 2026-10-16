package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const localOperatorBoundaryV1 = "workorch-local-operator/v1"

// This first-release boundary trusts local-machine operators, not a named OS
// account, browser-supplied actor, proxy or external identity provider.
type localOperatorIdentity struct {
	ProtocolRef     string
	SubjectID       string
	PermissionBasis string
}

type localOperatorContextKey struct{}

func localOperatorForRequest(r *http.Request) (localOperatorIdentity, bool) {
	if r == nil {
		return localOperatorIdentity{}, false
	}
	identity, ok := r.Context().Value(localOperatorContextKey{}).(localOperatorIdentity)
	return identity, ok && identity.ProtocolRef == localOperatorBoundaryV1 && identity.SubjectID == "local-machine-operator" && identity.PermissionBasis == "loopback-peer-and-same-origin/v1"
}

// No hostname resolution, wildcard bind or proxy interpretation. localhost is
// a fixed local convenience spelling, never resolved via DNS.
func localOperatorListenerAddress(addr string) (string, error) {
	if addr != strings.TrimSpace(addr) {
		return "", fmt.Errorf("%s requires a canonical loopback listener", localOperatorBoundaryV1)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("%s requires an explicit loopback host and numeric port", localOperatorBoundaryV1)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 0 || number > 65535 || strconv.Itoa(number) != port {
		return "", fmt.Errorf("%s requires a numeric listener port", localOperatorBoundaryV1)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("%s refuses non-loopback listeners", localOperatorBoundaryV1)
	}
	return net.JoinHostPort(host, port), nil
}

func localOperatorHTTPBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := validateLocalOperatorRequest(r); err != nil {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, localOperatorBoundaryV1+": local operator access required", http.StatusForbidden)
			return
		}
		identity := localOperatorIdentity{ProtocolRef: localOperatorBoundaryV1, SubjectID: "local-machine-operator", PermissionBasis: "loopback-peer-and-same-origin/v1"}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), localOperatorContextKey{}, identity)))
	})
}

func validateLocalOperatorRequest(r *http.Request) error {
	if r == nil {
		return fmt.Errorf("missing request")
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() {
		return fmt.Errorf("non-loopback peer")
	}
	// Transport-provided local address is checked when present. Direct handler
	// tests cannot supply network provenance, so only the peer gate is tested there.
	if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		host, _, err := net.SplitHostPort(local.String())
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("non-loopback listener")
		}
	}
	// A loopback reverse proxy must not turn remote clients into local actors.
	for name := range r.Header {
		if strings.EqualFold(name, "Forwarded") || strings.EqualFold(name, "X-Real-IP") || strings.HasPrefix(strings.ToLower(name), "x-forwarded-") {
			return fmt.Errorf("proxy provenance is not supported")
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	endpoint, err := localOperatorEndpoint(scheme, r.Host)
	if err != nil {
		return err
	} // Host allowlist rejects DNS rebinding.
	origins := r.Header.Values("Origin")
	if len(origins) > 1 || len(origins) == 1 && origins[0] == "" {
		return fmt.Errorf("ambiguous origin")
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid origin")
		}
		other, err := localOperatorEndpoint(u.Scheme, u.Host)
		if err != nil || other != endpoint {
			return fmt.Errorf("cross-origin request")
		}
	}
	sites := r.Header.Values("Sec-Fetch-Site")
	if len(sites) > 1 || len(sites) == 1 && sites[0] == "" {
		return fmt.Errorf("ambiguous fetch site")
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return fmt.Errorf("cross-site request")
	}
	return nil
}

func localOperatorEndpoint(scheme, authority string) (string, error) {
	if scheme != "http" && scheme != "https" || authority == "" || authority != strings.TrimSpace(authority) || strings.ContainsAny(authority, "/\\@?#") {
		return "", fmt.Errorf("invalid local endpoint")
	}
	u, err := url.Parse(scheme + "://" + authority)
	if err != nil || u.User != nil || strings.HasSuffix(authority, ":") {
		return "", fmt.Errorf("invalid local endpoint")
	}
	host := u.Hostname()
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("non-loopback host")
		}
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if scheme == "https" {
			port = "443"
		}
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return "", fmt.Errorf("invalid local port")
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}
