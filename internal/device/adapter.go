// Package device contains per-hardware adapters. An Adapter supplies the
// device-specific defaults (wifi paths, default port names, etc.) that the
// generic UCI renderer needs.
//
// Unknown devices fall back to the Generic adapter. Users describe quirks
// in spec.device.overrides; wrtbox layers them on top of the adapter
// defaults. This keeps per-device Go code limited to devices the project
// maintainers can actually validate.
package device

import "github.com/itslavrov/wrtbox/internal/config"

// Adapter is the per-device hook-point for wrtbox render.
type Adapter interface {
	// Model returns the canonical device model identifier used in YAML.
	Model() string
	// DefaultPorts returns the list of switch/bridge port names for the
	// device (e.g. lan1..lan5 on gl-mt6000, eth0 on x86_64).
	DefaultPorts() []string
	// DefaultRadios returns a set of stock mac80211 radios (empty for
	// devices without wifi).
	DefaultRadios() []config.Radio
	// DefaultWANDevice is the hardware-level WAN interface name.
	DefaultWANDevice() string
}

// KnownModels lists the built-in adapter keys. Anything else is treated
// as a generic OpenWrt device and expected to supply device quirks via
// spec.device.overrides in YAML.
var KnownModels = []string{"gl-mt6000", "x86_64", "generic"}

// Lookup returns the Adapter for a model. Unknown models fall back to
// Generic — wrtbox's philosophy is "recipes > per-device code": if a
// device isn't first-class, users describe it in YAML overrides and it
// Just Works on top of the generic profile.
func Lookup(model string) (Adapter, error) {
	switch model {
	case "gl-mt6000":
		return &GLMT6000{}, nil
	case "x86_64":
		return &X86_64{}, nil
	case "generic", "":
		return &Generic{}, nil
	default:
		return &Generic{}, nil
	}
}

// IsKnown reports whether the given model has a dedicated first-class
// adapter. Unknown models are handled by the Generic adapter + overrides.
func IsKnown(model string) bool {
	for _, m := range KnownModels {
		if m == model {
			return true
		}
	}
	return false
}

// ApplyDefaults uses the adapter for cfg.Spec.Device.Model to fill any
// unset device-specific fields in cfg. spec.device.overrides take
// precedence over adapter defaults — the order of resolution is:
//
//  1. Explicit YAML (spec.network.lan.ports, etc.) — highest priority
//  2. spec.device.overrides (user-supplied device quirks)
//  3. Adapter defaults (first-class per-device values)
func ApplyDefaults(cfg *config.Config) error {
	a, err := Lookup(cfg.Spec.Device.Model)
	if err != nil {
		return err
	}

	ov := cfg.Spec.Device.Overrides

	if len(cfg.Spec.Network.LAN.Ports) == 0 {
		switch {
		case ov != nil && len(ov.LANPorts) > 0:
			cfg.Spec.Network.LAN.Ports = append([]string(nil), ov.LANPorts...)
		default:
			cfg.Spec.Network.LAN.Ports = a.DefaultPorts()
		}
	}
	if cfg.Spec.Network.WAN.Device == "" {
		switch {
		case ov != nil && ov.WANInterface != "":
			cfg.Spec.Network.WAN.Device = ov.WANInterface
		default:
			cfg.Spec.Network.WAN.Device = a.DefaultWANDevice()
		}
	}
	if cfg.Spec.Wireless != nil {
		stock := a.DefaultRadios()
		if ov != nil && len(ov.Radios) > 0 {
			stock = mergeRadios(stock, ov.Radios)
		}
		for i := range cfg.Spec.Wireless.Radios {
			r := &cfg.Spec.Wireless.Radios[i]
			if r.Path != "" {
				continue
			}
			for _, s := range stock {
				if s.Name == r.Name {
					r.Path = s.Path
					if r.HTMode == "" {
						r.HTMode = s.HTMode
					}
					break
				}
			}
		}
	}
	return nil
}

// mergeRadios overlays user-supplied radio overrides on top of adapter
// defaults, matching by Name. Any radio named in overrides replaces the
// corresponding adapter entry; new radios are appended.
func mergeRadios(base, over []config.Radio) []config.Radio {
	out := make([]config.Radio, 0, len(base)+len(over))
	seen := make(map[string]bool, len(over))
	for _, r := range over {
		seen[r.Name] = true
	}
	for _, r := range base {
		if seen[r.Name] {
			continue
		}
		out = append(out, r)
	}
	out = append(out, over...)
	return out
}
