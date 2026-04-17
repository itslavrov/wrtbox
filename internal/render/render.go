// Package render turns a validated wrtbox config into a tree of files
// (keyed by their final router paths under /etc/, /usr/, /root/).
//
// The contract:
//
//	Files, err := Render(ctx, cfg, opts)
//	for path, data := range Files { os.WriteFile(filepath.Join(outRoot, path), data, mode) }
//
// No network access here — all list resolution goes through
// opts.Lists, which defaults to the embed+file+http registry.
package render

import (
	"context"
	"fmt"
	"sort"

	"github.com/itslavrov/wrtbox/internal/config"
	"github.com/itslavrov/wrtbox/internal/device"
	"github.com/itslavrov/wrtbox/internal/lists"
	"github.com/itslavrov/wrtbox/internal/transport/xray"
	"github.com/itslavrov/wrtbox/internal/uci"
)

// Options tunes Render.
type Options struct {
	// Lists is used to resolve ListRef entries. If nil a default
	// registry (embed+file+http) is built.
	Lists lists.Provider
}

// File is a single rendered artifact. Mode is the intended POSIX mode
// for the file on the router (scripts are 0755, configs 0644).
type File struct {
	Path string
	Data []byte
	Mode uint32
}

// Render produces the full set of files for cfg.
func Render(ctx context.Context, cfg *config.Config, opts Options) ([]File, error) {
	if err := device.ApplyDefaults(cfg); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	if opts.Lists == nil {
		opts.Lists = lists.NewRegistry()
	}

	var out []File

	add := func(path string, data []byte, mode uint32) {
		out = append(out, File{Path: path, Data: data, Mode: mode})
	}

	// UCI configs.
	if data, err := marshalUCI(buildNetwork(cfg)); err == nil {
		add("etc/config/network", data, 0o644)
	} else {
		return nil, err
	}
	if data, err := marshalUCI(buildFirewall(cfg)); err == nil {
		add("etc/config/firewall", data, 0o644)
	} else {
		return nil, err
	}
	if data, err := marshalUCI(buildDHCP(cfg)); err == nil {
		add("etc/config/dhcp", data, 0o644)
	} else {
		return nil, err
	}
	if cfg.Spec.Wireless != nil {
		if data, err := marshalUCI(buildWireless(cfg)); err == nil {
			add("etc/config/wireless", data, 0o644)
		} else {
			return nil, err
		}
	}

	// Xray configs.
	xc, err := xray.Build(ctx, cfg, xray.BuildOptions{Lists: opts.Lists})
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	add("etc/xray/config.json", xc, 0o600)
	add("etc/xray/nft.conf", xray.BuildNFT(cfg), 0o644)

	// Helper scripts (stable, invariant for v1 — parameters baked in).
	add("etc/init.d/xray-rules", []byte(initdXrayRules), 0o755)
	add("etc/hotplug.d/iface/99-xray-rules", []byte(hotplugXrayRules), 0o755)
	add("usr/bin/update-antifilter.sh", []byte(updateAntifilter), 0o755)
	add("usr/bin/update-geosite.sh", []byte(updateGeosite), 0o755)
	add("root/rollback.sh", []byte(rollback), 0o755)
	add("etc/crontabs/root", []byte(cron), 0o600)

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func marshalUCI(p uci.Package) ([]byte, error) {
	var buf = &byteBuf{}
	if err := uci.Render(buf, p); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// byteBuf is a minimal io.Writer backed by a byte slice — avoids
// importing bytes just for one use.
type byteBuf struct{ b []byte }

func (w *byteBuf) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *byteBuf) Bytes() []byte               { return w.b }
