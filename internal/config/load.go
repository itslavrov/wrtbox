package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// Load reads a wrtbox YAML document from path and returns a validated Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse deserialises a wrtbox YAML document and validates it.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// Validate runs struct-tag validation on an already-parsed Config.
func Validate(cfg *Config) error {
	v := validator.New(validator.WithRequiredStructEnabled())
	if err := v.Struct(cfg); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	if cfg.Spec.Transport.Xray == nil {
		return fmt.Errorf("validate: spec.transport.xray is required (no other transports supported in v1)")
	}
	if cfg.Spec.Routing.Profile == "full-tunnel" {
		return fmt.Errorf("validate: routing.profile=full-tunnel is reserved for a future release")
	}
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.Spec.Transport.Xray != nil {
		x := cfg.Spec.Transport.Xray
		if x.SocksPort == 0 {
			x.SocksPort = 10808
		}
		if x.LogLevel == "" {
			x.LogLevel = "warning"
		}
		if x.Mark == 0 {
			x.Mark = 255
		}
		if x.TProxy.Port == 0 {
			x.TProxy.Port = 12345
		}
		if x.TProxy.Listen == "" {
			x.TProxy.Listen = "0.0.0.0"
		}
		if x.Reality.Flow == "" {
			x.Reality.Flow = "xtls-rprx-vision"
		}
		if x.Reality.Fingerprint == "" {
			x.Reality.Fingerprint = "chrome"
		}
		if x.Reality.SpiderX == "" {
			x.Reality.SpiderX = "/"
		}
	}
	if cfg.Spec.Network.LAN.Bridge == "" {
		cfg.Spec.Network.LAN.Bridge = "br-lan"
	}
	if cfg.Spec.VPNLan != nil && cfg.Spec.VPNLan.Bridge == "" {
		cfg.Spec.VPNLan.Bridge = "br-vpnlan"
	}
	if cfg.Spec.Routing.DomainStrategy == "" {
		cfg.Spec.Routing.DomainStrategy = "IPIfNonMatch"
	}
	// vpnlan_via_vpn defaults to true when vpnlan is declared
	// (can't be expressed purely with YAML zero-value, so handled here).
	if cfg.Spec.VPNLan != nil && !cfg.Spec.Routing.VPNLanViaVPN {
		cfg.Spec.Routing.VPNLanViaVPN = true
	}
}
