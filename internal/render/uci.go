package render

import (
	"fmt"
	"strings"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/uci"
)

// buildNetwork assembles /etc/config/network from cfg.
func buildNetwork(cfg *config.Config) uci.Package {
	p := uci.Package{Name: "network"}

	p.Sections = append(p.Sections, uci.Section{
		Type: "interface", Name: "loopback",
		Items: []uci.Item{
			uci.Opt("device", "lo"),
			uci.Opt("proto", "static"),
			uci.Lst("ipaddr", "127.0.0.1/8"),
		},
	})

	// globals is kept minimal; real values (duid, ula_prefix) are device
	// state and generated at first boot by OpenWrt — we don't pretend to
	// own them.
	p.Sections = append(p.Sections, uci.Section{
		Type: "globals", Name: "globals",
		Items: []uci.Item{
			uci.Opt("packet_steering", "1"),
		},
	})

	lan := cfg.Spec.Network.LAN
	p.Sections = append(p.Sections, uci.Section{
		Type: "device",
		Items: []uci.Item{
			uci.Opt("name", lan.Bridge),
			uci.Opt("type", "bridge"),
			uci.Lst("ports", lan.Ports...),
		},
	})

	lanItems := []uci.Item{
		uci.Opt("device", lan.Bridge),
		uci.Opt("proto", "static"),
		uci.Lst("ipaddr", lan.IPAddr),
	}
	if !lan.IPv6 {
		lanItems = append(lanItems, uci.Opt("ipv6", "0"))
	}
	p.Sections = append(p.Sections, uci.Section{
		Type: "interface", Name: "lan",
		Items: lanItems,
	})

	p.Sections = append(p.Sections, buildWAN(cfg))

	if cfg.Spec.VPNLan != nil {
		p.Sections = append(p.Sections, uci.Section{
			Type: "device", Name: "vpnlan_dev",
			Items: []uci.Item{
				uci.Opt("name", cfg.Spec.VPNLan.Bridge),
				uci.Opt("type", "bridge"),
			},
		})
		ip, mask := splitCIDR(cfg.Spec.VPNLan.IPAddr)
		p.Sections = append(p.Sections, uci.Section{
			Type: "interface", Name: "vpnlan",
			Items: []uci.Item{
				uci.Opt("proto", "static"),
				uci.Opt("ipaddr", ip),
				uci.Opt("netmask", mask),
				uci.Opt("device", cfg.Spec.VPNLan.Bridge),
			},
		})
	}

	return p
}

func buildWAN(cfg *config.Config) uci.Section {
	w := cfg.Spec.Network.WAN
	items := []uci.Item{
		uci.Opt("device", w.Device),
		uci.Opt("proto", w.Proto),
	}
	switch w.Proto {
	case "pppoe":
		if w.Username != "" {
			items = append(items, uci.Opt("username", w.Username))
		}
		if w.Password != "" {
			items = append(items, uci.Opt("password", w.Password))
		}
		items = append(items, uci.Opt("norelease", "1"))
	case "static":
		if w.IPAddr != "" {
			items = append(items, uci.Opt("ipaddr", w.IPAddr))
		}
		if w.Gateway != "" {
			items = append(items, uci.Opt("gateway", w.Gateway))
		}
	}
	if len(w.DNS) > 0 {
		items = append(items, uci.Opt("peerdns", "0"))
		items = append(items, uci.Lst("dns", w.DNS...))
	}
	if !w.IPv6 {
		items = append(items, uci.Opt("ipv6", "0"))
	}
	return uci.Section{Type: "interface", Name: "wan", Items: items}
}

