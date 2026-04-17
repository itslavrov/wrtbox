#!/bin/sh
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
