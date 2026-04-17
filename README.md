# Declarative OpenWrt

[![CI](https://github.com/itslavrov/wrtbox/actions/workflows/ci.yml/badge.svg)](https://github.com/itslavrov/wrtbox/actions/workflows/ci.yml)
[![Integration (arm64)](https://github.com/itslavrov/wrtbox/actions/workflows/integration-arm64.yml/badge.svg)](https://github.com/itslavrov/wrtbox/actions/workflows/integration-arm64.yml)
[![License: MIT](https://img.shields.io/github/license/itslavrov/wrtbox)](LICENSE)
[![Go 1.23](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![OpenWrt 23.05+](https://img.shields.io/badge/OpenWrt-23.05+-00B5E2?logo=openwrt&logoColor=white)](https://openwrt.org/)
[![Telegram](https://img.shields.io/badge/Telegram-@itslavrov-26A5E4?logo=telegram&logoColor=white)](https://t.me/itslavrov)

One YAML file, atomic apply, auto-rollback.

## Quick start

### Build

```bash
git clone https://github.com/itslavrov/wrtbox.git && cd wrtbox
make
./bin/wrtbox --help
```

### Describe your router

```yaml
# wrtbox.yaml
apiVersion: wrtbox/v1
kind: Router
metadata:
  name: home
spec:
  device:
    model: gl-mt6000
  network:
    lan:  { ipaddr: 192.168.1.1/24 }
    wan:  { proto: dhcp }
  transport:
    xray:
      reality:
        server: vpn.example.com
        port: 443
        uuid: 00000000-0000-0000-0000-000000000000
        server_name: www.microsoft.com
        public_key: <x25519 base64>
        short_id: deadbeefcafebabe
  routing:
    profile: split
    force_via_vpn: [domain:youtube.com]
    force_direct:  [geoip:ru, geoip:private]
```

Full examples: [`examples/gl-mt6000.yaml`](examples/gl-mt6000.yaml), [`examples/x86_64-emu.yaml`](examples/x86_64-emu.yaml), [`examples/generic-xiaomi-ax3000t.yaml`](examples/generic-xiaomi-ax3000t.yaml).

### Bring your own device

Any OpenWrt 23.05+ router works via the generic profile + YAML overrides — no Go code per device:

```yaml
spec:
  device:
    model: my-cool-router         # any label
    overrides:
      wan_interface: eth1
      lan_ports: [lan1, lan2, lan3]
      radios:
        - { name: radio0, band: 2g, htmode: HE20, path: "platform/soc/18000000.wifi" }
        - { name: radio1, band: 5g, htmode: HE80, path: "platform/soc/18000000.wifi+1" }
      required_packages: [xray-core, kmod-nft-tproxy, xray-geodata]
```

`wrtbox detect --router <alias>` prints a ready-to-paste `spec.device` block from the live router. Community recipes live in [`docs/devices/`](docs/devices/).

### Preview changes

```bash
wrtbox diff --router home
```

### Apply atomically

```bash
wrtbox apply --router home
```

On failure the router is rolled back to the pre-apply snapshot automatically.

## Repository structure

```
├── cmd/wrtbox/     CLI entry point
├── internal/       Core packages (config, render, apply, ssh, device)
├── examples/       Example wrtbox.yaml files
├── docs/devices/   Per-device YAML override recipes
└── scripts/emu/    OpenWrt emulator VM lifecycle for CI (x86_64 + arm64)
```

## License

[MIT](LICENSE)
