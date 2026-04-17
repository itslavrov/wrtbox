// Package config defines the wrtbox YAML schema (v1) and its loader.
//
// The schema is hybrid: high-level declarative fields for the common
// anti-censorship / split-routing case, plus `raw:` escape hatches for
// arbitrary xray or UCI overrides.
package config

// SchemaVersion is the currently supported wrtbox.yaml apiVersion value.
const SchemaVersion = "wrtbox/v1"

// Config is the root document of wrtbox.yaml.
type Config struct {
	APIVersion string   `yaml:"apiVersion" validate:"required,eq=wrtbox/v1"`
	Kind       string   `yaml:"kind"       validate:"required,eq=Router"`
	Metadata   Metadata `yaml:"metadata"   validate:"required"`
	Spec       Spec     `yaml:"spec"       validate:"required"`
}

// Metadata carries human-readable identifiers for the rendered router.
type Metadata struct {
	Name        string            `yaml:"name"         validate:"required,hostname_rfc1123"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
}

// Spec is the declarative body of the router configuration.
type Spec struct {
	Device    Device    `yaml:"device"             validate:"required"`
	Network   Network   `yaml:"network"            validate:"required"`
	Wireless  *Wireless `yaml:"wireless,omitempty"`
	VPNLan    *VPNLan   `yaml:"vpnlan,omitempty"`
	Transport Transport `yaml:"transport"          validate:"required"`
	Routing   Routing   `yaml:"routing"            validate:"required"`
}

// Device selects the target hardware/software adapter.
type Device struct {
	Model  string `yaml:"model"          validate:"required,oneof=gl-mt6000 x86_64"`
	Target string `yaml:"target,omitempty"` // e.g. 192.168.1.1 — used by apply/diff, not render
}

// Network describes LAN/WAN/DHCP — the baseline OpenWrt network layout.
type Network struct {
	LAN  LAN   `yaml:"lan"  validate:"required"`
	WAN  WAN   `yaml:"wan"  validate:"required"`
	DHCP *DHCP `yaml:"dhcp,omitempty"`
}

// LAN is the bridged user-facing subnet.
type LAN struct {
	IPAddr string   `yaml:"ipaddr"    validate:"required,cidr"`
	Ports  []string `yaml:"ports,omitempty" validate:"omitempty,dive,required"`
	IPv6   bool     `yaml:"ipv6,omitempty"`
	Bridge string   `yaml:"bridge,omitempty"` // default: "br-lan"
}

// WAN is the upstream interface. Only PPPoE and DHCP are supported in v1.
type WAN struct {
	Proto    string   `yaml:"proto"              validate:"required,oneof=pppoe dhcp static"`
	Device   string   `yaml:"device"             validate:"required"`
	Username string   `yaml:"username,omitempty"`
	Password string   `yaml:"password,omitempty"`
	DNS      []string `yaml:"dns,omitempty"      validate:"dive,ip"`
	IPAddr   string   `yaml:"ipaddr,omitempty"`  // static only
	Gateway  string   `yaml:"gateway,omitempty"` // static only
	IPv6     bool     `yaml:"ipv6,omitempty"`
}

// DHCP overrides for dnsmasq + per-interface DHCP.
type DHCP struct {
	Domain string `yaml:"domain,omitempty"`
	LAN    *Pool  `yaml:"lan,omitempty"`
	VPNLan *Pool  `yaml:"vpnlan,omitempty"`
}

// Pool is a simple DHCP range/lease spec.
type Pool struct {
	Start     int    `yaml:"start"     validate:"omitempty,gte=2,lte=254"`
	Limit     int    `yaml:"limit"     validate:"omitempty,gte=1,lte=253"`
	Leasetime string `yaml:"leasetime,omitempty"`
}

// Wireless configures mac80211 radios and SSIDs.
type Wireless struct {
	Country string  `yaml:"country"  validate:"required,len=2"`
	Radios  []Radio `yaml:"radios"   validate:"required,min=1,dive"`
	SSIDs   []SSID  `yaml:"ssids"    validate:"required,min=1,dive"`
}

// Radio maps to wifi-device.
type Radio struct {
	Name    string `yaml:"name"    validate:"required"` // radio0 / radio1
	Band    string `yaml:"band"    validate:"required,oneof=2g 5g 6g"`
	Channel string `yaml:"channel,omitempty"` // "auto" or number
	HTMode  string `yaml:"htmode,omitempty"`
	Path    string `yaml:"path,omitempty"`
}

// SSID is a wifi-iface entry.
type SSID struct {
	Radio      string `yaml:"radio"      validate:"required"`
	Network    string `yaml:"network"    validate:"required,oneof=lan vpnlan"`
	SSID       string `yaml:"ssid"       validate:"required,min=1,max=32"`
	Encryption string `yaml:"encryption" validate:"required,oneof=psk psk2 sae none"`
	Password   string `yaml:"password,omitempty"`
	Hidden     bool   `yaml:"hidden,omitempty"`
}

// VPNLan is an optional dedicated bridge whose traffic is forced through
// the VPN outbound regardless of domain/IP routing rules.
type VPNLan struct {
	Bridge string `yaml:"bridge,omitempty"` // default: "br-vpnlan"
	IPAddr string `yaml:"ipaddr"             validate:"required,cidr"`
	Pool   *Pool  `yaml:"dhcp,omitempty"`
}

// Transport is the outbound protocol stack. Exactly one of the
// concrete transport fields must be set.
type Transport struct {
	Xray *Xray `yaml:"xray,omitempty"`
}

// Xray configures the core VLESS+Reality outbound and the TPROXY inbound.
type Xray struct {
	Reality   RealityOutbound `yaml:"reality"              validate:"required"`
	TProxy    TProxyInbound   `yaml:"tproxy,omitempty"`
	SocksPort int             `yaml:"socks_port,omitempty" validate:"omitempty,gte=1,lte=65535"`
	LogLevel  string          `yaml:"log_level,omitempty"  validate:"omitempty,oneof=debug info warning error none"`
	Mark      int             `yaml:"mark,omitempty"       validate:"omitempty,gte=1,lte=4294967294"`
}

// RealityOutbound is a VLESS+Reality uplink to a VPS.
type RealityOutbound struct {
	Server      string `yaml:"server"       validate:"required,hostname|ip"`
	Port        int    `yaml:"port"         validate:"required,gte=1,lte=65535"`
	UUID        string `yaml:"uuid"         validate:"required,uuid"`
	Flow        string `yaml:"flow,omitempty"` // default: xtls-rprx-vision
	ServerName  string `yaml:"server_name"  validate:"required,hostname"`
	PublicKey   string `yaml:"public_key"   validate:"required,min=32"`
	ShortID     string `yaml:"short_id"     validate:"required,min=1,max=32,hexadecimal"`
	Fingerprint string `yaml:"fingerprint,omitempty"` // default: chrome
	SpiderX     string `yaml:"spider_x,omitempty"`    // default: /
}

// TProxyInbound configures the dokodemo-door TPROXY inbound that fw rules
// redirect non-local traffic to.
type TProxyInbound struct {
	Listen string `yaml:"listen,omitempty"`                                       // default: 0.0.0.0
	Port   int    `yaml:"port,omitempty"    validate:"omitempty,gte=1,lte=65535"` // default: 12345
}

// Routing is the high-level split-routing policy plus raw escape hatches.
type Routing struct {
	Profile        string        `yaml:"profile"                   validate:"required,oneof=split full-tunnel"`
	DomainStrategy string        `yaml:"domain_strategy,omitempty" validate:"omitempty,oneof=AsIs IPIfNonMatch IPOnDemand"`
	BlockQUIC      bool          `yaml:"block_quic,omitempty"`
	ForceViaVPN    []string      `yaml:"force_via_vpn,omitempty"` // builtin tags or geosite: refs
	ForceDirect    []string      `yaml:"force_direct,omitempty"`
	Block          []string      `yaml:"block,omitempty"`
	Lists          []ListRef     `yaml:"lists,omitempty"            validate:"dive"`
	VPNLanViaVPN   bool          `yaml:"vpnlan_via_vpn,omitempty"` // default: true when vpnlan is set
	Raw            *RawOverrides `yaml:"raw,omitempty"`
}

// ListRef is a named external list used to feed routing rules.
type ListRef struct {
	Name        string `yaml:"name"        validate:"required"`
	Source      string `yaml:"source"      validate:"required"` // embed:, file:, http(s):
	OutboundTag string `yaml:"outbound_tag" validate:"required,oneof=vless-out direct block"`
	Kind        string `yaml:"kind,omitempty" validate:"omitempty,oneof=cidr domain"` // default: cidr
}

// RawOverrides is the hybrid escape-hatch: sink for data that the high-level
// fields can't express.
type RawOverrides struct {
	XrayRules     []map[string]interface{} `yaml:"xray_rules,omitempty"`
	XrayInbounds  []map[string]interface{} `yaml:"xray_inbounds,omitempty"`
	XrayOutbounds []map[string]interface{} `yaml:"xray_outbounds,omitempty"`
	UCI           map[string]string        `yaml:"uci,omitempty"` // file -> raw UCI text appended
}
