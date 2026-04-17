#!/bin/sh
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
