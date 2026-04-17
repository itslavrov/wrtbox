package device

import "github.com/itslavrov/wrtbox/internal/config"

// X86_64 is the generic x86_64 OpenWrt target. Used by the emulator
// VM (and any bare-metal/VM router running x86-64 OpenWrt): one NIC
// (eth0), no integrated wifi. A router image with extra NICs can
// still address them by explicit device names in the YAML.
type X86_64 struct{}

// Model implements Adapter.
func (X86_64) Model() string { return "x86_64" }

// DefaultPorts implements Adapter. With a single NIC there are no
// switch ports to attach to br-lan; callers that set wan.device=eth0
// do not use LAN ports anyway.
func (X86_64) DefaultPorts() []string { return nil }

// DefaultWANDevice implements Adapter.
func (X86_64) DefaultWANDevice() string { return "eth0" }

// DefaultRadios implements Adapter. x86_64 boxes have no onboard wifi.
func (X86_64) DefaultRadios() []config.Radio { return nil }
