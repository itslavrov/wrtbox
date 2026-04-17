#!/usr/bin/env bash
# ARM64 OpenWrt emulator via nested QEMU (aarch64 virt) inside the host
# x86_64 VM. Keeps the "all testing through VK Cloud + virtualisation"
# constraint: no physical hardware required.
#
#   arm64.sh up       — fetch image if missing, boot qemu in background
#   arm64.sh down     — kill the qemu process, keep the image
#   arm64.sh ip       — print the SSH endpoint (always host:2222)
#   arm64.sh ssh ...  — ssh into the booted guest (passes extra args)
#   arm64.sh status   — print process state + log tail
#
# When WRTBOX_ARM64_PUBKEY points to an SSH pubkey file, the work image
# is pre-provisioned on each `up`: pubkey baked into dropbear
# authorized_keys, root password cleared, dropbear set to key-only.
# Requires libguestfs-tools (virt-customize) when this path is taken.
# Without WRTBOX_ARM64_PUBKEY the image is booted untouched — retains
# the original smoke-test behaviour.
#
# Requires: qemu-system-aarch64, qemu-efi-aarch64, curl, gunzip, ssh.
# Optional (only if WRTBOX_ARM64_PUBKEY set): virt-customize (libguestfs-tools).
set -euo pipefail

HERE_DIR="$(cd "$(dirname "$0")" && pwd)"
STATE_DIR="${WRTBOX_ARM64_STATE:-$HOME/.cache/wrtbox-arm64}"
OPENWRT_VERSION="${OPENWRT_ARM64_VERSION:-23.05.5}"
IMG_URL="https://downloads.openwrt.org/releases/${OPENWRT_VERSION}/targets/armsr/armv8/openwrt-${OPENWRT_VERSION}-armsr-armv8-generic-ext4-combined.img.gz"
IMG_GZ="$STATE_DIR/openwrt-${OPENWRT_VERSION}-armsr-armv8.img.gz"
IMG="$STATE_DIR/openwrt-${OPENWRT_VERSION}-armsr-armv8.img"
WORK_IMG="$STATE_DIR/work.img"                    # copy-on-write overlay
EFI_BIOS="${WRTBOX_ARM64_EFI:-/usr/share/qemu-efi-aarch64/QEMU_EFI.fd}"
PIDFILE="$STATE_DIR/qemu.pid"
LOGFILE="$STATE_DIR/qemu.log"
SSH_PORT="${WRTBOX_ARM64_PORT:-2222}"
MEM="${WRTBOX_ARM64_MEM:-512M}"
CPU="${WRTBOX_ARM64_CPU:-cortex-a72}"
PUBKEY_FILE="${WRTBOX_ARM64_PUBKEY:-}"

log() { printf '[arm64] %s\n' "$*" >&2; }

ensure_image() {
  mkdir -p "$STATE_DIR"
  if [ ! -f "$IMG" ]; then
    log "fetching $IMG_URL"
    curl -fsSL -o "$IMG_GZ" "$IMG_URL"
    # OpenWrt's .img.gz carries a trailing padding block that makes
    # gzip exit 2 ("decompression OK, trailing garbage ignored"). That
    # is informational, not a failure — suppress it so `set -e` is happy.
    gunzip -kf "$IMG_GZ" || { rc=$?; [ "$rc" -eq 2 ] || { log "gunzip failed rc=$rc"; exit 1; }; }
    mv "${IMG_GZ%.gz}" "$IMG"
  fi
  # Always restart from a pristine copy so tests are idempotent.
  cp "$IMG" "$WORK_IMG"
  customize_image
}

