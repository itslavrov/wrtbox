package render

// These helper scripts are deployed alongside the generated configs.
// They are intentionally invariant in v1 — tproxy port and mark are
// fixed protocol-level constants that the rest of the rendered config
// already references. Parameterising them buys no real flexibility
// today and only adds surface area.

const initdXrayRules = `#!/bin/sh /etc/rc.common

START=90
STOP=10

start() {
    nft list table inet xray >/dev/null 2>&1 || nft add table inet xray
    nft -f /etc/xray/nft.conf 2>/dev/null

    ip rule list | grep -q 'fwmark 0x1' || ip rule add fwmark 1 lookup 100 prio 100
    ip route show table 100 | grep -q 'local default' || ip route add local 0.0.0.0/0 dev lo table 100

    echo "xray-rules started"
}

stop() {
    nft delete table inet xray 2>/dev/null
    ip rule del fwmark 1 lookup 100 2>/dev/null
    ip route flush table 100 2>/dev/null
    echo "xray-rules stopped"
}

restart() {
    stop
    sleep 1
    start
}
`

const hotplugXrayRules = `#!/bin/sh
# Restore xray ip rules + routing table after an interface comes up.

[ "$ACTION" = "ifup" ] || exit 0

case "$INTERFACE" in
    wan|lan|pppoe-wan) ;;
    *) exit 0 ;;
esac

logger -t xray-rules-hotplug "Interface $INTERFACE is up, restoring rules..."
sleep 2

if ! ip rule list | grep -q 'fwmark 0x1'; then
    ip rule add fwmark 1 lookup 100 prio 100
    logger -t xray-rules-hotplug "Added: ip rule fwmark 1 lookup 100"
fi

if ! ip route show table 100 | grep -q 'local default'; then
    ip route add local 0.0.0.0/0 dev lo table 100
    logger -t xray-rules-hotplug "Added: route local default dev lo table 100"
fi

if ! nft list table inet xray >/dev/null 2>&1; then
    nft -f /etc/xray/nft.conf 2>&1 | logger -t xray-rules-hotplug
fi

logger -t xray-rules-hotplug "Done"
`

const updateAntifilter = `#!/bin/sh
# Refresh /etc/xray/lists/antifilter.txt from antifilter.download and
# rewrite /etc/xray/config.json with the new CIDR set. Atomic: validates
# with 'xray -test' before swapping in.

TAG="antifilter-updater"
URL="https://antifilter.download/list/ipsum.lst"
LIST_FILE="/etc/xray/lists/antifilter.txt"
LIST_TMP="/tmp/antifilter.tmp"
CONFIG="/etc/xray/config.json"
CONFIG_TMP="/tmp/xray.config.new"
CIDRS_JSON="/tmp/xray.cidrs.json"

logger -t "$TAG" "Starting antifilter update..."

if ! curl -m 60 -s -o "$LIST_TMP" "$URL"; then
    logger -t "$TAG" "ERROR: download failed"
    rm -f "$LIST_TMP"
    exit 1
fi

SIZE=$(wc -c < "$LIST_TMP" 2>/dev/null || echo 0)
LINES=$(wc -l < "$LIST_TMP" 2>/dev/null || echo 0)
if [ "$SIZE" -lt 10000 ]; then
    logger -t "$TAG" "ERROR: file too small ($SIZE bytes)"
    rm -f "$LIST_TMP"
    exit 1
fi

FIRST_LINE=$(head -1 "$LIST_TMP")
if ! echo "$FIRST_LINE" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$'; then
    logger -t "$TAG" "ERROR: invalid format, first: $FIRST_LINE"
    rm -f "$LIST_TMP"
    exit 1
fi

if [ -f "$LIST_FILE" ] && cmp -s "$LIST_TMP" "$LIST_FILE"; then
    logger -t "$TAG" "No changes ($LINES lines)"
    rm -f "$LIST_TMP"
    exit 0
fi

mv "$LIST_TMP" "$LIST_FILE"
logger -t "$TAG" "List updated: $LINES lines, $SIZE bytes"

jq -R . "$LIST_FILE" | jq -s . > "$CIDRS_JSON"

jq --slurpfile cidrs "$CIDRS_JSON" '
  .routing.rules as $rules |
  .routing.rules = (
    reduce range(0; $rules | length) as $i ([];
      if ($rules[$i]._antifilter // false)
      then .
      elif ($rules[$i].ip // [] | contains(["geoip:ru"])) and (. | map(._antifilter // false) | any | not)
      then . + [{
        "type": "field",
        "ip": $cidrs[0],
        "outboundTag": "vless-out",
        "_antifilter": true
      }, $rules[$i]]
      else . + [$rules[$i]]
      end
    )
  ) |
  del(.routing.rules[]?._antifilter)
' "$CONFIG" > "$CONFIG_TMP"

if ! xray -test -c "$CONFIG_TMP" >/dev/null 2>&1; then
    logger -t "$TAG" "ERROR: xray config validation failed"
    rm -f "$CONFIG_TMP" "$CIDRS_JSON"
    exit 1
fi

mv "$CONFIG_TMP" "$CONFIG"
rm -f "$CIDRS_JSON"

/etc/init.d/xray restart 2>&1 | logger -t "$TAG"
sleep 2
if pgrep xray >/dev/null; then
    logger -t "$TAG" "Xray restarted OK, PID=$(pgrep xray)"
else
    logger -t "$TAG" "ERROR: Xray not running after restart!"
fi

logger -t "$TAG" "Done"
`

