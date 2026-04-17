package device

import "github.com/itslavrov/wrtbox/internal/config"

// Generic is the default OpenWrt 23.05+ DSA adapter. It assumes nothing
// device-specific: LAN ports, radios, and WAN interface must come from
// either the Spec (explicit YAML) or spec.device.overrides.
//
// This is the target any non-whitelisted device can use — user provides
// the device quirks in YAML as overrides, wrtbox treats them as canonical.
type Generic struct{}

// Model implements Adapter.
func (Generic) Model() string { return "generic" }

// DefaultPorts implements Adapter. Unknown — user must set network.lan.ports
// or device.overrides.lan_ports.
func (Generic) DefaultPorts() []string { return nil }

// DefaultWANDevice implements Adapter. eth1 is the most common WAN on
// multi-NIC OpenWrt boards (GL.iNet, Xiaomi, many TP-Link); single-NIC
// boards must override to eth0.
func (Generic) DefaultWANDevice() string { return "eth1" }

// DefaultRadios implements Adapter. Unknown — user must set radio paths
// via device.overrides.radios.
func (Generic) DefaultRadios() []config.Radio { return nil }
