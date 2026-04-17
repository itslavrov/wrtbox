// Package xray renders the /etc/xray/config.json document from a
// validated wrtbox config. It is intentionally self-contained: no
// text/template, no YAML leakage — the output is built from Go structs
// and serialised via encoding/json so JSON structural equality is the
// primary correctness property.
package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/lists"
)

// BuildOptions tunes the Build call.
type BuildOptions struct {
	// Lists is the provider used to resolve ListRef entries. Required
	// when cfg.Spec.Routing.Lists is non-empty.
	Lists lists.Provider
}

// privateV4 is the non-negotiable "never leak to VPS" set. Xray's routing
// engine consults this before any user rule; we inject it explicitly so
// the generated config is self-contained and auditable.
var privateV4 = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.168.0.0/16",
	"224.0.0.0/4",
	"240.0.0.0/4",
}

// Build renders the xray config.json bytes (4-space indented, trailing
// newline) for cfg.
func Build(ctx context.Context, cfg *config.Config, opts BuildOptions) ([]byte, error) {
	x := cfg.Spec.Transport.Xray
	if x == nil {
		return nil, fmt.Errorf("xray: transport.xray is nil")
	}

	doc := document{
		Log:       logSection{LogLevel: x.LogLevel},
		Inbounds:  buildInbounds(x),
		Outbounds: buildOutbounds(x),
		Routing: routingSection{
			DomainStrategy: cfg.Spec.Routing.DomainStrategy,
		},
	}

	rules, err := buildRules(ctx, cfg, opts)
	if err != nil {
		return nil, err
	}
	doc.Routing.Rules = rules

	buf, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("xray: marshal: %w", err)
	}
	return append(buf, '\n'), nil
}

func buildInbounds(x *config.Xray) []inbound {
	return []inbound{
		{
			Tag:      "socks-in",
			Listen:   "127.0.0.1",
			Port:     x.SocksPort,
			Protocol: "socks",
			Settings: map[string]interface{}{
				"auth": "noauth",
				"udp":  true,
			},
			Sniffing: &sniffing{Enabled: true, DestOverride: []string{"http", "tls"}},
		},
		{
			Tag:      "tproxy-in",
			Listen:   x.TProxy.Listen,
			Port:     x.TProxy.Port,
			Protocol: "dokodemo-door",
			Settings: map[string]interface{}{
				"network":        "tcp,udp",
				"followRedirect": true,
			},
			Sniffing:       &sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}, RouteOnly: true},
			StreamSettings: &streamSettings{Sockopt: &sockopt{TProxy: "tproxy"}},
		},
	}
}

func buildOutbounds(x *config.Xray) []outbound {
	vless := outbound{
		Tag:      "vless-out",
		Protocol: "vless",
		Settings: map[string]interface{}{
			"vnext": []map[string]interface{}{{
				"address": x.Reality.Server,
				"port":    x.Reality.Port,
				"users": []map[string]interface{}{{
					"id":         x.Reality.UUID,
					"encryption": "none",
					"flow":       x.Reality.Flow,
				}},
			}},
		},
		StreamSettings: &streamSettings{
			Network:  "tcp",
			Security: "reality",
			RealitySettings: &realitySettings{
				ServerName:  x.Reality.ServerName,
				Fingerprint: x.Reality.Fingerprint,
				PublicKey:   x.Reality.PublicKey,
				ShortID:     x.Reality.ShortID,
				SpiderX:     x.Reality.SpiderX,
			},
			Sockopt: &sockopt{Mark: x.Mark},
		},
	}
	return []outbound{
		vless,
		{Tag: "direct", Protocol: "freedom"},
		{Tag: "block", Protocol: "blackhole"},
		{Tag: "dns-out", Protocol: "dns"},
	}
}

