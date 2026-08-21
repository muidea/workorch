package web

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"

	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
)

type policyDecision struct {
	Decision string
	Reasons  []string
}

func (a *RuntimeAdapter) classifyURL(ctx context.Context, rawURL string, provider webmodels.ProviderConfig, fetch bool) policyDecision {
	parsed, decision := parseWebURLPolicy(rawURL, provider)
	if parsed == nil || decision.Decision == policyDenied || !fetch {
		return decision
	}
	addresses, err := a.resolveHostAddresses(ctx, parsed.Hostname())
	if err != nil {
		return policyDecision{Decision: policyDenied, Reasons: []string{"dns_lookup_failed"}}
	}
	return mergeURLAndAddressPolicy(decision, classifyAddresses(addresses))
}

func (a *RuntimeAdapter) classifyHostIPs(ctx context.Context, host string) policyDecision {
	addresses, err := a.resolveHostAddresses(ctx, host)
	if err != nil {
		return policyDecision{Decision: policyDenied, Reasons: []string{"dns_lookup_failed"}}
	}
	return classifyAddresses(addresses)
}

func parseWebURLPolicy(rawURL string, provider webmodels.ProviderConfig) (*url.URL, policyDecision) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || len(rawURL) > webURLMaxBytes {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"invalid_url"}}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"invalid_url"}}
	}
	if parsed.User != nil {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"userinfo_url"}}
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"unsupported_scheme"}}
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	if host == "" || strings.Contains(host, "%") {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"missing_host"}}
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value <= 0 || value > 65535 {
			return nil, policyDecision{Decision: policyDenied, Reasons: []string{"invalid_port"}}
		}
	}
	if metadataHost(host) {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"metadata_endpoint"}}
	}
	if domainDenied(host, provider.DenyDomains) {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"deny_domain"}}
	}
	if len(provider.AllowDomains) > 0 && !domainDenied(host, provider.AllowDomains) {
		return nil, policyDecision{Decision: policyDenied, Reasons: []string{"domain_not_allowed"}}
	}
	parsed.Scheme = scheme
	parsed.Fragment = ""
	reasons := []string(nil)
	if scheme == "http" {
		reasons = append(reasons, "plain_http")
	}
	if len(reasons) != 0 {
		return parsed, policyDecision{Decision: policyApproval, Reasons: stablePolicyReasons(reasons)}
	}
	return parsed, policyDecision{Decision: policyAllowed}
}

func (a *RuntimeAdapter) resolveHostAddresses(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address.Unmap()}, nil
	}
	ips, err := a.netResolver().LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	seen := map[string]netip.Addr{}
	for _, ip := range ips {
		address, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			return nil, fmt.Errorf("invalid resolved address")
		}
		address = address.Unmap()
		seen[address.String()] = address
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("host resolved to no addresses")
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]netip.Addr, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, seen[key])
	}
	return ret, nil
}

func classifyAddresses(addresses []netip.Addr) policyDecision {
	reasons := []string{}
	for _, address := range addresses {
		decision := classifyAddr(address)
		if decision.Decision == policyDenied {
			return decision
		}
		if decision.Decision == policyApproval {
			reasons = append(reasons, decision.Reasons...)
		}
	}
	if len(reasons) > 0 {
		return policyDecision{Decision: policyApproval, Reasons: stablePolicyReasons(reasons)}
	}
	return policyDecision{Decision: policyAllowed}
}

func mergeURLAndAddressPolicy(urlDecision policyDecision, addressDecision policyDecision) policyDecision {
	if urlDecision.Decision == policyDenied {
		return urlDecision
	}
	if addressDecision.Decision == policyDenied {
		return addressDecision
	}
	reasons := append(append([]string(nil), urlDecision.Reasons...), addressDecision.Reasons...)
	if len(reasons) != 0 {
		return policyDecision{Decision: policyApproval, Reasons: stablePolicyReasons(reasons)}
	}
	return policyDecision{Decision: policyAllowed}
}

func stablePolicyReasons(values []string) []string {
	ret := uniqueStrings(values)
	sort.Strings(ret)
	return ret
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

func (a *RuntimeAdapter) netResolver() IPResolver {
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