# customize_image injects the operator SSH pubkey into the work image via
# libguestfs. Idempotent per-boot because ensure_image always starts from
# a fresh copy of $IMG. No-op when WRTBOX_ARM64_PUBKEY is not set — that
# preserves the original smoke path.
#
# OpenWrt uses dropbear, which reads /etc/dropbear/authorized_keys. We
# also nudge dropbear into key-only mode so a leaked empty-password
# emu can't be abused if the port ever escaped the loopback.
customize_image() {
  if [ -z "$PUBKEY_FILE" ]; then
    return 0
  fi
  if [ ! -f "$PUBKEY_FILE" ]; then
    log "WRTBOX_ARM64_PUBKEY=$PUBKEY_FILE not found"
    exit 1
  fi
  if ! command -v virt-customize >/dev/null 2>&1; then
    log "virt-customize not installed — apt-get install libguestfs-tools"
    exit 1
  fi
  # Some Debian/Ubuntu kernels ship /boot/vmlinuz-* as mode 600, which
  # breaks libguestfs's appliance-less mode for non-root users. Harmless
  # if already readable; fails loud otherwise.
  if [ -d /boot ] && [ ! -r /boot ]; then
    log "warning: /boot is not world-readable — virt-customize may fail"
  fi
  log "baking pubkey from $PUBKEY_FILE into work image"
  local auth_tmp
  auth_tmp=$(mktemp)
  tr -d '\r' <"$PUBKEY_FILE" >"$auth_tmp"
  # Use only filesystem-level ops (mkdir/upload/chmod). --run-command
  # would require executing binaries from the guest image, which
  # libguestfs cannot do when host (x86_64) and guest (aarch64) archs
  # differ. FS ops go through the appliance kernel directly and work
  # cross-arch. /etc/dropbear already exists in the OpenWrt image, but
  # --mkdir is idempotent.
  #
  # Also replace /etc/config/network so eth0 is the WAN interface with
  # DHCP. OpenWrt's armsr-armv8 default puts eth0 in br-lan (LAN only),
  # which means QEMU user-mode NAT's DHCP offer goes unanswered and
  # host-forwarded SSH on :2222 can never reach the guest.
  local net_cfg="$HERE_DIR/arm64-network.config"
  local db_cfg="$HERE_DIR/arm64-dropbear.config"
  local fw_cfg="$HERE_DIR/arm64-firewall.config"
  local rc_local="$HERE_DIR/arm64-rc.local"
  for f in "$net_cfg" "$db_cfg" "$fw_cfg" "$rc_local"; do
    [ -f "$f" ] || { log "missing config template at $f"; exit 1; }
  done
  # Also overwrite /etc/config/dropbear to drop the default
  # `option Interface 'lan'` — without this, dropbear binds only to LAN
  # and ignores incoming connections on the WAN (eth0) where QEMU NAT
  # lives. Without that restriction it listens on 0.0.0.0:22.
  #
  # Finally, bake /etc/rc.local so the guest dumps network/dropbear
  # state to /dev/console at end of boot. Without this, qemu.log only
  # shows kernel messages — userspace output goes to logd and is
  # invisible from outside, which makes SSH-timeout failures impossible
  # to diagnose remotely.
  LIBGUESTFS_BACKEND=direct virt-customize -a "$WORK_IMG" \
    --mkdir /etc/dropbear \
    --upload "$auth_tmp:/etc/dropbear/authorized_keys" \
    --chmod '0600:/etc/dropbear/authorized_keys' \
    --chmod '0700:/etc/dropbear' \
    --upload "$net_cfg:/etc/config/network" \
    --chmod '0644:/etc/config/network' \
    --upload "$db_cfg:/etc/config/dropbear" \
    --chmod '0644:/etc/config/dropbear' \
    --upload "$fw_cfg:/etc/config/firewall" \
    --chmod '0644:/etc/config/firewall' \
    --upload "$rc_local:/etc/rc.local" \
    --chmod '0755:/etc/rc.local' \
    >/dev/null
  rm -f "$auth_tmp"
  log "pubkey + wan network + dropbear + firewall + rc.local baked"
}

pid_alive() {
  [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null
}

cmd_up() {
  if pid_alive; then
    log "already running (pid=$(cat "$PIDFILE"))"
    return 0
  fi
  ensure_image
  if [ ! -f "$EFI_BIOS" ]; then
    log "missing UEFI firmware at $EFI_BIOS — install qemu-efi-aarch64"
    exit 1
  fi
  log "booting qemu aarch64 (mem=$MEM cpu=$CPU ssh=$SSH_PORT)"
  : >"$LOGFILE"
  nohup qemu-system-aarch64 \
    -M virt \
    -cpu "$CPU" \
    -smp 2 \
    -m "$MEM" \
    -nographic \
    -bios "$EFI_BIOS" \
    -drive "file=$WORK_IMG,format=raw,if=virtio" \
    -netdev "user,id=net0,hostfwd=tcp::${SSH_PORT}-:22" \
    -device virtio-net-pci,netdev=net0 \
    >"$LOGFILE" 2>&1 &
  echo $! >"$PIDFILE"
  log "pid=$(cat "$PIDFILE") — waiting for SSH on 127.0.0.1:${SSH_PORT}"
  for _ in $(seq 1 120); do
    if (echo >/dev/tcp/127.0.0.1/"$SSH_PORT") >/dev/null 2>&1; then
      log "SSH port open"
      return 0
    fi
    sleep 2
  done
  log "SSH never came up — log tail:"
  tail -200 "$LOGFILE" >&2
  exit 1
}

cmd_down() {
  if ! pid_alive; then
    log "not running"
    rm -f "$PIDFILE"
    return 0
  fi
  local pid; pid=$(cat "$PIDFILE")
  log "stopping qemu pid=$pid"
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 30); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 1
  done
  kill -9 "$pid" 2>/dev/null || true
  rm -f "$PIDFILE"
}

cmd_ip() {
  # Canonical endpoint is always localhost:SSH_PORT — the outer VM is
  # the one CI reaches, so "ip" really means the port to use.
  echo "127.0.0.1:${SSH_PORT}"
}

cmd_ssh() {
  exec ssh -p "$SSH_PORT" -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile="$STATE_DIR/known_hosts" \
    -o LogLevel=ERROR root@127.0.0.1 "$@"
}

cmd_status() {
  if pid_alive; then
    printf 'running pid=%s port=%s\n' "$(cat "$PIDFILE")" "$SSH_PORT"
  else
    printf 'stopped\n'
  fi
  if [ -f "$LOGFILE" ]; then
    printf -- '---- qemu.log tail (200 lines) ----\n'
    tail -200 "$LOGFILE"
  fi
}

case "${1:-}" in
  up)     cmd_up ;;
  down)   cmd_down ;;
  ip)     cmd_ip ;;
  ssh)    shift; cmd_ssh "$@" ;;
  status) cmd_status ;;
  *)
    cat >&2 <<EOF
Usage: $(basename "$0") {up|down|ip|ssh [args…]|status}
EOF
    exit 2
    ;;
esac
