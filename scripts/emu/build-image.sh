#!/usr/bin/env bash
# Build a wrtbox-ready OpenWrt x86_64 Glance image on an OpenStack-
# compatible cloud.
#
# Flow: spin a throwaway Ubuntu builder VM whose cloud-init pulls the
# official OpenWrt ext4-combined image, mounts its rootfs, injects a
# uci-defaults script (SSH pubkey + DHCP WAN), converts to qcow2 →
# sftp pulled back → Glance upload → the builder is destroyed.
# Keypair/SG are preserved for re-use.
#
# Requires: an openrc sourced in the environment. Idempotent: a second
# run replaces the Glance image with the latest baked version.
set -euo pipefail

: "${OS_AUTH_URL:?source your openrc first}"

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

OPENWRT_VERSION="${OPENWRT_VERSION:-23.05.5}"
NETWORK_ID="${EMU_NETWORK_ID:?set EMU_NETWORK_ID to the target private-network UUID}"
NETWORK_CIDR="${EMU_NETWORK_CIDR:-10.0.0.0/8}"
KEYPAIR_NAME="${EMU_KEYPAIR:-wrtbox-emu}"
PUBKEY_FILE="${EMU_PUBKEY_FILE:-$HOME/.ssh/wrtbox-emu.pub}"
PRIVKEY_FILE="${EMU_PRIVKEY_FILE:-$HOME/.ssh/wrtbox-emu}"
BUILDER_NAME="wrtbox-builder-$$"
BUILDER_FLAVOR="${EMU_BUILDER_FLAVOR:-Basic-1-2-20}"
BUILDER_IMAGE="${EMU_BUILDER_IMAGE:-ubuntu-22-202602051629.gite7a38aaf}"
BUILDER_SG="wrtbox-builder-sg"
FINAL_IMAGE_NAME="openwrt-wrtbox-emu-${OPENWRT_VERSION}"

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

# --- preflight ---------------------------------------------------------------
[ -f "$PUBKEY_FILE" ]  || { echo "missing $PUBKEY_FILE"; exit 1; }
[ -f "$PRIVKEY_FILE" ] || { echo "missing $PRIVKEY_FILE"; exit 1; }

if ! openstack keypair show "$KEYPAIR_NAME" >/dev/null 2>&1; then
  log "registering keypair $KEYPAIR_NAME"
  openstack keypair create --public-key "$PUBKEY_FILE" "$KEYPAIR_NAME" >/dev/null
fi

if ! openstack security group show "$BUILDER_SG" >/dev/null 2>&1; then
  log "creating security group $BUILDER_SG"
  openstack security group create "$BUILDER_SG" --description "wrtbox builder (transient)" >/dev/null
  openstack security group rule create --proto tcp --dst-port 22 \
    --remote-ip "$NETWORK_CIDR" "$BUILDER_SG" >/dev/null
  openstack security group rule create --proto icmp \
    --remote-ip "$NETWORK_CIDR" "$BUILDER_SG" >/dev/null
fi

# --- render userdata ---------------------------------------------------------
TMP_UD="$(mktemp)"
trap 'rm -f "$TMP_UD"' EXIT
# Substitute placeholders. Use awk to keep pubkey intact (no sed-special chars).
PUBKEY="$(cat "$PUBKEY_FILE")"
awk -v pubkey="$PUBKEY" -v version="$OPENWRT_VERSION" '
  { gsub(/@@OPENWRT_PUBKEY@@/, pubkey); gsub(/@@OPENWRT_VERSION@@/, version); print }
' "$HERE/builder-userdata.sh.tmpl" > "$TMP_UD"

# --- boot builder ------------------------------------------------------------
log "booting builder $BUILDER_NAME (flavor=$BUILDER_FLAVOR image=$BUILDER_IMAGE)"
SERVER_RAW=$(openstack server create \
  --image "$BUILDER_IMAGE" \
  --flavor "$BUILDER_FLAVOR" \
  --key-name "$KEYPAIR_NAME" \
  --network "$NETWORK_ID" \
  --security-group "$BUILDER_SG" \
  --user-data "$TMP_UD" \
  --wait \
  -f value -c id \
  "$BUILDER_NAME")
SERVER_ID=$(printf '%s' "$SERVER_RAW" | grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)
[ -n "$SERVER_ID" ] || { echo "failed to capture server UUID, raw=$SERVER_RAW"; exit 1; }
log "builder server_id=$SERVER_ID"

