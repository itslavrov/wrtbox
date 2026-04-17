package config_test

import (
	"strings"
	"testing"

	"github.com/itslavrov/wrtbox/internal/config"
)

const minValidYAML = `
apiVersion: wrtbox/v1
kind: Router
metadata:
  name: router-test
spec:
  device: { model: gl-mt6000 }
  network:
    lan:  { ipaddr: 192.168.1.1/24, ports: [lan1] }
    wan:  { proto: dhcp, device: eth1 }
  transport:
    xray:
      reality:
        server: vpn.example.com
        port: 443
        uuid: 11111111-2222-3333-4444-555555555555
        server_name: www.microsoft.com
        public_key: 3WmS7tERswgfK1ABS-17QgksuPejtDE50fQvK-3vZAw
        short_id: deadbeef
  routing:
    profile: split
`

func TestParseMinimalValid(t *testing.T) {
	cfg, err := config.Parse([]byte(minValidYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Spec.Transport.Xray.SocksPort != 10808 {
		t.Errorf("default socks_port not applied: got %d", cfg.Spec.Transport.Xray.SocksPort)
	}
	if cfg.Spec.Transport.Xray.Mark != 255 {
		t.Errorf("default mark not applied: got %d", cfg.Spec.Transport.Xray.Mark)
	}
	if cfg.Spec.Network.LAN.Bridge != "br-lan" {
		t.Errorf("default lan.bridge not applied: got %q", cfg.Spec.Network.LAN.Bridge)
	}
	if cfg.Spec.Routing.DomainStrategy != "IPIfNonMatch" {
		t.Errorf("default domain_strategy not applied: got %q", cfg.Spec.Routing.DomainStrategy)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	_, err := config.Parse([]byte(minValidYAML + "  whimsy: true\n"))
	if err == nil || !strings.Contains(err.Error(), "whimsy") {
		t.Fatalf("expected KnownFields strictness to reject whimsy, got: %v", err)
	}
}

func TestParseRejectsFullTunnel(t *testing.T) {
	yml := strings.ReplaceAll(minValidYAML, "profile: split", "profile: full-tunnel")
	_, err := config.Parse([]byte(yml))
	if err == nil || !strings.Contains(err.Error(), "full-tunnel") {
		t.Fatalf("expected full-tunnel reservation error, got: %v", err)
	}
}

func TestParseRejectsBadUUID(t *testing.T) {
	yml := strings.ReplaceAll(minValidYAML, "11111111-2222-3333-4444-555555555555", "not-a-uuid")
	_, err := config.Parse([]byte(yml))
	if err == nil {
		t.Fatalf("expected validation error on bad UUID")
	}
}
