package xray

import (
	"fmt"
	"strings"

	"github.com/itslavrov/wrtbox/internal/config"
)

// BuildNFT renders /etc/xray/nft.conf — the TPROXY mark/redirect rules
// that feed the tproxy-in inbound.
func BuildNFT(cfg *config.Config) []byte {
	x := cfg.Spec.Transport.Xray
	var b strings.Builder
	b.WriteString("#!/usr/sbin/nft -f\n\n")
	b.WriteString("flush table inet xray\n\n")
	b.WriteString("table inet xray {\n")
	b.WriteString("    set private_ipv4 {\n")
	b.WriteString("        type ipv4_addr\n")
	b.WriteString("        flags interval\n")
	b.WriteString("        elements = {\n")
	for i, p := range privateV4 {
		sep := ","
		if i == len(privateV4)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "            %s%s\n", p, sep)
	}
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
	b.WriteString("    chain prerouting {\n")
	b.WriteString("        type filter hook prerouting priority mangle; policy accept;\n")
	b.WriteString("        ip daddr @private_ipv4 return\n")
	fmt.Fprintf(&b, "        meta l4proto { tcp, udp } th dport 53 meta mark set 1 tproxy to :%d accept\n", x.TProxy.Port)
	fmt.Fprintf(&b, "        meta l4proto { tcp, udp } meta mark set 1 tproxy to :%d accept\n", x.TProxy.Port)
	b.WriteString("    }\n\n")
	b.WriteString("    chain output {\n")
	b.WriteString("        type route hook output priority mangle; policy accept;\n")
	fmt.Fprintf(&b, "        meta mark 0x%x return\n", x.Mark)
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return []byte(b.String())
}
