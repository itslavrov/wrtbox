// Package device contains per-hardware adapters. An Adapter supplies the
// device-specific defaults (wifi paths, default port names, etc.) that the
// generic UCI renderer needs.
package device

import (
	"fmt"

	"github.com/itslavrov/wrtbox/internal/config"
)

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

// Lookup returns the Adapter for a model or an error if unknown.
func Lookup(model string) (Adapter, error) {
	switch model {
	case "gl-mt6000":
		return &GLMT6000{}, nil
	case "x86_64":
		return &X86_64{}, nil
	default:
		return nil, fmt.Errorf("device %q is not supported (known: gl-mt6000, x86_64)", model)
	}
}

// ApplyDefaults uses the adapter for cfg.Spec.Device.Model to fill any
// unset device-specific fields in cfg.
func ApplyDefaults(cfg *config.Config) error {
	a, err := Lookup(cfg.Spec.Device.Model)
	if err != nil {
		return err
	}
	if len(cfg.Spec.Network.LAN.Ports) == 0 {
		cfg.Spec.Network.LAN.Ports = a.DefaultPorts()
	}
	if cfg.Spec.Network.WAN.Device == "" {
		cfg.Spec.Network.WAN.Device = a.DefaultWANDevice()
	}
	if cfg.Spec.Wireless != nil {
		stock := a.DefaultRadios()
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
