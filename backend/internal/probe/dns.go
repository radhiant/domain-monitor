package probe

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"domain-monitor/backend/internal/model"
)

// DNSProber resolves a host and, for apex domains, its nameservers.
type DNSProber struct {
	resolver *net.Resolver
	timeout  time.Duration
}

// NewDNSProber builds a prober using the system resolver.
func NewDNSProber(timeout time.Duration) *DNSProber {
	return &DNSProber{resolver: net.DefaultResolver, timeout: timeout}
}

// Probe records the addresses a host resolves to and how long that took.
// NS records are only looked up for apex domains — every subdomain of a zone
// shares them, so 28 identical queries would tell us nothing new.
func (p *DNSProber) Probe(ctx context.Context, target model.Target) *model.DNSInfo {
	info := &model.DNSInfo{CheckedAt: time.Now().UTC()}

	host := target.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	start := time.Now()
	addrs, err := p.resolver.LookupHost(ctx, host)
	info.ResolveMS = msSince(start)

	if err != nil {
		info.Error = cleanError(err)
		return info
	}
	sort.Strings(addrs)
	info.Addrs = addrs

	if cname, err := p.resolver.LookupCNAME(ctx, host); err == nil {
		cname = strings.TrimSuffix(cname, ".")
		// The resolver echoes the queried name when no CNAME exists.
		if !strings.EqualFold(cname, host) {
			info.CNAME = cname
		}
	}

	if target.IsApex {
		if nss, err := p.resolver.LookupNS(ctx, host); err == nil {
			names := make([]string, 0, len(nss))
			for _, ns := range nss {
				names = append(names, strings.TrimSuffix(ns.Host, "."))
			}
			sort.Strings(names)
			info.NS = names
		}
	}

	return info
}
