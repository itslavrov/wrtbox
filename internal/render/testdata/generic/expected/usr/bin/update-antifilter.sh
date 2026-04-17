#!/bin/sh
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
