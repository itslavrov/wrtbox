# Device recipes

This directory collects **community-contributed device overrides** for
routers that wrtbox does not ship a built-in adapter for. Each recipe is
a small, self-contained YAML fragment you drop under
`spec.device.overrides` in your `wrtbox.yaml`.

> **Status:** recipes are tested by individual contributors, not by the
> wrtbox maintainers. They work for the person who submitted them.
> Confirmed? Open a PR adding your OpenWrt version to the "Tested on"
> section. Broken? Open an issue — we'll update or remove the recipe.

## How to use a recipe

1. Install OpenWrt 23.05+ on your device (check the [OpenWrt Table of
   Hardware](https://openwrt.org/toh/start) for instructions).
2. `wrtbox detect --router <alias>` — sanity-check that `ubus` runs.
3. Copy the YAML fragment from the recipe file into the `spec.device`
   block of your config:
   ```yaml
   spec:
     device:
       model: <recipe-name>
       overrides:
         # paste the whole overrides block from the recipe here
   ```
4. `wrtbox validate -c wrtbox.yaml` — schema check.
5. `wrtbox apply --router <alias>` — standard safe apply with automatic
   rollback on failure.

## Available recipes

| File | Device | Tested on |
|------|--------|-----------|
| [gl-axt1800.md](gl-axt1800.md) | GL.iNet GL-AXT1800 (Slate AX) | OpenWrt 23.05 (Community) |
| [xiaomi-ax3000t.md](xiaomi-ax3000t.md) | Xiaomi AX3000T (global) | OpenWrt SNAPSHOT (Community) |
| [keenetic-giga.md](keenetic-giga.md) | Keenetic Giga KN-1011 | OpenWrt 23.05 (Community) |

## Contributing a new recipe

A good recipe has five things:

1. **Model key**: a short hyphenated string (e.g. `xiaomi-ax3000t`). Used
   literally in `spec.device.model`.
2. **WAN interface**: what `ip -br link show` calls your upstream port.
3. **LAN ports**: the DSA member names on the switch (`ip -br link show`
   again, or `/sys/class/net/*`).
4. **Radio paths**: from `find /sys/devices -name 'phy*' -type d` — the
   part under `platform/`.
5. **Required packages**: `opkg list-installed | grep -E 'xray|nft'` to
   see what is preinstalled; ship the rest.

Keep recipes **minimal** — only the fields that actually differ from the
generic defaults. Anything a user can type themselves does not belong.
