package main

import (
	"net"
	"net/url"
	"strings"
	"unicode"
)

type mcpNavigationPolicy struct {
	WarnPrivate bool
	Allowlist   map[string]bool
}

type navigationWarning struct {
	URL     string `json:"url"`
	Host    string `json:"host"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func normalizeMCPPrivateAllowlist(raw string) map[string]bool {
	allowlist := map[string]bool{}
	for _, entry := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	}) {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry), "."))
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		if host == "" || strings.ContainsAny(host, "/?#@*") {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			host = ip.String()
		}
		allowlist[host] = true
	}
	return allowlist
}

func (a *app) setMCPNavigationPolicy(settings userSettings) {
	a.mu.Lock()
	a.mcpNavPolicy = mcpNavigationPolicy{
		WarnPrivate: settings.MCPWarnPrivate,
		Allowlist:   normalizeMCPPrivateAllowlist(settings.MCPPrivateAllowlist),
	}
	for _, p := range a.pages {
		raw := p.navWarnURL
		if raw == "" {
			raw = p.url
			p.navWarnURL = raw
		}
		p.navWarnings = a.navigationWarningsForPageLocked(raw)
	}
	a.mu.Unlock()
}

func (a *app) mcpNavigationPolicySnapshot() mcpNavigationPolicy {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mcpNavPolicy
}

func (a *app) navigationWarningsForURLs(urls []string) []navigationWarning {
	policy := a.mcpNavigationPolicySnapshot()
	warnings := make([]navigationWarning, 0, len(urls))
	seen := map[navigationWarning]bool{}
	for _, raw := range urls {
		warning, ok := navigationWarningForURL(raw, policy)
		if !ok || seen[warning] {
			continue
		}
		seen[warning] = true
		warnings = append(warnings, warning)
	}
	return warnings
}

func (a *app) navigationWarningForURLLocked(raw string) (navigationWarning, bool) {
	return navigationWarningForURL(raw, a.mcpNavPolicy)
}

func (a *app) notifyMCPNavigationWarning(warning navigationWarning) {
	if warning.Host == "" {
		return
	}
	a.setNoticeWarnKey("notice.mcpPrivateNavigation", warning.Host)
}

func (a *app) navigationWarningsForPageLocked(raw string) []navigationWarning {
	warning, ok := a.navigationWarningForURLLocked(raw)
	if !ok {
		return nil
	}
	return []navigationWarning{warning}
}

func (a *app) updatePageNavigationWarningsLocked(p *pageRecord, raw string) (navigationWarning, bool) {
	p.navWarnURL = raw
	warnings := a.navigationWarningsForPageLocked(raw)
	if navigationWarningsEqual(p.navWarnings, warnings) {
		return navigationWarning{}, false
	}
	p.navWarnings = warnings
	if len(warnings) == 0 {
		return navigationWarning{}, true
	}
	return warnings[0], true
}

func navigationWarningForURL(raw string, policy mcpNavigationPolicy) (navigationWarning, bool) {
	if !policy.WarnPrivate {
		return navigationWarning{}, false
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return navigationWarning{}, false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return navigationWarning{}, false
	}
	if ipHost, zone, _ := strings.Cut(host, "%"); zone != "" {
		host = ipHost
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	if policy.Allowlist[host] {
		return navigationWarning{}, false
	}
	reason := ""
	if ip := net.ParseIP(host); ip != nil {
		switch {
		case ip.IsLoopback():
			reason = "loopback"
		case ip.IsPrivate():
			reason = "private"
		case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
			reason = "link-local"
		case ip.IsUnspecified():
			reason = "unspecified"
		case ip.Equal(net.IPv4bcast):
			reason = "broadcast"
		case ip.IsMulticast():
			reason = "multicast"
		default:
			shared := net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}
			if shared.Contains(ip) {
				reason = "shared-address"
			}
		}
	} else {
		switch {
		case host == "localhost" || strings.HasSuffix(host, ".localhost"):
			reason = "localhost"
		case host == "local" || strings.HasSuffix(host, ".local"):
			reason = "mdns"
		}
	}
	if reason == "" {
		return navigationWarning{}, false
	}
	return navigationWarning{
		URL:     raw,
		Host:    host,
		Reason:  reason,
		Message: "MCP navigation targets a local or private address; access is still allowed.",
	}, true
}

func cloneNavigationWarnings(warnings []navigationWarning) []navigationWarning {
	return append([]navigationWarning(nil), warnings...)
}

func navigationWarningsEqual(a, b []navigationWarning) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
