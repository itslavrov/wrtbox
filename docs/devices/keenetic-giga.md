# Keenetic Giga (KN-1011)

MT7621 SoC + MT7915. Runs an OpenWrt 23.05 port; WAN is on eth4 in
vendor's original layout but OpenWrt's DSA driver renames it to `wan`
via the DTS. Keep the DTS-given names.

## Recipe

```yaml
spec:
  device:
    model: keenetic-giga
    overrides:
      wan_interface: wan
      lan_ports: [lan1, lan2, lan3, lan4]
      radios:
        - { name: radio0, band: 2g, htmode: HE40, path: "platform/1e100000.pcie/pci0000:00/0000:00:00.0/0000:01:00.0" }
        - { name: radio1, band: 5g, htmode: HE80, path: "platform/1e100000.pcie/pci0000:00/0000:00:00.0/0000:01:00.0+1" }
      required_packages:
        - xray-core
        - kmod-nft-tproxy
        - xray-geodata
      post_apply:
        - /etc/init.d/dnsmasq restart
```

## How to verify on your unit

Keenetic's switch layout changes between firmwares. Always run:

```sh
ip -br link show
cat /etc/board.json | jq '.network'
```

If `wan` doesn't show up, use the explicit DSA member name (`lan5`,
`eth0.2`, etc.) — whatever the DTS calls the upstream port.

## Notes

- Keep vendor-stock firmware away from this setup. The recipe assumes
  pure OpenWrt.
- PCIe-path radios can jitter between boots on some firmware builds; if
  wifi fails to come up after an apply, re-check the `path` with a
  fresh `find /sys/devices -name 'phy*'`.

## Tested on

- OpenWrt 23.05 (Community).