// buildFirewall assembles /etc/config/firewall with the standard lan/wan
// zones plus an optional vpnlan zone when vpnlan is declared.
func buildFirewall(cfg *config.Config) uci.Package {
	p := uci.Package{Name: "firewall"}

	p.Sections = append(p.Sections, uci.Section{
		Type: "defaults",
		Items: []uci.Item{
			uci.Opt("syn_flood", "1"),
			uci.Opt("input", "REJECT"),
			uci.Opt("output", "ACCEPT"),
			uci.Opt("forward", "REJECT"),
		},
	})

	p.Sections = append(p.Sections, uci.Section{
		Type: "zone",
		Items: []uci.Item{
			uci.Opt("name", "lan"),
			uci.Lst("network", "lan"),
			uci.Opt("input", "ACCEPT"),
			uci.Opt("output", "ACCEPT"),
			uci.Opt("forward", "ACCEPT"),
		},
	})

	p.Sections = append(p.Sections, uci.Section{
		Type: "zone",
		Items: []uci.Item{
			uci.Opt("name", "wan"),
			uci.Lst("network", "wan"),
			uci.Opt("input", "REJECT"),
			uci.Opt("output", "ACCEPT"),
			uci.Opt("forward", "DROP"),
			uci.Opt("masq", "1"),
			uci.Opt("mtu_fix", "1"),
		},
	})

	p.Sections = append(p.Sections, uci.Section{
		Type: "forwarding",
		Items: []uci.Item{
			uci.Opt("src", "lan"),
			uci.Opt("dest", "wan"),
		},
	})

	// Minimal stock WAN allow-rules: DHCP renew and ping. Skip IPv6 and
	// IPSec boilerplate — out of scope for v1 split-routing.
	p.Sections = append(p.Sections, uci.Section{
		Type: "rule",
		Items: []uci.Item{
			uci.Opt("name", "Allow-DHCP-Renew"),
			uci.Opt("src", "wan"),
			uci.Opt("proto", "udp"),
			uci.Opt("dest_port", "68"),
			uci.Opt("target", "ACCEPT"),
			uci.Opt("family", "ipv4"),
		},
	})
	p.Sections = append(p.Sections, uci.Section{
		Type: "rule",
		Items: []uci.Item{
			uci.Opt("name", "Allow-Ping"),
			uci.Opt("src", "wan"),
			uci.Opt("proto", "icmp"),
			uci.Opt("icmp_type", "echo-request"),
			uci.Opt("family", "ipv4"),
			uci.Opt("target", "ACCEPT"),
		},
	})

	if cfg.Spec.VPNLan != nil {
		p.Sections = append(p.Sections, uci.Section{
			Type: "zone",
			Items: []uci.Item{
				uci.Opt("name", "vpnlan"),
				uci.Opt("input", "ACCEPT"),
				uci.Opt("output", "ACCEPT"),
				uci.Opt("forward", "REJECT"),
				uci.Lst("network", "vpnlan"),
			},
		})
		p.Sections = append(p.Sections, uci.Section{
			Type: "forwarding",
			Items: []uci.Item{
				uci.Opt("src", "vpnlan"),
				uci.Opt("dest", "wan"),
			},
		})
	}

	return p
}

// buildDHCP assembles /etc/config/dhcp.
func buildDHCP(cfg *config.Config) uci.Package {
	p := uci.Package{Name: "dhcp"}

	dnsmasqItems := []uci.Item{
		uci.Opt("domainneeded", "1"),
		uci.Opt("boguspriv", "1"),
		uci.Opt("filterwin2k", "0"),
		uci.Opt("localise_queries", "1"),
		uci.Opt("rebind_protection", "1"),
		uci.Opt("rebind_localhost", "1"),
		uci.Opt("local", "/lan/"),
		uci.Opt("domain", "lan"),
		uci.Opt("expandhosts", "1"),
		uci.Opt("authoritative", "1"),
		uci.Opt("readethers", "1"),
		uci.Opt("leasefile", "/tmp/dhcp.leases"),
		uci.Opt("resolvfile", "/tmp/resolv.conf.d/resolv.conf.auto"),
		uci.Opt("localservice", "1"),
	}
	if cfg.Spec.Network.DHCP != nil && cfg.Spec.Network.DHCP.Domain != "" {
		// replace domain
		for i, it := range dnsmasqItems {
			if it.Key == "domain" {
				dnsmasqItems[i] = uci.Opt("domain", cfg.Spec.Network.DHCP.Domain)
			}
		}
	}
	p.Sections = append(p.Sections, uci.Section{
		Type:  "dnsmasq",
		Items: dnsmasqItems,
	})

	lanPool := defaultPool(cfg.Spec.Network.DHCP, func(d *config.DHCP) *config.Pool {
		if d == nil {
			return nil
		}
		return d.LAN
	})
	p.Sections = append(p.Sections, uci.Section{
		Type: "dhcp", Name: "lan",
		Items: []uci.Item{
			uci.Opt("interface", "lan"),
			uci.Opt("start", fmt.Sprintf("%d", lanPool.Start)),
			uci.Opt("limit", fmt.Sprintf("%d", lanPool.Limit)),
			uci.Opt("leasetime", lanPool.Leasetime),
			uci.Opt("dhcpv4", "server"),
			uci.Opt("dhcpv6", "disabled"),
			uci.Opt("ra", "disabled"),
			uci.Opt("ndp", "disabled"),
		},
	})

	p.Sections = append(p.Sections, uci.Section{
		Type: "dhcp", Name: "wan",
		Items: []uci.Item{
			uci.Opt("interface", "wan"),
			uci.Opt("ignore", "1"),
		},
	})

	if cfg.Spec.VPNLan != nil {
		vpnPool := defaultPool(cfg.Spec.Network.DHCP, func(d *config.DHCP) *config.Pool {
			if d == nil {
				return nil
			}
			return d.VPNLan
		})
		p.Sections = append(p.Sections, uci.Section{
			Type: "dhcp", Name: "vpnlan",
			Items: []uci.Item{
				uci.Opt("interface", "vpnlan"),
				uci.Opt("start", fmt.Sprintf("%d", vpnPool.Start)),
				uci.Opt("limit", fmt.Sprintf("%d", vpnPool.Limit)),
				uci.Opt("leasetime", vpnPool.Leasetime),
				uci.Opt("dhcpv4", "server"),
			},
		})
	}
	return p
}

