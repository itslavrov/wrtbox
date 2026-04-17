package xray

// document is the top-level xray config.json shape.
type document struct {
	Log       logSection     `json:"log"`
	Inbounds  []inbound      `json:"inbounds"`
	Outbounds []outbound     `json:"outbounds"`
	Routing   routingSection `json:"routing"`
}

type logSection struct {
	LogLevel string `json:"loglevel"`
}

type inbound struct {
	Tag            string                 `json:"tag"`
	Listen         string                 `json:"listen"`
	Port           int                    `json:"port"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings"`
	Sniffing       *sniffing              `json:"sniffing,omitempty"`
	StreamSettings *streamSettings        `json:"streamSettings,omitempty"`
}

type sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride,omitempty"`
	RouteOnly    bool     `json:"routeOnly,omitempty"`
}

type outbound struct {
	Tag            string                 `json:"tag"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings,omitempty"`
	StreamSettings *streamSettings        `json:"streamSettings,omitempty"`
}

type streamSettings struct {
	Network         string           `json:"network,omitempty"`
	Security        string           `json:"security,omitempty"`
	RealitySettings *realitySettings `json:"realitySettings,omitempty"`
	Sockopt         *sockopt         `json:"sockopt,omitempty"`
}

type realitySettings struct {
	ServerName  string `json:"serverName"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"publicKey"`
	ShortID     string `json:"shortId"`
	SpiderX     string `json:"spiderX"`
}

type sockopt struct {
	Mark   int    `json:"mark,omitempty"`
	TProxy string `json:"tproxy,omitempty"`
}

type routingSection struct {
	DomainStrategy string `json:"domainStrategy"`
	Rules          []rule `json:"rules"`
}

type rule struct {
	Type        string      `json:"type"`
	InboundTag  []string    `json:"inboundTag,omitempty"`
	Source      []string    `json:"source,omitempty"`
	Network     string      `json:"network,omitempty"`
	Port        interface{} `json:"port,omitempty"`
	IP          []string    `json:"ip,omitempty"`
	Domain      []string    `json:"domain,omitempty"`
	OutboundTag string      `json:"outboundTag"`
}
