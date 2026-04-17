# GL.iNet GL-AXT1800 (Slate AX)

IPQ6000 SoC, dual-band. Four-port switch; WAN is on a dedicated NIC
(eth1), the LAN ports are the DSA `lan1..lan4` members.

## Recipe

```yaml
spec:
  device:
    model: gl-axt1800
    overrides:
      wan_interface: eth1
      lan_ports: [lan1, lan2, lan3, lan4]
      radios:
        - { name: radio0, band: 2g, htmode: HE40, path: "platform/soc/c000000.wifi" }
        - { name: radio1, band: 5g, htmode: HE80, path: "platform/soc/c000000.wifi+1" }
      required_packages:
        - xray-core
        - kmod-nft-tproxy
        - xray-geodata
```

## How to verify on your unit

```sh
ip -br link show
find /sys/devices -name 'phy*' -type d
opkg list-installed | grep -E 'xray|nft'
```

The IPQ6000 radios commonly sit at `c000000.wifi` / `c000000.wifi+1`.
If your firmware reshuffles them, read the paths off `/sys/class/ieee80211/phy*/device/of_node`.

## Notes

- Stock GL.iNet firmware is heavily customised; these overrides assume
  vanilla OpenWrt 23.05+ built from `ipq60xx/generic`.
- If you keep GL.iNet's stock firmware, use their LuCI and don't run
  wrtbox against it — no guarantees on overlapping UCI namespaces.

## Tested on

- OpenWrt 23.05 (Community).