func buildRules(ctx context.Context, cfg *config.Config, opts BuildOptions) ([]rule, error) {
	r := cfg.Spec.Routing
	var rules []rule

	// 1. DNS hijack (port 53 on tproxy inbound → dns-out).
	rules = append(rules, rule{
		Type:        "field",
		InboundTag:  []string{"tproxy-in"},
		Port:        53,
		OutboundTag: "dns-out",
	})

	// 2. vpnlan source → vless.
	if cfg.Spec.VPNLan != nil && r.VPNLanViaVPN {
		rules = append(rules, rule{
			Type:        "field",
			Source:      []string{cidrOfVPNLan(cfg.Spec.VPNLan.IPAddr)},
			OutboundTag: "vless-out",
		})
	}

	// 3. QUIC block.
	if r.BlockQUIC {
		rules = append(rules, rule{
			Type:        "field",
			Network:     "udp",
			Port:        "443",
			OutboundTag: "block",
		})
	}

	// 4. Private IPs → direct (safety).
	rules = append(rules, rule{
		Type:        "field",
		IP:          append([]string(nil), privateV4...),
		OutboundTag: "direct",
	})

	// 5. block (domains first, then ips).
	if blockDom, blockIP := partitionEntries(r.Block); len(blockDom) > 0 || len(blockIP) > 0 {
		if len(blockDom) > 0 {
			rules = append(rules, rule{Type: "field", Domain: blockDom, OutboundTag: "block"})
		}
		if len(blockIP) > 0 {
			rules = append(rules, rule{Type: "field", IP: blockIP, OutboundTag: "block"})
		}
	}

	// 6. force_via_vpn (domains, ips).
	if dom, ip := partitionEntries(r.ForceViaVPN); len(dom) > 0 || len(ip) > 0 {
		if len(dom) > 0 {
			rules = append(rules, rule{Type: "field", Domain: dom, OutboundTag: "vless-out"})
		}
		if len(ip) > 0 {
			rules = append(rules, rule{Type: "field", IP: ip, OutboundTag: "vless-out"})
		}
	}

	// 7. force_direct: split into non-geoip (domains/geosite) and geoip.
	directDom, directIP := partitionEntries(r.ForceDirect)
	var directGeoIP, directOtherIP []string
	for _, v := range directIP {
		if strings.HasPrefix(v, "geoip:") {
			directGeoIP = append(directGeoIP, v)
		} else {
			directOtherIP = append(directOtherIP, v)
		}
	}
	if len(directDom) > 0 {
		rules = append(rules, rule{Type: "field", Domain: directDom, OutboundTag: "direct"})
	}
	if len(directOtherIP) > 0 {
		rules = append(rules, rule{Type: "field", IP: directOtherIP, OutboundTag: "direct"})
	}

	// 8. Named lists.
	for _, l := range r.Lists {
		entries, err := opts.Lists.Fetch(ctx, l.Source)
		if err != nil {
			return nil, fmt.Errorf("xray: list %q: %w", l.Name, err)
		}
		if len(entries) == 0 {
			continue
		}
		kind := l.Kind
		if kind == "" {
			kind = "cidr"
		}
		switch kind {
		case "cidr":
			rules = append(rules, rule{Type: "field", IP: entries, OutboundTag: l.OutboundTag})
		case "domain":
			rules = append(rules, rule{Type: "field", Domain: entries, OutboundTag: l.OutboundTag})
		}
	}

	// 9. force_direct geoip entries — placed last before default so RU
	// traffic falls through to direct after more specific rules matched.
	if len(directGeoIP) > 0 {
		rules = append(rules, rule{Type: "field", IP: directGeoIP, OutboundTag: "direct"})
	}

	// 10. raw escape hatches (append after canonical rules).
	if r.Raw != nil {
		for _, raw := range r.Raw.XrayRules {
			rules = append(rules, ruleFromMap(raw))
		}
	}

	// 11. Default rule.
	defaultTag := "direct"
	if r.Profile == "full-tunnel" {
		defaultTag = "vless-out"
	}
	rules = append(rules, rule{
		Type:        "field",
		Network:     "tcp,udp",
		OutboundTag: defaultTag,
	})

	return rules, nil
}

// partitionEntries splits a flat routing-entry list into (domains, ips)
// by xray tag conventions: `ip:`, `geoip:` → ip bucket; `domain:`,
// `geosite:`, bare FQDN, bare CIDR — inferred.
func partitionEntries(entries []string) (domains, ips []string) {
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e, "ip:"):
			ips = append(ips, strings.TrimPrefix(e, "ip:"))
		case strings.HasPrefix(e, "geoip:"):
			ips = append(ips, e)
		case strings.HasPrefix(e, "domain:"), strings.HasPrefix(e, "geosite:"), strings.HasPrefix(e, "full:"), strings.HasPrefix(e, "regexp:"):
			domains = append(domains, e)
		case strings.Contains(e, "/"):
			// Looks like a CIDR (e.g. 192.168.1.0/24)
			ips = append(ips, e)
		default:
			// Default to domain — matches xray's own treatment of bare strings.
			domains = append(domains, e)
		}
	}
	return
}

// cidrOfVPNLan returns the network CIDR that covers the gateway ipaddr of
// the vpnlan interface (we want the whole subnet routed, not just .1).
func cidrOfVPNLan(ipCIDR string) string {
	_, ipnet, err := net.ParseCIDR(ipCIDR)
	if err == nil && ipnet != nil {
		return ipnet.String()
	}
	// Fall back to /24 over the first three octets.
	parts := strings.Split(strings.SplitN(ipCIDR, "/", 2)[0], ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
	}
	return ipCIDR
}

func ruleFromMap(m map[string]interface{}) rule {
	var r rule
	if v, ok := m["type"].(string); ok {
		r.Type = v
	}
	if v, ok := m["inboundTag"].([]interface{}); ok {
		for _, s := range v {
			if ss, ok := s.(string); ok {
				r.InboundTag = append(r.InboundTag, ss)
			}
		}
	}
	if v, ok := m["port"]; ok {
		r.Port = v
	}
	if v, ok := m["network"].(string); ok {
		r.Network = v
	}
	if v, ok := m["source"].([]interface{}); ok {
		for _, s := range v {
			if ss, ok := s.(string); ok {
				r.Source = append(r.Source, ss)
			}
		}
	}
	if v, ok := m["ip"].([]interface{}); ok {
		for _, s := range v {
			if ss, ok := s.(string); ok {
				r.IP = append(r.IP, ss)
			}
		}
	}
	if v, ok := m["domain"].([]interface{}); ok {
		for _, s := range v {
			if ss, ok := s.(string); ok {
				r.Domain = append(r.Domain, ss)
			}
		}
	}
	if v, ok := m["outboundTag"].(string); ok {
		r.OutboundTag = v
	}
	return r
}
