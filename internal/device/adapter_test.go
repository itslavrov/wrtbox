package device

import (
	"testing"

	"github.com/itslavrov/wrtbox/internal/config"
)

func TestLookup_UnknownFallsBackToGeneric(t *testing.T) {
	cases := []string{"xiaomi-ax3000t", "keenetic-giga", "tp-link-whatever", ""}
	for _, model := range cases {
		t.Run(model, func(t *testing.T) {
			a, err := Lookup(model)
			if err != nil {
				t.Fatalf("Lookup(%q) err: %v", model, err)
			}
			if a.Model() != "generic" {
				t.Fatalf("Lookup(%q).Model() = %q, want generic", model, a.Model())
			}
		})
	}
}

func TestLookup_KnownModels(t *testing.T) {
	for _, tc := range []struct {
		model, want string
	}{
		{"gl-mt6000", "gl-mt6000"},
		{"x86_64", "x86_64"},
		{"generic", "generic"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			a, err := Lookup(tc.model)
			if err != nil {
				t.Fatalf("Lookup(%q) err: %v", tc.model, err)
			}
			if a.Model() != tc.want {
				t.Fatalf("Lookup(%q).Model() = %q, want %q", tc.model, a.Model(), tc.want)
			}
		})
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown("gl-mt6000") {
		t.Error("gl-mt6000 should be known")
	}
	if IsKnown("xiaomi-ax3000t") {
		t.Error("xiaomi-ax3000t should NOT be known (generic fallback)")
	}
}

// TestApplyDefaults_OverridesFillGeneric checks that a generic device
// with YAML overrides gets WAN/ports/radios filled from the overrides,
// not from the empty generic adapter defaults.
func TestApplyDefaults_OverridesFillGeneric(t *testing.T) {
	cfg := &config.Config{
		Spec: config.Spec{
			Device: config.Device{
				Model: "xiaomi-ax3000t",
				Overrides: &config.DeviceOverrides{
					WANInterface: "eth9",
					LANPorts:     []string{"lan1", "lan2"},
					Radios: []config.Radio{
						{Name: "radio0", Band: "2g", HTMode: "HE40", Path: "custom/path/0"},
						{Name: "radio1", Band: "5g", Path: "custom/path/1"},
					},
				},
			},
			Network: config.Network{
				LAN: config.LAN{IPAddr: "192.168.1.1/24"},
				WAN: config.WAN{Proto: "dhcp"},
			},
			Wireless: &config.Wireless{
				Country: "RU",
				Radios: []config.Radio{
					{Name: "radio0", Band: "2g"},
					{Name: "radio1", Band: "5g"},
				},
			},
		},
	}
	if err := ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if cfg.Spec.Network.WAN.Device != "eth9" {
		t.Errorf("WAN.Device = %q, want eth9 (from override)", cfg.Spec.Network.WAN.Device)
	}
	if got := cfg.Spec.Network.LAN.Ports; len(got) != 2 || got[0] != "lan1" || got[1] != "lan2" {
		t.Errorf("LAN.Ports = %v, want [lan1 lan2]", got)
	}
	if p := cfg.Spec.Wireless.Radios[0].Path; p != "custom/path/0" {
		t.Errorf("radio0.Path = %q, want custom/path/0", p)
	}
	if ht := cfg.Spec.Wireless.Radios[0].HTMode; ht != "HE40" {
		t.Errorf("radio0.HTMode = %q, want HE40 (from override)", ht)
	}
	if p := cfg.Spec.Wireless.Radios[1].Path; p != "custom/path/1" {
		t.Errorf("radio1.Path = %q, want custom/path/1", p)
	}
}

// TestApplyDefaults_ExplicitYAMLBeatsOverride ensures explicit values in
// spec.network take precedence over overrides.
func TestApplyDefaults_ExplicitYAMLBeatsOverride(t *testing.T) {
	cfg := &config.Config{
		Spec: config.Spec{
			Device: config.Device{
				Model: "generic",
				Overrides: &config.DeviceOverrides{
					WANInterface: "eth9",
				},
			},
			Network: config.Network{
				LAN: config.LAN{IPAddr: "192.168.1.1/24"},
				WAN: config.WAN{Proto: "dhcp", Device: "wan-explicit"},
			},
		},
	}
	if err := ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if cfg.Spec.Network.WAN.Device != "wan-explicit" {
		t.Errorf("explicit WAN device was overwritten: got %q", cfg.Spec.Network.WAN.Device)
	}
}

func TestMapBoardToModel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		board BoardInfo
		want  string
	}{
		{"gl-mt6000 by board_name", BoardInfo{BoardName: "glinet,mt6000"}, "gl-mt6000"},
		{"gl-mt6000 by substring", BoardInfo{BoardName: "foo-gl-mt6000-bar"}, "gl-mt6000"},
		{"x86_64 by board_name prefix", BoardInfo{BoardName: "x86,generic"}, "x86_64"},
		{"x86_64 by release.target", BoardInfo{BoardName: "weird", Release: &Release{Target: "x86/64"}}, "x86_64"},
		{"xiaomi → generic", BoardInfo{BoardName: "xiaomi,ax3000t"}, "generic"},
		{"empty → generic", BoardInfo{}, "generic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := MapBoardToModel(tc.board); got != tc.want {
				t.Errorf("MapBoardToModel(%+v) = %q, want %q", tc.board, got, tc.want)
			}
		})
	}
}

// TestApplyDefaults_KnownModelStillWorks guards against regression when
// overrides are absent for a first-class device.
func TestApplyDefaults_KnownModelStillWorks(t *testing.T) {
	cfg := &config.Config{
		Spec: config.Spec{
			Device: config.Device{Model: "gl-mt6000"},
			Network: config.Network{
				LAN: config.LAN{IPAddr: "192.168.1.1/24"},
				WAN: config.WAN{Proto: "dhcp"},
			},
		},
	}
	if err := ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if cfg.Spec.Network.WAN.Device != "eth1" {
		t.Errorf("gl-mt6000 WAN = %q, want eth1", cfg.Spec.Network.WAN.Device)
	}
	if got := cfg.Spec.Network.LAN.Ports; len(got) != 5 {
		t.Errorf("gl-mt6000 LAN.Ports = %v, want 5 ports", got)
	}
}
