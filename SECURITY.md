# Security

`wrtbox` is a small tool that generates OpenWrt configs and applies them to routers via SSH. Please read this before using it on anything that matters.

## Reporting a vulnerability

If you find a security issue, **do not open a public GitHub issue** — use a private channel so it can be fixed before disclosure:

1. [GitHub Security Advisory](https://github.com/itslavrov/wrtbox/security/advisories/new) — preferred
2. Direct message to the maintainer on GitHub

A useful report includes: what the issue is, how to reproduce it, and what an attacker could do with it. A suggested fix is a bonus, not a requirement.

This is a personal project — I'll respond as fast as I reasonably can, but please don't expect enterprise SLAs.

## Threat model

`wrtbox` runs on your workstation and pushes configuration to one or more OpenWrt routers over SSH. The main trust boundaries:

- **Workstation → router.** The tool executes commands as `root` on the router. If your workstation is compromised, so is every router it can reach.
- **YAML configs carry secrets.** Xray UUIDs, Reality keys, VPN credentials live in `wrtbox.yaml`. Treat it like any other secret: don't commit it to public repos, don't share unfiltered.
- **SSH auth.** `wrtbox` uses a local private key you point it at. New host keys are accepted only with explicit `--accept-new-host-key`; any existing mismatch is fatal.
- **Rendered files are executed on the router.** UCI files, xray JSON and shell scripts produced from YAML land in `/etc/` and `/usr/bin/`. Fields that end up in shell scripts need careful escaping — see `internal/render` tests.

## What `wrtbox` does NOT do

- It does not encrypt your `wrtbox.yaml` at rest. If you need that, use `git-crypt`, `sops`, or similar.
- It does not remove files it did not write. The list of files it manages is hardcoded (`internal/apply/apply.go:managedPaths`). Anything else on the router is left alone.
- It does not roll back automatically if your workstation dies mid-apply — only if wrtbox itself sees a post-check failure. In the worst case, the staged config is in `/root/wrtbox-staging/<ts>/` and a snapshot is in `/root/wrtbox-backups/<ts>/` on the router.

## Integration tests

Integration tests spin up throwaway OpenWrt VMs in an OpenStack-compatible cloud and run the full apply pipeline against them. Only use non-production credentials and isolated network zones for the CI account.
