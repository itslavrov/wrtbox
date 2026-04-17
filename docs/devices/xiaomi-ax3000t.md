# Xiaomi AX3000T (global)

MT7981B SoC + MT7976 radios. Standard OpenWrt DSA board; WAN is eth1.

## Recipe

```yaml
spec:
  device:
    model: xiaomi-ax3000t
    overrides:
      wan_interface: eth1
      lan_ports: [lan1, lan2, lan3]
      radios:
        - { name: radio0, band: 2g, htmode: HE20, path: "platform/soc/18000000.wifi" }
        - { name: radio1, band: 5g, htmode: HE80, path: "platform/soc/18000000.wifi+1" }
      required_packages:
        - xray-core
        - kmod-nft-tproxy
        - xray-geodata
      post_apply:
        - /etc/init.d/dnsmasq restart
```

## How to verify on your unit

```sh
# WAN:
ip -br link show | grep -i eth
# Radios:
find /sys/devices -name 'phy*' -type d | head
# Packages:
opkg list-installed | grep -E 'xray|nft'
```

If `find` returns paths different from the ones above (e.g. the SoC base
address differs), swap them into `radios[].path`.

## Tested on

- OpenWrt SNAPSHOT r25xxx — Community (open a PR to add your version).