const updateGeosite = `#!/bin/sh
# Refresh v2ray-geosite + v2ray-geoip via apk and restart Xray.

TAG="geosite-updater"
logger -t "$TAG" "Starting geo-data update..."

if ! apk update 2>&1 | logger -t "$TAG"; then
    logger -t "$TAG" "ERROR: apk update failed, aborting"
    exit 1
fi

UPGRADES=$(apk version 2>/dev/null | grep -E 'v2ray-geo' || true)
if [ -z "$UPGRADES" ]; then
    logger -t "$TAG" "No updates for v2ray-geo packages"
    exit 0
fi

logger -t "$TAG" "Updates available:"
echo "$UPGRADES" | logger -t "$TAG"

if apk upgrade v2ray-geosite v2ray-geoip 2>&1 | logger -t "$TAG"; then
    logger -t "$TAG" "Packages upgraded successfully"
    /etc/init.d/xray restart 2>&1 | logger -t "$TAG"
    sleep 2
    if pgrep xray >/dev/null; then
        logger -t "$TAG" "Xray restarted OK, PID=$(pgrep xray)"
    else
        logger -t "$TAG" "ERROR: Xray not running after restart!"
    fi
else
    logger -t "$TAG" "ERROR: apk upgrade failed"
fi
logger -t "$TAG" "Done"
`

const rollback = `#!/bin/sh
# Roll back wrtbox changes without touching the stock fw4 / network.

echo "[$(date)] Rollback starting..."

if nft list table inet xray >/dev/null 2>&1; then
    nft delete table inet xray
    echo "  [OK] Removed nft table inet xray"
else
    echo "  [SKIP] nft table inet xray not found"
fi

if ip rule list | grep -q 'fwmark 0x1'; then
    ip rule del fwmark 1 lookup 100 2>/dev/null
    echo "  [OK] Removed ip rule fwmark 1"
else
    echo "  [SKIP] No ip rule fwmark 1"
fi

ip route flush table 100 2>/dev/null && echo "  [OK] Flushed route table 100"

echo "[$(date)] Rollback complete"
`

const cron = `0 */6 * * * /usr/bin/update-antifilter.sh
0 4 * * 0 /usr/bin/update-geosite.sh
`
