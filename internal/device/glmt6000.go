package device

import "github.com/itslavrov/wrtbox/internal/config"

// GLMT6000 (Flint 2) is the reference target: MT7986 + MT7976, five-port
// switch (lan1..lan5), 2.4 GHz + 5 GHz mac80211 radios on
// platform/soc/18000000.wifi[+1], WAN on eth1.
type GLMT6000 struct{}

// Model implements Adapter.
func (GLMT6000) Model() string { return "gl-mt6000" }

// DefaultPorts implements Adapter.
func (GLMT6000) DefaultPorts() []string {
	return []string{"lan1", "lan2", "lan3", "lan4", "lan5"}
}

// DefaultWANDevice implements Adapter.
func (GLMT6000) DefaultWANDevice() string { return "eth1" }

// DefaultRadios implements Adapter.
func (GLMT6000) DefaultRadios() []config.Radio {
	return []config.Radio{
		{Name: "radio0", Band: "2g", HTMode: "HE20", Path: "platform/soc/18000000.wifi"},
		{Name: "radio1", Band: "5g", HTMode: "HE80", Path: "platform/soc/18000000.wifi+1"},
	}
}
