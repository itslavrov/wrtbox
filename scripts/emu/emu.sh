#!/usr/bin/env bash
# Emulator VM lifecycle helper for an OpenStack-compatible cloud.
#
#   emu.sh up       — boot wrtbox-emu VM from the baked Glance image
#   emu.sh down     — destroy wrtbox-emu VM (image is kept)
#   emu.sh rebuild  — destroy VM + bake a fresh Glance image + boot VM
#   emu.sh ip       — print fixed IP of the running VM
#   emu.sh ssh      — open an SSH session to the VM
#
# Requires: an openrc sourced in the environment.
set -euo pipefail

: "${OS_AUTH_URL:?source your openrc first}"

HERE="$(cd "$(dirname "$0")" && pwd)"
OPENWRT_VERSION="${OPENWRT_VERSION:-23.05.5}"
NETWORK_ID="${EMU_NETWORK_ID:?set EMU_NETWORK_ID to the target private-network UUID}"
NETWORK_CIDR="${EMU_NETWORK_CIDR:-10.0.0.0/8}"
KEYPAIR_NAME="${EMU_KEYPAIR:-wrtbox-emu}"
PRIVKEY_FILE="${EMU_PRIVKEY_FILE:-$HOME/.ssh/wrtbox-emu}"
VM_NAME="${EMU_VM_NAME:-wrtbox-emu}"
VM_FLAVOR="${EMU_VM_FLAVOR:-Basic-1-2-20}"
SG_NAME="wrtbox-emu-sg"
IMAGE_NAME="openwrt-wrtbox-emu-${OPENWRT_VERSION}"
KNOWN_HOSTS="$HOME/.ssh/known_hosts_wrtbox"

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

ensure_sg() {
  if ! openstack security group show "$SG_NAME" >/dev/null 2>&1; then
    log "creating SG $SG_NAME"
    openstack security group create "$SG_NAME" --description "wrtbox emu SG" >/dev/null
    openstack security group rule create --proto tcp --dst-port 22 \
      --remote-ip "$NETWORK_CIDR" "$SG_NAME" >/dev/null
    openstack security group rule create --proto icmp \
      --remote-ip "$NETWORK_CIDR" "$SG_NAME" >/dev/null
  fi
}

vm_id() {
  openstack server list --name "^${VM_NAME}$" -f value -c ID 2>/dev/null | head -1
}

vm_ip() {
  local id="$1" ip=""
  # addresses is rendered like "<net>=<ip>".
  ip=$(openstack server show "$id" -f value -c addresses 2>/dev/null \
    | grep -oE '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
  printf '%s' "$ip"
}

cmd_up() {
  local id
  id=$(vm_id)
  if [ -n "$id" ]; then
    log "$VM_NAME already exists ($id) — reuse"
    local ip; ip=$(vm_ip "$id")
    log "ip=$ip"
    return 0
  fi
  openstack image show "$IMAGE_NAME" >/dev/null 2>&1 \
    || { echo "Glance image $IMAGE_NAME not found — run build-image.sh first" >&2; exit 1; }
  ensure_sg
  log "booting $VM_NAME from $IMAGE_NAME"
  local raw
  raw=$(openstack server create \
    --image "$IMAGE_NAME" \
    --flavor "$VM_FLAVOR" \
    --key-name "$KEYPAIR_NAME" \
    --network "$NETWORK_ID" \
    --security-group "$SG_NAME" \
    --wait \
    -f value -c id \
    "$VM_NAME")
  id=$(printf '%s' "$raw" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)
  local ip; ip=$(vm_ip "$id")
  ssh-keygen -f "$KNOWN_HOSTS" -R "$ip" >/dev/null 2>&1 || true
  log "up: id=$id ip=$ip"
  log "hint: ssh -i $PRIVKEY_FILE -o UserKnownHostsFile=$KNOWN_HOSTS root@$ip"
}

cmd_down() {
  local id; id=$(vm_id)
  [ -n "$id" ] || { log "$VM_NAME already gone"; return 0; }
  local ip; ip=$(vm_ip "$id")
  log "destroying $VM_NAME ($id, ip=$ip)"
  openstack server delete --wait "$id"
  [ -n "$ip" ] && ssh-keygen -f "$KNOWN_HOSTS" -R "$ip" >/dev/null 2>&1 || true
}

cmd_rebuild() {
  cmd_down
  log "deleting Glance image $IMAGE_NAME"
  openstack image delete "$IMAGE_NAME" >/dev/null 2>&1 || true
  log "running build-image.sh to bake a fresh image"
  bash "$HERE/build-image.sh"
  cmd_up
}

cmd_ip() {
  local id; id=$(vm_id)
  [ -n "$id" ] || { echo "not-running" >&2; exit 1; }
  vm_ip "$id"
  echo
}

cmd_ssh() {
  local id; id=$(vm_id)
  [ -n "$id" ] || { echo "$VM_NAME not running" >&2; exit 1; }
  local ip; ip=$(vm_ip "$id")
  exec ssh -i "$PRIVKEY_FILE" \
    -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile="$KNOWN_HOSTS" \
    "root@$ip" "$@"
}

case "${1:-}" in
  up)      cmd_up ;;
  down)    cmd_down ;;
  rebuild) cmd_rebuild ;;
  ip)      cmd_ip ;;
  ssh)     shift; cmd_ssh "$@" ;;
  *) echo "usage: $0 {up|down|rebuild|ip|ssh [cmd...]}" >&2; exit 2 ;;
esac
