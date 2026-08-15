package web

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"

	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
)

type policyDecision struct {
	Decision string
	Reasons  []string
}

func (a *RuntimeAdapter) classifyURL(ctx context.Context, rawURL string, provider webmodels.ProviderConfig, fetch bool) policyDecision {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return policyDecision{Decision: policyDenied, Reasons: []string{"invalid_url"}}
	}
	if parsed.User != nil {
		return policyDecision{Decision: policyDenied, Reasons: []string{"userinfo_url"}}
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return policyDecision{Decision: policyDenied, Reasons: []string{"unsupported_scheme"}}
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	if host == "" {
		return policyDecision{Decision: policyDenied, Reasons: []string{"missing_host"}}
	}
	if metadataHost(host) {
		return policyDecision{Decision: policyDenied, Reasons: []string{"metadata_endpoint"}}
	}
	if domainDenied(host, provider.DenyDomains) {
		return policyDecision{Decision: policyDenied, Reasons: []string{"deny_domain"}}
	}
	if len(provider.AllowDomains) > 0 && !domainDenied(host, provider.AllowDomains) {
		return policyDecision{Decision: policyDenied, Reasons: []string{"domain_not_allowed"}}
	}
	reasons := []string{}
	if scheme == "http" {
		reasons = append(reasons, "plain_http")
	}
	if fetch {
		ipDecision := a.classifyHostIPs(ctx, host)
		if ipDecision.Decision == policyDenied {
			return ipDecision
		}
		if ipDecision.Decision == policyApproval {
			reasons = append(reasons, ipDecision.Reasons...)
		}
	}
	if len(reasons) > 0 {
		return policyDecision{Decision: policyApproval, Reasons: uniqueStrings(reasons)}
	}
	return policyDecision{Decision: policyAllowed}
}

func (a *RuntimeAdapter) classifyHostIPs(ctx context.Context, host string) policyDecision {
	if addr, err := netip.ParseAddr(host); err == nil {
		return classifyAddr(addr)
	}
	ips, err := a.netResolver().LookupIPAddr(ctx, host)
	if err != nil {
		return policyDecision{Decision: policyDenied, Reasons: []string{"dns_lookup_failed"}}
	}
	reasons := []string{}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			return policyDecision{Decision: policyDenied, Reasons: []string{"invalid_resolved_ip"}}
		}
		decision := classifyAddr(addr)
		if decision.Decision == policyDenied {
			return decision
		}
		if decision.Decision == policyApproval {
			reasons = append(reasons, decision.Reasons...)
		}
	}
	if len(reasons) > 0 {
		return policyDecision{Decision: policyApproval, Reasons: uniqueStrings(reasons)}
	}
	return policyDecision{Decision: policyAllowed}
}

func classifyAddr(addr netip.Addr) policyDecision {
	if addr.IsLoopback() {
		return policyDecision{Decision: policyApproval, Reasons: []string{"loopback"}}
	}
	if addr.IsPrivate() {
		return policyDecision{Decision: policyApproval, Reasons: []string{"private_network"}}
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return policyDecision{Decision: policyDenied, Reasons: []string{"link_local"}}
	}
	if addr.IsUnspecified() {
		return policyDecision{Decision: policyDenied, Reasons: []string{"unspecified_address"}}
	}
	if addr.IsMulticast() {
		return policyDecision{Decision: policyDenied, Reasons: []string{"multicast_address"}}
	}
	if addr.String() == "169.254.169.254" || addr.String() == "100.100.100.200" || strings.EqualFold(addr.String(), "fd00:ec2::254") {
		return policyDecision{Decision: policyDenied, Reasons: []string{"metadata_endpoint"}}
	}
	return policyDecision{Decision: policyAllowed}
}

func (a *RuntimeAdapter) netResolver() *net.Resolver {
	if a != nil && a.resolver != nil {
		return a.resolver
	}
	return net.DefaultResolver
}

func metadataHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "metadata.google.internal", "169.254.169.254", "100.100.100.200", "fd00:ec2::254":
		return true
	default:
		return false
	}
}

func domainDenied(host string, domains []string) bool {
	host = strings.ToLower(strings.Trim(host, "."))
	for _, domain := range domains {
		domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
		if domain == "" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