cleanup_builder() {
  log "destroying builder $BUILDER_NAME"
  openstack server delete --wait "$SERVER_ID" 2>/dev/null || true
}
trap 'cleanup_builder; rm -f "$TMP_UD"' EXIT

# Poll for fixed IP (ACTIVE state doesn't guarantee IP is attached immediately).
# addresses is rendered as either "net=<ip>" (value fmt) or JSON dict.
BUILDER_IP=""
for _ in $(seq 1 30); do
  BUILDER_IP=$(openstack server show "$SERVER_ID" -f json -c addresses 2>/dev/null \
    | python3 -c '
import json,sys
try: d=json.load(sys.stdin)["addresses"]
except Exception: sys.exit(0)
def walk(x):
  if isinstance(x,str):
    for t in x.replace(";"," ").split():
      if "=" in t: t=t.split("=",1)[1]
      for ip in t.split(","):
        if ip and ip[0].isdigit(): print(ip); sys.exit(0)
  elif isinstance(x,list):
    for v in x: walk(v)
  elif isinstance(x,dict):
    for v in x.values(): walk(v)
walk(d)
' 2>/dev/null || true)
  [ -n "$BUILDER_IP" ] && break
  sleep 2
done
[ -n "$BUILDER_IP" ] || { echo "no IP assigned"; exit 1; }
log "builder IP=$BUILDER_IP"

# Builder IPs are transient and often reused by later builds — always
# drop any stale host key for this IP before we attempt SSH.
ssh-keygen -f "$HOME/.ssh/known_hosts_wrtbox" -R "$BUILDER_IP" >/dev/null 2>&1 || true
SSH="ssh -i $PRIVKEY_FILE -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=$HOME/.ssh/known_hosts_wrtbox -o ConnectTimeout=5"

# Wait for SSH up.
log "waiting for SSH on $BUILDER_IP"
for _ in $(seq 1 60); do
  if $SSH "ubuntu@$BUILDER_IP" true 2>/dev/null; then break; fi
  sleep 5
done
$SSH "ubuntu@$BUILDER_IP" true

# Wait for cloud-init + build.sh to finish.
log "waiting for image bake (polls /root/BUILD_DONE)"
for i in $(seq 1 120); do  # up to 20 min
  STATE=$($SSH "ubuntu@$BUILDER_IP" "sudo test -f /root/BUILD_DONE && sudo head -c 6 /root/BUILD_DONE || echo PENDING" 2>/dev/null || echo PENDING)
  case "$STATE" in
    -rw*|*"qcow2"*|*"1 root"*|*"ow.qcow"*|*"openwrt"*) log "bake complete after ~$((i*10))s"; break ;;
    FAILED) echo "build failed on builder; tail of log:"; $SSH "ubuntu@$BUILDER_IP" "sudo tail -50 /root/build.log"; exit 1 ;;
  esac
  sleep 10
done
$SSH "ubuntu@$BUILDER_IP" "sudo test -f /root/openwrt-wrtbox-emu.qcow2" \
  || { echo "qcow2 missing on builder"; $SSH "ubuntu@$BUILDER_IP" "sudo tail -80 /root/build.log"; exit 1; }

# Pull qcow2 to Mac.
LOCAL_QCOW="$(mktemp -d)/openwrt-wrtbox-emu.qcow2"
log "fetching qcow2 → $LOCAL_QCOW"
$SSH "ubuntu@$BUILDER_IP" "sudo cp /root/openwrt-wrtbox-emu.qcow2 /tmp/ && sudo chown ubuntu /tmp/openwrt-wrtbox-emu.qcow2"
scp -i "$PRIVKEY_FILE" -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=$HOME/.ssh/known_hosts_wrtbox \
  "ubuntu@$BUILDER_IP:/tmp/openwrt-wrtbox-emu.qcow2" "$LOCAL_QCOW"

# Replace existing Glance image if present.
if openstack image show "$FINAL_IMAGE_NAME" >/dev/null 2>&1; then
  log "deleting previous Glance image $FINAL_IMAGE_NAME"
  openstack image delete "$FINAL_IMAGE_NAME"
fi
log "uploading to Glance as $FINAL_IMAGE_NAME"
openstack image create --disk-format qcow2 --container-format bare \
  --file "$LOCAL_QCOW" --private "$FINAL_IMAGE_NAME" -f value -c id

rm -f "$LOCAL_QCOW"
rmdir "$(dirname "$LOCAL_QCOW")" 2>/dev/null || true

log "done — Glance image: $FINAL_IMAGE_NAME"