// buildWireless assembles /etc/config/wireless. Returns an empty package
// when wireless is not declared (caller skips writing the file).
func buildWireless(cfg *config.Config) uci.Package {
	p := uci.Package{Name: "wireless"}
	if cfg.Spec.Wireless == nil {
		return p
	}
	for _, r := range cfg.Spec.Wireless.Radios {
		items := []uci.Item{
			uci.Opt("type", "mac80211"),
			uci.Opt("path", r.Path),
			uci.Opt("band", r.Band),
		}
		if r.Channel != "" {
			items = append(items, uci.Opt("channel", r.Channel))
		} else {
			items = append(items, uci.Opt("channel", "auto"))
		}
		items = append(items, uci.Opt("htmode", r.HTMode))
		items = append(items, uci.Opt("country", cfg.Spec.Wireless.Country))
		items = append(items, uci.Opt("cell_density", "0"))
		p.Sections = append(p.Sections, uci.Section{
			Type: "wifi-device", Name: r.Name, Items: items,
		})
	}
	for i, s := range cfg.Spec.Wireless.SSIDs {
		name := fmt.Sprintf("default_%s", s.Radio)
		// Distinguish multiple SSIDs sharing a radio with a numbered name.
		for j := 0; j < i; j++ {
			if cfg.Spec.Wireless.SSIDs[j].Radio == s.Radio {
				name = fmt.Sprintf("wifinet_%s_%d", s.Radio, i)
			}
		}
		items := []uci.Item{
			uci.Opt("device", s.Radio),
			uci.Opt("network", s.Network),
			uci.Opt("mode", "ap"),
			uci.Opt("ssid", s.SSID),
			uci.Opt("encryption", s.Encryption),
		}
		if s.Password != "" {
			items = append(items, uci.Opt("key", s.Password))
		}
		if s.Hidden {
			items = append(items, uci.Opt("hidden", "1"))
		}
		p.Sections = append(p.Sections, uci.Section{
			Type: "wifi-iface", Name: name, Items: items,
		})
	}
	return p
}

func splitCIDR(cidr string) (ip, mask string) {
	parts := strings.SplitN(cidr, "/", 2)
	ip = parts[0]
	mask = "255.255.255.0"
	if len(parts) == 2 {
		switch parts[1] {
		case "8":
			mask = "255.0.0.0"
		case "16":
			mask = "255.255.0.0"
		case "24":
			mask = "255.255.255.0"
		case "32":
			mask = "255.255.255.255"
		}
	}
	return
}

func defaultPool(d *config.DHCP, pick func(*config.DHCP) *config.Pool) config.Pool {
	p := config.Pool{Start: 100, Limit: 150, Leasetime: "12h"}
	sel := pick(d)
	if sel == nil {
		return p
	}
	if sel.Start != 0 {
		p.Start = sel.Start
	}
	if sel.Limit != 0 {
		p.Limit = sel.Limit
	}
	if sel.Leasetime != "" {
		p.Leasetime = sel.Leasetime
	}
	return p
}
